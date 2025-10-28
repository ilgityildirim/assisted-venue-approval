package processor

import (
	"context"
	"fmt"
	"math"
	"strings"

	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/trust"
	"assisted-venue-approval/pkg/utils"
)

// Constants for early exit logic
const (
	// earlyExitManualReviewScore is the score assigned to venues that bypass automated review
	// Set to 0 because no AI scoring was performed during early exit
	earlyExitManualReviewScore = 0
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

	DuplicateVenue = func(duplicateID int64, duplicateName string, distanceMeters int, similarity float64) EarlyExitReason {
		return EarlyExitReason{
			Code:        "duplicate_venue",
			Description: fmt.Sprintf("Possible duplicate venue found: '%s' (ID: %d) within %dm (%.0f%% name match) - requires manual review", duplicateName, duplicateID, distanceMeters, similarity*100),
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

// checkDuplicateVenue searches for potential duplicate venues by name and location
func checkDuplicateVenue(ctx context.Context, repo domain.Repository, venue *models.Venue) (skip bool, reason EarlyExitReason) {
	// Skip check if venue doesn't have location data
	if venue.Lat == nil || venue.Lng == nil || *venue.Lat == 0.0 || *venue.Lng == 0.0 {
		return false, EarlyExitReason{}
	}

	// Skip check if venue name is too short (less than 3 characters)
	if len(strings.TrimSpace(venue.Name)) < 3 {
		return false, EarlyExitReason{}
	}

	// Search for duplicates within 500m radius (matching Google validation logic)
	const radiusMeters = 500
	const similarityThreshold = 0.7 // 70% name similarity

	duplicates, err := repo.FindDuplicateVenuesByNameAndLocation(ctx, venue.Name, *venue.Lat, *venue.Lng, radiusMeters, venue.ID)
	if err != nil {
		// Log error but don't block processing - database errors shouldn't prevent validation
		// In production, this would be logged with a proper logger
		return false, EarlyExitReason{}
	}

	// Check each potential duplicate for name similarity
	for _, dup := range duplicates {
		// Calculate distance using Haversine formula
		if dup.Lat == nil || dup.Lng == nil {
			continue
		}

		distance := calculateHaversineDistance(*venue.Lat, *venue.Lng, *dup.Lat, *dup.Lng)

		// Calculate name similarity
		similarity := utils.CalculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(venue.Name)),
			strings.ToLower(strings.TrimSpace(dup.Name)),
		)

		// If similarity is above threshold and within radius, mark as duplicate
		if similarity >= similarityThreshold {
			return true, DuplicateVenue(dup.ID, dup.Name, int(distance), similarity)
		}
	}

	return false, EarlyExitReason{}
}

// calculateHaversineDistance computes the great-circle distance between two lat/lng points in meters
func calculateHaversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusMeters = 6371000

	// Convert to radians
	lat1Rad := lat1 * (math.Pi / 180)
	lat2Rad := lat2 * (math.Pi / 180)
	deltaLatRad := (lat2 - lat1) * (math.Pi / 180)
	deltaLngRad := (lng2 - lng1) * (math.Pi / 180)

	// Haversine formula
	a := math.Sin(deltaLatRad/2)*math.Sin(deltaLatRad/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLngRad/2)*math.Sin(deltaLngRad/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}
