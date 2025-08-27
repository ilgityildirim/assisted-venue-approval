package processor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"automatic-vendor-validation/internal/decision"
	"automatic-vendor-validation/internal/models"
	"automatic-vendor-validation/internal/scorer"
	"automatic-vendor-validation/internal/scraper"
	"automatic-vendor-validation/pkg/database"
)

// ProcessingJob represents a venue processing job
type ProcessingJob struct {
	Venue    models.Venue
	User     models.User // User who submitted the venue
	Priority int         // Higher values = higher priority
	Retry    int         // Retry attempt count
}

// ProcessingResult represents the result of processing a venue
type ProcessingResult struct {
	VenueID          int64
	Success          bool
	ValidationResult *models.ValidationResult
	GoogleData       *models.GooglePlaceData
	Error            error
	ProcessingTimeMs int64
	Retries          int
}

// ProcessingStats tracks processing statistics
type ProcessingStats struct {
	TotalJobs      int64
	CompletedJobs  int64
	SuccessfulJobs int64
	FailedJobs     int64
	AutoApproved   int64
	ManualReview   int64
	AutoRejected   int64
	AverageTimeMs  int64
	StartTime      time.Time
	LastActivity   time.Time
	WorkerCount    int
	QueueSize      int64
	APICallsGoogle int64
	APICallsOpenAI int64
	TotalCostUSD   float64
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens   chan struct{}
	interval time.Duration
	capacity int
	ticker   *time.Ticker
	mu       sync.Mutex
	running  bool
}

func NewRateLimiter(rps int, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst <= 0 {
		burst = rps
	}

	rl := &RateLimiter{
		tokens:   make(chan struct{}, burst),
		interval: time.Duration(1000000000/rps) * time.Nanosecond,
		capacity: burst,
	}

	// Fill initial tokens
	for i := 0; i < burst; i++ {
		rl.tokens <- struct{}{}
	}

	return rl
}

func (rl *RateLimiter) Start() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.running {
		return
	}

	rl.ticker = time.NewTicker(rl.interval)
	rl.running = true

	go func() {
		for range rl.ticker.C {
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Bucket is full, drop token
			}
		}
	}()
}

func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.running {
		return
	}

	rl.ticker.Stop()
	rl.running = false
}

func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ProcessingEngine handles concurrent venue processing with rate limiting and error recovery
type ProcessingEngine struct {
	db             *database.DB
	scraper        *scraper.GoogleMapsScraper
	scorer         *scorer.AIScorer
	decisionEngine *decision.DecisionEngine

	// Configuration
	workerCount int
	maxRetries  int
	retryDelay  time.Duration
	jobTimeout  time.Duration

	// Mode flags
	scoreOnly bool

	// Rate limiters
	googleRateLimit *RateLimiter
	openAIRateLimit *RateLimiter

	// Processing control
	jobQueue   chan ProcessingJob
	resultChan chan ProcessingResult
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Statistics
	stats   ProcessingStats
	statsMu sync.RWMutex

	// Shutdown control
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// ProcessingConfig holds configuration for the processing engine
type ProcessingConfig struct {
	WorkerCount int
	MaxRetries  int
	RetryDelay  time.Duration
	JobTimeout  time.Duration
	GoogleRPS   int // Google Places API requests per second
	GoogleBurst int // Google Places API burst capacity
	OpenAIRPS   int // OpenAI API requests per second
	OpenAIBurst int // OpenAI API burst capacity
	QueueSize   int // Job queue buffer size
}

// DefaultProcessingConfig returns a sensible default configuration optimized for cost efficiency
func DefaultProcessingConfig() ProcessingConfig {
	return ProcessingConfig{
		WorkerCount: 15, // Increased workers for better throughput
		MaxRetries:  3,
		RetryDelay:  5 * time.Second,
		JobTimeout:  90 * time.Second, // Increased timeout for complex venues
		GoogleRPS:   15,               // Optimized rate for Google Places API (within quota limits)
		GoogleBurst: 30,               // Higher burst for peak processing
		OpenAIRPS:   8,                // Optimized rate for OpenAI API (cost-conscious)
		OpenAIBurst: 15,               // Controlled burst to manage costs
		QueueSize:   2000,             // Larger queue for batch processing
	}
}

// NewProcessingEngine creates a new concurrent processing engine
func NewProcessingEngine(db *database.DB, scraper *scraper.GoogleMapsScraper, scorer *scorer.AIScorer, config ProcessingConfig) *ProcessingEngine {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize decision engine with default configuration
	decisionConfig := decision.DefaultDecisionConfig()
	decisionEngine := decision.NewDecisionEngine(decisionConfig)

	engine := &ProcessingEngine{
		db:              db,
		scraper:         scraper,
		scorer:          scorer,
		decisionEngine:  decisionEngine,
		workerCount:     config.WorkerCount,
		maxRetries:      config.MaxRetries,
		retryDelay:      config.RetryDelay,
		jobTimeout:      config.JobTimeout,
		googleRateLimit: NewRateLimiter(config.GoogleRPS, config.GoogleBurst),
		openAIRateLimit: NewRateLimiter(config.OpenAIRPS, config.OpenAIBurst),
		jobQueue:        make(chan ProcessingJob, config.QueueSize),
		resultChan:      make(chan ProcessingResult, config.QueueSize),
		ctx:             ctx,
		cancel:          cancel,
		shutdown:        make(chan struct{}),
		scoreOnly:       false,
		stats: ProcessingStats{
			StartTime:    time.Now(),
			LastActivity: time.Now(),
			WorkerCount:  config.WorkerCount,
		},
	}

	return engine
}

// Start begins the processing engine with workers and rate limiters
func (e *ProcessingEngine) Start() {
	log.Printf("Starting processing engine with %d workers", e.workerCount)

	// Start rate limiters
	e.googleRateLimit.Start()
	e.openAIRateLimit.Start()

	// Start workers
	for i := 0; i < e.workerCount; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}

	// Start result processor
	e.wg.Add(1)
	go e.resultProcessor()

	log.Println("Processing engine started successfully")
}

// Stop gracefully shuts down the processing engine
func (e *ProcessingEngine) Stop(timeout time.Duration) error {
	var err error

	e.shutdownOnce.Do(func() {
		log.Println("Initiating graceful shutdown of processing engine")

		// Signal shutdown to all components
		close(e.shutdown)
		e.cancel()

		// Stop accepting new jobs
		close(e.jobQueue)

		// Wait for workers to finish with timeout
		done := make(chan struct{})
		go func() {
			e.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("All workers shut down gracefully")
		case <-time.After(timeout):
			log.Println("Shutdown timeout reached, forcing exit")
			err = fmt.Errorf("shutdown timeout exceeded")
		}

		// Stop rate limiters
		e.googleRateLimit.Stop()
		e.openAIRateLimit.Stop()

		// Close result channel
		close(e.resultChan)

		log.Println("Processing engine shutdown complete")
	})

	return err
}

// ProcessVenues adds venues to the processing queue (legacy method)
func (e *ProcessingEngine) ProcessVenues(venues []models.Venue) error {
	// Convert venues to VenueWithUser with anonymous user
	venuesWithUser := make([]models.VenueWithUser, len(venues))
	for i, venue := range venues {
		venuesWithUser[i] = models.VenueWithUser{
			Venue: venue,
			User: models.User{
				ID:       0,
				Username: "anonymous",
				Trusted:  false,
			},
			IsVenueAdmin:     false,
			AmbassadorLevel:  nil,
			AmbassadorPoints: nil,
			AmbassadorPath:   nil,
		}
	}

	return e.ProcessVenuesWithUsers(venuesWithUser)
}

// ProcessVenuesWithUsers adds venues with user data to the processing queue
func (e *ProcessingEngine) ProcessVenuesWithUsers(venuesWithUser []models.VenueWithUser) error {
	e.statsMu.Lock()
	e.stats.TotalJobs = int64(len(venuesWithUser))
	e.statsMu.Unlock()

	log.Printf("Queuing %d venues with user data for processing", len(venuesWithUser))

	for _, venueWithUser := range venuesWithUser {
		priority := e.calculatePriorityWithUser(venueWithUser.Venue, venueWithUser.User)
		job := ProcessingJob{
			Venue:    venueWithUser.Venue,
			User:     venueWithUser.User,
			Priority: priority,
			Retry:    0,
		}

		select {
		case e.jobQueue <- job:
			atomic.AddInt64(&e.stats.QueueSize, 1)
		case <-e.ctx.Done():
			return fmt.Errorf("processing engine is shutting down")
		default:
			return fmt.Errorf("job queue is full")
		}
	}

	log.Printf("Successfully queued %d venues with user data", len(venuesWithUser))
	return nil
}

// GetStats returns current processing statistics
func (e *ProcessingEngine) SetScoreOnly(scoreOnly bool) {
	e.statsMu.Lock()
	e.scoreOnly = scoreOnly
	e.statsMu.Unlock()
}

func (e *ProcessingEngine) GetStats() ProcessingStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()

	// Get cost stats from AI scorer
	_, _, costUSD, _ := e.scorer.GetCostStats()

	stats := e.stats
	stats.TotalCostUSD = costUSD
	stats.QueueSize = atomic.LoadInt64(&e.stats.QueueSize)

	return stats
}

// calculatePriority determines job priority based on venue characteristics (legacy method)
func (e *ProcessingEngine) calculatePriority(venue models.Venue) int {
	return e.calculatePriorityWithUser(venue, models.User{})
}

// calculatePriorityWithUser determines job priority based on venue and user characteristics
func (e *ProcessingEngine) calculatePriorityWithUser(venue models.Venue, user models.User) int {
	priority := 0

	// Highest priority for venue owners/admins (fast-track approval)
	if user.ID > 0 && (user.Trusted || isVenueAdmin(user, venue)) {
		priority += 1000
	}

	// High priority for ambassadors
	if hasAmbassadorLevel(user) {
		priority += 500
	}

	// Higher priority for Korean/Chinese venues (special handling required)
	if venue.Location != "" {
		location := venue.Location
		if containsAny(location, []string{"korea", "korean", "china", "chinese", "seoul", "beijing", "shanghai"}) {
			priority += 100
		}
	}

	// Higher priority for venues with more complete data
	if venue.Phone != nil && *venue.Phone != "" {
		priority += 10
	}
	if venue.URL != nil && *venue.URL != "" {
		priority += 10
	}
	if venue.AdditionalInfo != nil && *venue.AdditionalInfo != "" {
		priority += 5
	}

	return priority
}

// worker processes jobs from the queue
func (e *ProcessingEngine) worker(id int) {
	defer e.wg.Done()

	log.Printf("Worker %d started", id)
	defer log.Printf("Worker %d stopped", id)

	for {
		select {
		case job, ok := <-e.jobQueue:
			if !ok {
				return // Queue closed, worker should exit
			}

			atomic.AddInt64(&e.stats.QueueSize, -1)
			result := e.processJob(job)

			select {
			case e.resultChan <- result:
			case <-e.ctx.Done():
				return
			}

		case <-e.ctx.Done():
			return
		}
	}
}

// processJob processes a single venue with error recovery
func (e *ProcessingEngine) processJob(job ProcessingJob) ProcessingResult {
	startTime := time.Now()
	venue := job.Venue
	user := job.User

	// Create job-specific context with timeout
	jobCtx, cancel := context.WithTimeout(e.ctx, e.jobTimeout)
	defer cancel()

	result := ProcessingResult{
		VenueID:          venue.ID,
		Success:          false,
		ProcessingTimeMs: 0,
		Retries:          job.Retry,
	}

	// Process venue with exponential backoff retry logic
	var err error
	var validationResult *models.ValidationResult
	var googleData *models.GooglePlaceData

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff delay
			delay := time.Duration(attempt*attempt) * e.retryDelay
			log.Printf("Retrying venue %d (attempt %d) after %v delay", venue.ID, attempt+1, delay)

			select {
			case <-time.After(delay):
			case <-jobCtx.Done():
				result.Error = fmt.Errorf("job cancelled during retry delay: %w", jobCtx.Err())
				result.ProcessingTimeMs = time.Since(startTime).Milliseconds()
				return result
			}
		}

		// Process the venue
		validationResult, googleData, err = e.processVenueWithRateLimit(jobCtx, venue, user)
		if err == nil {
			result.Success = true
			result.ValidationResult = validationResult
			result.GoogleData = googleData
			break
		}

		// Preserve any Google data we obtained, even if AI failed or other errors occurred
		if googleData != nil {
			result.GoogleData = googleData
		}

		// Check if error is retryable
		if !e.isRetryableError(err) {
			log.Printf("Non-retryable error for venue %d: %v", venue.ID, err)
			break
		}

		log.Printf("Retryable error for venue %d (attempt %d): %v", venue.ID, attempt+1, err)
	}

	result.Error = err
	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	return result
}

// processVenueWithRateLimit processes a venue with proper rate limiting and user context
func (e *ProcessingEngine) processVenueWithRateLimit(ctx context.Context, venue models.Venue, user models.User) (*models.ValidationResult, *models.GooglePlaceData, error) {
	// Rate limit Google Maps API call
	if err := e.googleRateLimit.Wait(ctx); err != nil {
		return nil, nil, fmt.Errorf("google rate limit wait cancelled: %w", err)
	}

	// Enhance venue with Google Maps data
	enhancedVenue, err := e.scraper.EnhanceVenueWithValidation(ctx, venue)
	if err != nil {
		atomic.AddInt64(&e.stats.APICallsGoogle, 1)
		return nil, nil, fmt.Errorf("failed to enhance venue: %w", err)
	}
	atomic.AddInt64(&e.stats.APICallsGoogle, 1)

	// Prepare Google data (if any) early so we can return it even on AI failure
	var gData *models.GooglePlaceData
	if enhancedVenue.GoogleData != nil {
		gData = enhancedVenue.GoogleData
	}

	// Rate limit OpenAI API call (only if needed for basic venues or vegan relevance)
	if enhancedVenue.ValidationDetails == nil || !enhancedVenue.ValidationDetails.GooglePlaceFound {
		if err := e.openAIRateLimit.Wait(ctx); err != nil {
			return nil, gData, fmt.Errorf("openai rate limit wait cancelled: %w", err)
		}
	}

	// Score venue with AI
	validationResult, err := e.scorer.ScoreVenue(ctx, *enhancedVenue, user)
	if err != nil {
		atomic.AddInt64(&e.stats.APICallsOpenAI, 1)
		return nil, gData, fmt.Errorf("failed to score venue: %w", err)
	}
	atomic.AddInt64(&e.stats.APICallsOpenAI, 1)

	// Use decision engine to make final decision with user context
	decisionResult := e.decisionEngine.MakeDecision(*enhancedVenue, user, validationResult)

	// Override validation result with decision engine output
	validationResult.Status = decisionResult.FinalStatus
	validationResult.Notes = decisionResult.DecisionReason
	validationResult.Score = decisionResult.FinalScore

	// Add decision metadata to score breakdown
	if validationResult.ScoreBreakdown == nil {
		validationResult.ScoreBreakdown = make(map[string]int)
	}
	if decisionResult.Authority != nil {
		validationResult.ScoreBreakdown["authority_bonus"] = decisionResult.Authority.BonusPoints
	}
	validationResult.ScoreBreakdown["quality_flags"] = len(decisionResult.QualityFlags)

	return validationResult, gData, nil
}

// isRetryableError determines if an error should trigger a retry
func (e *ProcessingEngine) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Retryable errors
	retryableErrors := []string{
		"timeout",
		"rate limit",
		"quota exceeded",
		"service unavailable",
		"internal server error",
		"connection refused",
		"connection reset",
		"temporary failure",
	}

	for _, retryable := range retryableErrors {
		if containsAny(errStr, []string{retryable}) {
			return true
		}
	}

	return false
}

// resultProcessor handles processing results and database updates
func (e *ProcessingEngine) resultProcessor() {
	defer e.wg.Done()

	log.Println("Result processor started")
	defer log.Println("Result processor stopped")

	for {
		select {
		case result, ok := <-e.resultChan:
			if !ok {
				return // Result channel closed
			}

			e.handleResult(result)

		case <-e.ctx.Done():
			return
		}
	}
}

// handleResult processes a venue processing result
func (e *ProcessingEngine) handleResult(result ProcessingResult) {
	e.statsMu.Lock()
	e.stats.CompletedJobs++
	e.stats.LastActivity = time.Now()

	// Update processing time average
	if e.stats.CompletedJobs == 1 {
		e.stats.AverageTimeMs = result.ProcessingTimeMs
	} else {
		e.stats.AverageTimeMs = (e.stats.AverageTimeMs + result.ProcessingTimeMs) / 2
	}
	e.statsMu.Unlock()

	if result.Success && result.ValidationResult != nil {
		e.handleSuccessfulResult(result)
	} else {
		e.handleFailedResult(result)
	}
}

// handleSuccessfulResult processes a successful validation result
func (e *ProcessingEngine) handleSuccessfulResult(result ProcessingResult) {
	atomic.AddInt64(&e.stats.SuccessfulJobs, 1)

	validationResult := result.ValidationResult
	var dbStatus int

	// Status already set by decision engine
	switch validationResult.Status {
	case "approved":
		dbStatus = 1
		atomic.AddInt64(&e.stats.AutoApproved, 1)
		log.Printf("Auto-approved venue %d with score %d (Decision engine)", result.VenueID, validationResult.Score)

	case "rejected":
		dbStatus = -1
		atomic.AddInt64(&e.stats.AutoRejected, 1)
		log.Printf("Auto-rejected venue %d with score %d (Decision engine)", result.VenueID, validationResult.Score)

	default: // manual_review
		dbStatus = 0
		atomic.AddInt64(&e.stats.ManualReview, 1)
		log.Printf("Venue %d requires manual review (score: %d) (Decision engine)", result.VenueID, validationResult.Score)
	}

	if e.scoreOnly {
		// Score-only mode: do not update venue status, only record history with Google data
		if err := e.db.SaveValidationResultWithGoogleData(validationResult, result.GoogleData); err != nil {
			log.Printf("Failed to save validation history for venue %d: %v", result.VenueID, err)
		}
		return
	}

	// Normal mode: update venue active only (do not write process notes into venues); save validation result + history separately
	if err := e.db.UpdateVenueActive(result.VenueID, dbStatus); err != nil {
		log.Printf("Failed to update venue %d active status: %v", result.VenueID, err)
	}

	if err := e.db.SaveValidationResult(validationResult); err != nil {
		log.Printf("Failed to save validation result for venue %d: %v", result.VenueID, err)
	}
}

// handleFailedResult processes a failed processing result
func (e *ProcessingEngine) handleFailedResult(result ProcessingResult) {
	atomic.AddInt64(&e.stats.FailedJobs, 1)
	atomic.AddInt64(&e.stats.ManualReview, 1)

	// Do not write error details into venues.admin_note; set active to manual review only
	if err := e.db.UpdateVenueActive(result.VenueID, 0); err != nil {
		log.Printf("Failed to set venue %d to manual review: %v", result.VenueID, err)
	}

	// If we have Google Places data, persist it to validation history even when AI scoring failed
	if result.GoogleData != nil {
		vr := &models.ValidationResult{
			VenueID:        result.VenueID,
			Score:          0,
			Status:         "manual_review",
			Notes:          "AI scoring failed; saved Google data for manual review",
			ScoreBreakdown: map[string]int{"google_data_only": 1},
		}
		if err := e.db.SaveValidationResultWithGoogleData(vr, result.GoogleData); err != nil {
			log.Printf("Failed to save Google data on failure for venue %d: %v", result.VenueID, err)
		}
	}

	log.Printf("Failed to process venue %d after %d retries: %v", result.VenueID, result.Retries, result.Error)
}

// Utility functions

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	s = strings.ToLower(s)
	for _, substring := range substrings {
		if strings.Contains(s, strings.ToLower(substring)) {
			return true
		}
	}
	return false
}

// isVenueAdmin checks if a user is an admin for a specific venue
func isVenueAdmin(user models.User, venue models.Venue) bool {
	// This would typically check a venue_admin table
	// For now, we'll use a simple heuristic based on trusted status
	return user.Trusted
}

// hasAmbassadorLevel checks if a user has ambassador privileges
func hasAmbassadorLevel(user models.User) bool {
	return user.ID > 0 && user.Trusted // Simplified check
}
