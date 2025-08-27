package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automatic-vendor-validation/internal/models"
	"automatic-vendor-validation/internal/processor"
	"automatic-vendor-validation/internal/scorer"
	"automatic-vendor-validation/pkg/config"
	"automatic-vendor-validation/pkg/database"

	"github.com/gorilla/mux"
)

// DashboardData represents data for the validation dashboard
type DashboardData struct {
	Stats            processor.ProcessingStats
	PendingVenues    []models.VenueWithUser
	PendingTotal     int
	AssistedReady    int
	PendingWithoutAI int
	RecentResults    []models.ValidationResult
	SystemHealth     SystemHealth
}

type SystemHealth struct {
	DatabaseStatus     string
	ProcessingEngine   string
	APIConnections     string
	LastProcessingTime time.Time
}

func HomeHandler(db *database.DB, engine *processor.ProcessingEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get processing statistics
		stats := engine.GetStats()

		// Get pending venues with user data
		venuesWithUser, err := db.GetPendingVenuesWithUser()
		if err != nil {
			log.Printf("Error fetching pending venues: %v", err)
			venuesWithUser = []models.VenueWithUser{}
		}

		// Get recent validation results
		recentResults, err := db.GetRecentValidationResults(50)
		if err != nil {
			log.Printf("Error fetching recent results: %v", err)
			recentResults = []models.ValidationResult{}
		}

		// System health check
		health := SystemHealth{
			DatabaseStatus:     "Connected",
			ProcessingEngine:   "Running",
			APIConnections:     "Healthy",
			LastProcessingTime: stats.LastActivity,
		}

		pendingTotal := len(venuesWithUser)

		// Count pending venues that already have AI-assisted review results (validation history)
		_, _, assistedTotal, err := db.GetManualReviewVenues("", 1, 0)
		if err != nil {
			log.Printf("Error fetching manual review count: %v", err)
			assistedTotal = 0
		}
		pendingWithoutAI := pendingTotal - assistedTotal
		if pendingWithoutAI < 0 {
			pendingWithoutAI = 0
		}

		dashboardData := DashboardData{
			Stats:            stats,
			PendingVenues:    venuesWithUser[:min(len(venuesWithUser), 100)],
			PendingTotal:     pendingTotal,
			AssistedReady:    assistedTotal,
			PendingWithoutAI: pendingWithoutAI,
			RecentResults:    recentResults,
			SystemHealth:     health,
		}

		if err := ExecuteTemplate(w, "dashboard.tmpl", dashboardData); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func PendingVenuesHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters (only search and pagination; status is always pending)
		search := r.URL.Query().Get("search")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit := 50
		offset := (page - 1) * limit

		// Always fetch pending venues only
		venues, total, err := db.GetVenuesFiltered("pending", search, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching venues: %v", err), http.StatusInternalServerError)
			return
		}

		data := struct {
			Venues     []models.VenueWithUser
			Total      int
			Page       int
			TotalPages int
			Search     string
		}{
			Venues:     venues,
			Total:      total,
			Page:       page,
			TotalPages: (total + limit - 1) / limit,
			Search:     search,
		}

		if err := ExecuteTemplate(w, "pending.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// ManualReviewHandler lists venues pending manual review (those with validation history and still active=0)
func ManualReviewHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("search")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit := 50
		offset := (page - 1) * limit

		venues, scores, total, err := db.GetManualReviewVenues(search, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching manual review venues: %v", err), http.StatusInternalServerError)
			return
		}

		// Build a view model combining scores with venues for the template
		type Item struct {
			VenueWithUser models.VenueWithUser
			Score         int
		}
		items := make([]Item, 0, len(venues))
		for i := range venues {
			items = append(items, Item{VenueWithUser: venues[i], Score: scores[i]})
		}

		data := struct {
			Items      []Item
			Total      int
			Page       int
			TotalPages int
			Search     string
		}{
			Items:      items,
			Total:      total,
			Page:       page,
			TotalPages: (total + limit - 1) / limit,
			Search:     search,
		}

		if err := ExecuteTemplate(w, "manual_review.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func ApproveVenueHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, _ := strconv.ParseInt(vars["id"], 10, 64)

		// Get reviewer info from session/auth (simplified)
		reviewer := "admin" // This should come from authentication
		notes := r.FormValue("notes")
		if notes == "" {
			notes = "Manually approved by " + reviewer
		} else {
			notes = fmt.Sprintf("Manually approved by %s: %s", reviewer, notes)
		}

		// Create validation result for audit trail
		validationResult := &models.ValidationResult{
			VenueID:        id,
			Score:          95, // Manual approval gets high score
			Status:         "approved",
			Notes:          notes,
			ScoreBreakdown: map[string]int{"manual_approval": 95},
		}

		// Update venue status
		err := db.UpdateVenueStatus(id, 1, notes, &reviewer)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error updating venue: %v", err), http.StatusInternalServerError)
			return
		}

		// Save validation result
		if err := db.SaveValidationResult(validationResult); err != nil {
			log.Printf("Failed to save validation result for manual approval: %v", err)
		}

		// Return JSON for AJAX requests
		if r.Header.Get("Content-Type") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
			return
		}

		http.Redirect(w, r, "/venues/pending", http.StatusFound)
	}
}

func RejectVenueHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, _ := strconv.ParseInt(vars["id"], 10, 64)

		// Get reviewer info from session/auth (simplified)
		reviewer := "admin" // This should come from authentication
		reason := r.FormValue("reason")
		if reason == "" {
			reason = "Manually rejected by " + reviewer
		} else {
			reason = fmt.Sprintf("Manually rejected by %s: %s", reviewer, reason)
		}

		// Create validation result for audit trail
		validationResult := &models.ValidationResult{
			VenueID:        id,
			Score:          5, // Manual rejection gets low score
			Status:         "rejected",
			Notes:          reason,
			ScoreBreakdown: map[string]int{"manual_rejection": 5},
		}

		// Update venue status
		err := db.UpdateVenueStatus(id, -1, reason, &reviewer)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error updating venue: %v", err), http.StatusInternalServerError)
			return
		}

		// Save validation result
		if err := db.SaveValidationResult(validationResult); err != nil {
			log.Printf("Failed to save validation result for manual rejection: %v", err)
		}

		// Return JSON for AJAX requests
		if r.Header.Get("Content-Type") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
			return
		}

		http.Redirect(w, r, "/venues/pending", http.StatusFound)
	}
}

// VenueDetailHandler shows detailed venue information with validation data
func VenueDetailHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, err := strconv.ParseInt(vars["id"], 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		// Get venue with user data
		venue, err := db.GetVenueWithUserByID(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Venue not found: %v", err), http.StatusNotFound)
			return
		}

		// Get validation history
		history, err := db.GetVenueValidationHistory(id)
		if err != nil {
			log.Printf("Error fetching validation history: %v", err)
			history = []models.ValidationHistory{}
		}

		// Get similar venues for comparison (will be removed from UI, still fetched safely)
		similarVenues, err := db.GetSimilarVenues(venue.Venue, 5)
		if err != nil {
			log.Printf("Error fetching similar venues: %v", err)
			similarVenues = []models.Venue{}
		}

		// Get cached Google Places data if available
		googleData, err := db.GetCachedGooglePlaceData(id)
		if err != nil {
			log.Printf("Error fetching cached Google data: %v", err)
		}

		// Build Combined Information per rules
		// Treat Trusted==true as trust >= 0.8
		isTrusted := venue.User.Trusted
		// Recency: within last 3 months
		lastUpdate := time.Now().AddDate(0, -3, 0)
		var userUpdatedAt *time.Time
		if venue.Venue.DateUpdated != nil {
			userUpdatedAt = venue.Venue.DateUpdated
		} else if venue.Venue.CreatedAt != nil {
			userUpdatedAt = venue.Venue.CreatedAt
		}
		useUserData := false
		if isTrusted && userUpdatedAt != nil && userUpdatedAt.After(lastUpdate) {
			useUserData = true
		}

		pick := func(userVal *string, googleVal string) (val string, source string) {
			if useUserData && userVal != nil && strings.TrimSpace(*userVal) != "" {
				return *userVal, "user"
			}
			if strings.TrimSpace(googleVal) != "" {
				return googleVal, "google"
			}
			if userVal != nil {
				return *userVal, "user"
			}
			return "", ""
		}

		combinedAddress, addrSource := pick(&venue.Venue.Location, func() string {
			if googleData != nil {
				return googleData.FormattedAddress
			}
			return ""
		}())
		combinedPhone, phoneSource := pick(venue.Venue.Phone, func() string {
			if googleData != nil {
				return googleData.FormattedPhone
			}
			return ""
		}())
		combinedWebsite, siteSource := pick(venue.Venue.URL, func() string {
			if googleData != nil {
				return googleData.Website
			}
			return ""
		}())

		// Opening hours representation
		var combinedHours []string
		hoursSource := ""
		if useUserData && venue.Venue.OpenHours != nil && strings.TrimSpace(*venue.Venue.OpenHours) != "" {
			combinedHours = []string{*venue.Venue.OpenHours}
			hoursSource = "user"
		} else if googleData != nil && googleData.OpeningHours != nil && len(googleData.OpeningHours.WeekdayText) > 0 {
			combinedHours = googleData.OpeningHours.WeekdayText
			hoursSource = "google"
		} else if venue.Venue.OpenHours != nil {
			combinedHours = []string{*venue.Venue.OpenHours}
			hoursSource = "user"
		}

		// Prepare a combined venue for AI scoring
		combinedVenue := venue.Venue
		combinedVenue.Location = combinedAddress
		if combinedPhone != "" {
			p := combinedPhone
			combinedVenue.Phone = &p
		} else {
			combinedVenue.Phone = nil
		}
		if combinedWebsite != "" {
			u := combinedWebsite
			combinedVenue.URL = &u
		} else {
			combinedVenue.URL = nil
		}
		if len(combinedHours) > 0 {
			h := strings.Join(combinedHours, " | ")
			combinedVenue.OpenHours = &h
		} else {
			combinedVenue.OpenHours = nil
		}

		// Score the combined information via AI
		combinedScore := 0
		combinedStatus := ""
		if apiCfg := config.Load(); apiCfg != nil && apiCfg.OpenAIAPIKey != "" {
			ai := scorer.NewAIScorer(apiCfg.OpenAIAPIKey)
			if res, err := ai.ScoreVenue(r.Context(), combinedVenue); err == nil && res != nil {
				combinedScore = res.Score
				combinedStatus = res.Status
			} else if err != nil {
				log.Printf("AI scoring (combined info) failed for venue %d: %v", id, err)
			}
		}

		type CombinedInfo struct {
			Name        string
			Address     string
			Phone       string
			Website     string
			Hours       []string
			Lat         *float64
			Lng         *float64
			Types       []string
			Description string
			VeganLevel  string
			Sources     map[string]string
			Score       int
			ScoreStatus string
		}

		// Additional combined fields per new requirements
		// Name
		combinedName := venue.Venue.Name
		nameSource := "user"
		if !useUserData && googleData != nil && strings.TrimSpace(googleData.Name) != "" {
			combinedName = googleData.Name
			nameSource = "google"
		}
		// Lat/Lng
		var combinedLat, combinedLng *float64
		latlngSource := ""
		if useUserData && venue.Venue.Lat != nil && venue.Venue.Lng != nil {
			combinedLat = venue.Venue.Lat
			combinedLng = venue.Venue.Lng
			latlngSource = "user"
		} else if googleData != nil {
			l := googleData.Geometry.Location.Lat
			g := googleData.Geometry.Location.Lng
			combinedLat = &l
			combinedLng = &g
			latlngSource = "google"
		} else if venue.Venue.Lat != nil && venue.Venue.Lng != nil {
			combinedLat = venue.Venue.Lat
			combinedLng = venue.Venue.Lng
			latlngSource = "user"
		}
		// Types (Google only)
		var combinedTypes []string
		typesSource := ""
		if googleData != nil && len(googleData.Types) > 0 {
			combinedTypes = googleData.Types
			typesSource = "google"
		}
		// Description (user submitted AdditionalInfo)
		descSource := ""
		combinedDesc := ""
		if venue.Venue.AdditionalInfo != nil && strings.TrimSpace(*venue.Venue.AdditionalInfo) != "" {
			combinedDesc = *venue.Venue.AdditionalInfo
			descSource = "user"
		}
		// Vegan Level (from venue fields Vegan/VegOnly)
		veganLevel := fmt.Sprintf("%d/%d", venue.Venue.Vegan, venue.Venue.VegOnly)
		veganSource := "user"

		combined := CombinedInfo{
			Name:        combinedName,
			Address:     combinedAddress,
			Phone:       combinedPhone,
			Website:     combinedWebsite,
			Hours:       combinedHours,
			Lat:         combinedLat,
			Lng:         combinedLng,
			Types:       combinedTypes,
			Description: combinedDesc,
			VeganLevel:  veganLevel,
			Sources: map[string]string{
				"name":        nameSource,
				"address":     addrSource,
				"phone":       phoneSource,
				"website":     siteSource,
				"hours":       hoursSource,
				"latlng":      latlngSource,
				"types":       typesSource,
				"description": descSource,
				"vegan_level": veganSource,
			},
			Score:       combinedScore,
			ScoreStatus: combinedStatus,
		}

		data := struct {
			Venue         models.VenueWithUser
			History       []models.ValidationHistory
			SimilarVenues []models.Venue
			GoogleData    *models.GooglePlaceData
			Combined      CombinedInfo
			TrustPercent  int
		}{
			Venue:         *venue,
			History:       history,
			SimilarVenues: similarVenues,
			GoogleData:    googleData,
			Combined:      combined,
			TrustPercent: func() int {
				if venue.User.Trusted {
					return 80
				}
				return 30
			}(),
		}

		if err := ExecuteTemplate(w, "venue_detail.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// BatchOperationHandler handles bulk approval/rejection operations
func BatchOperationHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		action := r.FormValue("action")      // approve, reject, manual_review
		venueIDs := r.FormValue("venue_ids") // comma-separated IDs
		reason := r.FormValue("reason")
		reviewer := "admin" // This should come from authentication

		if action == "" || venueIDs == "" {
			http.Error(w, "Missing required parameters", http.StatusBadRequest)
			return
		}

		// Parse venue IDs
		idStrs := strings.Split(venueIDs, ",")
		var ids []int64
		for _, idStr := range idStrs {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				continue
			}
			ids = append(ids, id)
		}

		if len(ids) == 0 {
			http.Error(w, "No valid venue IDs provided", http.StatusBadRequest)
			return
		}

		// Perform batch operation
		results := make(map[string]interface{})
		successCount := 0

		for _, id := range ids {
			var dbStatus int
			var score int
			var status string

			switch action {
			case "approve":
				dbStatus = 1
				score = 95
				status = "approved"
			case "reject":
				dbStatus = -1
				score = 5
				status = "rejected"
			default:
				dbStatus = 0
				score = 50
				status = "manual_review"
			}

			notes := fmt.Sprintf("Batch %s by %s: %s", action, reviewer, reason)

			// Update venue status
			if err := db.UpdateVenueStatus(id, dbStatus, notes, &reviewer); err != nil {
				log.Printf("Failed to update venue %d: %v", id, err)
				continue
			}

			// Create validation result
			validationResult := &models.ValidationResult{
				VenueID:        id,
				Score:          score,
				Status:         status,
				Notes:          notes,
				ScoreBreakdown: map[string]int{"batch_operation": score},
			}

			if err := db.SaveValidationResult(validationResult); err != nil {
				log.Printf("Failed to save validation result for venue %d: %v", id, err)
			}

			successCount++
		}

		results["success_count"] = successCount
		results["total_count"] = len(ids)
		results["action"] = action

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// ValidationHistoryHandler shows comprehensive validation history
func ValidationHistoryHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit := 100
		offset := (page - 1) * limit

		// Get validation history with pagination
		history, total, err := db.GetValidationHistoryPaginated(limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching history: %v", err), http.StatusInternalServerError)
			return
		}

		data := struct {
			History    []models.ValidationHistory
			Total      int
			Page       int
			TotalPages int
		}{
			History:    history,
			Total:      total,
			Page:       page,
			TotalPages: (total + limit - 1) / limit,
		}

		if err := ExecuteTemplate(w, "history.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// AnalyticsHandler provides analytics and reporting
func AnalyticsHandler(db *database.DB, engine *processor.ProcessingEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get processing statistics
		stats := engine.GetStats()

		// Get venue statistics from database
		venueStats, err := db.GetVenueStatistics()
		if err != nil {
			log.Printf("Error fetching venue statistics: %v", err)
			venueStats = &models.VenueStats{}
		}

		// Calculate efficiency metrics
		automationRate := float64(0)
		if stats.TotalJobs > 0 {
			automationRate = float64(stats.AutoApproved+stats.AutoRejected) / float64(stats.TotalJobs) * 100
		}

		data := struct {
			ProcessingStats processor.ProcessingStats
			VenueStats      *models.VenueStats
			AutomationRate  float64
			CostPerVenue    float64
		}{
			ProcessingStats: stats,
			VenueStats:      venueStats,
			AutomationRate:  automationRate,
			CostPerVenue:    stats.TotalCostUSD / float64(max(stats.TotalJobs, 1)),
		}

		if err := ExecuteTemplate(w, "analytics.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// APIStatsHandler provides real-time statistics via JSON API
func APIStatsHandler(db *database.DB, engine *processor.ProcessingEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := engine.GetStats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
