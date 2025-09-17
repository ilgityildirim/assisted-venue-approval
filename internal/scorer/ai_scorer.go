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

	"assisted-venue-approval/internal/models"
	errs "assisted-venue-approval/pkg/errors"
	"assisted-venue-approval/pkg/metrics"

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

// metrics
var (
	mScoringDuration = metrics.Default.Histogram("venue_scoring_duration_seconds", "AI scoring duration (seconds)", []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 60})
)

// calcTrustLevelFromUser derives a trust level (0.0-1.0) from submitter info, mirroring venue detail logic
func (s *AIScorer) calcTrustLevelFromUser(user models.User) float64 {
	// Base trust
	trust := 0.3

	// Venue admin: highest trust
	if user.IsVenueAdmin {
		return 1.0
	}

	// Ambassador heuristic (if fields present)
	if user.AmbassadorLevel != nil && user.AmbassadorPoints != nil {
		// High ranking ambassadors
		if *user.AmbassadorLevel >= 3 || *user.AmbassadorPoints >= 1000 {
			trust = 0.8
		} else {
			trust = 0.6
		}
	}

	// Trusted member elevates baseline
	if user.Trusted && trust < 0.7 {
		trust = 0.7
	}

	// Contribution-based minor boosts
	if user.Contributions > 100 {
		trust += 0.1
	}
	if user.Contributions > 500 {
		trust += 0.1
	}
	if trust > 1.0 {
		trust = 1.0
	}
	return trust
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

// GetBufferPoolStats returns pooled buffer statistics (currently stubbed)
// TODO: implement pooled buffer for prompt building if profiling shows it helps
func (s *AIScorer) GetBufferPoolStats() (gets, puts, misses int64) {
	return 0, 0, 0
}

// ScoreVenue performs AI-based venue validation with caching and cost optimization
func (s *AIScorer) ScoreVenue(ctx context.Context, venue models.Venue, user models.User) (*models.ValidationResult, error) {
	// Check cache first to avoid duplicate API calls
	cacheKey := s.cache.generateKey(venue)
	// Include submitter trust/user in cache key to avoid cross-user cache collisions
	trust := s.calcTrustLevelFromUser(user)
	cacheKey = fmt.Sprintf("%s|trust=%.2f|uid=%d", cacheKey, trust, user.ID)
	if cached, found := s.cache.Get(cacheKey); found {
		return &cached, nil
	}

	// Unified scoring regardless of Google data presence
	t := mScoringDuration.Start()
	result, err := s.scoreUnifiedVenue(ctx, venue, trust)
	t.Observe()
	if err != nil {
		return nil, errs.NewExternal("scorer.ScoreVenue", "openai", "AI scoring failed", err)
	}

	// Cache the result
	s.cache.Set(cacheKey, *result)

	return result, nil
}

// scoreUnifiedVenue uses a single prompt for all venues and enforces JSON response
func (s *AIScorer) scoreUnifiedVenue(ctx context.Context, venue models.Venue, trustLevel float64) (*models.ValidationResult, error) {
	prompt := s.buildUnifiedPrompt(venue, trustLevel)

	// Add per-request timeout for OpenAI call; TODO: make configurable
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
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
		MaxTokens:   250,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	// Track API usage
	s.costTracker.AddUsage(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// Parse the structured response
	result, perr := s.parseStructuredResponse(resp.Choices[0].Message.Content, venue.ID)
	if perr != nil {
		// Fallback parsing if structured parsing fails
		fallback := s.parseResponseFallback(resp.Choices[0].Message.Content, venue.ID)
		return &fallback, nil
	}
	return &result, nil
}

// buildUnifiedPrompt creates a single prompt with Combined Information (if available) or Venue Information as JSON strings
func (s *AIScorer) buildUnifiedPrompt(venue models.Venue, trustLevel float64) string {
	// Venue info
	phone := ""
	if venue.Phone != nil {
		phone = *venue.Phone
	}
	url := ""
	if venue.URL != nil {
		url = *venue.URL
	}
	description := ""
	if venue.AdditionalInfo != nil {
		description = *venue.AdditionalInfo
	}
	zipcode := ""
	if venue.Zipcode != nil {
		zipcode = *venue.Zipcode
	}
	adminNote := ""
	if venue.AdminNote != nil {
		adminNote = *venue.AdminNote
	}
	adminHoldEmailNote := ""
	if venue.AdminHoldEmailNote != nil {
		adminHoldEmailNote = *venue.AdminHoldEmailNote
	}

	// Google-related
	googleStatus := ""
	googleTypes := []string{}
	googleWebsite := ""
	googlePhone := ""
	googleAddress := ""
	var latPtr, lngPtr *float64
	if venue.GoogleData != nil {
		googleStatus = venue.GoogleData.BusinessStatus
		googleTypes = venue.GoogleData.Types
		googleWebsite = venue.GoogleData.Website
		googlePhone = venue.GoogleData.FormattedPhone
		googleAddress = venue.GoogleData.FormattedAddress
		lat := venue.GoogleData.Geometry.Location.Lat
		lng := venue.GoogleData.Geometry.Location.Lng
		latPtr = &lat
		lngPtr = &lng
	}

	// Combined Info logic (lightweight adaptation of UI logic)
	type CombinedInfo struct {
		Name        string   `json:"name"`
		Address     string   `json:"address"`
		Phone       string   `json:"phone"`
		Website     string   `json:"website"`
		Hours       []string `json:"hours"`
		Lat         *float64 `json:"lat"`
		Lng         *float64 `json:"lng"`
		Types       []string `json:"types"`
		Description string   `json:"description"`
	}

	hours := []string{}
	if venue.OpenHours != nil && *venue.OpenHours != "" {
		hours = []string{*venue.OpenHours}
	} else if venue.GoogleData != nil && venue.GoogleData.OpeningHours != nil && len(venue.GoogleData.OpeningHours.WeekdayText) > 0 {
		hours = venue.GoogleData.OpeningHours.WeekdayText
	}

	combined := CombinedInfo{
		Name: venue.Name,
		Address: func() string {
			if googleAddress != "" {
				return googleAddress
			}
			return venue.Location
		}(),
		Phone: func() string {
			if googlePhone != "" {
				return googlePhone
			}
			return phone
		}(),
		Website: func() string {
			if googleWebsite != "" {
				return googleWebsite
			}
			return url
		}(),
		Hours:       hours,
		Lat:         latPtr,
		Lng:         lngPtr,
		Types:       googleTypes,
		Description: description,
	}

	combinedJSON, _ := json.Marshal(combined)

	// Venue info JSON fallback
	type VenueInfo struct {
		Name        string   `json:"name"`
		Address     string   `json:"address"`
		Zipcode     string   `json:"zipcode"`
		Phone       string   `json:"phone"`
		Website     string   `json:"website"`
		Description string   `json:"description"`
		VegOnly     int      `json:"vegonly"`
		Vegan       int      `json:"vegan"`
		Category    int      `json:"category"`
		Types       []string `json:"types_from_google"`
	}
	venueInfo := VenueInfo{
		Name:        venue.Name,
		Address:     venue.Location,
		Zipcode:     zipcode,
		Phone:       phone,
		Website:     url,
		Description: description,
		VegOnly:     venue.VegOnly,
		Vegan:       venue.Vegan,
		Category:    venue.Category,
		Types:       googleTypes,
	}
	venueJSON, _ := json.Marshal(venueInfo)

	return fmt.Sprintf(`You must score the venue and reply with JSON only.

Data:
- Combined Information (JSON): %s
- Venue Information Data (JSON): %s
- Admin Note: %s
- Admin Hold Email Note: %s
- Google Business Status: %s
- Google Types: %v
- Venue Type Indicators: {"vegonly": %d, "vegan": %d, "category": %d}
- Trust Level (0.0-1.0): %.2f

Instructions:
- Always produce a single JSON object: {"score": X, "notes": "brief", "breakdown": {"legitimacy": X, "completeness": X, "relevance": X}}
- Score range: 0-100. Keep notes concise.
- If admin_note or admin_hold_email_note indicates this venue should not be approved for ANY reason, set score=0 and explain briefly in notes (manual review).
- If Google business_status is provided and is not OPERATIONAL, set score=0 unless trust_level >= 0.80, and note why using notes.
- For type validation: Check if Google types are LOGICALLY compatible with food venues. Food venues should have at least one food-related type (restaurant, food, meal_takeaway, meal_delivery, cafe, bakery, bar, establishment, point_of_interest). Types like ["premise", "street_address"] alone or ["lodging", "travel_agency"] suggest non-food business. Only set score=0 for clear mismatches when trust_level < 0.80.
- Venue Type indicators (vegonly/vegan/category) suggest this should be a food-related venue.
- LEGITIMACY (35 points): Score based on data verification and completeness. If venue has Google-verified data (formatted address, coordinates, phone, business status), award 25-35 points regardless of trust_level. Trust level is for validation rules only, not legitimacy assessment. Missing or unverified data reduces legitimacy points.
- COMPLETENESS (30 points): Award points for available fields (name, address, phone, website, hours, coordinates).
- RELEVANCE (35 points): Assess how well this fits HappyCow directory (vegan/vegetarian focus).
- Use Combined Information if present; otherwise rely on Venue Information Data.
- TRUST LEVEL usage: Trust level should only affect strict validation rules (business status, type mismatches) but not prevent scoring legitimate venues with good Google verification.
- Consider venue description (can be empty) in relevance.
- Allocate points roughly: legitimacy 35, completeness 30, relevance 35.
- Respond with JSON only, no extra text.`,
		string(combinedJSON),
		string(venueJSON),
		escapeForPrompt(adminNote),
		escapeForPrompt(adminHoldEmailNote),
		googleStatus,
		googleTypes,
		venue.VegOnly,
		venue.Vegan,
		venue.Category,
		trustLevel,
	)
}

// escapeForPrompt ensures multi-line strings are safe in the prompt context
func escapeForPrompt(s string) string {
	return s
}

// Optimized prompt functions for cost efficiency

func (s *AIScorer) getSystemPrompt() string {
	return `System role: You are an expert venue data validator for HappyCow (vegan/vegetarian directory).
Goals:
- Score venues for (1) legitimacy (35), (2) completeness (30), (3) relevance (35).
- Always output a single JSON object, no prose, no Markdown.
Output JSON schema:
{"score": X, "notes": "brief", "breakdown": {"legitimacy": X, "completeness": X, "relevance": X}}
Rules:
- If admin notes indicate the venue should not be approved for any reason, set score=0 and explain briefly in notes of JSON output.
- If Google Business Status exists and is not OPERATIONAL, set score=0 unless trust_level >= 0.80.
- For Google types vs Venue Type validation: Check for LOGICAL compatibility, not exact match. A restaurant venue with Google types ["restaurant", "food", "establishment"] is valid. A restaurant venue with only ["premise", "street_address"] or ["lodging"] is suspicious and should set score=0 if trust_level < 0.80.
- Restaurant/food venues should have at least one food-related Google type: restaurant, food, meal_takeaway, meal_delivery, cafe, bakery, bar, etc.
- LEGITIMACY scoring: Base legitimacy on data quality and verification, NOT just trust level. If Combined Information contains verified Google data (address, phone, coordinates), legitimacy should be high (25-35) regardless of trust level. Only reduce legitimacy for data conflicts or missing critical information.
- TRUST LEVEL usage: Trust level should only affect strict validation rules (business status, type mismatches) but not prevent scoring legitimate venues with good Google verification.
- Consider the venue description (empty allowed). Do not invent facts beyond provided data.
- Keep notes concise (<= 200 chars).`
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
		AIOutputData:   &response,
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
		VenueID:      venueID,
		Score:        score,
		Status:       status,
		Notes:        "Fallback parsing used - manual review recommended",
		AIOutputData: &response,
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
		// Respect cancellation between batches
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
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
			// Early cancellation check per item
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			go func(index int, v models.Venue) {
				defer wg.Done()
				// Check context before heavy work
				select {
				case <-ctx.Done():
					return
				default:
				}
				sem <- struct{}{}        // Acquire semaphore
				defer func() { <-sem }() // Release semaphore

				// Another cancellation check after waiting for semaphore
				select {
				case <-ctx.Done():
					return
				default:
				}

				result, err := s.ScoreVenue(ctx, v, models.User{})

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
