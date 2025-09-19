package specs

import (
	"context"
	"os"
	"strconv"

	"assisted-venue-approval/internal/models"
)

// VenueRuleOptions controls how the composite venue specs are built.
// Sourced from environment to keep it simple and avoid touching global config wiring.
// ENV vars (with defaults):
//  SPEC_MIN_CONTACT_FIELDS (2)
//  SPEC_REQUIRE_GOOGLE_DATA (true)
//  SPEC_MAX_DISTANCE_METERS (500)
//  SPEC_ENABLE_VEGAN_RELEVANCE (true)

type VenueRuleOptions struct {
	MinContactFields     int
	RequireGoogleData    bool
	MaxDistanceMeters    float64
	EnableVeganRelevance bool
}

func defaultOpts() VenueRuleOptions {
	return VenueRuleOptions{MinContactFields: 2, RequireGoogleData: true, MaxDistanceMeters: 500, EnableVeganRelevance: true}
}

func optsFromEnv() VenueRuleOptions {
	o := defaultOpts()
	if v := os.Getenv("SPEC_MIN_CONTACT_FIELDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			o.MinContactFields = n
		}
	}
	if v := os.Getenv("SPEC_REQUIRE_GOOGLE_DATA"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			o.RequireGoogleData = b
		}
	}
	if v := os.Getenv("SPEC_MAX_DISTANCE_METERS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			o.MaxDistanceMeters = f
		}
	}
	if v := os.Getenv("SPEC_ENABLE_VEGAN_RELEVANCE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			o.EnableVeganRelevance = b
		}
	}
	return o
}

// BuildApprovalSpecFromEnv builds a composite spec used for authority-based auto-approval checks.
// It requires: basic geo+name, vegan relevance (optional), contact info, and optionally Google data + distance.
func BuildApprovalSpecFromEnv() Specification[models.Venue] {
	o := optsFromEnv()

	base := HasBasicGeoAndName().And(HasCompleteContactInfo(o.MinContactFields))
	if o.EnableVeganRelevance {
		base = base.And(IsVeganRelevant())
	}
	if o.RequireGoogleData {
		base = base.And(HasValidGoogleData()).And(PassesDistanceCheck(o.MaxDistanceMeters))
	}
	return base
}

// Evaluate evaluates a spec with the provided context.
// Keeping it simple: callers should pass their request or processing ctx.
func Evaluate[T any](ctx context.Context, s Specification[T], v T) bool {
	return s.IsSatisfiedBy(ctx, v)
}
