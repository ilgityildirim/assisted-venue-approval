package utils

import (
	"regexp"
	"strings"
)

// NormalizeAddress normalizes a postal address string for comparison.
// Lowercases, trims, replaces common words with abbreviations, and strips punctuation.
func NormalizeAddress(address string) string {
	if address == "" {
		return ""
	}

	n := strings.ToLower(strings.TrimSpace(address))

	abbr := map[string]string{
		"street":    "st",
		"avenue":    "ave",
		"boulevard": "blvd",
		"road":      "rd",
		"drive":     "dr",
		"lane":      "ln",
		"court":     "ct",
		"place":     "pl",
		"square":    "sq",
		"north":     "n",
		"south":     "s",
		"east":      "e",
		"west":      "w",
		"northeast": "ne",
		"northwest": "nw",
		"southeast": "se",
		"southwest": "sw",
	}

	words := strings.Fields(n)
	for i, w := range words {
		if a, ok := abbr[w]; ok {
			words[i] = a
		}
		words[i] = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(words[i], "")
	}
	return strings.Join(words, " ")
}

// ExtractAddressComponents pulls simple components for comparison: number, street, zip.
func ExtractAddressComponents(address string) map[string]string {
	comp := make(map[string]string)
	n := NormalizeAddress(address)

	if m := regexp.MustCompile(`^\d+`).FindString(n); m != "" {
		comp["number"] = m
	}
	if m := regexp.MustCompile(`\b\d{5}(-\d{4})?\b`).FindString(n); m != "" {
		comp["zip"] = m
	}
	street := regexp.MustCompile(`^\d+\s*`).ReplaceAllString(n, "")
	street = regexp.MustCompile(`\s+\d{5}(-\d{4})?\b.*$`).ReplaceAllString(street, "")
	comp["street"] = strings.TrimSpace(street)
	return comp
}

// CompareAddresses computes a fuzzy match score for two addresses in [0,1].
// It weighs street name most, then number, then zip as a tie-breaker.
func CompareAddresses(a1, a2 string) float64 {
	if a1 == "" || a2 == "" {
		return 0.0
	}

	n1 := NormalizeAddress(a1)
	n2 := NormalizeAddress(a2)
	if n1 == n2 {
		return 1.0
	}

	c1 := ExtractAddressComponents(a1)
	c2 := ExtractAddressComponents(a2)

	score := 0.0
	total := 0.0

	// number (30%)
	if c1["number"] != "" && c2["number"] != "" {
		if c1["number"] == c2["number"] {
			score += 0.3
		}
		total += 0.3
	}
	// street (60%)
	if c1["street"] != "" && c2["street"] != "" {
		streetSim := CalculateStringSimilarity(c1["street"], c2["street"])
		score += 0.6 * streetSim
		total += 0.6
	}
	// zip (10%)
	if c1["zip"] != "" && c2["zip"] != "" {
		if c1["zip"] == c2["zip"] {
			score += 0.1
		}
		total += 0.1
	}

	if total > 0 {
		return score / total
	}
	return CalculateStringSimilarity(n1, n2)
}
