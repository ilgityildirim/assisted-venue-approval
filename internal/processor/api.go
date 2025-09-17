package processor

import (
	"time"

	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/events"
)

// Engine exposes the minimal contract used by web and admin layers.
// Keep it small to decouple from implementation details.
// Why: improves testability; callers can mock this interface.
// NOTE: Extend carefully — prefer adding helper functions over expanding the surface.

type Engine interface {
	Start()
	Stop(timeout time.Duration) error
	ProcessVenuesWithUsers(venuesWithUser []models.VenueWithUser) error
	SetScoreOnly(scoreOnly bool)
	SetEventStore(es events.EventStore)
}

// Ensure ProcessingEngine implements Engine.
var _ Engine = (*ProcessingEngine)(nil)
