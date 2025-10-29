package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"assisted-venue-approval/internal/auth"
	"assisted-venue-approval/internal/drafts"
	"assisted-venue-approval/internal/validation"
	"assisted-venue-approval/pkg/database"

	"github.com/gorilla/mux"
)

// SaveVenueDraftHandler handles POST /venues/{id}/draft
// Saves editor modifications to in-memory store
func SaveVenueDraftHandler(store *drafts.DraftStore, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get venue ID from URL
		vars := mux.Vars(r)
		venueIDStr := vars["id"]
		venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		// Get admin ID from context
		adminID, ok := auth.GetAdminIDFromContext(ctx)
		if !ok || adminID == 0 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse draft fields from JSON body
		var draftFields map[string]drafts.DraftField
		if err := json.NewDecoder(r.Body).Decode(&draftFields); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Invalid JSON: " + err.Error(),
			})
			return
		}

		// Validate all fields
		validationErrors := validation.ValidateVenueDraft(draftFields)
		if len(validationErrors) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Validation failed",
				"errors":  validationErrors,
			})
			return
		}

		// Save to in-memory store
		if err := store.Save(venueID, adminID, draftFields); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Failed to save draft: " + err.Error(),
			})
			return
		}

		log.Printf("Draft saved for venue %d by admin %d (%d fields modified)", venueID, adminID, len(draftFields))

		// Return success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Draft saved successfully",
		})
	}
}

// GetVenueDraftHandler handles GET /venues/{id}/draft
// Returns draft data if it exists
func GetVenueDraftHandler(store *drafts.DraftStore, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get venue ID from URL
		vars := mux.Vars(r)
		venueIDStr := vars["id"]
		venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		// Get draft from store
		draft, exists := store.Get(venueID)

		// Get editor info if draft exists
		var editorName string
		if exists {
			// Simplified: just use admin ID for now
			// TODO: Look up actual admin name from database if needed
			editorName = fmt.Sprintf("Admin #%d", draft.EditorID)
		}

		// Return JSON response
		w.Header().Set("Content-Type", "application/json")
		if exists {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"has_draft":   true,
				"draft_data":  draft.Fields,
				"editor_id":   draft.EditorID,
				"editor_name": editorName,
				"updated_at":  draft.UpdatedAt,
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"has_draft": false,
			})
		}
	}
}

// ClearVenueDraftHandler handles DELETE /venues/{id}/draft
// Removes draft from store
func ClearVenueDraftHandler(store *drafts.DraftStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get venue ID from URL
		vars := mux.Vars(r)
		venueIDStr := vars["id"]
		venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		// Check authorization
		adminID, ok := auth.GetAdminIDFromContext(ctx)
		if !ok || adminID == 0 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Delete draft
		store.Delete(venueID)

		log.Printf("Draft cleared for venue %d by admin %d", venueID, adminID)

		// Return success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Draft cleared successfully",
		})
	}
}
