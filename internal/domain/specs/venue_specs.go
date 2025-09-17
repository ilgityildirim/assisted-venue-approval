package specs

import (
	"context"
	"strings"

	"assisted-venue-approval/internal/models"
)

// HasCompleteContactInfo checks that venue has at least N contact channels.
// Channels considered: Email, Phone, URL, InstagramUrl, FBUrl.
func HasCompleteContactInfo(minCount int) Specification[models.Venue] {
	if minCount < 1 {
		minCount = 1
	}
	return New(func(ctx context.Context, v models.Venue) bool {
		if ctx.Err() != nil {
			return false
		}
		c := 0
		if v.Email != nil && *v.Email != "" {
			c++
		}
		if v.Phone != nil && *v.Phone != "" {
			c++
		}
		if v.URL != nil && *v.URL != "" {
			c++
		}
		if v.InstagramUrl != nil && *v.InstagramUrl != "" {
			c++
		}
		if v.FBUrl != nil && *v.FBUrl != "" {
			c++
		}
		return c >= minCount
	})
}

// IsVeganRelevant checks explicit fields or keyword hints in text fields.
func IsVeganRelevant() Specification[models.Venue] {
	keywords := []string{"vegan", "vegetarian", "plant-based", "veggie"}
	return New(func(ctx context.Context, v models.Venue) bool {
		if ctx.Err() != nil {
			return false
		}
		if v.Vegan > 0 || v.VegOnly > 0 {
			return true
		}
		in := func(s *string) string {
			if s == nil {
				return ""
			}
			return strings.ToLower(*s)
		}
		text := in(v.AdditionalInfo) + " " + strings.ToLower(v.VDetails)
		for _, k := range keywords {
			if strings.Contains(text, k) {
				return true
			}
		}
		return false
	})
}

// HasValidGoogleData checks that Google data exists and looks usable.
func HasValidGoogleData() Specification[models.Venue] {
	return New(func(ctx context.Context, v models.Venue) bool {
		if ctx.Err() != nil {
			return false
		}
		if v.ValidationDetails == nil {
			return false
		}
		if !v.ValidationDetails.GooglePlaceFound {
			return false
		}
		// Consider also that if GooglePlaceData exists, basic fields should be non-empty.
		if v.GoogleData != nil {
			if v.GoogleData.PlaceID == "" || v.GoogleData.Name == "" {
				return false
			}
		}
		return true
	})
}

// PassesDistanceCheck ensures venue is within max meters from Google location.
func PassesDistanceCheck(maxMeters float64) Specification[models.Venue] {
	if maxMeters <= 0 {
		maxMeters = 500 // default safety
	}
	return New(func(ctx context.Context, v models.Venue) bool {
		if ctx.Err() != nil {
			return false
		}
		if v.ValidationDetails == nil {
			return false
		}
		return v.ValidationDetails.DistanceMeters <= maxMeters
	})
}

// HasBasicGeoAndName requires name, location and coordinates.
func HasBasicGeoAndName() Specification[models.Venue] {
	return New(func(ctx context.Context, v models.Venue) bool {
		if ctx.Err() != nil {
			return false
		}
		if strings.TrimSpace(v.Name) == "" {
			return false
		}
		if strings.TrimSpace(v.Location) == "" {
			return false
		}
		return v.Lat != nil && v.Lng != nil
	})
}
