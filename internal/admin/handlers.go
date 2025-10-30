package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"assisted-venue-approval/internal/approval"
	"assisted-venue-approval/internal/auth"
	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/drafts"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/processor"
	"assisted-venue-approval/internal/trust"
	"assisted-venue-approval/pkg/config"
	"assisted-venue-approval/pkg/database"
	"assisted-venue-approval/pkg/events"
	"assisted-venue-approval/pkg/metrics"

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

// Event sink for admin actions. Set from main.
var eventSink events.EventStore

// metrics
var (
	mAdminApproved = metrics.Default.Counter("admin_approved_total", "Admin manual approvals")
	mAdminRejected = metrics.Default.Counter("admin_rejected_total", "Admin manual rejections")
	gManualPending = metrics.Default.Gauge("manual_review_pending_gauge", "Current number of venues pending manual review")
	gApprovalRate  = metrics.Default.Gauge("approval_rate_percent", "Overall approval rate percentage")
	gThroughputMin = metrics.Default.Gauge("processing_throughput_per_min", "Processing throughput per minute (approx)")
)

func SetEventStore(es events.EventStore) { eventSink = es }

func HomeHandler(repo domain.Repository, engine *processor.ProcessingEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get processing statistics
		stats := engine.GetStats()

		// Get pending venues with user data
		venuesWithUser, err := repo.GetPendingVenuesWithUserCtx(r.Context())
		if err != nil {
			log.Printf("Error fetching pending venues: %v", err)
			venuesWithUser = []models.VenueWithUser{}
		}

		// Get recent validation results
		recentResults, err := repo.GetRecentValidationResultsCtx(r.Context(), 50)
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

		// Count pending venues that already have AVA review results (validation history)
		_, _, assistedTotal, err := repo.GetManualReviewVenuesCtx(r.Context(), "", 0, false, "created_at", 1, 0)
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
		venues, total, err := db.GetVenuesFilteredCtx(r.Context(), "pending", search, limit, offset)
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

		// Check if "high scores only" filter is enabled
		highScoresOnly := r.URL.Query().Get("high_scores_only") == "true"
		minScore := 0
		cfg := config.Load()
		if highScoresOnly {
			minScore = cfg.ApprovalThreshold
		}

		// Check if "trusted users only" filter is enabled
		trustedOnly := r.URL.Query().Get("trusted_only") == "true"

		// Get sort parameter (default: last_updated)
		sort := r.URL.Query().Get("sort")
		if sort == "" {
			sort = "last_updated"
		}

		venues, scores, total, err := db.GetManualReviewVenuesCtx(r.Context(), search, minScore, trustedOnly, sort, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching manual review venues: %v", err), http.StatusInternalServerError)
			return
		}
		// update gauge
		gManualPending.SetFloat64(float64(total))

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
			Items             []Item
			Total             int
			Page              int
			TotalPages        int
			Search            string
			HighScoresOnly    bool
			TrustedOnly       bool
			ApprovalThreshold int
			Sort              string
		}{
			Items:             items,
			Total:             total,
			Page:              page,
			TotalPages:        (total + limit - 1) / limit,
			Search:            search,
			HighScoresOnly:    highScoresOnly,
			TrustedOnly:       trustedOnly,
			ApprovalThreshold: cfg.ApprovalThreshold,
			Sort:              sort,
		}

		if err := ExecuteTemplate(w, "manual_review.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func ApproveVenueHandler(repo domain.Repository, cfg *config.Config, draftStore *drafts.DraftStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, _ := strconv.ParseInt(vars["id"], 10, 64)

		// Get admin ID from context (set by middleware)
		adminID, ok := auth.GetAdminIDFromContext(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Admin ID not found in context",
			})
			return
		}

		var draft *drafts.VenueDraft
		if draftStore != nil {
			if d, exists := draftStore.Get(id); exists {
				draft = d
				log.Printf("Loaded draft for venue %d with %d modified fields", id, len(draft.Fields))
			}
		}

		reviewer := fmt.Sprintf("admin_%d", adminID)
		rawNotes := strings.TrimSpace(r.FormValue("notes"))
		notes := rawNotes
		if notes == "" {
			notes = "Manually approved by " + reviewer
		} else {
			notes = fmt.Sprintf("Manually approved by %s: %s", reviewer, rawNotes)
		}

		// Validate that venue has a valid validation history before approval
		// Venue can only be approved if there's already a validation history with status='approved' and score >= threshold
		approvalThreshold := cfg.ApprovalThreshold
		if err := repo.ValidateApprovalEligibility(id, approvalThreshold); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Cannot approve venue: %v", err),
			})
			return
		}

		// Get validation history
		history, err := repo.GetVenueValidationHistoryCtx(r.Context(), id)
		if err != nil || len(history) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Cannot approve venue: no validation history found",
			})
			return
		}

		// Find the most recent history entry
		latestHistory := history[0]
		for _, h := range history {
			if h.ProcessedAt.After(latestHistory.ProcessedAt) {
				latestHistory = h
			}
		}

		// Additional check: ensure latest status is "approved"
		if latestHistory.ValidationStatus != "approved" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Cannot approve venue: latest validation status is '%s' (not 'approved')", latestHistory.ValidationStatus),
			})
			return
		}

		venueWithUser, err := repo.GetVenueWithUserByIDCtx(r.Context(), id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Error fetching venue: %v", err),
			})
			return
		}

		venue := venueWithUser.Venue
		tc := trust.NewDefault()
		assessment := tc.Assess(venueWithUser.User, venue.Location)
		mergeResult, err := approval.Assemble(approval.MergeInput{
			Venue:         venue,
			User:          venueWithUser.User,
			TrustScore:    assessment.Trust,
			LatestHistory: &latestHistory,
			Draft:         draft,
		})
		if err != nil {
			log.Printf("failed to assemble approval data for venue %d: %v", id, err)
			http.Error(w, "Failed to prepare approval payload", http.StatusInternalServerError)
			return
		}

		approvalData := approval.BuildApprovalData(mergeResult, &venue, adminID, notes)
		if approvalData == nil {
			log.Printf("approval data assembly returned nil for venue %d", id)
			http.Error(w, "Failed to prepare approval payload", http.StatusInternalServerError)
			return
		}

		// Approve venue
		if err := repo.ApproveVenueWithDataReplacement(r.Context(), approvalData); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Error approving venue: %v", err),
			})
			return
		}

		if draftStore != nil && draft != nil {
			draftStore.Delete(id)
			log.Printf("[approval] ✓ Deleted draft for venue %d after approval", id)
		}

		// metrics
		mAdminApproved.Inc(1)

		// Create audit log with data replacements
		histID := latestHistory.ID
		var auditLog *domain.VenueValidationAuditLog
		if approvalData.Replacements != nil && approvalData.Replacements.HasReplacements() {
			replacementsJSON, err := approvalData.Replacements.ToJSON()
			if err != nil {
				log.Printf("Failed to serialize data replacements: %v", err)
				auditLog = domain.NewAuditLog(id, &histID, &adminID, "approved", &notes)
			} else {
				auditLog = domain.NewAuditLogWithReplacements(id, &histID, &adminID, "approved", &notes, &replacementsJSON)
			}
		} else {
			auditLog = domain.NewAuditLog(id, &histID, &adminID, "approved", &notes)
		}
		if err := repo.CreateAuditLogCtx(r.Context(), auditLog); err != nil {
			log.Printf("Failed to create audit log for venue approval: %v", err)
		}

		// Publish event
		if eventSink != nil {
			_ = eventSink.Append(r.Context(), events.VenueApproved{
				Base:   events.Base{Ts: time.Now(), VID: id, Adm: &reviewer},
				Reason: notes,
				Score:  latestHistory.ValidationScore,
			})
		}

		// Always return JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
	}
}

func RejectVenueHandler(repo domain.Repository, draftStore *drafts.DraftStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, _ := strconv.ParseInt(vars["id"], 10, 64)

		// Get admin ID from context (set by middleware)
		adminID, ok := auth.GetAdminIDFromContext(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Admin ID not found in context",
			})
			return
		}

		// Get reviewer info from session/auth
		reviewer := fmt.Sprintf("admin_%d", adminID)
		reason := strings.TrimSpace(r.FormValue("reason"))

		// Rejection reason is required
		if reason == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Rejection reason is required",
			})
			return
		}

		reason = fmt.Sprintf("Manually rejected by %s: %s", reviewer, reason)

		// Update venue status
		err := repo.UpdateVenueStatusCtx(r.Context(), id, -1, reason, &reviewer)
		// metrics
		mAdminRejected.Inc(1)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Error updating venue: %v", err),
			})
			return
		}

		// Delete draft after successful rejection
		if draftStore != nil {
			draftStore.Delete(id)
			log.Printf("[rejection] ✓ Deleted draft for venue %d after rejection", id)
		}

		// Get the validation history ID to create audit log
		history, err := repo.GetVenueValidationHistoryCtx(r.Context(), id)
		if err == nil && len(history) > 0 {
			// Find the most recent history entry
			latestHistory := history[0]
			for _, h := range history {
				if h.ProcessedAt.After(latestHistory.ProcessedAt) {
					latestHistory = h
				}
			}

			// Create audit log entry
			histID := latestHistory.ID
			auditLog := domain.NewAuditLog(id, &histID, &adminID, "rejected", &reason)
			if err := repo.CreateAuditLogCtx(r.Context(), auditLog); err != nil {
				log.Printf("Failed to create audit log for venue rejection: %v", err)
			}
		}

		// Publish event
		if eventSink != nil {
			score := 0
			if len(history) > 0 {
				score = history[0].ValidationScore
			}
			_ = eventSink.Append(r.Context(), events.VenueRejected{
				Base:   events.Base{Ts: time.Now(), VID: id, Adm: &reviewer},
				Reason: reason,
				Score:  score,
			})
		}

		// Always return JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
	}
}

// VenueDetailHandler shows detailed venue information with validation data
func VenueDetailHandler(db *database.DB, draftStore *drafts.DraftStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, err := strconv.ParseInt(vars["id"], 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		// Get admin ID from context
		adminID, _ := auth.GetAdminIDFromContext(r.Context())

		// Get venue with user data
		venue, err := db.GetVenueWithUserByIDCtx(r.Context(), id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Venue not found: %v", err), http.StatusNotFound)
			return
		}

		// Get validation history
		history, err := db.GetVenueValidationHistoryCtx(r.Context(), id)
		if err != nil {
			log.Printf("Error fetching validation history: %v", err)
			history = []models.ValidationHistory{}
		}

		// Get audit logs for this venue
		auditLogs, err := db.GetAuditLogsByVenueIDCtx(r.Context(), id)
		if err != nil {
			log.Printf("Error fetching audit logs: %v", err)
			auditLogs = []domain.VenueValidationAuditLog{}
		}

		// Get similar venues for comparison (will be removed from UI, still fetched safely)
		similarVenues, err := db.GetSimilarVenuesCtx(r.Context(), venue.Venue, 5)
		if err != nil {
			log.Printf("Error fetching similar venues: %v", err)
			similarVenues = []models.Venue{}
		}

		// Get cached Google Places data if available
		googleData, err := db.GetCachedGooglePlaceDataCtx(r.Context(), id)
		if err != nil {
			log.Printf("Error fetching cached Google data: %v", err)
		}

		// Build Combined Information centrally
		tc := trust.NewDefault()
		assessment := tc.Assess(venue.User, venue.Venue.Location)

		var latestHistory *models.ValidationHistory
		if len(history) > 0 {
			idx := 0
			for i := range history {
				if history[i].ProcessedAt.After(history[idx].ProcessedAt) {
					idx = i
				}
			}
			latestHistory = &history[idx]
		}

		var draft *drafts.VenueDraft
		if draftStore != nil {
			if d, exists := draftStore.Get(id); exists {
				draft = d
			}
		}

		mergeResult, err := approval.Assemble(approval.MergeInput{
			Venue:         venue.Venue,
			User:          venue.User,
			TrustScore:    assessment.Trust,
			GoogleData:    googleData,
			LatestHistory: latestHistory,
			Draft:         draft,
		})

		var combined models.CombinedInfo
		var suggestions *models.AISuggestions
		if err != nil {
			log.Printf("combined info warning: failed to assemble venue detail for %d: %v", id, err)
			combined, _ = models.GetCombinedVenueInfo(venue.Venue, venue.User, assessment.Trust)
			suggestions = &models.AISuggestions{}
		} else {
			combined = mergeResult.Combined
			if mergeResult.AISuggestions != nil {
				suggestions = mergeResult.AISuggestions
			} else {
				suggestions = &models.AISuggestions{}
			}
		}

		categoryOptions := models.StoreCategoryOptions()
		categoryMap := make(map[int]string, len(categoryOptions))
		for _, opt := range categoryOptions {
			categoryMap[opt.ID] = opt.Label
		}
		categoryJSONBytes, err := json.Marshal(categoryMap)
		if err != nil {
			log.Printf("failed to marshal category options: %v", err)
			categoryJSONBytes = []byte("{}")
		}
		categoryOptionsJSON := template.JS(string(categoryJSONBytes))

		approvalHoursNote := ""
		if mergeResult != nil && mergeResult.ApprovalFields != nil {
			approvalHoursNote = strings.TrimSpace(mergeResult.ApprovalFields.HoursNote)
		}

		draftData, hasDraft, draftEditorID, draftEditorName, draftUpdatedAt := extractDraftMeta(draft)

		// Get venue path with count of venues using that path
		var venuePath, venuePathRaw string
		if venue.Venue.Path != nil && *venue.Venue.Path != "" {
			venuePathRaw = *venue.Venue.Path
			count, err := db.CountVenuesByPathCtx(r.Context(), venuePathRaw, id)
			if err != nil {
				log.Printf("Error counting venues by path: %v", err)
				venuePath = venuePathRaw // fallback to raw path
			} else {
				venuePath = fmt.Sprintf("%s(%d)", venuePathRaw, count+1) // +1 to include current venue
			}
		}

		data := struct {
			Venue              models.VenueWithUser
			History            []models.ValidationHistory
			AuditLogs          []domain.VenueValidationAuditLog
			SimilarVenues      []models.Venue
			GoogleData         *models.GooglePlaceData
			Combined           models.CombinedInfo
			TrustPercent       int
			TrustAuthority     string
			TrustReason        string
			LatestHist         *models.ValidationHistory
			PrettyBreakdown    string
			AIReviewNote       string
			AIScore            int
			AIScoreFormatted   string
			AIOutputNotes      string
			AIOutputRestPretty string
			AIOutputFullPretty string
			// NEW: Classification data for templates
			VenueTypeLabel      string
			VeganStatusLabel    string
			CategoryLabel       string
			CategoryOptions     []models.CategoryOption
			CategoryOptionsJSON template.JS
			TypeMismatchAlert   bool
			// Quality suggestions fields
			DescriptionSuggestion string
			NameSuggestion        string
			ClosedDaysSuggestion  string
			ApprovalHoursNote     string
			// Venue path fields
			VenuePath    string
			VenuePathRaw string
			// Path validation fields
			PathValidationValid      bool
			PathValidationIssue      string
			PathValidationConfidence string
			// Draft fields
			HasDraft        bool
			DraftData       map[string]interface{}
			DraftEditorID   int
			DraftEditorName string
			DraftUpdatedAt  string
			CurrentAdminID  int
		}{
			Venue:          *venue,
			History:        history,
			AuditLogs:      auditLogs,
			SimilarVenues:  similarVenues,
			GoogleData:     googleData,
			Combined:       combined,
			TrustPercent:   int(assessment.Trust * 100),
			TrustAuthority: assessment.Authority,
			TrustReason:    assessment.Reason,
			// NEW: Add classification data from combined info
			VenueTypeLabel:        combined.VenueType,
			VeganStatusLabel:      combined.VeganStatus,
			CategoryLabel:         combined.Category,
			CategoryOptions:       categoryOptions,
			CategoryOptionsJSON:   categoryOptionsJSON,
			TypeMismatchAlert:     combined.TypeMismatch,
			DescriptionSuggestion: suggestions.DescriptionSuggestion,
			NameSuggestion:        suggestions.NameSuggestion,
			ClosedDaysSuggestion:  suggestions.ClosedDays,
			ApprovalHoursNote:     approvalHoursNote,
			// Add venue path data
			VenuePath:    venuePath,
			VenuePathRaw: venuePathRaw,
			// Draft data
			HasDraft:        hasDraft,
			DraftData:       draftData,
			DraftEditorID:   draftEditorID,
			DraftEditorName: draftEditorName,
			DraftUpdatedAt:  draftUpdatedAt,
			CurrentAdminID:  adminID,
		}

		// Prepare latest history and AI review fields
		if latestHistory != nil {
			data.LatestHist = latestHistory
			data.AIReviewNote = latestHistory.ValidationNotes
			data.AIScore = latestHistory.ValidationScore
			data.AIScoreFormatted = fmt.Sprintf("%.2f", float64(latestHistory.ValidationScore))
			if latestHistory.ScoreBreakdown != nil {
				if b, err := json.MarshalIndent(latestHistory.ScoreBreakdown, "", "  "); err == nil {
					data.PrettyBreakdown = string(b)
				}
			}
			if latestHistory.AIOutputData != nil && *latestHistory.AIOutputData != "" {
				var raw map[string]interface{}
				if err := json.Unmarshal([]byte(*latestHistory.AIOutputData), &raw); err == nil {
					if scoringMap, ok := raw["scoring"].(map[string]interface{}); ok {
						if n, ok := scoringMap["notes"].(string); ok {
							data.AIOutputNotes = n
						}
					}
					if qualityMap, ok := raw["quality"].(map[string]interface{}); ok {
						if desc, ok := qualityMap["description"].(string); ok && data.DescriptionSuggestion == "" {
							data.DescriptionSuggestion = desc
						}
						if name, ok := qualityMap["name"].(string); ok && data.NameSuggestion == "" {
							data.NameSuggestion = name
						}
						if closedDays, ok := qualityMap["closed_days"].(string); ok && data.ClosedDaysSuggestion == "" {
							data.ClosedDaysSuggestion = closedDays
						}
						if pathValidation, ok := qualityMap["pathValidation"].(map[string]interface{}); ok {
							if isValid, ok := pathValidation["isValid"].(bool); ok {
								data.PathValidationValid = isValid
							}
							if issue, ok := pathValidation["issue"].(string); ok {
								data.PathValidationIssue = issue
							}
							if confidence, ok := pathValidation["confidence"].(string); ok {
								data.PathValidationConfidence = confidence
							}
						}
					}
					if rb, err := json.MarshalIndent(raw, "", "  "); err == nil {
						data.AIOutputFullPretty = string(rb)
					}
				}
			}
		}

		if err := ExecuteTemplate(w, "venue_detail.tmpl", data); err != nil {
			http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func extractDraftMeta(draft *drafts.VenueDraft) (map[string]interface{}, bool, int, string, string) {
	if draft == nil {
		return nil, false, 0, "", ""
	}

	fields := make(map[string]interface{}, len(draft.Fields))
	for key, value := range draft.Fields {
		fields[key] = map[string]interface{}{
			"value":           value.Value,
			"original_source": value.OriginalSource,
		}
	}

	editorName := fmt.Sprintf("Admin #%d", draft.EditorID)
	updatedAt := draft.UpdatedAt.Format(time.RFC3339)
	return fields, true, draft.EditorID, editorName, updatedAt
}

// BatchResult represents the result of a single venue operation in a batch
type BatchResult struct {
	VenueID   int64  `json:"venue_id"`
	VenueName string `json:"venue_name"`
	Status    string `json:"status"` // "Approved", "Rejected", or "Failed"
	Reason    string `json:"reason,omitempty"`
	Success   bool   `json:"success"`
}

// BatchOperationHandler handles bulk approval/rejection operations
func BatchOperationHandler(repo domain.Repository, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get admin ID from context (set by middleware)
		adminID, ok := auth.GetAdminIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Admin ID not found in context", http.StatusForbidden)
			return
		}

		action := r.FormValue("action")      // approve, reject, manual_review
		venueIDs := r.FormValue("venue_ids") // comma-separated IDs
		reason := r.FormValue("reason")
		reviewer := fmt.Sprintf("admin_%d", adminID)

		if action == "" || venueIDs == "" {
			http.Error(w, "Missing required parameters", http.StatusBadRequest)
			return
		}

		// Validate rejection reason is provided
		if action == "reject" && strings.TrimSpace(reason) == "" {
			http.Error(w, "Rejection reason is required", http.StatusBadRequest)
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

		// Perform batch operation with detailed result tracking
		var batchResults []BatchResult
		successCount := 0

		for _, id := range ids {
			result := BatchResult{
				VenueID: id,
				Success: false,
			}

			// Get venue name for result tracking
			venueWithUser, err := repo.GetVenueWithUserByIDCtx(r.Context(), id)
			if err != nil {
				result.VenueName = fmt.Sprintf("Unknown (ID: %d)", id)
				result.Status = "Failed"
				result.Reason = fmt.Sprintf("Failed to fetch venue: %v", err)
				batchResults = append(batchResults, result)
				log.Printf("Batch operation: failed to fetch venue %d: %v", id, err)
				continue
			}
			result.VenueName = venueWithUser.Venue.Name

			switch action {
			case "approve":
				// Apply the same validation as single venue approval
				if err := processBatchApproval(r.Context(), repo, cfg, id, adminID, reviewer, venueWithUser); err != nil {
					result.Status = "Failed"
					result.Reason = err.Error()
					batchResults = append(batchResults, result)
					log.Printf("Batch approval failed for venue %d: %v", id, err)
					continue
				}
				result.Status = "Approved"
				result.Success = true
				successCount++
				mAdminApproved.Inc(1)

			case "reject":
				// Apply the same validation as single venue rejection
				if err := processBatchRejection(r.Context(), repo, id, adminID, reviewer, reason); err != nil {
					result.Status = "Failed"
					result.Reason = err.Error()
					batchResults = append(batchResults, result)
					log.Printf("Batch rejection failed for venue %d: %v", id, err)
					continue
				}
				result.Status = "Rejected"
				result.Reason = reason
				result.Success = true
				successCount++
				mAdminRejected.Inc(1)

			default:
				// manual_review or other actions (basic status update)
				notes := fmt.Sprintf("Batch %s by %s: %s", action, reviewer, reason)
				if err := repo.UpdateVenueStatusCtx(r.Context(), id, 0, notes, &reviewer); err != nil {
					result.Status = "Failed"
					result.Reason = fmt.Sprintf("Failed to update status: %v", err)
					batchResults = append(batchResults, result)
					log.Printf("Batch operation failed for venue %d: %v", id, err)
					continue
				}
				result.Status = "Updated"
				result.Success = true
				successCount++
			}

			batchResults = append(batchResults, result)
		}

		response := map[string]interface{}{
			"results":       batchResults,
			"success_count": successCount,
			"total_count":   len(ids),
			"action":        action,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// processBatchApproval handles approval for a single venue in a batch operation
// Applies the same validation rules as single venue approval
func processBatchApproval(ctx context.Context, repo domain.Repository, cfg *config.Config, venueID int64, adminID int, reviewer string, venueWithUser *models.VenueWithUser) error {
	approvalThreshold := cfg.ApprovalThreshold

	// Validate approval eligibility (same as single approval)
	if err := repo.ValidateApprovalEligibility(venueID, approvalThreshold); err != nil {
		return fmt.Errorf("cannot approve venue: %v", err)
	}

	// Get validation history
	history, err := repo.GetVenueValidationHistoryCtx(ctx, venueID)
	if err != nil || len(history) == 0 {
		return fmt.Errorf("cannot approve venue: no validation history found")
	}

	// Find the most recent history entry
	latestHistory := history[0]
	for _, h := range history {
		if h.ProcessedAt.After(latestHistory.ProcessedAt) {
			latestHistory = h
		}
	}

	// Ensure latest status is "approved"
	if latestHistory.ValidationStatus != "approved" {
		return fmt.Errorf("cannot approve venue: latest validation status is '%s' (not 'approved')", latestHistory.ValidationStatus)
	}

	venue := venueWithUser.Venue

	// Build Combined Information using trust-based merging
	if latestHistory.GooglePlaceData != nil {
		venue.GoogleData = latestHistory.GooglePlaceData
	}
	tc := trust.NewDefault()
	assessment := tc.Assess(venueWithUser.User, *venue.Path)
	combined, cerr := models.GetCombinedVenueInfo(venue, venueWithUser.User, assessment.Trust)
	if cerr != nil {
		log.Printf("Warning: failed to build combined info for venue %d: %v", venueID, cerr)
	}

	// Extract AI suggestions from validation history
	var nameSuggestion, descSuggestion, closedDaysSuggestion string
	if latestHistory.AIOutputData != nil && *latestHistory.AIOutputData != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(*latestHistory.AIOutputData), &raw); err == nil {
			if qualityMap, ok := raw["quality"].(map[string]interface{}); ok {
				if name, ok := qualityMap["name"].(string); ok {
					nameSuggestion = strings.TrimSpace(name)
				}
				if desc, ok := qualityMap["description"].(string); ok {
					descSuggestion = strings.TrimSpace(desc)
				}
				if closed, ok := qualityMap["closed_days"].(string); ok {
					closedDaysSuggestion = strings.TrimSpace(closed)
				}
			}
		}
	}

	// Build approval data with notes
	notes := fmt.Sprintf("Batch approval by %s", reviewer)
	approvalData := domain.NewApprovalData(venueID, adminID, notes)

	// Name: Use NameSuggestion if available, otherwise Combined.Name
	finalName := combined.Name
	if nameSuggestion != "" {
		finalName = nameSuggestion
	}
	if finalName != "" && finalName != venue.Name {
		approvalData.Name = &finalName
	}

	// Address: Use Combined.Address
	if combined.Address != "" && combined.Address != venue.Location {
		approvalData.Address = &combined.Address
	}

	// Description: Use DescriptionSuggestion if available, otherwise Combined.Description
	finalDesc := combined.Description
	if descSuggestion != "" {
		finalDesc = descSuggestion
	}
	if finalDesc != "" {
		approvalData.Description = &finalDesc
	}

	// Lat/Lng: Use Combined coordinates
	if combined.Lat != nil && combined.Lng != nil {
		approvalData.Lat = combined.Lat
		approvalData.Lng = combined.Lng
	}

	// Phone: Use Combined.Phone
	if combined.Phone != "" {
		approvalData.Phone = &combined.Phone
	}

	// Website: Use Combined.Website
	if combined.Website != "" {
		approvalData.Website = &combined.Website
	}

	// OpenHours: Format from Combined.Hours
	log.Printf("[batch-approval] Venue %d: Processing opening hours (combined.Hours has %d lines, source=%s)",
		venueID, len(combined.Hours), combined.Sources["hours"])

	if len(combined.Hours) > 0 {
		log.Printf("[batch-approval] Venue %d: Formatting hours from Combined.Hours: %v", venueID, combined.Hours)
		formattedHours, err := approval.FormatOpenHoursFromCombined(combined.Hours)
		if err != nil {
			log.Printf("[batch-approval] ❌ Venue %d: Failed to format open hours: %v", venueID, err)
		} else if formattedHours != "" {
			approvalData.OpenHours = &formattedHours
			log.Printf("[batch-approval] ✓ Venue %d: Formatted hours set: %s", venueID, formattedHours)
		} else {
			log.Printf("[batch-approval] ⚠️  Venue %d: Hours formatting returned empty string (no valid hours parsed)", venueID)
		}
	} else {
		log.Printf("[batch-approval] ⚠️  Venue %d: No hours data available in Combined.Hours (Google data may be missing)", venueID)
	}

	// OpenHoursNote: Use ClosedDaysSuggestion if available
	if closedDaysSuggestion != "" {
		approvalData.OpenHoursNote = &closedDaysSuggestion
		log.Printf("[batch-approval] ✓ Venue %d: Closed days note set: %s", venueID, closedDaysSuggestion)
	} else {
		log.Printf("[batch-approval] Venue %d: No closed days suggestion from AI", venueID)
	}

	// Build data replacements for audit trail
	approvalData.Replacements = domain.BuildVenueDataReplacements(&venue, approvalData)

	// Approve venue with data replacement
	if err := repo.ApproveVenueWithDataReplacement(ctx, approvalData); err != nil {
		return fmt.Errorf("error approving venue: %v", err)
	}

	// Create audit log with data replacements
	histID := latestHistory.ID
	var auditLog *domain.VenueValidationAuditLog
	if approvalData.Replacements != nil && approvalData.Replacements.HasReplacements() {
		replacementsJSON, err := approvalData.Replacements.ToJSON()
		if err != nil {
			log.Printf("Failed to serialize data replacements: %v", err)
			auditLog = domain.NewAuditLog(venueID, &histID, &adminID, "approved", &notes)
		} else {
			auditLog = domain.NewAuditLogWithReplacements(venueID, &histID, &adminID, "approved", &notes, &replacementsJSON)
		}
	} else {
		auditLog = domain.NewAuditLog(venueID, &histID, &adminID, "approved", &notes)
	}
	if err := repo.CreateAuditLogCtx(ctx, auditLog); err != nil {
		log.Printf("Failed to create audit log for batch approval venue %d: %v", venueID, err)
	}

	// Publish event
	if eventSink != nil {
		_ = eventSink.Append(ctx, events.VenueApproved{
			Base:   events.Base{Ts: time.Now(), VID: venueID, Adm: &reviewer},
			Reason: notes,
			Score:  latestHistory.ValidationScore,
		})
	}

	return nil
}

// processBatchRejection handles rejection for a single venue in a batch operation
// Applies the same validation rules as single venue rejection
func processBatchRejection(ctx context.Context, repo domain.Repository, venueID int64, adminID int, reviewer string, reason string) error {
	// Format rejection reason
	fullReason := fmt.Sprintf("Batch rejection by %s: %s", reviewer, reason)

	// Update venue status
	if err := repo.UpdateVenueStatusCtx(ctx, venueID, -1, fullReason, &reviewer); err != nil {
		return fmt.Errorf("error updating venue: %v", err)
	}

	// Get the validation history ID to create audit log
	history, err := repo.GetVenueValidationHistoryCtx(ctx, venueID)
	if err == nil && len(history) > 0 {
		// Find the most recent history entry
		latestHistory := history[0]
		for _, h := range history {
			if h.ProcessedAt.After(latestHistory.ProcessedAt) {
				latestHistory = h
			}
		}

		// Create audit log entry
		histID := latestHistory.ID
		auditLog := domain.NewAuditLog(venueID, &histID, &adminID, "rejected", &fullReason)
		if err := repo.CreateAuditLogCtx(ctx, auditLog); err != nil {
			log.Printf("Failed to create audit log for batch rejection venue %d: %v", venueID, err)
		}

		// Publish event
		if eventSink != nil {
			_ = eventSink.Append(ctx, events.VenueRejected{
				Base:   events.Base{Ts: time.Now(), VID: venueID, Adm: &reviewer},
				Reason: fullReason,
				Score:  latestHistory.ValidationScore,
			})
		}
	}

	return nil
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
		history, total, err := db.GetValidationHistoryPaginatedCtx(r.Context(), limit, offset)
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
		venueStats, err := db.GetVenueStatisticsCtx(r.Context())
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

		// Update business metrics gauges
		if venueStats != nil && venueStats.Total > 0 {
			apr := float64(venueStats.Approved) / float64(venueStats.Total) * 100.0
			gApprovalRate.SetFloat64(apr)
		}
		mins := time.Since(stats.StartTime).Minutes()
		if mins > 0 {
			gThroughputMin.SetFloat64(float64(stats.CompletedJobs) / mins)
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
