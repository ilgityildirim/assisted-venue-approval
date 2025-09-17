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
	"assisted-venue-approval/internal/decision"
	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/infrastructure/repository"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/processor"
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
	_ = c.Provide(func(cfg *config.Config) *scorer.AIScorer { return scorer.NewAIScorer(cfg.OpenAIAPIKey) }, true)

	// Processing engine (singleton)
	_ = c.Provide(func(repo domain.Repository, uow domain.UnitOfWorkFactory, g *scraper.GoogleMapsScraper, s *scorer.AIScorer, cfg *config.Config) *processor.ProcessingEngine {
		pc := processor.DefaultProcessingConfig()
		if cfg.WorkerCount > 0 {
			pc.WorkerCount = cfg.WorkerCount
		}
		dc := decision.DefaultDecisionConfig()
		if cfg.ApprovalThreshold > 0 {
			dc.ApprovalThreshold = cfg.ApprovalThreshold
		}
		return processor.NewProcessingEngine(repo, uow, g, s, pc, dc)
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

	// HTTP routing
	router := mux.NewRouter()

	var metrics *monitoring.Metrics
	if cfg.MetricsEnabled {
		metrics = monitoring.NewMetrics(512)
		router.Use(monitoring.Middleware(metrics))
	}

	router.HandleFunc("/", admin.HomeHandler(repo, eng)).Methods("GET")
	router.HandleFunc("/analytics", admin.AnalyticsHandler(db, eng)).Methods("GET")

	router.HandleFunc("/validate", app.validateHandler).Methods("POST")
	router.HandleFunc("/validate/batch", app.validateBatchHandler).Methods("POST")
	router.HandleFunc("/api/stats", admin.APIStatsHandler(db, eng)).Methods("GET")

	router.HandleFunc("/venues/pending", admin.PendingVenuesHandler(db)).Methods("GET")
	router.HandleFunc("/venues/manual-review", admin.ManualReviewHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}", admin.VenueDetailHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}/approve", admin.ApproveVenueHandler(db)).Methods("POST")
	router.HandleFunc("/venues/{id}/reject", admin.RejectVenueHandler(db)).Methods("POST")
	router.HandleFunc("/venues/{id}/validate", app.validateSingleHandler).Methods("POST")

	router.HandleFunc("/batch-operation", admin.BatchOperationHandler(db)).Methods("POST")
	router.HandleFunc("/validation/history", admin.ValidationHistoryHandler(db)).Methods("GET")

	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(Static()))))

	server := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	var adminServer *http.Server
	if cfg.ProfilingEnabled || cfg.MetricsEnabled {
		mux := http.NewServeMux()
		if cfg.ProfilingEnabled {
			monitoring.RegisterPprof(mux)
		}
		if cfg.MetricsEnabled && metrics != nil {
			mux.Handle(cfg.MetricsPath, monitoring.MetricsHandler(metrics))
		}
		if cfg.MetricsEnabled {
			mux.Handle("/metrics", metricsPkg.Handler())
		}
		adminServer = &http.Server{Addr: ":" + cfg.ProfilingPort, Handler: mux}
		go func() {
			fmt.Printf("Admin server (pprof/metrics) starting on port %s\n", cfg.ProfilingPort)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Admin HTTP server error: %v", err)
			}
		}()
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

// validateSingleHandler starts AI-assisted review for a single venue
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

	// Queue just this venue for processing
	if err := app.engine.ProcessVenuesWithUsers([]models.VenueWithUser{*venueWithUser}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue venue for processing: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "queued",
		"venueId": id,
	})
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
