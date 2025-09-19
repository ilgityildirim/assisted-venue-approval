package models

import (
	"errors"
	"fmt"
	"strings"
)

// CombinedInfo is a standardized, merged view of venue data from user and Google
// It is shared by admin UI and AI scorer to avoid divergent logic.
// JSON tags are provided for prompt building.
// Sources map indicates where each field came from: "user" | "google" | "" (unknown)
// Note: Keep this struct stable across packages. Add fields conservatively.
// TODO: consider adding validation warnings for UI display
//
// why: Centralizes combination logic to ensure consistent behavior.

type CombinedInfo struct {
	Name        string            `json:"name"`
	Address     string            `json:"address"`
	Phone       string            `json:"phone"`
	Website     string            `json:"website"`
	Hours       []string          `json:"hours"`
	Lat         *float64          `json:"lat"`
	Lng         *float64          `json:"lng"`
	Types       []string          `json:"types"`
	Description string            `json:"description"`
	Sources     map[string]string `json:"-"`
}

// GetCombinedVenueInfo merges venue data from user submission and Google data
// using trust-based prioritization and validation rules.
//
// Priority rules:
// - High trust (trust >= 0.8) OR venue owner/admin: user data wins, Google is fallback
// - Regular users: Google wins, user is fallback
// Location fallback:
// - If user coordinates are missing or zero, use Google coordinates when available
// Validation:
//   - Returns error for invalid input (nil venue name) but generally prefers lenient merging
//     to avoid blocking flows. Errors indicate severe issues.
func GetCombinedVenueInfo(v Venue, u User, trust float64) (CombinedInfo, error) {
	ci := CombinedInfo{Sources: make(map[string]string)}

	// Basic input sanity
	if strings.TrimSpace(v.Name) == "" {
		return ci, fmt.Errorf("invalid venue: empty name")
	}

	gd := v.GoogleData
	preferUser := isHighTrust(trust) || isOwnerOrAdmin(v, u)

	// Name
	ci.Name, ci.Sources["name"] = pickStringPrefer(preferUser, strPtr(v.Name), getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.Name }))
	if ci.Name == "" {
		ci.Name = v.Name // fallback to raw
		ci.Sources["name"] = "user"
	}

	// Address
	ci.Address, ci.Sources["address"] = pickStringPrefer(preferUser, &v.Location, getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.FormattedAddress }))

	// Phone
	ci.Phone, ci.Sources["phone"] = pickStringPrefer(preferUser, v.Phone, getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.FormattedPhone }))

	// Website
	ci.Website, ci.Sources["website"] = pickStringPrefer(preferUser, v.URL, getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.Website }))

	// Hours: user stored as raw string vs google weekday text
	var userHours []string
	if v.OpenHours != nil && strings.TrimSpace(*v.OpenHours) != "" {
		userHours = []string{*v.OpenHours}
	}
	var googleHours []string
	if gd != nil && gd.OpeningHours != nil && len(gd.OpeningHours.WeekdayText) > 0 {
		googleHours = gd.OpeningHours.WeekdayText
	}
	ci.Hours, ci.Sources["hours"] = pickSlicePrefer(preferUser, userHours, googleHours)

	// Coordinates with fallback for zero/missing
	userLat, userLng := v.Lat, v.Lng
	if isZeroCoord(userLat, userLng) {
		userLat, userLng = nil, nil
	}
	var gLat, gLng *float64
	if gd != nil {
		lat := gd.Geometry.Location.Lat
		lng := gd.Geometry.Location.Lng
		if lat != 0 || lng != 0 {
			gLat, gLng = &lat, &lng
		}
	}
	ci.Lat, ci.Lng, ci.Sources["latlng"] = pickCoordsPrefer(preferUser, userLat, userLng, gLat, gLng)

	// Types - from Google only for now
	if gd != nil && len(gd.Types) > 0 {
		ci.Types = gd.Types
		ci.Sources["types"] = "google"
	} else {
		ci.Types = nil
		ci.Sources["types"] = ""
	}

	// Description - user AdditionalInfo only
	if v.AdditionalInfo != nil && strings.TrimSpace(*v.AdditionalInfo) != "" {
		ci.Description = *v.AdditionalInfo
		ci.Sources["description"] = "user"
	}

	// Minimal validation: ensure we have at least some location/address signal
	if strings.TrimSpace(ci.Address) == "" && isZeroCoord(ci.Lat, ci.Lng) {
		// Not fatal, but indicate via error for callers that care
		return ci, errors.New("insufficient location data: missing address and coordinates")
	}

	return ci, nil
}

func isHighTrust(trust float64) bool { return trust >= 0.8 }

func isOwnerOrAdmin(v Venue, u User) bool {
	if u.ID != 0 && uint(v.UserID) == u.ID {
		return true
	}
	if u.IsVenueAdmin {
		return true
	}
	return false
}

func strPtr(s string) *string { return &s }

func getGoogleString(gd *GooglePlaceData, f func(GooglePlaceData) string) string {
	if gd == nil {
		return ""
	}
	return f(*gd)
}

func getGoogleStringPtr(gd *GooglePlaceData, f func(GooglePlaceData) string) *string {
	if gd == nil {
		return nil
	}
	val := f(*gd)
	return &val
}

func pickStringPrefer(preferUser bool, userVal *string, googleVal *string) (string, string) {
	trim := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	uv := trim(userVal)
	gv := trim(googleVal)
	if preferUser {
		if uv != "" {
			return uv, "user"
		}
		if gv != "" {
			return gv, "google"
		}
		return uv, "user"
	}
	// prefer Google
	if gv != "" {
		return gv, "google"
	}
	if uv != "" {
		return uv, "user"
	}
	return "", ""
}

func pickSlicePrefer(preferUser bool, userVals []string, googleVals []string) ([]string, string) {
	if preferUser {
		if len(userVals) > 0 {
			return userVals, "user"
		}
		if len(googleVals) > 0 {
			return googleVals, "google"
		}
		return nil, ""
	}
	if len(googleVals) > 0 {
		return googleVals, "google"
	}
	if len(userVals) > 0 {
		return userVals, "user"
	}
	return nil, ""
}

func isZeroCoord(lat *float64, lng *float64) bool {
	if lat == nil || lng == nil {
		return true
	}
	return *lat == 0 && *lng == 0
}

func pickCoordsPrefer(preferUser bool, uLat, uLng, gLat, gLng *float64) (lat *float64, lng *float64, source string) {
	// If user prefers but coords invalid, fall back to google
	if preferUser {
		if uLat != nil && uLng != nil && !(isZeroCoord(uLat, uLng)) {
			return uLat, uLng, "user"
		}
		if gLat != nil && gLng != nil {
			return gLat, gLng, "google"
		}
		return uLat, uLng, "user"
	}
	// prefer google
	if gLat != nil && gLng != nil {
		return gLat, gLng, "google"
	}
	if uLat != nil && uLng != nil && !(isZeroCoord(uLat, uLng)) {
		return uLat, uLng, "user"
	}
	return nil, nil, ""
}
