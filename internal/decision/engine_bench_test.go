package decision

import (
	"context"
	"testing"

	"assisted-venue-approval/internal/models"
)

func sptr(s string) *string { return &s }

func BenchmarkMakeDecision(b *testing.B) {
	cfg := DefaultDecisionConfig()
	de := NewDecisionEngine(cfg)

	venue := models.Venue{
		ID:       123,
		Name:     "Green Leaf",
		Location: "123 Vegan St, Plant City",
		Phone:    sptr("+1234567890"),
		URL:      sptr("https://example.com/green-leaf"),
		VDetails: "Casual vegan-friendly spot",
	}
	user := models.User{ID: 42, Trusted: true, Contributions: 250}
	vr := &models.ValidationResult{
		VenueID: venue.ID,
		Score:   85,
		Status:  "approved",
		Notes:   "benchmark case",
		ScoreBreakdown: map[string]int{
			"total": 85,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_ = de.MakeDecision(ctx, venue, user, vr)
	}
}
