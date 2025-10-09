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
	// New fields for venue classification
	VenueType    string   `json:"venue_type"`
	VeganStatus  string   `json:"vegan_status"`
	Category     string   `json:"category"`
	GoogleTypes  []string `json:"google_types"`
	TypeMismatch bool     `json:"type_mismatch"`
	Path         string   `json:"path"`
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

	// Name - always prefer user for consistency
	ci.Name, ci.Sources["name"] = pickStringPrefer(true, strPtr(v.Name), getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.Name }))
	if ci.Name == "" {
		ci.Name = v.Name // fallback to raw
		ci.Sources["name"] = "user"
	}

	// Address - always prefer Google for standardized formatting (St, Ln, Rd abbreviations)
	// User address only used as fallback when Google is unavailable
	ci.Address, ci.Sources["address"] = pickStringPrefer(false, &v.Location, getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.FormattedAddress }))

	// Phone - fallback to Google for regular users OR if user data is empty
	userPhone := v.Phone
	if userPhone != nil && strings.TrimSpace(*userPhone) == "" {
		userPhone = nil // treat empty as missing
	}
	ci.Phone, ci.Sources["phone"] = pickStringPrefer(preferUser && userPhone != nil, userPhone, getGoogleStringPtr(gd, func(g GooglePlaceData) string { return g.FormattedPhone }))

	// Website - fallback to Google if user left empty
	userWebsite := v.URL
	if userWebsite != nil && strings.TrimSpace(*userWebsite) == "" {
		userWebsite = nil
	}
	if userWebsite == nil && gd != nil && gd.Website != "" {
		ci.Website = gd.Website
		ci.Sources["website"] = "google"
	} else if userWebsite != nil {
		ci.Website = *userWebsite
		ci.Sources["website"] = "user"
	}

	// Hours - use Google data if user left empty
	var userHours []string
	if v.OpenHours != nil && strings.TrimSpace(*v.OpenHours) != "" {
		userHours = []string{*v.OpenHours}
	}
	var googleHours []string
	if gd != nil && gd.OpeningHours != nil && len(gd.OpeningHours.WeekdayText) > 0 {
		googleHours = gd.OpeningHours.WeekdayText
	}

	if len(userHours) == 0 && len(googleHours) > 0 {
		ci.Hours = googleHours
		ci.Sources["hours"] = "google"
	} else if len(userHours) > 0 {
		ci.Hours = userHours
		ci.Sources["hours"] = "user"
	}

	// Coordinates - if not high trust user, use Google data
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

	coordPreferUser := preferUser
	if !isHighTrust(trust) && !isOwnerOrAdmin(v, u) {
		coordPreferUser = false // Force Google for non-high-trust users
	}
	ci.Lat, ci.Lng, ci.Sources["latlng"] = pickCoordsPrefer(coordPreferUser, userLat, userLng, gLat, gLng)

	// Store Google types separately for comparison
	if gd != nil && len(gd.Types) > 0 {
		ci.GoogleTypes = gd.Types
		// Keep original behavior for Types field
		ci.Types = gd.Types
		ci.Sources["types"] = "google"
	}

	// Description - user AdditionalInfo only
	if v.AdditionalInfo != nil && strings.TrimSpace(*v.AdditionalInfo) != "" {
		ci.Description = *v.AdditionalInfo
		ci.Sources["description"] = "user"
	}

	// Path - user-submitted URL slug (always from user)
	if v.Path != nil && strings.TrimSpace(*v.Path) != "" {
		ci.Path = strings.TrimSpace(*v.Path)
		ci.Sources["path"] = "user"
	}

	// New venue classification fields
	ci.VenueType = getVenueType(v.EntryType)
	ci.VeganStatus = getVeganStatus(v.EntryType, v.VegOnly, v.Vegan)
	ci.Category = getCategory(v.EntryType, v.Category)

	// Check for type mismatch between user classification and Google types
	ci.TypeMismatch = checkTypeMismatch(ci.VenueType, ci.Category, ci.GoogleTypes)

	// Minimal validation: ensure we have at least some location/address signal
	if strings.TrimSpace(ci.Address) == "" && isZeroCoord(ci.Lat, ci.Lng) {
		return ci, errors.New("insufficient location data: missing address and coordinates")
	}

	return ci, nil
}

// Helper functions for venue classification
func getVenueType(entrytype int) string {
	if entrytype == 2 {
		return "Store"
	}
	return "Restaurant"
}

func getVeganStatus(entrytype, vegonly, vegan int) string {
	if entrytype == 2 {
		return "Store" // stores don't use vegan/non-veg labels
	}
	if vegonly == 1 && vegan == 1 {
		return "Vegan"
	}
	if vegonly == 1 && vegan == 0 {
		return "Vegetarian"
	}
	return "Vegan Options"
}

func getCategory(entrytype, category int) string {
	if category == 0 || entrytype == 1 {
		return ""
	}
	categories := map[int]string{
		1: "Health Store", 2: "Veg Store", 3: "Bakery", 4: "B&B",
		5: "Delivery", 6: "Catering", 7: "Organization", 8: "Farmer's Market",
		10: "Food Truck", 11: "Market Vendor", 12: "Ice Cream", 13: "Juice Bar",
		14: "Professional", 15: "Coffee & Tea", 16: "Spa", 99: "Other",
	}
	if name, ok := categories[category]; ok {
		return name
	}
	return ""
}

func checkTypeMismatch(venueType, category string, googleTypes []string) bool {
	// Simple heuristic - can be enhanced
	if len(googleTypes) == 0 {
		return false
	}

	// Convert our classifications to comparable Google type patterns
	expectedTypes := map[string][]string{
		"Restaurant":   {"restaurant", "food", "meal_takeaway", "cafe"},
		"Store":        {"establishment", "store", "supermarket", "food", "cafe", "grocery_or_supermarket"},
		"Bakery":       {"bakery", "food", "cafe", "establishment", "store"},
		"Juice Bar":    {"restaurant", "food"},
		"Coffee & Tea": {"cafe", "food", "bakery", "store", "restaurant"},
		"Food Truck":   {"restaurant", "food", "meal_takeaway"},
	}

	searchKey := category
	if searchKey == "" {
		searchKey = venueType
	}

	expected, exists := expectedTypes[searchKey]
	if !exists {
		return false // Can't determine mismatch
	}

	// Check if any Google type matches our expectations
	for _, gType := range googleTypes {
		for _, exp := range expected {
			if strings.Contains(strings.ToLower(gType), exp) {
				return false // Match found, no mismatch
			}
		}
	}

	return true // No matches found, potential mismatch
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
