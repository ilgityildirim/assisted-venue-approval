package utils

import "strings"

// CalculateStringSimilarity returns a similarity score between two strings in the range [0,1].
// It uses a simple character-overlap heuristic that works well enough for fuzzy matching
// of human-entered data (names, streets, domains) without external dependencies.
// NOTE: This is intentionally lightweight; swap for a stronger metric if needed.
func CalculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if s1 == "" || s2 == "" {
		return 0.0
	}

	longer := s1
	shorter := s2
	if len(s2) > len(s1) {
		longer = s2
		shorter = s1
	}
	if len(longer) == 0 {
		return 1.0
	}

	common := 0
	for _, r := range shorter {
		if strings.ContainsRune(longer, r) {
			common++
		}
	}
	return float64(common) / float64(len(longer))
}
