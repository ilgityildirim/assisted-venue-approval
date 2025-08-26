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

	"automatic-vendor-validation/internal/admin"
	"automatic-vendor-validation/internal/models"
	"automatic-vendor-validation/internal/processor"
	"automatic-vendor-validation/internal/scorer"
	"automatic-vendor-validation/internal/scraper"
	"automatic-vendor-validation/pkg/config"
	"automatic-vendor-validation/pkg/database"
)

func main() {
	cfg := config.Load()

	log.Println("Starting venue validation system")

	db, err := database.NewWithConfig(cfg.DatabaseURL, cfg)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	gmapsScraper, err := scraper.NewGoogleMapsScraper(cfg.GoogleMapsAPIKey)
	if err != nil {
		log.Fatal("Failed to initialize Google Maps scraper:", err)
	}

	aiScorer := scorer.NewAIScorer(cfg.OpenAIAPIKey)

	// Initialize processing engine with configuration
	processingConfig := processor.DefaultProcessingConfig()

	// Override defaults with environment-specific values if needed
	if cfg.WorkerCount > 0 {
		processingConfig.WorkerCount = cfg.WorkerCount
	}

	processingEngine := processor.NewProcessingEngine(db, gmapsScraper, aiScorer, processingConfig)

	// Load templates
	if err := admin.LoadTemplates(); err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	app := &App{
		db:      db,
		scraper: gmapsScraper,
		scorer:  aiScorer,
		config:  cfg,
		engine:  processingEngine,
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Received shutdown signal, initiating graceful shutdown...")

		// Stop processing engine with 30-second timeout
		if err := processingEngine.Stop(30 * time.Second); err != nil {
			log.Printf("Processing engine shutdown error: %v", err)
		}

		cancel()
	}()

	// Set up routes
	router := mux.NewRouter()

	// Dashboard and main pages
	router.HandleFunc("/", admin.HomeHandler(db, processingEngine)).Methods("GET")
	router.HandleFunc("/analytics", admin.AnalyticsHandler(db, processingEngine)).Methods("GET")

	// Processing controls
	router.HandleFunc("/validate", app.validateHandler).Methods("POST")
	// Removed /validate/stats endpoint
	router.HandleFunc("/api/stats", admin.APIStatsHandler(db, processingEngine)).Methods("GET")

	// Venue management
	router.HandleFunc("/venues/pending", admin.PendingVenuesHandler(db)).Methods("GET")
	router.HandleFunc("/venues/manual-review", admin.ManualReviewHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}", admin.VenueDetailHandler(db)).Methods("GET")
	router.HandleFunc("/venues/{id}/approve", admin.ApproveVenueHandler(db)).Methods("POST")
	router.HandleFunc("/venues/{id}/reject", admin.RejectVenueHandler(db)).Methods("POST")
	router.HandleFunc("/venues/{id}/validate", app.validateSingleHandler).Methods("POST")

	// Batch operations
	router.HandleFunc("/batch-operation", admin.BatchOperationHandler(db)).Methods("POST")

	// History and audit
	router.HandleFunc("/validation/history", admin.ValidationHistoryHandler(db)).Methods("GET")

	// Static files
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))

	// Start HTTP server with graceful shutdown
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		fmt.Printf("Server starting on port %s\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server error:", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
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
