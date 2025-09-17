package events

import (
	"context"
	"encoding/json"
	"time"

	"assisted-venue-approval/internal/models"
)

// Event is the base interface for all venue-related audit events.
// Keep payloads small, use JSON-friendly fields.
// Why: Enables replay and audit without coupling to DB schema.
// TODO: consider schema versioning if payloads evolve.
type Event interface {
	Type() string
	VenueID() int64
	Timestamp() time.Time
	Admin() *string
	MarshalData() ([]byte, error)
}

// Base contains common event metadata.
type Base struct {
	Ts  time.Time `json:"ts"`
	VID int64     `json:"venue_id"`
	Adm *string   `json:"admin,omitempty"`
}

func (b Base) Timestamp() time.Time { return b.Ts }
func (b Base) VenueID() int64       { return b.VID }
func (b Base) Admin() *string       { return b.Adm }

// --- Concrete events ---

const (
	TypeValidationStarted = "venue.validation.started"
	TypeValidationDone    = "venue.validation.completed"
	TypeApproved          = "venue.approved"
	TypeRejected          = "venue.rejected"
	TypeManualReview      = "venue.manual_review"
)

// VenueValidationStarted is emitted when processing for a venue begins.
// Includes minimal context about the user initiating processing.
type VenueValidationStarted struct {
	Base
	UserID    *uint   `json:"user_id,omitempty"`
	Triggered string  `json:"triggered"` // system|admin|api
	Note      *string `json:"note,omitempty"`
}

func (e VenueValidationStarted) Type() string                 { return TypeValidationStarted }
func (e VenueValidationStarted) MarshalData() ([]byte, error) { return json.Marshal(e) }

// VenueValidationCompleted captures AI scores and Google data presence.
// Keep Google payload small; store only IDs and booleans we need for audit.
// Full Google cache remains in existing tables.
type VenueValidationCompleted struct {
	Base
	Score          int                   `json:"score"`
	Status         int                   `json:"status"`
	Notes          string                `json:"notes"`
	ScoreBreakdown map[string]int        `json:"score_breakdown,omitempty"`
	GoogleFound    bool                  `json:"google_found"`
	GooglePlaceID  string                `json:"google_place_id,omitempty"`
	Conflicts      []models.DataConflict `json:"conflicts,omitempty"`
}

func (e VenueValidationCompleted) Type() string                 { return TypeValidationDone }
func (e VenueValidationCompleted) MarshalData() ([]byte, error) { return json.Marshal(e) }

// Decision events from decision engine or admin actions.
// We use the same structs; admin will set Admin field and may add decision notes.

type VenueApproved struct {
	Base
	Reason  string            `json:"reason"`
	Score   int               `json:"score"`
	Flags   []string          `json:"flags,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

func (e VenueApproved) Type() string                 { return TypeApproved }
func (e VenueApproved) MarshalData() ([]byte, error) { return json.Marshal(e) }

type VenueRejected struct {
	Base
	Reason  string            `json:"reason"`
	Score   int               `json:"score"`
	Flags   []string          `json:"flags,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

func (e VenueRejected) Type() string                 { return TypeRejected }
func (e VenueRejected) MarshalData() ([]byte, error) { return json.Marshal(e) }

type VenueRequiresManualReview struct {
	Base
	Reason  string            `json:"reason"`
	Score   int               `json:"score"`
	Flags   []string          `json:"flags,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

func (e VenueRequiresManualReview) Type() string                 { return TypeManualReview }
func (e VenueRequiresManualReview) MarshalData() ([]byte, error) { return json.Marshal(e) }

// EventStore defines persistence and replay.
// Implementations must guarantee ordering per venue.
type EventStore interface {
	Append(ctx context.Context, e Event) error
	ListByVenue(ctx context.Context, venueID int64) ([]StoredEvent, error)
	ReplayVenue(ctx context.Context, venueID int64) (*RebuiltState, error)
}

// StoredEvent is a durable representation.
// Seq is a monotonic order within the DB (BIGINT AUTO_INCREMENT/BIGSERIAL).
type StoredEvent struct {
	Seq     int64     `json:"seq"`
	VenueID int64     `json:"venue_id"`
	Type    string    `json:"type"`
	Ts      time.Time `json:"ts"`
	Admin   *string   `json:"admin,omitempty"`
	Payload []byte    `json:"payload"` // original JSON
}

// RebuiltState is the result of replay for a venue.
// This is intentionally small: current status and last decision info.
// UIs can still show full history by listing events.

type RebuiltState struct {
	VenueID      int64      `json:"venue_id"`
	Status       int        `json:"status"`
	LastUpdated  time.Time  `json:"last_updated"`
	LastApproved *time.Time `json:"last_approved,omitempty"`
	LastRejected *time.Time `json:"last_rejected,omitempty"`
	ManualReview bool       `json:"manual_review"`
	LastReason   string     `json:"last_reason"`
	LastScore    int        `json:"last_score"`
}

// Replay applies events in order and rebuilds state.
func Replay(events []StoredEvent) *RebuiltState {
	st := &RebuiltState{}
	for _, se := range events {
		st.VenueID = se.VenueID
		st.LastUpdated = se.Ts
		switch se.Type {
		case TypeValidationDone:
			var ev VenueValidationCompleted
			_ = json.Unmarshal(se.Payload, &ev)
			st.Status = ev.Status
			st.LastReason = ev.Notes
			st.LastScore = ev.Score
		case TypeApproved:
			var ev VenueApproved
			_ = json.Unmarshal(se.Payload, &ev)
			st.Status = 1
			st.LastReason = ev.Reason
			st.LastScore = ev.Score
			ap := se.Ts
			st.LastApproved = &ap
			st.ManualReview = false
		case TypeRejected:
			var ev VenueRejected
			_ = json.Unmarshal(se.Payload, &ev)
			st.Status = 0
			st.LastReason = ev.Reason
			st.LastScore = ev.Score
			rj := se.Ts
			st.LastRejected = &rj
			st.ManualReview = false
		case TypeManualReview:
			var ev VenueRequiresManualReview
			_ = json.Unmarshal(se.Payload, &ev)
			st.ManualReview = true
			st.LastReason = ev.Reason
			st.LastScore = ev.Score
		}
	}
	return st
}
