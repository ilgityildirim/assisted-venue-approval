package utils

import (
	"regexp"
	"strings"
)

// NormalizeURL lowercases, adds protocol if missing, removes common prefixes and trailing slash.
// Intended for comparing websites from different sources where formatting varies.
func NormalizeURL(u string) string {
	if u == "" {
		return ""
	}

	n := strings.ToLower(strings.TrimSpace(u))
	if !strings.HasPrefix(n, "http://") && !strings.HasPrefix(n, "https://") {
		n = "https://" + n
	}
	// strip www.
	n = regexp.MustCompile(`^(https?://)www\.`).ReplaceAllString(n, "$1")
	// strip trailing slash
	n = strings.TrimSuffix(n, "/")
	return n
}

// ExtractDomain returns just the host portion of a URL-like string.
func ExtractDomain(u string) string {
	n := NormalizeURL(u)
	d := regexp.MustCompile(`^https?://`).ReplaceAllString(n, "")
	d = regexp.MustCompile(`/.*$`).ReplaceAllString(d, "")
	return d
}

// CompareURLs computes a similarity score for two URLs in [0,1].
// Exact normalized match => 1.0; same domain => 0.8; else fallback to domain similarity.
func CompareURLs(a, b string) float64 {
	if a == "" || b == "" {
		return 0.0
	}

	na := NormalizeURL(a)
	nb := NormalizeURL(b)
	if na == nb {
		return 1.0
	}

	da := ExtractDomain(a)
	db := ExtractDomain(b)
	if da == db {
		return 0.8
	}
	return CalculateStringSimilarity(da, db)
}
