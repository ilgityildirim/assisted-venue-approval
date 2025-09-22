package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/database"

	"github.com/gorilla/mux"
)

// SubmitFeedbackHandler handles POST /venues/{id}/feedback
func SubmitFeedbackHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, err := strconv.ParseInt(vars["id"], 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid venue id", http.StatusBadRequest)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		ft := strings.TrimSpace(r.FormValue("feedback_type"))
		var ftype models.FeedbackType
		switch ft {
		case string(models.FeedbackThumbsUp):
			ftype = models.FeedbackThumbsUp
		case string(models.FeedbackThumbsDown):
			ftype = models.FeedbackThumbsDown
		default:
			http.Error(w, "invalid feedback_type", http.StatusBadRequest)
			return
		}
		var pv *string
		if p := strings.TrimSpace(r.FormValue("prompt_version")); p != "" {
			if len(p) > 32 {
				http.Error(w, "prompt_version too long", http.StatusBadRequest)
				return
			}
			pv = &p
		}
		var cmt *string
		if c := strings.TrimSpace(r.FormValue("comment")); c != "" {
			cmt = &c
		}

		ip := clientIP(r)
		ipb := models.IPToBytes(ip)

		// Prevent multiple submissions per venue/prompt_version from same IP
		if ok, err := db.HasVenueFeedbackFromIPCtx(r.Context(), id, ipb, pv); err != nil {
			log.Printf("feedback dup check failed: %v", err)
		} else if ok {
			w.WriteHeader(http.StatusConflict)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "duplicate"})
			return
		}

		rec := &models.EditorFeedback{VenueID: id, PromptVersion: pv, FeedbackType: ftype, Comment: cmt, IP: ipb}
		if err := rec.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("validation error: %v", err), http.StatusBadRequest)
			return
		}
		if err := db.CreateEditorFeedbackCtx(r.Context(), rec); err != nil {
			http.Error(w, fmt.Sprintf("failed to save feedback: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": rec.ID})
	}
}

// VenueFeedbackHandler handles GET /venues/{id}/feedback
func VenueFeedbackHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, err := strconv.ParseInt(vars["id"], 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid venue id", http.StatusBadRequest)
			return
		}
		list, up, down, err := db.GetVenueFeedbackCtx(r.Context(), id, 50)
		if err != nil {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":       list,
			"thumbs_up":   up,
			"thumbs_down": down,
		})
	}
}

// APIFeedbackStatsHandler handles GET /api/feedback/stats
func APIFeedbackStatsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var pv *string
		if p := strings.TrimSpace(q.Get("prompt_version")); p != "" {
			if len(p) > 32 {
				http.Error(w, "prompt_version too long", http.StatusBadRequest)
				return
			}
			pv = &p
		}
		st, err := db.GetFeedbackStatsCtx(r.Context(), pv)
		if err != nil {
			http.Error(w, fmt.Sprintf("stats error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	}
}

// clientIP extracts the first client IP from common headers or RemoteAddr.
func clientIP(r *http.Request) net.IP {
	// X-Forwarded-For can have multiple IPs, use the first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed
			}
		}
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		if parsed := net.ParseIP(ip); parsed != nil {
			return parsed
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if parsed := net.ParseIP(host); parsed != nil {
			return parsed
		}
	}
	return nil
}
