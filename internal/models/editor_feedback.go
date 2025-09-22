package models

import (
	"errors"
	"net"
	"time"
)

// FeedbackType represents allowed feedback.
type FeedbackType string

const (
	FeedbackThumbsUp   FeedbackType = "thumbs_up"
	FeedbackThumbsDown FeedbackType = "thumbs_down"
)

// EditorFeedback maps to editor_feedback table.
type EditorFeedback struct {
	ID            int64        `json:"id"`
	VenueID       int64        `json:"venue_id"`
	PromptVersion *string      `json:"prompt_version,omitempty"`
	FeedbackType  FeedbackType `json:"feedback_type"`
	Comment       *string      `json:"comment,omitempty"`
	IP            []byte       `json:"-"` // VARBINARY(16)
	CreatedAt     time.Time    `json:"created_at"`
}

// Validate basic constraints. Keep it simple.
func (e *EditorFeedback) Validate() error {
	if e.VenueID <= 0 {
		return errors.New("invalid venue id")
	}
	switch e.FeedbackType {
	case FeedbackThumbsUp, FeedbackThumbsDown:
		// ok
	default:
		return errors.New("invalid feedback type")
	}
	if e.PromptVersion != nil {
		pv := *e.PromptVersion
		if len(pv) == 0 || len(pv) > 32 {
			return errors.New("invalid prompt_version")
		}
	}
	return nil
}

// IPToBytes ensures IPv4/IPv6 as 16-byte slice when possible.
func IPToBytes(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		// Store as 4 bytes for IPv4 (fits VARBINARY(16))
		return []byte(v4)
	}
	v6 := ip.To16()
	if v6 == nil {
		return nil
	}
	b := make([]byte, 16)
	copy(b, v6)
	return b
}

// FeedbackStats is a compact aggregate response.
type FeedbackStats struct {
	Total      int                               `json:"total"`
	ThumbsUp   int                               `json:"thumbs_up"`
	ThumbsDown int                               `json:"thumbs_down"`
	ByVersion  map[string]struct{ Up, Down int } `json:"by_version,omitempty"`
	Daily      []DailyCount                      `json:"daily,omitempty"`
}

type DailyCount struct {
	Date       string `json:"date"`
	ThumbsUp   int    `json:"thumbs_up"`
	ThumbsDown int    `json:"thumbs_down"`
}
