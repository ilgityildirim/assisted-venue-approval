package geography

import (
	"testing"

	"googlemaps.github.io/maps"
)

func TestGetContinent(t *testing.T) {
	tests := []struct {
		name     string
		country  string
		expected string
	}{
		{"USA exact match", "united states", "north_america"},
		{"USA alternate", "usa", "north_america"},
		{"USA with case", "United States", "north_america"},
		{"USA with spaces", "  USA  ", "north_america"},
		{"South Korea", "south korea", "asia"},
		{"China", "china", "asia"},
		{"Japan", "japan", "asia"},
		{"Germany", "germany", "europe"},
		{"Brazil", "brazil", "south_america"},
		{"Australia", "australia", "oceania"},
		{"Kenya", "kenya", "africa"},
		{"UK", "uk", "europe"},
		{"United Kingdom", "united kingdom", "europe"},
		{"Russia", "russia", "europe"},
		{"Czech Republic", "czech republic", "europe"},
		{"Czechia", "czechia", "europe"},
		{"Unknown country", "atlantis", ""},
		{"Empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContinent(tt.country)
			if result != tt.expected {
				t.Errorf("GetContinent(%q) = %q, want %q", tt.country, result, tt.expected)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple lowercase", "chicago", "chicago"},
		{"With spaces", "New York", "new_york"},
		{"Multiple spaces", "Los Angeles County", "los_angeles_county"},
		{"With trim", "  Chicago  ", "chicago"},
		{"Mixed case", "SaN FrAnCiScO", "san_francisco"},
		{"Already normalized", "north_america", "north_america"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateVenuePath(t *testing.T) {
	tests := []struct {
		name       string
		components []maps.AddressComponent
		expected   string
	}{
		{
			name: "Full US address",
			components: []maps.AddressComponent{
				{LongName: "Chicago", ShortName: "Chicago", Types: []string{"locality"}},
				{LongName: "Cook County", ShortName: "Cook County", Types: []string{"administrative_area_level_2"}},
				{LongName: "Illinois", ShortName: "IL", Types: []string{"administrative_area_level_1"}},
				{LongName: "United States", ShortName: "US", Types: []string{"country"}},
			},
			expected: "north_america|united_states|illinois|cook_county|chicago",
		},
		{
			name: "US with neighborhood",
			components: []maps.AddressComponent{
				{LongName: "Loop", ShortName: "Loop", Types: []string{"sublocality"}},
				{LongName: "Chicago", ShortName: "Chicago", Types: []string{"locality"}},
				{LongName: "Cook County", ShortName: "Cook County", Types: []string{"administrative_area_level_2"}},
				{LongName: "Illinois", ShortName: "IL", Types: []string{"administrative_area_level_1"}},
				{LongName: "United States", ShortName: "US", Types: []string{"country"}},
			},
			expected: "north_america|united_states|illinois|cook_county|chicago|loop",
		},
		{
			name: "South Korea address",
			components: []maps.AddressComponent{
				{LongName: "Seoul", ShortName: "Seoul", Types: []string{"locality"}},
				{LongName: "South Korea", ShortName: "KR", Types: []string{"country"}},
			},
			expected: "asia|south_korea|seoul",
		},
		{
			name: "Japan with province",
			components: []maps.AddressComponent{
				{LongName: "Tokyo", ShortName: "Tokyo", Types: []string{"locality"}},
				{LongName: "Tokyo", ShortName: "Tokyo", Types: []string{"administrative_area_level_1"}},
				{LongName: "Japan", ShortName: "JP", Types: []string{"country"}},
			},
			expected: "asia|japan|tokyo|tokyo",
		},
		{
			name: "Germany address",
			components: []maps.AddressComponent{
				{LongName: "Berlin", ShortName: "Berlin", Types: []string{"locality"}},
				{LongName: "Berlin", ShortName: "Berlin", Types: []string{"administrative_area_level_1"}},
				{LongName: "Germany", ShortName: "DE", Types: []string{"country"}},
			},
			expected: "europe|germany|berlin|berlin",
		},
		{
			name: "UK address",
			components: []maps.AddressComponent{
				{LongName: "London", ShortName: "London", Types: []string{"locality"}},
				{LongName: "Greater London", ShortName: "Greater London", Types: []string{"administrative_area_level_2"}},
				{LongName: "England", ShortName: "England", Types: []string{"administrative_area_level_1"}},
				{LongName: "United Kingdom", ShortName: "GB", Types: []string{"country"}},
			},
			expected: "europe|united_kingdom|england|greater_london|london",
		},
		{
			name: "Minimal - Country only",
			components: []maps.AddressComponent{
				{LongName: "Brazil", ShortName: "BR", Types: []string{"country"}},
			},
			expected: "south_america|brazil",
		},
		{
			name: "No country - should still work",
			components: []maps.AddressComponent{
				{LongName: "Chicago", ShortName: "Chicago", Types: []string{"locality"}},
				{LongName: "Illinois", ShortName: "IL", Types: []string{"administrative_area_level_1"}},
			},
			expected: "illinois|chicago",
		},
		{
			name: "Unknown country",
			components: []maps.AddressComponent{
				{LongName: "Some City", ShortName: "Some City", Types: []string{"locality"}},
				{LongName: "Atlantis", ShortName: "ATL", Types: []string{"country"}},
			},
			expected: "atlantis|some_city",
		},
		{
			name:       "Empty components",
			components: []maps.AddressComponent{},
			expected:   "",
		},
		{
			name: "Multi-word names with spaces",
			components: []maps.AddressComponent{
				{LongName: "New York", ShortName: "New York", Types: []string{"locality"}},
				{LongName: "New York County", ShortName: "New York County", Types: []string{"administrative_area_level_2"}},
				{LongName: "New York", ShortName: "NY", Types: []string{"administrative_area_level_1"}},
				{LongName: "United States", ShortName: "US", Types: []string{"country"}},
			},
			expected: "north_america|united_states|new_york|new_york_county|new_york",
		},
		{
			name: "Canada address",
			components: []maps.AddressComponent{
				{LongName: "Toronto", ShortName: "Toronto", Types: []string{"locality"}},
				{LongName: "Toronto Division", ShortName: "Toronto Division", Types: []string{"administrative_area_level_2"}},
				{LongName: "Ontario", ShortName: "ON", Types: []string{"administrative_area_level_1"}},
				{LongName: "Canada", ShortName: "CA", Types: []string{"country"}},
			},
			expected: "north_america|canada|ontario|toronto_division|toronto",
		},
		{
			name: "Australia address",
			components: []maps.AddressComponent{
				{LongName: "Melbourne", ShortName: "Melbourne", Types: []string{"locality"}},
				{LongName: "Victoria", ShortName: "VIC", Types: []string{"administrative_area_level_1"}},
				{LongName: "Australia", ShortName: "AU", Types: []string{"country"}},
			},
			expected: "oceania|australia|victoria|melbourne",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateVenuePath(tt.components)
			if result != tt.expected {
				t.Errorf("GenerateVenuePath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCountryToContinent_AllCountries(t *testing.T) {
	// Test that all entries in the map are valid
	validContinents := map[string]bool{
		"africa":        true,
		"asia":          true,
		"europe":        true,
		"north_america": true,
		"south_america": true,
		"oceania":       true,
	}

	for country, continent := range countryToContinentMap {
		if !validContinents[continent] {
			t.Errorf("Country %q has invalid continent %q", country, continent)
		}
	}

	// Ensure we have a reasonable number of countries (should be 195+)
	if len(countryToContinentMap) < 195 {
		t.Errorf("Expected at least 195 countries, got %d", len(countryToContinentMap))
	}
}

func BenchmarkGetContinent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetContinent("united states")
	}
}

func BenchmarkNormalizeName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NormalizeName("New York City")
	}
}

func BenchmarkGenerateVenuePath(b *testing.B) {
	components := []maps.AddressComponent{
		{LongName: "Chicago", ShortName: "Chicago", Types: []string{"locality"}},
		{LongName: "Cook County", ShortName: "Cook County", Types: []string{"administrative_area_level_2"}},
		{LongName: "Illinois", ShortName: "IL", Types: []string{"administrative_area_level_1"}},
		{LongName: "United States", ShortName: "US", Types: []string{"country"}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateVenuePath(components)
	}
}
