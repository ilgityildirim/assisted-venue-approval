package utils

import (
	"regexp"
	"strings"
)

// NormalizePhoneNumber normalizes a phone number into a canonical format.
// Rules:
// - keep leading '+' if present, otherwise assume +1 for 10-digit US numbers
// - remove all spaces and punctuation
// - preserve country code when possible
func NormalizePhoneNumber(phone string) string {
	if phone == "" {
		return ""
	}

	// Remove all non-digit characters except +
	clean := regexp.MustCompile(`[^\d+]`).ReplaceAllString(phone, "")

	// Keep international format
	if strings.HasPrefix(clean, "+") {
		return clean
	}

	// Common US formats
	if len(clean) == 10 {
		return "+1" + clean
	}
	if len(clean) == 11 && strings.HasPrefix(clean, "1") {
		return "+" + clean
	}

	// Fallback: prefix +
	return "+" + clean
}

// ExtractPhoneDigits returns just the digits in a phone number string.
// Useful for loose comparisons where formatting differences are expected.
func ExtractPhoneDigits(phone string) string {
	return regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
}

// ComparePhoneNumbers compares two phone numbers with a fuzzy strategy.
// Returns a score in [0,1], with 1 being an exact normalized match.
// Heuristics:
// - exact normalized match => 1.0
// - same raw digits => 0.9
// - same last 10 digits => 0.8 (handles different country codes)
// - otherwise => 0.0
func ComparePhoneNumbers(p1, p2 string) float64 {
	if p1 == "" || p2 == "" {
		return 0.0
	}

	n1 := NormalizePhoneNumber(p1)
	n2 := NormalizePhoneNumber(p2)
	if n1 == n2 {
		return 1.0
	}

	d1 := ExtractPhoneDigits(p1)
	d2 := ExtractPhoneDigits(p2)
	if d1 == d2 {
		return 0.9
	}

	if len(d1) >= 10 && len(d2) >= 10 {
		s1 := d1[len(d1)-10:]
		s2 := d2[len(d2)-10:]
		if s1 == s2 {
			return 0.8
		}
	}

	return 0.0
}
