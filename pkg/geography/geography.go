package geography

import (
	_ "embed"
	"encoding/json"
	"strings"

	"googlemaps.github.io/maps"
)

//go:embed countries.json
var countriesJSON []byte

var countryToContinentMap map[string]string

func init() {
	if err := json.Unmarshal(countriesJSON, &countryToContinentMap); err != nil {
		panic("failed to load countries.json: " + err.Error())
	}
}

// GetContinent returns the continent for a given country name (case-insensitive).
// Returns empty string if country not found.
func GetContinent(country string) string {
	normalized := strings.ToLower(strings.TrimSpace(country))
	return countryToContinentMap[normalized]
}

// NormalizeName converts a string to lowercase with spaces replaced by underscores.
func NormalizeName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return strings.ReplaceAll(normalized, " ", "_")
}

// GenerateVenuePath generates a venue path from Google Places address components.
// Returns a path string in format: "continent|country|state|county|city|neighborhood"
// Only includes components that are present in the address.
func GenerateVenuePath(addressComponents []maps.AddressComponent) string {
	// Map to track component types we want, in order from broad to specific
	wantedTypes := map[string]int{
		"country":                     0,
		"administrative_area_level_1": 1, // State/Province
		"administrative_area_level_2": 2, // County
		"locality":                    3, // City
		"sublocality":                 4, // Neighborhood
	}

	// Create a map to store found components
	found := make(map[int]string)

	// Extract relevant address components
	for _, component := range addressComponents {
		for _, compType := range component.Types {
			if priority, exists := wantedTypes[compType]; exists {
				if _, alreadyFound := found[priority]; !alreadyFound {
					found[priority] = component.LongName
				}
			}
		}
	}

	// Build path starting with continent
	var pathComponents []string

	// First, add continent if we found a country
	if countryName, hasCountry := found[0]; hasCountry {
		continent := GetContinent(countryName)
		if continent != "" {
			pathComponents = append(pathComponents, continent)
		}
	}

	// Add remaining components in order
	for i := 0; i < len(wantedTypes); i++ {
		if component, exists := found[i]; exists {
			normalized := NormalizeName(component)
			pathComponents = append(pathComponents, normalized)
		}
	}

	return strings.Join(pathComponents, "|")
}
