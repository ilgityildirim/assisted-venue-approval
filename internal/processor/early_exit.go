package processor

import (
	"fmt"

	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/trust"
)

// Constants for early exit logic
const (
	// earlyExitManualReviewScore is the score assigned to venues that bypass automated review
	earlyExitManualReviewScore = 50
)

// EarlyExitReason represents a structured reason for bypassing automated review
type EarlyExitReason struct {
	Code        string
	Description string
}

// String returns the description for logging/display
func (r EarlyExitReason) String() string {
	return r.Description
}

// Predefined early exit reasons for type safety
var (
	InsufficientPoints = func(required, actual int) EarlyExitReason {
		return EarlyExitReason{
			Code:        "insufficient_points",
			Description: fmt.Sprintf("User has insufficient ambassador points (required: %d, has: %d)", required, actual),
		}
	}

	NoAmbassadorPoints = func(required int) EarlyExitReason {
		return EarlyExitReason{
			Code:        "no_ambassador_points",
			Description: fmt.Sprintf("User is not an ambassador (required: %d points minimum)", required),
		}
	}

	NonTrustedUser = func(trust float64, authority string) EarlyExitReason {
		return EarlyExitReason{
			Code:        "non_trusted_user",
			Description: fmt.Sprintf("Non-trusted user (trust: %.2f, authority: %s) - requires manual review", trust, authority),
		}
	}

	NoTrustData = EarlyExitReason{
		Code:        "no_trust_data",
		Description: "Non-trusted user (no trust data available) - requires manual review",
	}

	NonVeganVenue = func(vegan, vegOnly int) EarlyExitReason {
		return EarlyExitReason{
			Code:        "non_vegan_venue",
			Description: fmt.Sprintf("Non-vegan/non-vegetarian venue (Vegan=%d, VegOnly=%d) - requires manual review", vegan, vegOnly),
		}
	}

	AmbassadorOnlyMode = EarlyExitReason{
		Code:        "ambassador_only_mode",
		Description: "Ambassador-only mode enabled - non-ambassador submission requires manual review",
	}

	NonGenericRestaurant = func(entryType, category int) EarlyExitReason {
		categoryName := map[int]string{
			0: "Generic Restaurant", 1: "Health Store", 2: "Veg Store", 3: "Bakery", 4: "B&B",
			5: "Delivery", 6: "Catering", 7: "Organization", 8: "Farmer's Market",
			10: "Food Truck", 11: "Market Vendor", 12: "Ice Cream", 13: "Juice Bar",
			14: "Professional", 15: "Coffee & Tea", 16: "Spa", 99: "Other",
		}[category]
		if categoryName == "" {
			categoryName = fmt.Sprintf("Category %d", category)
		}

		entryTypeName := "Restaurant"
		if entryType == 2 {
			entryTypeName = "Store"
		}

		return EarlyExitReason{
			Code:        "non_generic_restaurant",
			Description: fmt.Sprintf("Only generic restaurants can be viewed by AVA. Given Entry Type: %s (%d), Category: %s (%d) ", entryTypeName, entryType, categoryName, category),
		}
	}
)

// checkMinimumPoints verifies user meets minimum ambassador points requirement
func checkMinimumPoints(user *models.User, minUserPoints int) (skip bool, reason EarlyExitReason) {
	// If no minimum is set, skip this check
	if minUserPoints <= 0 {
		return false, EarlyExitReason{}
	}

	// User must have ambassador points set and meet minimum threshold
	if user.AmbassadorPoints == nil {
		return true, NoAmbassadorPoints(minUserPoints)
	}

	if *user.AmbassadorPoints < minUserPoints {
		return true, InsufficientPoints(minUserPoints, *user.AmbassadorPoints)
	}

	return false, EarlyExitReason{}
}

// checkTrustLevel verifies user has sufficient trust for automated review
func checkTrustLevel(trustAssessment *trust.Assessment) (skip bool, reason EarlyExitReason) {
	if trustAssessment == nil {
		return true, NoTrustData
	}

	// Users with zero trust AND regular authority should be manually reviewed
	// Trust > 0 indicates some level of trust/authority (venue admin, ambassador, etc.)
	if trustAssessment.Trust == 0.0 && trustAssessment.Authority == "regular" {
		return true, NonTrustedUser(trustAssessment.Trust, trustAssessment.Authority)
	}

	return false, EarlyExitReason{}
}

// checkVenueType verifies venue is vegan/vegetarian-only
func checkVenueType(venue *models.Venue) (skip bool, reason EarlyExitReason) {
	// Only fully vegan (Vegan=1) or vegetarian-only (VegOnly=1) venues can proceed to automated review
	if venue.Vegan != 1 && venue.VegOnly != 1 {
		return true, NonVeganVenue(venue.Vegan, venue.VegOnly)
	}

	return false, EarlyExitReason{}
}

// checkAmbassadorRequirement verifies ambassador-only mode requirements if enabled
func checkAmbassadorRequirement(user *models.User, onlyAmbassadors bool) (skip bool, reason EarlyExitReason) {
	// If ambassador-only mode is not enabled, skip this check
	if !onlyAmbassadors {
		return false, EarlyExitReason{}
	}

	// Check if user is actually an ambassador (not just has the field set to nil/0)
	isAmbassador := (user.AmbassadorLevel != nil && *user.AmbassadorLevel > 0) ||
		(user.AmbassadorPoints != nil && *user.AmbassadorPoints > 0)

	// Venue admins/owners bypass ambassador requirement
	isVenuePrivileged := user.IsVenueAdmin || user.IsVenueOwner

	if !isAmbassador && !isVenuePrivileged {
		return true, AmbassadorOnlyMode
	}

	return false, EarlyExitReason{}
}

// checkRestaurantCategory verifies venue is a generic restaurant (EntryType=1, Category=0)
func checkRestaurantCategory(venue *models.Venue) (skip bool, reason EarlyExitReason) {
	// Only generic restaurants (EntryType=1, Category=0) can proceed to automated review
	// This excludes: Stores (EntryType=2), Food Trucks, Bakeries, Ice Cream, etc. (Category!=0)
	if venue.EntryType != 1 || venue.Category != 0 {
		return true, NonGenericRestaurant(venue.EntryType, venue.Category)
	}

	return false, EarlyExitReason{}
}
