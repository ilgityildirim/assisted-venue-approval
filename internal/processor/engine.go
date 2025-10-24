package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assisted-venue-approval/internal/decision"
	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/trust"
	"assisted-venue-approval/pkg/events"
	"assisted-venue-approval/pkg/metrics"
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

// Reset clears a ProcessingJob for reuse
func (j *ProcessingJob) Reset() {
	j.Venue = models.Venue{}
	j.User = models.User{}
	j.Priority = 0
	j.Retry = 0
}

// Reset clears a ProcessingResult for reuse
func (r *ProcessingResult) Reset() {
	r.VenueID = 0
	r.Success = false
	r.ValidationResult = nil
	r.GoogleData = nil
	r.Error = nil
	r.ProcessingTimeMs = 0
	r.Retries = 0
}

// Pools and stats for hot-path objects
var (
	jobPool = sync.Pool{New: func() any {
		atomic.AddInt64(&jobPoolMisses, 1)
		return &ProcessingJob{}
	}}
	resultPool = sync.Pool{New: func() any {
		atomic.AddInt64(&resultPoolMisses, 1)
		return &ProcessingResult{}
	}}

	jobPoolGets      int64
	jobPoolPuts      int64
	jobPoolMisses    int64
	resultPoolGets   int64
	resultPoolPuts   int64
	resultPoolMisses int64

	// metrics
	mProcQueued       = metrics.Default.Counter("venue_processing_queued_total", "Venues queued for processing")
	mProcCompleted    = metrics.Default.Counter("venue_processing_completed_total", "Venues processed (completed)")
	mProcSuccess      = metrics.Default.Counter("venue_processing_success_total", "Successful processing results")
	mProcFailed       = metrics.Default.Counter("venue_processing_failed_total", "Failed processing results")
	mProcDuration     = metrics.Default.Histogram("venue_processing_duration_seconds", "Processing time per venue (seconds)", []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 60, 120})
	mQueueGauge       = metrics.Default.Gauge("venue_processing_queue_size", "Current processing queue size")
	mApiGoogle        = metrics.Default.Counter("google_api_calls_total", "Google Maps API calls")
	mApiOpenAI        = metrics.Default.Counter("openai_api_calls_total", "OpenAI API calls")
	mDecisionAutoAppr = metrics.Default.Counter("decision_auto_approved_total", "Auto-approved venues")
	mDecisionAutoRej  = metrics.Default.Counter("decision_auto_rejected_total", "Auto-rejected venues")
	mDecisionManual   = metrics.Default.Counter("decision_manual_review_total", "Venues sent to manual review")
)

func getProcessingJob() *ProcessingJob {
	atomic.AddInt64(&jobPoolGets, 1)
	return jobPool.Get().(*ProcessingJob)
}

func putProcessingJob(j *ProcessingJob) {
	if j == nil {
		return
	}
	j.Reset()
	atomic.AddInt64(&jobPoolPuts, 1)
	jobPool.Put(j)
}

func getProcessingResult() *ProcessingResult {
	atomic.AddInt64(&resultPoolGets, 1)
	return resultPool.Get().(*ProcessingResult)
}

func putProcessingResult(r *ProcessingResult) {
	if r == nil {
		return
	}
	r.Reset()
	atomic.AddInt64(&resultPoolPuts, 1)
	resultPool.Put(r)
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

	// Pool stats (hot paths)
	JobPoolGets      int64
	JobPoolPuts      int64
	JobPoolMisses    int64
	ResultPoolGets   int64
	ResultPoolPuts   int64
	ResultPoolMisses int64
	BufferPoolGets   int64
	BufferPoolPuts   int64
	BufferPoolMisses int64
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
// GoogleScraper abstracts the Google Maps integration used by the engine.
type GoogleScraper interface {
	EnhanceVenueWithValidation(ctx context.Context, venue models.Venue) (*models.Venue, error)
}

// VenueScorer abstracts the AI scoring used by the engine.
type VenueScorer interface {
	ScoreVenue(ctx context.Context, venue models.Venue, user models.User) (*models.ValidationResult, error)
	GetCostStats() (totalTokens int, totalRequests int, estimatedCostUSD float64, duration time.Duration)
	GetBufferPoolStats() (gets int64, puts int64, misses int64)
}

// QualityReviewer abstracts the quality suggestions used by the engine.
type QualityReviewer interface {
	ReviewQuality(ctx context.Context, venue models.Venue, user models.User, category string, trustLevel float64) (*models.QualitySuggestions, error)
}

type ProcessingEngine struct {
	repo            domain.Repository
	uowFactory      domain.UnitOfWorkFactory
	scraper         GoogleScraper
	scorer          VenueScorer
	qualityReviewer QualityReviewer
	decisionEngine  *decision.DecisionEngine
	trustCalc       *trust.Calculator
	eventStore      events.EventStore

	// Configuration
	workerCount int
	maxRetries  int
	retryDelay  time.Duration
	jobTimeout  time.Duration
	// AVA qualification configuration
	avaConfigMu         sync.RWMutex
	minUserPointsForAVA int
	onlyAmbassadors     bool

	// Mode flags
	scoreOnly bool

	// Rate limiters
	googleRateLimit *RateLimiter
	openAIRateLimit *RateLimiter

	// Processing control
	jobQueue   chan *ProcessingJob
	resultChan chan *ProcessingResult
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// worker management
	workersMu    sync.Mutex
	workerStops  []chan struct{}
	nextWorkerID int

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
	// Automatic Venue Approval (AVA) qualification requirements
	MinUserPointsForAVA int  // Minimum ambassador points required for automated reviews (0 = disabled)
	OnlyAmbassadors     bool // If true, only ambassadors can submit for automated review
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
		// AVA qualification defaults - cost-optimized
		MinUserPointsForAVA: 150,
		OnlyAmbassadors:     false,
	}
}

// NewProcessingEngine creates a new concurrent processing engine
func NewProcessingEngine(repo domain.Repository, uowFactory domain.UnitOfWorkFactory, scraper GoogleScraper, scorer VenueScorer, qualityReviewer QualityReviewer, config ProcessingConfig, decisionConfig decision.DecisionConfig) *ProcessingEngine {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize decision engine with provided configuration (env-driven)
	decisionEngine := decision.NewDecisionEngine(decisionConfig)

	engine := &ProcessingEngine{
		repo:                repo,
		uowFactory:          uowFactory,
		scraper:             scraper,
		scorer:              scorer,
		qualityReviewer:     qualityReviewer,
		decisionEngine:      decisionEngine,
		trustCalc:           trust.NewDefault(),
		workerCount:         config.WorkerCount,
		maxRetries:          config.MaxRetries,
		retryDelay:          config.RetryDelay,
		jobTimeout:          config.JobTimeout,
		minUserPointsForAVA: config.MinUserPointsForAVA,
		onlyAmbassadors:     config.OnlyAmbassadors,
		googleRateLimit:     NewRateLimiter(config.GoogleRPS, config.GoogleBurst),
		openAIRateLimit:     NewRateLimiter(config.OpenAIRPS, config.OpenAIBurst),
		jobQueue:            make(chan *ProcessingJob, config.QueueSize),
		resultChan:          make(chan *ProcessingResult, config.QueueSize),
		ctx:                 ctx,
		cancel:              cancel,
		shutdown:            make(chan struct{}),
		scoreOnly:           false,
		stats: ProcessingStats{
			StartTime:    time.Now(),
			LastActivity: time.Now(),
			WorkerCount:  config.WorkerCount,
		},
	}

	return engine
}

// ApplyConfig applies runtime-configurable changes safely.
// Supports resizing worker pool and updating AVA qualification settings.
func (e *ProcessingEngine) ApplyConfig(newWorkers int, approvalThreshold int) {
	// Resize workers if needed
	if newWorkers > 0 {
		e.resizeWorkers(newWorkers)
	}
	// Forward to decision engine for threshold update if provided (>0)
	if e.decisionEngine != nil && approvalThreshold > 0 {
		// best-effort; decision engine will clamp values as needed
		type applier interface{ ApplyConfig(approvalThreshold int) }
		if a, ok := interface{}(e.decisionEngine).(applier); ok {
			a.ApplyConfig(approvalThreshold)
		}
	}
}

// ApplyAVAConfig updates AVA qualification requirements at runtime with thread safety.
func (e *ProcessingEngine) ApplyAVAConfig(minUserPoints int, onlyAmbassadors bool) {
	e.avaConfigMu.Lock()
	defer e.avaConfigMu.Unlock()

	// Only log if values actually changed
	changed := e.minUserPointsForAVA != minUserPoints || e.onlyAmbassadors != onlyAmbassadors

	e.minUserPointsForAVA = minUserPoints
	e.onlyAmbassadors = onlyAmbassadors

	if changed {
		log.Printf("AVA config updated: MinUserPointsForAVA=%d, OnlyAmbassadors=%v",
			minUserPoints, onlyAmbassadors)
	}
}

func (e *ProcessingEngine) resizeWorkers(target int) {
	e.workersMu.Lock()
	defer e.workersMu.Unlock()

	cur := len(e.workerStops)
	if target == cur {
		return
	}
	if target > cur {
		// scale up
		for i := 0; i < target-cur; i++ {
			stopCh := make(chan struct{})
			e.workerStops = append(e.workerStops, stopCh)
			id := e.nextWorkerID
			e.nextWorkerID++
			e.wg.Add(1)
			go e.worker(id, stopCh)
		}
		e.statsMu.Lock()
		e.stats.WorkerCount = target
		e.statsMu.Unlock()
		log.Printf("Scaled workers up: %d -> %d", cur, target)
		return
	}
	// scale down: close extra stop channels; workers will exit gracefully after current job
	toStop := cur - target
	for i := 0; i < toStop; i++ {
		idx := len(e.workerStops) - 1
		close(e.workerStops[idx])
		e.workerStops = e.workerStops[:idx]
	}
	e.statsMu.Lock()
	e.stats.WorkerCount = target
	e.statsMu.Unlock()
	log.Printf("Scaled workers down: %d -> %d", cur, target)
}

// SetEventStore wires an EventStore for audit trail publishing.
func (e *ProcessingEngine) SetEventStore(es events.EventStore) {
	e.eventStore = es
	// Try to set on decision engine if it supports it
	if e.decisionEngine != nil {
		type setter interface{ SetEventStore(events.EventStore) }
		if s, ok := interface{}(e.decisionEngine).(setter); ok {
			s.SetEventStore(es)
		}
	}
}

// Start begins the processing engine with workers and rate limiters
func (e *ProcessingEngine) Start() {
	log.Printf("Starting processing engine with %d workers", e.workerCount)

	// Start rate limiters
	e.googleRateLimit.Start()
	e.openAIRateLimit.Start()

	// Start workers
	e.workersMu.Lock()
	for i := 0; i < e.workerCount; i++ {
		stopCh := make(chan struct{})
		e.workerStops = append(e.workerStops, stopCh)
		id := e.nextWorkerID
		e.nextWorkerID++
		e.wg.Add(1)
		go e.worker(id, stopCh)
	}
	e.workersMu.Unlock()

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

// ProcessVenuesWithUsers adds venues with user data to the processing queue
func (e *ProcessingEngine) ProcessVenuesWithUsers(venuesWithUser []models.VenueWithUser) error {
	e.statsMu.Lock()
	e.stats.TotalJobs = int64(len(venuesWithUser))
	e.statsMu.Unlock()

	log.Printf("Queuing %d venues with user data for processing", len(venuesWithUser))

	for _, vw := range venuesWithUser {
		priority := e.calculatePriorityWithUser(vw.Venue, vw.User)
		job := getProcessingJob()
		job.Venue = vw.Venue
		job.User = vw.User
		job.Priority = priority
		job.Retry = 0

		select {
		case e.jobQueue <- job:
			atomic.AddInt64(&e.stats.QueueSize, 1)
			mProcQueued.Inc(1)
			mQueueGauge.SetFloat64(float64(atomic.LoadInt64(&e.stats.QueueSize)))
		case <-e.ctx.Done():
			// return job to pool if we can't enqueue
			putProcessingJob(job)
			return fmt.Errorf("processing engine is shutting down")
		default:
			putProcessingJob(job)
			return fmt.Errorf("job queue is full")
		}
	}

	log.Printf("Successfully queued %d venues with user data", len(venuesWithUser))
	return nil
}

// ProcessSingleVenueSync processes a single venue synchronously without using the job queue.
// This is intended for UI-triggered single venue reviews where immediate feedback is needed.
// For batch operations and automated tasks, use ProcessVenuesWithUsers instead.
func (e *ProcessingEngine) ProcessSingleVenueSync(ctx context.Context, venueWithUser models.VenueWithUser) (*ProcessingResult, error) {
	log.Printf("Starting synchronous processing for venue %d", venueWithUser.Venue.ID)

	// Create a job struct for processing (not using pool since we're not queuing)
	job := &ProcessingJob{
		Venue:    venueWithUser.Venue,
		User:     venueWithUser.User,
		Priority: e.calculatePriorityWithUser(venueWithUser.Venue, venueWithUser.User),
		Retry:    0,
	}

	// Process the job directly
	result := e.processJob(job)

	// Persist the result to database
	if result.Success && result.ValidationResult != nil {
		// In score-only mode, just save validation result with Google data (no venue status update)
		if e.scoreOnly {
			if err := e.repo.SaveValidationResultWithGoogleDataCtx(ctx, result.ValidationResult, result.GoogleData); err != nil {
				log.Printf("Failed to save validation history for venue %d: %v", result.VenueID, err)
				result.Error = fmt.Errorf("failed to save validation result: %w", err)
				result.Success = false
				return result, err
			}
		} else {
			// Normal mode: update venue status atomically with validation result
			uow, err := e.uowFactory.Begin(ctx)
			if err != nil {
				log.Printf("Failed to begin unit of work for venue %d: %v", result.VenueID, err)
				result.Error = fmt.Errorf("failed to begin transaction: %w", err)
				result.Success = false
				return result, err
			}
			defer uow.Rollback()

			// Update venue status
			newStatus := map[string]int{
				"approved":      1,
				"rejected":      -1,
				"manual_review": 0,
			}[result.ValidationResult.Status]

			if err := uow.UpdateVenueStatusCtx(ctx, result.VenueID, newStatus, result.ValidationResult.Notes, nil); err != nil {
				log.Printf("Failed to update venue status for venue %d: %v", result.VenueID, err)
				result.Error = fmt.Errorf("failed to update venue status: %w", err)
				result.Success = false
				return result, err
			}

			// Save validation result with Google data
			if result.GoogleData != nil {
				if err := uow.SaveValidationResultWithGoogleDataCtx(ctx, result.ValidationResult, result.GoogleData); err != nil {
					log.Printf("Failed to save validation result for venue %d: %v", result.VenueID, err)
					result.Error = fmt.Errorf("failed to save validation result: %w", err)
					result.Success = false
					return result, err
				}
			} else {
				if err := uow.SaveValidationResultCtx(ctx, result.ValidationResult); err != nil {
					log.Printf("Failed to save validation result for venue %d: %v", result.VenueID, err)
					result.Error = fmt.Errorf("failed to save validation result: %w", err)
					result.Success = false
					return result, err
				}
			}

			if err := uow.Commit(); err != nil {
				log.Printf("Failed to commit transaction for venue %d: %v", result.VenueID, err)
				result.Error = fmt.Errorf("failed to commit transaction: %w", err)
				result.Success = false
				return result, err
			}
		}
	}

	// Update stats
	e.statsMu.Lock()
	e.stats.CompletedJobs++
	if result.Success {
		e.stats.SuccessfulJobs++
	} else {
		e.stats.FailedJobs++
	}
	if result.ValidationResult != nil {
		switch result.ValidationResult.Status {
		case "approved":
			e.stats.AutoApproved++
		case "rejected":
			e.stats.AutoRejected++
		case "manual_review":
			e.stats.ManualReview++
		}
	}
	e.stats.LastActivity = time.Now()
	e.statsMu.Unlock()

	log.Printf("Synchronous processing completed for venue %d: success=%v", result.VenueID, result.Success)

	return result, nil
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

	// Pool stats snapshot
	stats.JobPoolGets = atomic.LoadInt64(&jobPoolGets)
	stats.JobPoolPuts = atomic.LoadInt64(&jobPoolPuts)
	stats.JobPoolMisses = atomic.LoadInt64(&jobPoolMisses)
	stats.ResultPoolGets = atomic.LoadInt64(&resultPoolGets)
	stats.ResultPoolPuts = atomic.LoadInt64(&resultPoolPuts)
	stats.ResultPoolMisses = atomic.LoadInt64(&resultPoolMisses)

	// If scorer exposes buffer pool stats, include them (optional)
	if bGets, bPuts, bMiss := e.scorer.GetBufferPoolStats(); bGets >= 0 {
		stats.BufferPoolGets = bGets
		stats.BufferPoolPuts = bPuts
		stats.BufferPoolMisses = bMiss
	}

	return stats
}

// calculatePriorityWithUser determines job priority based on venue and user characteristics
func (e *ProcessingEngine) calculatePriorityWithUser(venue models.Venue, user models.User) int {
	priority := 0

	// Highest priority for venue owners/admins (fast-track approval)
	if user.ID > 0 && (user.Trusted || (user.IsVenueAdmin || user.IsVenueOwner)) {
		priority += 1000
	}

	// High priority for ambassadors
	if user.AmbassadorLevel != nil && *user.AmbassadorLevel > 0 {
		priority += 500
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

// requiresManualReviewEarly checks if venue should bypass automated review for cost optimization
func (e *ProcessingEngine) requiresManualReviewEarly(venue *models.Venue, user *models.User, trustAssessment *trust.Assessment) (bool, EarlyExitReason) {
	// Thread-safe read of AVA configuration
	e.avaConfigMu.RLock()
	minUserPointsForAVA := e.minUserPointsForAVA
	onlyAmbassadors := e.onlyAmbassadors
	e.avaConfigMu.RUnlock()

	// Run all early exit checks using helper functions
	if skip, reason := checkMinimumPoints(user, minUserPointsForAVA); skip {
		return true, reason
	}

	if skip, reason := checkTrustLevel(trustAssessment); skip {
		return true, reason
	}

	if skip, reason := checkVenueType(venue); skip {
		return true, reason
	}

	if skip, reason := checkAmbassadorRequirement(user, onlyAmbassadors); skip {
		return true, reason
	}

	// All checks passed - venue can proceed to API calls
	return false, EarlyExitReason{}
}

// worker processes jobs from the queue
func (e *ProcessingEngine) worker(id int, stopCh <-chan struct{}) {
	defer e.wg.Done()

	log.Printf("Worker %d started", id)
	defer log.Printf("Worker %d stopped", id)

	for {
		select {
		case <-stopCh:
			return
		case job, ok := <-e.jobQueue:
			if !ok {
				return // Queue closed, worker should exit
			}

			atomic.AddInt64(&e.stats.QueueSize, -1)
			mQueueGauge.SetFloat64(float64(atomic.LoadInt64(&e.stats.QueueSize)))
			result := e.processJob(job)

			select {
			case e.resultChan <- result:
				// after successful send, return job to pool
				putProcessingJob(job)
			case <-e.ctx.Done():
				// if shutting down, return both objects
				putProcessingResult(result)
				putProcessingJob(job)
				return
			}

		case <-e.ctx.Done():
			return
		}
	}
}

// processJob processes a single venue with error recovery
func (e *ProcessingEngine) processJob(job *ProcessingJob) *ProcessingResult {
	startTime := time.Now()
	venue := job.Venue
	user := job.User

	// Create job-specific context with timeout
	jobCtx, cancel := context.WithTimeout(e.ctx, e.jobTimeout)
	defer cancel()

	result := getProcessingResult()
	result.VenueID = venue.ID
	result.Success = false
	result.ProcessingTimeMs = 0
	result.Retries = job.Retry

	// Centralized manual review checks (admin notes, region restrictions)
	// This check runs early to prevent API costs for venues with admin notes or Asian region restrictions
	if skip, reason := models.ShouldRequireManualReview(job.Venue); skip {
		log.Printf("[Early Exit] Venue %d: %s", venue.ID, reason)

		key := "manual_review"
		if strings.Contains(reason, "Admin") {
			key = "admin_note_block"
		} else if strings.Contains(reason, "Asian") {
			key = "asian_venue_block"
		}
		result.ValidationResult = &models.ValidationResult{
			VenueID:        job.Venue.ID,
			Score:          0,
			Status:         "manual_review",
			Notes:          reason,
			ScoreBreakdown: map[string]int{key: 0},
		}
		result.Success = true

		// Publish early exit event for consistency
		if e.eventStore != nil {
			if err := e.eventStore.Append(jobCtx, events.VenueRequiresManualReview{
				Base:   events.Base{Ts: time.Now(), VID: venue.ID},
				Reason: reason,
			}); err != nil {
				log.Printf("[Warning] Failed to append manual review event for venue %d: %v", venue.ID, err)
			}
		}

		// Update metrics
		atomic.AddInt64(&e.stats.ManualReview, 1)
		mDecisionManual.Inc(1)

		return result
	}

	// Path validation check - prevent API costs for venues with incorrect/unusual paths
	if venue.Path != nil && *venue.Path != "" {
		path := strings.TrimSpace(*venue.Path)

		// Check 1: Path must contain pipe separator for hierarchical structure (e.g., "asia|china")
		if !strings.Contains(path, "|") {
			result.ValidationResult = &models.ValidationResult{
				VenueID:        venue.ID,
				Score:          0,
				Status:         "manual_review",
				Notes:          "Manual Review: Venue path format is invalid (missing hierarchy separator '|')",
				ScoreBreakdown: map[string]int{"invalid_path_format": 0},
			}
			result.Success = true
			return result
		}

		// Check 2: Count other venues in this path
		count, err := e.repo.CountVenuesByPathCtx(jobCtx, path, venue.ID)
		if err == nil {
			if count == 0 {
				// No other venues use this path - likely incorrect path selection
				result.ValidationResult = &models.ValidationResult{
					VenueID:        venue.ID,
					Score:          0,
					Status:         "manual_review",
					Notes:          "Manual Review: No other venues found in this path - likely incorrect path selection",
					ScoreBreakdown: map[string]int{"unique_path_flag": 0},
				}
				result.Success = true
				return result
			}
			//} else if count <= 2 {
			//	// Very few venues in path - uncommon, flag for review
			//	result.ValidationResult = &models.ValidationResult{
			//		VenueID:        venue.ID,
			//		Score:          0,
			//		Status:         "manual_review",
			//		Notes:          fmt.Sprintf("Manual Review: Only %d venue(s) in this path - potentially incorrect path selection", count),
			//		ScoreBreakdown: map[string]int{"uncommon_path_flag": 0},
			//	}
			//	result.Success = true
			//	return result
			//}
		}
	}

	// Calculate trust assessment early (before API calls) for early exit logic
	// User might be empty for some venues, handle gracefully
	var trustAssessment *trust.Assessment
	if user.ID > 0 {
		assessment := e.trustCalc.Assess(user, venue.Location)
		trustAssessment = &assessment
	} else {
		// No user data available - venue will likely require manual review
		log.Printf("[Warning] Venue %d has no associated user data", venue.ID)
	}

	// Early exit check - bypass API calls if venue should go directly to manual review
	// This prevents unnecessary costs for venues that don't meet automated review criteria
	requiresEarlyReview, exitReason := e.requiresManualReviewEarly(&venue, &user, trustAssessment)
	if requiresEarlyReview {
		log.Printf("[Early Exit] Venue %d bypassing API calls: %s", venue.ID, exitReason.String())

		// Create validation result for manual review
		result.ValidationResult = &models.ValidationResult{
			VenueID:        venue.ID,
			Score:          earlyExitManualReviewScore,
			Status:         "manual_review",
			Notes:          exitReason.String(),
			ScoreBreakdown: map[string]int{exitReason.Code: 0},
		}
		result.Success = true

		// Publish early exit event
		if e.eventStore != nil {
			if err := e.eventStore.Append(jobCtx, events.VenueRequiresManualReview{
				Base:   events.Base{Ts: time.Now(), VID: venue.ID},
				Reason: exitReason.String(),
			}); err != nil {
				log.Printf("[Warning] Failed to append early exit event for venue %d: %v", venue.ID, err)
			}
		}

		// Update metrics
		atomic.AddInt64(&e.stats.ManualReview, 1)
		mDecisionManual.Inc(1)

		return result
	}

	// Publish start event
	if e.eventStore != nil {
		uid := user.ID
		if err := e.eventStore.Append(jobCtx, events.VenueValidationStarted{
			Base:      events.Base{Ts: time.Now(), VID: venue.ID},
			UserID:    &uid,
			Triggered: "system",
		}); err != nil {
			log.Printf("[Warning] Failed to append validation started event for venue %d: %v", venue.ID, err)
		}
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
		validationResult, googleData, err = e.processVenueWithRateLimit(jobCtx, venue, user, trustAssessment)
		if err == nil {
			result.Success = true
			result.ValidationResult = validationResult
			result.GoogleData = googleData
			// Publish completion event with summary details
			if e.eventStore != nil && validationResult != nil {
				gdFound := false
				gpID := ""
				if googleData != nil {
					gdFound = true
					gpID = googleData.PlaceID
				}
				if err := e.eventStore.Append(jobCtx, events.VenueValidationCompleted{
					Base:           events.Base{Ts: time.Now(), VID: venue.ID},
					Score:          validationResult.Score,
					Status:         map[string]int{"approved": 1, "rejected": -1, "manual_review": 0}[validationResult.Status],
					Notes:          validationResult.Notes,
					ScoreBreakdown: validationResult.ScoreBreakdown,
					GoogleFound:    gdFound,
					GooglePlaceID:  gpID,
					Conflicts:      nil,
				}); err != nil {
					log.Printf("[Warning] Failed to append validation completed event for venue %d: %v", venue.ID, err)
				}
			}
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
func (e *ProcessingEngine) processVenueWithRateLimit(ctx context.Context, venue models.Venue, user models.User, trustAssessment *trust.Assessment) (*models.ValidationResult, *models.GooglePlaceData, error) {
	// Rate limit Google Maps API call
	if err := e.googleRateLimit.Wait(ctx); err != nil {
		return nil, nil, fmt.Errorf("google rate limit wait cancelled: %w", err)
	}

	// Enhance venue with Google Maps data
	enhancedVenue, err := e.scraper.EnhanceVenueWithValidation(ctx, venue)
	if err != nil {
		atomic.AddInt64(&e.stats.APICallsGoogle, 1)
		mApiGoogle.Inc(1)
		return nil, nil, fmt.Errorf("failed to enhance venue: %w", err)
	}
	atomic.AddInt64(&e.stats.APICallsGoogle, 1)
	mApiGoogle.Inc(1)

	// Prepare Google data (if any) early so we can return it even on AI failure
	var gData *models.GooglePlaceData
	if enhancedVenue.GoogleData != nil {
		gData = enhancedVenue.GoogleData
	}

	// If no location information available after Google enrichment, require manual review
	if enhancedVenue.Lat == nil || enhancedVenue.Lng == nil ||
		(*enhancedVenue.Lat == 0.0 && *enhancedVenue.Lng == 0.0) {
		vr := &models.ValidationResult{
			VenueID:        venue.ID,
			Score:          0,
			Status:         "manual_review",
			Notes:          "No location coordinates available - manual review required",
			ScoreBreakdown: map[string]int{"no_location": 0},
		}
		return vr, gData, nil
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
		mApiOpenAI.Inc(1)
		return nil, gData, fmt.Errorf("failed to score venue: %w", err)
	}
	atomic.AddInt64(&e.stats.APICallsOpenAI, 1)
	mApiOpenAI.Inc(1)

	// Use trust assessment calculated earlier (or calculate if not provided)
	var trustLevel float64
	if trustAssessment != nil {
		trustLevel = trustAssessment.Trust
	} else {
		// Fallback: calculate trust if not provided (shouldn't happen in normal flow)
		assessment := e.trustCalc.Assess(user, venue.Location)
		trustLevel = assessment.Trust
	}

	// Run quality review (separate API call) - optional, doesn't fail scoring
	var qualitySuggestions *models.QualitySuggestions
	if e.qualityReviewer != nil {
		category := getCategoryFromVenue(*enhancedVenue)
		qualitySuggestions, err = e.qualityReviewer.ReviewQuality(ctx, *enhancedVenue, user, category, trustLevel)
		if err != nil {
			log.Printf("quality review failed for venue %d: %v (continuing without quality data)", venue.ID, err)
			// Don't fail the whole process, continue without quality data
		}
	}

	// Combine scoring + quality suggestions into ai_output_data JSON
	if qualitySuggestions != nil {
		combinedJSON := buildCombinedOutput(validationResult, qualitySuggestions)
		validationResult.AIOutputData = &combinedJSON
	}

	// Use decision engine to make final decision with user context
	decisionResult := e.decisionEngine.MakeDecision(ctx, *enhancedVenue, user, validationResult)

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
			putProcessingResult(result)

		case <-e.ctx.Done():
			return
		}
	}
}

// handleResult processes a venue processing result
func (e *ProcessingEngine) handleResult(result *ProcessingResult) {
	// metrics first
	mProcCompleted.Inc(1)
	mProcDuration.Observe(float64(result.ProcessingTimeMs) / 1000.0)

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
		mProcSuccess.Inc(1)
		e.handleSuccessfulResult(result)
	} else {
		mProcFailed.Inc(1)
		e.handleFailedResult(result)
	}
}

// handleSuccessfulResult processes a successful validation result
func (e *ProcessingEngine) handleSuccessfulResult(result *ProcessingResult) {
	atomic.AddInt64(&e.stats.SuccessfulJobs, 1)

	validationResult := result.ValidationResult
	var dbStatus int

	// Status already set by decision engine
	switch validationResult.Status {
	case "approved":
		dbStatus = 1
		atomic.AddInt64(&e.stats.AutoApproved, 1)
		mDecisionAutoAppr.Inc(1)
		log.Printf("Auto-approved venue %d with score %d (Decision engine)", result.VenueID, validationResult.Score)

	case "rejected":
		dbStatus = -1
		atomic.AddInt64(&e.stats.AutoRejected, 1)
		mDecisionAutoRej.Inc(1)
		log.Printf("Auto-rejected venue %d with score %d (Decision engine)", result.VenueID, validationResult.Score)

	default: // manual_review
		dbStatus = 0
		atomic.AddInt64(&e.stats.ManualReview, 1)
		mDecisionManual.Inc(1)
		log.Printf("Venue %d requires manual review (score: %d) (Decision engine)", result.VenueID, validationResult.Score)
	}

	if e.scoreOnly {
		// Score-only mode: do not update venue status, only record history with Google data
		if err := e.repo.SaveValidationResultWithGoogleDataCtx(e.ctx, validationResult, result.GoogleData); err != nil {
			log.Printf("Failed to save validation history for venue %d: %v", result.VenueID, err)
		}
		return
	}

	// Normal mode: perform both writes atomically via UnitOfWork
	uow, err := e.uowFactory.Begin(e.ctx)
	if err != nil {
		log.Printf("Failed to begin unit of work for venue %d: %v", result.VenueID, err)
		return
	}
	defer uow.Rollback()

	// Save validation result FIRST so it can be validated before status update
	if err := uow.SaveValidationResultCtx(e.ctx, validationResult); err != nil {
		log.Printf("Failed to save validation result for venue %d: %v", result.VenueID, err)
		return
	}

	// For approvals, validate that the venue has a valid validation history before updating status
	// Venue can only be approved if there's a validation history with status='approved' and score >= threshold
	if dbStatus == 1 { // Approval
		const approvalThreshold = 75
		if err := e.repo.ValidateApprovalEligibility(result.VenueID, approvalThreshold); err != nil {
			log.Printf("Cannot approve venue %d: %v", result.VenueID, err)
			// Set to manual review instead
			dbStatus = 0
		}
	}

	if err := uow.UpdateVenueActiveCtx(e.ctx, result.VenueID, dbStatus); err != nil {
		log.Printf("Failed to update venue %d active status: %v", result.VenueID, err)
		return
	}

	if err := uow.Commit(); err != nil {
		log.Printf("Failed to commit unit of work for venue %d: %v", result.VenueID, err)
	}
}

// handleFailedResult processes a failed processing result
func (e *ProcessingEngine) handleFailedResult(result *ProcessingResult) {
	atomic.AddInt64(&e.stats.FailedJobs, 1)
	atomic.AddInt64(&e.stats.ManualReview, 1)

	// Do not write error details into venues.admin_note; set active to manual review only
	uow, err := e.uowFactory.Begin(e.ctx)
	if err != nil {
		log.Printf("Failed to begin unit of work for failed result %d: %v", result.VenueID, err)
		return
	}
	defer uow.Rollback()

	if err := uow.UpdateVenueActiveCtx(e.ctx, result.VenueID, 0); err != nil {
		log.Printf("Failed to set venue %d to manual review: %v", result.VenueID, err)
		return
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
		if err := uow.SaveValidationResultWithGoogleDataCtx(e.ctx, vr, result.GoogleData); err != nil {
			log.Printf("Failed to save Google data on failure for venue %d: %v", result.VenueID, err)
			return
		}
	}
	if err := uow.Commit(); err != nil {
		log.Printf("Failed to commit failed-result unit of work for venue %d: %v", result.VenueID, err)
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

// getCategoryFromVenue extracts category name from venue
func getCategoryFromVenue(venue models.Venue) string {
	// Map HappyCow category IDs to display names
	categoryMap := map[int]string{
		1: "Restaurant",
		2: "Health Food Store",
		3: "Bakery",
		4: "Caterer",
		5: "Juice Bar",
		6: "Market/Co-op",
		7: "Ice Cream Shop",
		8: "Organization",
		9: "B&B/Hotel",
	}

	if category, ok := categoryMap[venue.Category]; ok {
		return category
	}
	return "Restaurant" // Default fallback
}

// buildCombinedOutput combines scoring and quality suggestions into JSON for ai_output_data field
func buildCombinedOutput(scoring *models.ValidationResult, quality *models.QualitySuggestions) string {
	combined := map[string]interface{}{
		"scoring": map[string]interface{}{
			"score":     scoring.Score,
			"notes":     scoring.Notes,
			"breakdown": scoring.ScoreBreakdown,
		},
	}

	if quality != nil {
		combined["quality"] = quality
	}

	data, _ := json.Marshal(combined)
	return string(data)
}
