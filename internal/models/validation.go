package models

import (
	"fmt"
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

// ShouldRequireManualReviewForLocation checks if venue location mismatch requires manual review
// based on user trust level and operational status.
// Returns true with reason if manual review is required.
func ShouldRequireManualReviewForLocation(v Venue, u User, trustLevel float64) (bool, string) {
	// No validation details means no distance check needed
	if v.ValidationDetails == nil {
		return false, ""
	}

	// High trust users, venue owners, and venue admins can have location discrepancies
	if trustLevel >= 0.8 || u.IsVenueAdmin || uint(v.UserID) == u.ID {
		return false, ""
	}

	// Check if Google business is operational
	if v.GoogleData != nil && v.GoogleData.BusinessStatus != "" {
		status := strings.ToLower(v.GoogleData.BusinessStatus)
		if status == "closed_permanently" || status == "closed_temporarily" {
			return true, "Business is closed - manual review required"
		}
	}

	// For regular users, check distance if Google business is operational
	if v.ValidationDetails.DistanceMeters > 500 && !u.Trusted {
		return true, fmt.Sprintf("Location mismatch detected: %.0fm from Google location", v.ValidationDetails.DistanceMeters)
	}

	return false, ""
}
