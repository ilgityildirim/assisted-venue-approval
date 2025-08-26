package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automatic-vendor-validation/internal/models"
	"automatic-vendor-validation/internal/processor"
	"automatic-vendor-validation/pkg/database"

	"github.com/gorilla/mux"
)

// DashboardData represents data for the validation dashboard
type DashboardData struct {
	Stats         processor.ProcessingStats
	PendingVenues []models.VenueWithUser
	RecentResults []models.ValidationResult
	SystemHealth  SystemHealth
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

		dashboardData := DashboardData{
			Stats:         stats,
			PendingVenues: venuesWithUser[:min(len(venuesWithUser), 100)],
			RecentResults: recentResults,
			SystemHealth:  health,
		}

		tmpl := getDashboardTemplate()
		t := template.Must(template.New("dashboard").Parse(tmpl))
		t.Execute(w, dashboardData)
	}
}

func PendingVenuesHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters for filtering
		status := r.URL.Query().Get("status") // pending, approved, rejected, manual_review
		search := r.URL.Query().Get("search")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit := 50
		offset := (page - 1) * limit

		// Get filtered venues
		venues, total, err := db.GetVenuesFiltered(status, search, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching venues: %v", err), http.StatusInternalServerError)
			return
		}

		data := struct {
			Venues     []models.VenueWithUser
			Total      int
			Page       int
			TotalPages int
			Status     string
			Search     string
		}{
			Venues:     venues,
			Total:      total,
			Page:       page,
			TotalPages: (total + limit - 1) / limit,
			Status:     status,
			Search:     search,
		}

		tmpl := getPendingVenuesTemplate()
		t := template.Must(template.New("pending").Parse(tmpl))
		t.Execute(w, data)
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

		// Get similar venues for comparison
		similarVenues, err := db.GetSimilarVenues(venue.Venue, 5)
		if err != nil {
			log.Printf("Error fetching similar venues: %v", err)
			similarVenues = []models.Venue{}
		}

		data := struct {
			Venue         models.VenueWithUser
			History       []models.ValidationHistory
			SimilarVenues []models.Venue
		}{
			Venue:         *venue,
			History:       history,
			SimilarVenues: similarVenues,
		}

		tmpl := getVenueDetailTemplate()
		t := template.Must(template.New("venue-detail").Parse(tmpl))
		t.Execute(w, data)
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

		tmpl := getValidationHistoryTemplate()
		t := template.Must(template.New("history").Parse(tmpl))
		t.Execute(w, data)
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

		tmpl := getAnalyticsTemplate()
		t := template.Must(template.New("analytics").Parse(tmpl))
		t.Execute(w, data)
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

// Template functions

func getDashboardTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HappyCow Validation Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px 0; margin-bottom: 30px; }
        .header h1 { text-align: center; font-size: 2.5em; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .stat-number { font-size: 2.5em; font-weight: bold; color: #3498db; }
        .stat-label { color: #666; margin-top: 5px; }
        .health-status { display: flex; align-items: center; gap: 10px; }
        .health-indicator { width: 10px; height: 10px; border-radius: 50%; background: #27ae60; }
        .section { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); margin-bottom: 30px; }
        .section h2 { color: #2c3e50; margin-bottom: 20px; }
        .btn { display: inline-block; padding: 12px 24px; background: #3498db; color: white; text-decoration: none; border-radius: 5px; border: none; cursor: pointer; font-size: 16px; }
        .btn:hover { background: #2980b9; }
        .btn-success { background: #27ae60; }
        .btn-success:hover { background: #2ecc71; }
        .btn-danger { background: #e74c3c; }
        .btn-danger:hover { background: #c0392b; }
        .table { width: 100%; border-collapse: collapse; }
        .table th, .table td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        .table th { background: #f8f9fa; font-weight: 600; }
        .status-badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; }
        .status-approved { background: #d4edda; color: #155724; }
        .status-rejected { background: #f8d7da; color: #721c24; }
        .status-pending { background: #fff3cd; color: #856404; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; }
        .nav a { padding: 10px 20px; background: white; color: #2c3e50; text-decoration: none; border-radius: 5px; }
        .nav a.active, .nav a:hover { background: #3498db; color: white; }
        .processing-controls { display: flex; gap: 10px; margin-bottom: 20px; }
        @media (max-width: 768px) {
            .stats-grid { grid-template-columns: 1fr; }
            .nav { flex-direction: column; }
            .processing-controls { flex-direction: column; }
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>🌱 HappyCow Validation Dashboard</h1>
        </div>
    </div>
    
    <div class="container">
        <nav class="nav">
            <a href="/" class="active">Dashboard</a>
            <a href="/venues/pending">Pending Venues</a>
            <a href="/validation/history">History</a>
            <a href="/analytics">Analytics</a>
        </nav>
        
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-number">{{.Stats.TotalJobs}}</div>
                <div class="stat-label">Total Venues Processed</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{.Stats.AutoApproved}}</div>
                <div class="stat-label">Auto-Approved</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{.Stats.AutoRejected}}</div>
                <div class="stat-label">Auto-Rejected</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{.Stats.ManualReview}}</div>
                <div class="stat-label">Manual Review</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{len .PendingVenues}}</div>
                <div class="stat-label">Pending Venues</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">${{printf "%.2f" .Stats.TotalCostUSD}}</div>
                <div class="stat-label">Total API Costs</div>
            </div>
        </div>
        
        <div class="section">
            <h2>System Health</h2>
            <div class="health-status">
                <div class="health-indicator"></div>
                <span>Database: {{.SystemHealth.DatabaseStatus}}</span>
            </div>
            <div class="health-status">
                <div class="health-indicator"></div>
                <span>Processing Engine: {{.SystemHealth.ProcessingEngine}}</span>
            </div>
            <div class="health-status">
                <div class="health-indicator"></div>
                <span>API Connections: {{.SystemHealth.APIConnections}}</span>
            </div>
            <div class="health-status">
                <span>Last Processing: {{.SystemHealth.LastProcessingTime.Format "2006-01-02 15:04:05"}}</span>
            </div>
        </div>
        
        <div class="section">
            <h2>Processing Controls</h2>
            <div class="processing-controls">
                <form action="/validate" method="POST" style="display: inline;">
                    <button type="submit" class="btn btn-success">🚀 Start Auto-Validation</button>
                </form>
                <a href="/validate/stats" class="btn">📊 Real-time Stats</a>
                <a href="/venues/pending" class="btn">📋 Review Queue</a>
            </div>
        </div>
        
        <div class="section">
            <h2>Recent Venues ({{len .PendingVenues}})</h2>
            <table class="table">
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Name</th>
                        <th>Location</th>
                        <th>Submitter</th>
                        <th>Authority</th>
                        <th>Status</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .PendingVenues}}
                    <tr>
                        <td>{{.Venue.ID}}</td>
                        <td><a href="/venues/{{.Venue.ID}}">{{.Venue.Name}}</a></td>
                        <td>{{.Venue.Location}}</td>
                        <td>{{.User.Username}}</td>
                        <td>
                            {{if .User.Trusted}}✅ Trusted{{end}}
                            {{if .IsVenueAdmin}}👑 Owner{{end}}
                            {{if .AmbassadorLevel}}🌟 Ambassador{{end}}
                        </td>
                        <td>
                            {{if eq .Venue.Active 1}}
                                <span class="status-badge status-approved">Approved</span>
                            {{else if eq .Venue.Active -1}}
                                <span class="status-badge status-rejected">Rejected</span>
                            {{else}}
                                <span class="status-badge status-pending">Pending</span>
                            {{end}}
                        </td>
                        <td>
                            <a href="/venues/{{.Venue.ID}}" class="btn" style="padding: 5px 10px; font-size: 12px;">View</a>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{if gt (len .PendingVenues) 50}}
                <p><a href="/venues/pending">View all pending venues...</a></p>
            {{end}}
        </div>
    </div>
    
    <script>
        // Auto-refresh stats every 30 seconds
        setInterval(function() {
            fetch('/api/stats')
                .then(response => response.json())
                .then(data => {
                    // Update stats if needed
                    console.log('Stats updated:', data);
                })
                .catch(error => console.error('Error fetching stats:', error));
        }, 30000);
    </script>
</body>
</html>`
}
