package events

import (
	"context"
	"encoding/json"
	"time"
)

// EventType constants keep event names consistent
const (
	TypeValidationStarted   = "venue.validation.started"
	TypeValidationCompleted = "venue.validation.completed"
	TypeVenueApproved       = "venue.approved"
	TypeVenueRejected       = "venue.rejected"
	TypeVenueManualReview   = "venue.manual_review"
)

// Event is the domain event interface used for persistence
// Keep it small and serializable.
type Event interface {
	Type() string
	GetVenueID() int64
	GetTime() time.Time
	GetAdmin() *string
	GetAdminID() *int
	Payload() map[string]any
}

// BaseEvent provides common fields and default implementations.
type BaseEvent struct {
	VenueID int64
	At      time.Time
	Admin   *string
	AdminID *int
	Data    map[string]any
}

func (b BaseEvent) GetVenueID() int64       { return b.VenueID }
func (b BaseEvent) GetTime() time.Time      { return b.At }
func (b BaseEvent) GetAdmin() *string       { return b.Admin }
func (b BaseEvent) GetAdminID() *int        { return b.AdminID }
func (b BaseEvent) Payload() map[string]any { return b.Data }

// Concrete events

type VenueValidationStarted struct{ BaseEvent }

func (e VenueValidationStarted) Type() string { return TypeValidationStarted }

type VenueValidationCompleted struct{ BaseEvent }

func (e VenueValidationCompleted) Type() string { return TypeValidationCompleted }

type VenueApproved struct{ BaseEvent }

func (e VenueApproved) Type() string { return TypeVenueApproved }

type VenueRejected struct{ BaseEvent }

func (e VenueRejected) Type() string { return TypeVenueRejected }

type VenueRequiresManualReview struct{ BaseEvent }

func (e VenueRequiresManualReview) Type() string { return TypeVenueManualReview }

// StoredEvent is a persisted event record
// Data holds the JSON payload as-is for replay and external consumption.
type StoredEvent struct {
	ID      int64           `json:"id"`
	VenueID int64           `json:"venue_id"`
	Type    string          `json:"type"`
	At      time.Time       `json:"at"`
	Admin   *string         `json:"admin,omitempty"`
	AdminID *int            `json:"admin_id,omitempty"`
	Data    json.RawMessage `json:"data"`
}

// VenueState is a projection rebuilt from events (kept minimal for audit)
type VenueState struct {
	VenueID          int64     `json:"venue_id"`
	Status           string    `json:"status"`
	LastScore        int       `json:"last_score"`
	DecisionReason   string    `json:"decision_reason"`
	GooglePlaceFound bool      `json:"google_place_found"`
	GooglePlaceID    string    `json:"google_place_id"`
	LastUpdated      time.Time `json:"last_updated"`
	LastAdminAction  *string   `json:"last_admin_action,omitempty"`
}

// EventStore persists and replays events
// Append should guarantee ordering via transactional insert order or monotonic ID.
type EventStore interface {
	Append(ctx context.Context, ev ...Event) error
	ListByVenue(ctx context.Context, venueID int64) ([]StoredEvent, error)
	Replay(ctx context.Context, venueID int64) (*VenueState, error)
}

// New helper constructors
func NewVenueValidationStarted(venueID int64) Event {
	return VenueValidationStarted{BaseEvent{
		VenueID: venueID,
		At:      time.Now(),
		Data:    map[string]any{"message": "validation started"},
	}}
}

func NewVenueValidationCompleted(venueID int64, score int, notes string, googleFound bool, googlePlaceID string, scoreBreakdown map[string]int, extra map[string]any) Event {
	data := map[string]any{
		"score":           score,
		"notes":           notes,
		"google_found":    googleFound,
		"google_place_id": googlePlaceID,
		"score_breakdown": scoreBreakdown,
	}
	for k, v := range extra {
		data[k] = v
	}
	return VenueValidationCompleted{BaseEvent{VenueID: venueID, At: time.Now(), Data: data}}
}

func NewVenueApproved(venueID int64, reason string, score int, admin *string, adminID *int, extra map[string]any) Event {
	data := map[string]any{"reason": reason, "score": score, "source": "auto"}
	for k, v := range extra {
		data[k] = v
	}
	return VenueApproved{BaseEvent{VenueID: venueID, At: time.Now(), Admin: admin, AdminID: adminID, Data: data}}
}

func NewVenueRejected(venueID int64, reason string, score int, admin *string, adminID *int, extra map[string]any) Event {
	data := map[string]any{"reason": reason, "score": score, "source": "auto"}
	for k, v := range extra {
		data[k] = v
	}
	return VenueRejected{BaseEvent{VenueID: venueID, At: time.Now(), Admin: admin, AdminID: adminID, Data: data}}
}

func NewVenueRequiresManualReview(venueID int64, reviewReason string, score int, admin *string, adminID *int, extra map[string]any) Event {
	data := map[string]any{"review_reason": reviewReason, "score": score, "source": "auto"}
	for k, v := range extra {
		data[k] = v
	}
	return VenueRequiresManualReview{BaseEvent{VenueID: venueID, At: time.Now(), Admin: admin, AdminID: adminID, Data: data}}
}

// RebuildState derives the latest state from a sequence of stored events
func RebuildState(events []StoredEvent) *VenueState {
	st := &VenueState{}
	for _, se := range events {
		st.VenueID = se.VenueID
		st.LastUpdated = se.At
		switch se.Type {
		case TypeValidationStarted:
			if st.Status == "" {
				st.Status = "processing"
			}
		case TypeValidationCompleted:
			var m map[string]any
			_ = json.Unmarshal(se.Data, &m)
			if v, ok := m["score"].(float64); ok {
				st.LastScore = int(v)
			}
			if v, ok := m["notes"].(string); ok {
				st.DecisionReason = v
			}
			if v, ok := m["google_found"].(bool); ok {
				st.GooglePlaceFound = v
			}
			if v, ok := m["google_place_id"].(string); ok {
				st.GooglePlaceID = v
			}
			if st.Status == "processing" {
				st.Status = "validated"
			}
		case TypeVenueApproved:
			st.Status = "approved"
			var m map[string]any
			_ = json.Unmarshal(se.Data, &m)
			if v, ok := m["reason"].(string); ok {
				st.DecisionReason = v
			}
			if v, ok := m["score"].(float64); ok {
				st.LastScore = int(v)
			}
			st.LastAdminAction = se.Admin
		case TypeVenueRejected:
			st.Status = "rejected"
			var m map[string]any
			_ = json.Unmarshal(se.Data, &m)
			if v, ok := m["reason"].(string); ok {
				st.DecisionReason = v
			}
			if v, ok := m["score"].(float64); ok {
				st.LastScore = int(v)
			}
			st.LastAdminAction = se.Admin
		case TypeVenueManualReview:
			st.Status = "manual_review"
			var m map[string]any
			_ = json.Unmarshal(se.Data, &m)
			if v, ok := m["review_reason"].(string); ok {
				st.DecisionReason = v
			}
			if v, ok := m["score"].(float64); ok {
				st.LastScore = int(v)
			}
			st.LastAdminAction = se.Admin
		}
	}
	return st
}
