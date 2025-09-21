package models

import (
	"strings"

	"assisted-venue-approval/internal/constants"
)

// ShouldRequireManualReview centralizes manual review skip logic.
// Returns true with a human-readable reason if the venue should be routed to manual review.
// Why: both the processor and AI scorer need to consistently skip venues with
// admin notes and for certain regions where automated validation is unreliable.
func ShouldRequireManualReview(v Venue) (bool, string) {
	// Admin notes always require manual review
	if v.AdminNote != nil && strings.TrimSpace(*v.AdminNote) != "" {
		return true, "Admin note present - manual review required"
	}
	if v.AdminHoldEmailNote != nil && strings.TrimSpace(*v.AdminHoldEmailNote) != "" {
		// Keeping the same tone; slight distinction for clarity
		return true, "Admin hold email note present - manual review required"
	}

	// Region-based rule: certain Asian regions require manual review (no API calls)
	if v.Path != nil {
		p := strings.ToLower(strings.TrimSpace(*v.Path))
		if strings.HasPrefix(p, constants.PathAsiaChina) ||
			strings.HasPrefix(p, constants.PathAsiaJapan) ||
			strings.HasPrefix(p, constants.PathAsiaSouthKorea) {
			return true, "Asian venue - manual review required (no API calls)"
		}
	}

	return false, ""
}
