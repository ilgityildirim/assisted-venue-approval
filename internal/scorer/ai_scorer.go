package scorer

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"automatic-vendor-validation/internal/models"

	"github.com/sashabaranov/go-openai"
)

// CostTracker tracks OpenAI API usage and costs
type CostTracker struct {
	mu               sync.RWMutex
	totalTokens      int
	totalRequests    int
	estimatedCostUSD float64
	startTime        time.Time
}

func (c *CostTracker) AddUsage(promptTokens, completionTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalTokens += promptTokens + completionTokens
	c.totalRequests++

	// GPT-3.5-turbo pricing (as of 2024): $0.0005/1K prompt tokens, $0.0015/1K completion tokens
	promptCost := float64(promptTokens) * 0.0005 / 1000
	completionCost := float64(completionTokens) * 0.0015 / 1000
	c.estimatedCostUSD += promptCost + completionCost
}

func (c *CostTracker) GetStats() (totalTokens, totalRequests int, estimatedCostUSD float64, duration time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.totalTokens, c.totalRequests, c.estimatedCostUSD, time.Since(c.startTime)
}

// VenueCache caches AI scoring results to avoid duplicate API calls with memory management
type VenueCache struct {
	mu            sync.RWMutex
	cache         map[string]CachedResult
	maxSize       int
	cleanupTicker *time.Ticker
	stopChan      chan bool
}

type CachedResult struct {
	Result    models.ValidationResult
	Timestamp time.Time
}

func NewVenueCache() *VenueCache {
	cache := &VenueCache{
		cache:    make(map[string]CachedResult),
		maxSize:  1000, // Limit cache size to prevent memory leaks
		stopChan: make(chan bool, 1),
	}

	// Start cleanup routine to manage memory
	cache.startCleanup()
	return cache
}

// startCleanup starts a background goroutine to periodically clean expired entries
func (c *VenueCache) startCleanup() {
	c.cleanupTicker = time.NewTicker(30 * time.Minute) // Clean every 30 minutes

	go func() {
		for {
			select {
			case <-c.cleanupTicker.C:
				c.cleanupExpired()
			case <-c.stopChan:
				c.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanupExpired removes expired entries and enforces size limits
func (c *VenueCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	// Find expired entries
	for key, cached := range c.cache {
		if now.Sub(cached.Timestamp) > 24*time.Hour {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Remove expired entries
	for _, key := range expiredKeys {
		delete(c.cache, key)
	}

	// Enforce size limit by removing oldest entries if needed
	if len(c.cache) > c.maxSize {
		// Create slice of all entries with timestamps for sorting
		type cacheEntry struct {
			key       string
			timestamp time.Time
		}

		entries := make([]cacheEntry, 0, len(c.cache))
		for key, cached := range c.cache {
			entries = append(entries, cacheEntry{key: key, timestamp: cached.Timestamp})
		}

		// Sort by timestamp (oldest first)
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].timestamp.After(entries[j].timestamp) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		// Remove oldest entries to get under the limit
		entriesToRemove := len(c.cache) - c.maxSize
		for i := 0; i < entriesToRemove; i++ {
			delete(c.cache, entries[i].key)
		}
	}
}

// Stop stops the cleanup routine
func (c *VenueCache) Stop() {
	select {
	case c.stopChan <- true:
	default:
	}
}

// GetSize returns the current cache size
func (c *VenueCache) GetSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

func (c *VenueCache) Get(key string) (models.ValidationResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.cache[key]
	if !exists {
		return models.ValidationResult{}, false
	}

	// Cache expires after 24 hours
	if time.Since(cached.Timestamp) > 24*time.Hour {
		return models.ValidationResult{}, false
	}

	return cached.Result, true
}

func (c *VenueCache) Set(key string, result models.ValidationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = CachedResult{
		Result:    result,
		Timestamp: time.Now(),
	}
}

func (c *VenueCache) generateKey(venue models.Venue) string {
	// Create cache key based on venue data that affects AI scoring
	data := fmt.Sprintf("%s|%s|%v|%v|%v",
		venue.Name,
		venue.Location,
		venue.Phone,
		venue.URL,
		venue.AdditionalInfo)

	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// AIScorer optimized for cost efficiency and structured responses
type AIScorer struct {
	client      *openai.Client
	costTracker *CostTracker
	cache       *VenueCache
}

func NewAIScorer(apiKey string) *AIScorer {
	return &AIScorer{
		client: openai.NewClient(apiKey),
		costTracker: &CostTracker{
			startTime: time.Now(),
		},
		cache: NewVenueCache(),
	}
}

// GetCostStats returns current API usage statistics
func (s *AIScorer) GetCostStats() (totalTokens, totalRequests int, estimatedCostUSD float64, duration time.Duration) {
	return s.costTracker.GetStats()
}

// ScoreVenue performs AI-based venue validation with caching and cost optimization
func (s *AIScorer) ScoreVenue(ctx context.Context, venue models.Venue) (*models.ValidationResult, error) {
	// Check cache first to avoid duplicate API calls
	cacheKey := s.cache.generateKey(venue)
	if cached, found := s.cache.Get(cacheKey); found {
		return &cached, nil
	}

	// Use validation details from scraper if available, otherwise use simple mode
	var result *models.ValidationResult
	var err error

	if venue.ValidationDetails != nil && venue.ValidationDetails.GooglePlaceFound {
		// Enhanced mode: venue has Google data and validation details
		result, err = s.scoreEnhancedVenue(ctx, venue)
	} else {
		// Basic mode: venue without Google data
		result, err = s.scoreBasicVenue(ctx, venue)
	}

	if err != nil {
		return nil, fmt.Errorf("AI scoring failed: %w", err)
	}

	// Cache the result
	s.cache.Set(cacheKey, *result)

	return result, nil
}

// scoreEnhancedVenue uses AI only for vegan relevance scoring, relying on Google data for other criteria
func (s *AIScorer) scoreEnhancedVenue(ctx context.Context, venue models.Venue) (*models.ValidationResult, error) {
	// For venues with Google data, we only need AI to evaluate vegan/vegetarian relevance
	// This significantly reduces token usage and API costs

	vegRelevanceScore, notes, err := s.evaluateVeganRelevance(ctx, venue)
	if err != nil {
		return nil, err
	}

	// Combine Google-based validation scores with AI vegan relevance score
	validationDetails := venue.ValidationDetails
	scoreBreakdown := validationDetails.ScoreBreakdown

	// Replace the basic vegan relevance score (0-5 points) with AI-enhanced score (0-25 points)
	enhancedTotal := scoreBreakdown.Total - scoreBreakdown.VeganRelevance + vegRelevanceScore

	// Determine status
	status := "manual_review"
	if enhancedTotal >= 85 {
		status = "approved"
	} else if enhancedTotal < 50 {
		status = "rejected"
	}

	return &models.ValidationResult{
		VenueID: venue.ID,
		Score:   enhancedTotal,
		Status:  status,
		Notes:   notes,
		ScoreBreakdown: map[string]int{
			"venue_name_match":     scoreBreakdown.VenueNameMatch,
			"address_accuracy":     scoreBreakdown.AddressAccuracy,
			"geolocation_accuracy": scoreBreakdown.GeolocationAccuracy,
			"phone_verification":   scoreBreakdown.PhoneVerification,
			"business_hours":       scoreBreakdown.BusinessHours,
			"website_verification": scoreBreakdown.WebsiteVerification,
			"business_status":      scoreBreakdown.BusinessStatus,
			"postal_code":          scoreBreakdown.PostalCode,
			"vegan_relevance":      vegRelevanceScore,
		},
	}, nil
}

// scoreBasicVenue uses AI for full venue evaluation when Google data is unavailable
func (s *AIScorer) scoreBasicVenue(ctx context.Context, venue models.Venue) (*models.ValidationResult, error) {
	prompt := s.buildBasicPrompt(venue)

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: s.getSystemPrompt(),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.1,
		MaxTokens:   200, // Limit response size to reduce costs
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	// Track API usage
	s.costTracker.AddUsage(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// Parse the structured response
	result, err := s.parseStructuredResponse(resp.Choices[0].Message.Content, venue.ID)
	if err != nil {
		// Fallback parsing if structured parsing fails
		result = s.parseResponseFallback(resp.Choices[0].Message.Content, venue.ID)
	}

	return &result, nil
}

// evaluateVeganRelevance uses AI only for vegan/vegetarian relevance assessment (cost-optimized)
func (s *AIScorer) evaluateVeganRelevance(ctx context.Context, venue models.Venue) (score int, notes string, err error) {
	// Build minimal prompt focused only on vegan relevance
	prompt := s.buildVeganRelevancePrompt(venue)

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are a vegan/vegetarian restaurant expert. Evaluate ONLY the vegan/vegetarian relevance of venues for HappyCow directory.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.1,
		MaxTokens:   100, // Very limited response to minimize costs
	})

	if err != nil {
		return 0, "", fmt.Errorf("vegan relevance API call failed: %w", err)
	}

	// Track API usage
	s.costTracker.AddUsage(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// Parse vegan relevance score from response
	score, notes = s.parseVeganRelevanceResponse(resp.Choices[0].Message.Content)

	return score, notes, nil
}

// Optimized prompt functions for cost efficiency

func (s *AIScorer) getSystemPrompt() string {
	return `You are a venue validation expert for HappyCow, a vegan/vegetarian restaurant directory. 
Analyze venues for business legitimacy, data completeness, and vegan/vegetarian relevance.
Always respond in the exact JSON format requested. Be concise to minimize tokens.`
}

// buildVeganRelevancePrompt creates a minimal prompt for vegan relevance assessment only
func (s *AIScorer) buildVeganRelevancePrompt(venue models.Venue) string {
	additionalInfo := ""
	if venue.AdditionalInfo != nil {
		additionalInfo = *venue.AdditionalInfo
	}

	// Minimal prompt to reduce token usage
	return fmt.Sprintf(`Venue: "%s"
Description: "%s"
Vegan types: %d,%d

Rate vegan/vegetarian relevance (0-25): {"score": X, "note": "brief reason"}`,
		venue.Name, additionalInfo, venue.Vegan, venue.VegOnly)
}

// buildBasicPrompt for venues without Google data (full AI evaluation)
func (s *AIScorer) buildBasicPrompt(venue models.Venue) string {
	phone := "N/A"
	if venue.Phone != nil {
		phone = *venue.Phone
	}
	url := "N/A"
	if venue.URL != nil {
		url = *venue.URL
	}
	additionalInfo := "N/A"
	if venue.AdditionalInfo != nil {
		additionalInfo = *venue.AdditionalInfo
	}

	return fmt.Sprintf(`Venue: %s
Address: %s  
Phone: %s
Website: %s
Info: %s

Score 0-100 for HappyCow directory:
1. Business legitimacy (35pts)
2. Info completeness (30pts) 
3. Vegan relevance (35pts)

JSON: {"score": X, "notes": "brief", "breakdown": {"legitimacy": X, "completeness": X, "relevance": X}}`,
		venue.Name, venue.Location, phone, url, additionalInfo)
}

// Enhanced parsing functions with validation

func (s *AIScorer) parseStructuredResponse(response string, venueID int64) (models.ValidationResult, error) {
	// Try to parse the JSON response
	var parsed struct {
		Score     int            `json:"score"`
		Notes     string         `json:"notes"`
		Breakdown map[string]int `json:"breakdown"`
	}

	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return models.ValidationResult{}, fmt.Errorf("JSON parsing failed: %w", err)
	}

	// Validate score range
	if parsed.Score < 0 || parsed.Score > 100 {
		parsed.Score = 50 // Default to manual review
	}

	// Determine status
	status := "manual_review"
	if parsed.Score >= 85 {
		status = "approved"
	} else if parsed.Score < 50 {
		status = "rejected"
	}

	return models.ValidationResult{
		VenueID:        venueID,
		Score:          parsed.Score,
		Status:         status,
		Notes:          parsed.Notes,
		ScoreBreakdown: parsed.Breakdown,
	}, nil
}

func (s *AIScorer) parseResponseFallback(response string, venueID int64) models.ValidationResult {
	// Extract score using regex as fallback
	scoreRegex := regexp.MustCompile(`"?score"?:\s*(\d+)`)
	matches := scoreRegex.FindStringSubmatch(response)

	score := 50 // Default to manual review
	if len(matches) > 1 {
		if parsedScore, err := strconv.Atoi(matches[1]); err == nil && parsedScore >= 0 && parsedScore <= 100 {
			score = parsedScore
		}
	}

	// Determine status
	status := "manual_review"
	if score >= 85 {
		status = "approved"
	} else if score < 50 {
		status = "rejected"
	}

	return models.ValidationResult{
		VenueID: venueID,
		Score:   score,
		Status:  status,
		Notes:   "Fallback parsing used - manual review recommended",
		ScoreBreakdown: map[string]int{
			"total": score,
		},
	}
}

func (s *AIScorer) parseVeganRelevanceResponse(response string) (score int, notes string) {
	// Parse minimal vegan relevance response
	var parsed struct {
		Score int    `json:"score"`
		Note  string `json:"note"`
	}

	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		// Fallback parsing with regex
		scoreRegex := regexp.MustCompile(`"?score"?:\s*(\d+)`)
		matches := scoreRegex.FindStringSubmatch(response)
		if len(matches) > 1 {
			if parsedScore, parseErr := strconv.Atoi(matches[1]); parseErr == nil && parsedScore >= 0 && parsedScore <= 25 {
				return parsedScore, "AI assessment of vegan relevance"
			}
		}
		return 10, "Fallback vegan relevance score" // Conservative default
	}

	// Validate score range for vegan relevance (0-25 points)
	if parsed.Score < 0 || parsed.Score > 25 {
		parsed.Score = 10 // Conservative default
	}

	return parsed.Score, parsed.Note
}

// BatchScoreVenues processes multiple venues efficiently to reduce API overhead
func (s *AIScorer) BatchScoreVenues(ctx context.Context, venues []models.Venue, maxBatchSize int) ([]models.ValidationResult, error) {
	if maxBatchSize <= 0 {
		maxBatchSize = 1 // Process individually if invalid batch size
	}

	var results []models.ValidationResult
	var errors []error

	// Process in batches to avoid overwhelming the API
	for i := 0; i < len(venues); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(venues) {
			end = len(venues)
		}

		batch := venues[i:end]
		batchResults := make([]models.ValidationResult, len(batch))

		// Process batch venues concurrently (with goroutine limit)
		sem := make(chan struct{}, 5) // Limit to 5 concurrent API calls
		var wg sync.WaitGroup
		var mu sync.Mutex

		for j, venue := range batch {
			wg.Add(1)
			go func(index int, v models.Venue) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire semaphore
				defer func() { <-sem }() // Release semaphore

				result, err := s.ScoreVenue(ctx, v)

				mu.Lock()
				if err != nil {
					errors = append(errors, fmt.Errorf("venue %d failed: %w", v.ID, err))
					// Create default result for failed venues
					batchResults[index] = models.ValidationResult{
						VenueID:        v.ID,
						Score:          0,
						Status:         "manual_review",
						Notes:          fmt.Sprintf("AI scoring failed: %v", err),
						ScoreBreakdown: map[string]int{"error": 0},
					}
				} else {
					batchResults[index] = *result
				}
				mu.Unlock()
			}(j, venue)
		}

		wg.Wait()
		results = append(results, batchResults...)
	}

	// Return results even if some venues failed
	var combinedError error
	if len(errors) > 0 {
		combinedError = fmt.Errorf("batch processing completed with %d errors: %v", len(errors), errors)
	}

	return results, combinedError
}
