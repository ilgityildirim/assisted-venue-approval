package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/joho/godotenv/autoload"

	"assisted-venue-approval/internal/admin"
	"assisted-venue-approval/internal/auth"
	"assisted-venue-approval/internal/decision"
	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/infrastructure/repository"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/processor"
	"assisted-venue-approval/internal/prompts"
	"assisted-venue-approval/internal/scorer"
	"assisted-venue-approval/internal/scraper"
	"assisted-venue-approval/pkg/config"
	"assisted-venue-approval/pkg/container"
	"assisted-venue-approval/pkg/database"
	"assisted-venue-approval/pkg/events"
	metricsPkg "assisted-venue-approval/pkg/metrics"
	"assisted-venue-approval/pkg/monitoring"
)

func main() {
	// Set embedded config filesystem for prompts package
	prompts.ConfigFilesFS = ConfigFiles()

	// Build container and register providers
	c := container.New()

	// Config (singleton)
	_ = c.Provide(func() *config.Config { return config.Load() }, true)

	// Database (singleton)
	_ = c.Provide(func(cfg *config.Config) (*database.DB, error) { return database.NewWithConfig(cfg.DatabaseURL, cfg) }, true)

	// Repository and UoW factory (singletons)
	_ = c.Provide(func(db *database.DB) domain.Repository { return repository.NewSQLRepository(db) }, true)
	_ = c.Provide(func(db *database.DB) domain.UnitOfWorkFactory { return repository.NewSQLUnitOfWorkFactory(db) }, true)

	// External clients (singletons)
	_ = c.Provide(func(cfg *config.Config) (*scraper.GoogleMapsScraper, error) {
		return scraper.NewGoogleMapsScraper(cfg.GoogleMapsAPIKey)
	}, true)
	// Prompts manager with optional external overrides
	_ = c.Provide(func(cfg *config.Config) (*prompts.Manager, error) {
		return prompts.NewManager(cfg.PromptDir)
	}, true)
	_ = c.Provide(func(cfg *config.Config, pm *prompts.Manager) *scorer.AIScorer {
		return scorer.NewAIScorerWithTimeoutAndPrompts(cfg.OpenAIAPIKey, cfg.OpenAITimeout, pm)
	}, true)

	// Quality reviewer (singleton)
	_ = c.Provide(func(cfg *config.Config, pm *prompts.Manager) *scorer.QualityReviewer {
		return scorer.NewQualityReviewer(cfg.OpenAIAPIKey, pm, cfg.OpenAITimeout)
	}, true)

	// Processing engine (singleton)
	_ = c.Provide(func(repo domain.Repository, uow domain.UnitOfWorkFactory, g *scraper.GoogleMapsScraper, s *scorer.AIScorer, qr *scorer.QualityReviewer, cfg *config.Config) *processor.ProcessingEngine {
		pc := processor.DefaultProcessingConfig()
		if cfg.WorkerCount > 0 {
			pc.WorkerCount = cfg.WorkerCount
		}
		// Apply AVA qualification configuration
		pc.MinUserPointsForAVA = cfg.MinUserPointsForAVA
		pc.OnlyAmbassadors = cfg.OnlyAmbassadors
		dc := decision.DefaultDecisionConfig()
		if cfg.ApprovalThreshold > 0 {
			dc.ApprovalThreshold = cfg.ApprovalThreshold
		}
		return processor.NewProcessingEngine(repo, uow, g, s, qr, pc, dc)
	}, true)

	// Event store (singleton)
	_ = c.Provide(func(db *database.DB) (events.EventStore, error) { return events.NewSQLEventStore(db) }, true)

	// Resolve config early for monitoring setup
	var cfg *config.Config
	if err := c.Resolve(&cfg); err != nil {
		log.Fatal("config resolve:", err)
	}
	monitoring.EnableProfiling(cfg.ProfilingEnabled)
	log.Println("Starting venue validation system")

	// Load templates
	if err := admin.LoadTemplates(Templates()); err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	// Set base path for templates
	admin.SetBasePath(cfg.BasePath)

	// Wire event store into engine and admin
	if err := c.Invoke(func(pe *processor.ProcessingEngine, es events.EventStore) {
		pe.SetEventStore(es)
		admin.SetEventStore(es)
	}); err != nil {
		log.Printf("Event store init failed: %v", err)
	}

	// Resolve runtime dependencies
	var (
		db   *database.DB
		repo domain.Repository
		eng  *processor.ProcessingEngine
	)
	if err := c.Resolve(&db); err != nil {
		log.Fatal("db resolve:", err)
	}
	if err := c.Resolve(&repo); err != nil {
		log.Fatal("repo resolve:", err)
	}
	if err := c.Resolve(&eng); err != nil {
		log.Fatal("engine resolve:", err)
	}

	app := &App{db: db, config: cfg, engine: eng}

	// Start config watcher for hot-reload (applies worker count, approval threshold, and AVA config)
	cw := config.NewWatcher(time.Duration(cfg.ConfigReloadIntervalSeconds) * time.Second)
	cw.Start()
	chgCh := cw.Subscribe()
	go func() {
		for chg := range chgCh {
			if chg.Err != nil {
				log.Printf("Config reload failed: %v", chg.Err)
				continue
			}
			// Apply relevant changes
			wc := chg.New.WorkerCount
			if wc <= 0 {
				wc = cfg.WorkerCount
			}
			eng.ApplyConfig(wc, chg.New.ApprovalThreshold)
			// Apply AVA qualification config updates
			eng.ApplyAVAConfig(chg.New.MinUserPointsForAVA, chg.New.OnlyAmbassadors)
			cfg = chg.New
			log.Printf("Config applied. Changed fields: %v", chg.Fields)
		}
	}()

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Received shutdown signal, initiating graceful shutdown...")
		if err := eng.Stop(30 * time.Second); err != nil {
			log.Printf("Processing engine shutdown error: %v", err)
		}
		cancel()
	}()

	// Initialize admin resolver for IP-based authentication
	adminResolver := auth.NewAdminResolver()

	// Create admin authentication middleware
	adminAuthMiddleware := auth.NewAdminAuthMiddleware(adminResolver, admin.RenderUnauthorized)

	// HTTP routing
	router := mux.NewRouter()

	var metrics *monitoring.Metrics
	if cfg.MetricsEnabled {
		metrics = monitoring.NewMetrics(512)
		router.Use(monitoring.Middleware(metrics))
	}

	// Apply admin authentication middleware to all routes
	router.Use(adminAuthMiddleware.Handler)

	router.HandleFunc("/", admin.HomeHandler(repo, eng)).Methods("GET")
	router.HandleFunc("/analytics", admin.AnalyticsHandler(db, eng)).Methods("GET")

	router.HandleFunc("/validate", app.validateHandler).Methods("POST")
	router.HandleFunc("/validate/batch", app.validateBatchHandler).Methods("POST")
	router.HandleFunc("/api/stats", admin.APIStatsHandler(db, eng)).Methods("GET")
	// Feedback analytics
	router.HandleFunc("/api/feedback/stats", admin.APIFeedbackStatsHandler(db)).Methods("GET")

	router.HandleFunc("/venues/pending", admin.PendingVenuesHandler(db)).Methods("GET")
	router.HandleFunc("/venues/manual-review", admin.ManualReviewHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}", admin.VenueDetailHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}/approve", admin.ApproveVenueHandler(repo, cfg)).Methods("POST")
	router.HandleFunc("/venues/{id}/reject", admin.RejectVenueHandler(repo)).Methods("POST")
	router.HandleFunc("/venues/{id}/validate", app.validateSingleHandler).Methods("POST")
	// Editor feedback submit/list
	router.HandleFunc("/venues/{id}/feedback", admin.SubmitFeedbackHandler(db)).Methods("POST")
	router.HandleFunc("/venues/{id}/feedback", admin.VenueFeedbackHandler(db)).Methods("GET")

	router.HandleFunc("/venues/batch-operation", admin.BatchOperationHandler(repo, cfg)).Methods("POST")
	router.HandleFunc("/validation/history", admin.ValidationHistoryHandler(db)).Methods("GET")
	router.HandleFunc("/editorial-feedback", admin.EditorialFeedbackListHandler(db)).Methods("GET")

	staticPath := cfg.BasePath + "static/"
	router.PathPrefix(staticPath).Handler(http.StripPrefix(staticPath, http.FileServer(http.FS(Static()))))
	server := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	var adminServer *http.Server
	if cfg.ProfilingEnabled || cfg.MetricsEnabled {
		mux := http.NewServeMux()
		if cfg.ProfilingEnabled {
			monitoring.RegisterPprof(mux)
		}
		if cfg.MetricsEnabled {
			// Expose Prometheus-compatible metrics at configurable path (default: /metrics)
			mux.Handle(cfg.MetricsPath, metricsPkg.Handler())
		}
		// Keep lightweight JSON metrics for humans at /metrics.json (non-Prometheus)
		if cfg.MetricsEnabled && metrics != nil && cfg.MetricsPath != "/metrics.json" {
			mux.Handle("/metrics.json", monitoring.MetricsHandlerWithCosts(metrics, func() (monitoring.CostMetrics, error) {
				st := eng.GetStats()
				var cpv float64
				if st.CompletedJobs > 0 {
					cpv = st.TotalCostUSD / float64(st.CompletedJobs)
				}
				return monitoring.CostMetrics{
					TotalCostUSD: st.TotalCostUSD,
					TotalVenues:  st.CompletedJobs,
					CostPerVenue: cpv,
				}, nil
			}))
		}
		adminServer = &http.Server{Addr: ":" + cfg.ProfilingPort, Handler: mux}
		go func() {
			fmt.Printf("Admin server (pprof/metrics) starting on port %s\n", cfg.ProfilingPort)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Admin HTTP server error: %v", err)
			}
		}()
	}

	// Start runtime performance monitor (alerts)
	if cfg.AlertsEnabled && cfg.MetricsEnabled && metrics != nil {
		go monitoring.StartRuntimeMonitor(ctx, cfg, metrics, func(format string, a ...any) { log.Printf(format, a...) })
	}

	go func() {
		fmt.Printf("Server starting on port %s\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server error:", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	if adminServer != nil {
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Admin HTTP server shutdown error: %v", err)
		}
	}
	log.Println("Application shutdown complete")
}

type App struct {
	db      *database.DB
	scraper *scraper.GoogleMapsScraper
	scorer  *scorer.AIScorer
	config  *config.Config
	engine  *processor.ProcessingEngine
}

// validateHandler starts concurrent venue processing using the processing engine
func (app *App) validateHandler(w http.ResponseWriter, r *http.Request) {
	venuesWithUser, err := app.db.GetPendingVenuesWithUser()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get pending venues: %v", err), http.StatusInternalServerError)
		return
	}

	if len(venuesWithUser) == 0 {
		fmt.Fprintf(w, "No pending venues to process\n")
		return
	}

	// Filter out venues that already have at least one validation history (batch should skip those)
	filtered := make([]models.VenueWithUser, 0, len(venuesWithUser))
	for _, vw := range venuesWithUser {
		hasHist, err := app.db.HasAnyValidationHistory(vw.Venue.ID)
		if err != nil {
			log.Printf("Error checking validation history for venue %d: %v", vw.Venue.ID, err)
			continue
		}
		if !hasHist {
			filtered = append(filtered, vw)
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintf(w, "All pending venues already have validation history; nothing to process\n")
		return
	}

	log.Printf("Starting processing of %d venues (filtered from %d)", len(filtered), len(venuesWithUser))
	fmt.Fprintf(w, "Starting concurrent processing of %d venues...\n", len(filtered))

	// Start processing engine if not already running
	app.engine.Start()

	// Ensure score-only mode for this run
	app.engine.SetScoreOnly(true)

	// Add venues to processing queue
	if err := app.engine.ProcessVenuesWithUsers(filtered); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue venues for processing: %v", err), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Successfully queued %d venues for processing\n", len(filtered))
}

// validateSingleHandler starts AI-assisted review for a single venue synchronously
func (app *App) validateSingleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		http.Error(w, "missing venue id", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid venue id", http.StatusBadRequest)
		return
	}

	venueWithUser, err := app.db.GetVenueWithUserByID(id)
	if err != nil || venueWithUser == nil {
		http.Error(w, fmt.Sprintf("Venue not found: %v", err), http.StatusNotFound)
		return
	}

	// Start processing engine if not already running
	app.engine.Start()
	// Ensure score-only mode for this run
	app.engine.SetScoreOnly(true)

	// Create a context with 2-minute timeout for processing
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Process the venue synchronously (not using job queue)
	result, err := app.engine.ProcessSingleVenueSync(ctx, *venueWithUser)

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		log.Printf("Error processing venue %d: %v", id, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "error",
			"message":   fmt.Sprintf("Failed to process venue: %v", err),
			"venueId":   id,
			"completed": false,
		})
		return
	}

	if !result.Success {
		errorMsg := "Processing failed"
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}
		log.Printf("Processing failed for venue %d: %s", id, errorMsg)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "error",
			"message":   errorMsg,
			"venueId":   id,
			"completed": false,
		})
		return
	}

	// Success - return detailed result
	response := map[string]interface{}{
		"status":    "success",
		"message":   "AI-Assisted Review completed successfully",
		"venueId":   id,
		"completed": true,
	}

	if result.ValidationResult != nil {
		response["aiStatus"] = result.ValidationResult.Status
		response["aiScore"] = result.ValidationResult.Score
	}

	json.NewEncoder(w).Encode(response)
}

// validateBatchHandler starts AI-assisted review for selected venues
func (app *App) validateBatchHandler(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		VenueIDs []int64 `json:"venue_ids"`
		Force    bool    `json:"force"`
	}
	var body reqBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(body.VenueIDs) == 0 {
		http.Error(w, "no venue_ids provided", http.StatusBadRequest)
		return
	}

	// Fetch venues, optionally skip ones that already have history
	var queue []models.VenueWithUser
	for _, id := range body.VenueIDs {
		venueWithUser, err := app.db.GetVenueWithUserByID(id)
		if err != nil || venueWithUser == nil {
			continue
		}
		// Skip if already has any validation history to avoid duplicates, unless Force is true
		if !body.Force {
			hasHist, err := app.db.HasAnyValidationHistory(id)
			if err != nil {
				log.Printf("error checking validation history for %d: %v", id, err)
			}
			if hasHist {
				continue
			}
		}
		queue = append(queue, *venueWithUser)
	}

	if len(queue) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "skipped",
			"queued": 0,
			"reason": "nothing to queue (already has history or invalid IDs)",
		})
		return
	}

	app.engine.Start()
	app.engine.SetScoreOnly(true)
	if err := app.engine.ProcessVenuesWithUsers(queue); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue venues: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "queued",
		"queued": len(queue),
	})
}
