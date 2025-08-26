package scraper

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"automatic-vendor-validation/internal/models"

	"googlemaps.github.io/maps"
)

type GoogleMapsScraper struct {
	client *maps.Client
}

func NewGoogleMapsScraper(apiKey string) (*GoogleMapsScraper, error) {
	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	return &GoogleMapsScraper{client: client}, nil
}

type EnhancedVenueData struct {
	Venue          models.Venue
	PlaceDetails   *maps.PlaceDetailsResult
	IsBusinessOpen bool
	HasWebsite     bool
	HasPhone       bool
	Rating         float32
	ReviewCount    int
}

func (s *GoogleMapsScraper) EnhanceVenue(ctx context.Context, venue models.Venue) (*EnhancedVenueData, error) {
	// Search for the place using name and location
	searchReq := &maps.TextSearchRequest{
		Query: venue.Name + " " + venue.Location,
	}

	searchResp, err := s.client.TextSearch(ctx, searchReq)
	if err != nil || len(searchResp.Results) == 0 {
		return &EnhancedVenueData{Venue: venue}, nil
	}

	placeID := searchResp.Results[0].PlaceID

	// Get detailed information
	detailsReq := &maps.PlaceDetailsRequest{
		PlaceID: placeID,
		Fields: []maps.PlaceDetailsFieldMask{
			maps.PlaceDetailsFieldMaskName,
			maps.PlaceDetailsFieldMaskFormattedPhoneNumber,
			maps.PlaceDetailsFieldMaskWebsite,
			// maps.PlaceDetailsFieldMaskRating, // Note: Rating field not available in current maps API
			maps.PlaceDetailsFieldMaskUserRatingsTotal,
			maps.PlaceDetailsFieldMaskBusinessStatus,
			maps.PlaceDetailsFieldMaskOpeningHours,
		},
	}

	details, err := s.client.PlaceDetails(ctx, detailsReq)
	if err != nil {
		return &EnhancedVenueData{Venue: venue}, nil
	}

	enhanced := &EnhancedVenueData{
		Venue:        venue,
		PlaceDetails: &details,
		HasWebsite:   details.Website != "",
		HasPhone:     details.FormattedPhoneNumber != "",
		Rating:       details.Rating,
		ReviewCount:  details.UserRatingsTotal,
	}

	if details.BusinessStatus == "OPERATIONAL" {
		enhanced.IsBusinessOpen = true
	}

	return enhanced, nil
}

// NormalizePhoneNumber Phone number normalization functions
func NormalizePhoneNumber(phone string) string {
	if phone == "" {
		return ""
	}

	// Remove all non-digit characters except +
	cleaned := regexp.MustCompile(`[^\d+]`).ReplaceAllString(phone, "")

	// Handle international format
	if strings.HasPrefix(cleaned, "+") {
		return cleaned
	}

	// Handle common US formats
	if len(cleaned) == 10 {
		return "+1" + cleaned
	}

	if len(cleaned) == 11 && strings.HasPrefix(cleaned, "1") {
		return "+" + cleaned
	}

	// Return as-is for other formats
	return "+" + cleaned
}

// ExtractPhoneDigits Extract just the digits for loose comparison
func ExtractPhoneDigits(phone string) string {
	return regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
}

// ComparePhoneNumbers Compare phone numbers with fuzzy matching
func ComparePhoneNumbers(phone1, phone2 string) float64 {
	if phone1 == "" || phone2 == "" {
		return 0.0
	}

	// Exact match after normalization
	norm1 := NormalizePhoneNumber(phone1)
	norm2 := NormalizePhoneNumber(phone2)
	if norm1 == norm2 {
		return 1.0
	}

	// Compare just digits (handles formatting differences)
	digits1 := ExtractPhoneDigits(phone1)
	digits2 := ExtractPhoneDigits(phone2)

	if digits1 == digits2 {
		return 0.9
	}

	// Compare last 10 digits (handles country code differences)
	if len(digits1) >= 10 && len(digits2) >= 10 {
		suffix1 := digits1[len(digits1)-10:]
		suffix2 := digits2[len(digits2)-10:]
		if suffix1 == suffix2 {
			return 0.8
		}
	}

	return 0.0
}

// NormalizeAddress Address normalization functions
func NormalizeAddress(address string) string {
	if address == "" {
		return ""
	}

	// Convert to lowercase
	normalized := strings.ToLower(strings.TrimSpace(address))

	// Common address abbreviations
	abbreviations := map[string]string{
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

	// Apply abbreviations
	words := strings.Fields(normalized)
	for i, word := range words {
		if abbrev, exists := abbreviations[word]; exists {
			words[i] = abbrev
		}
		// Remove punctuation
		words[i] = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(words[i], "")
	}

	return strings.Join(words, " ")
}

// ExtractAddressComponents Extract address components for comparison
func ExtractAddressComponents(address string) map[string]string {
	components := make(map[string]string)
	normalized := NormalizeAddress(address)

	// Extract street number
	if match := regexp.MustCompile(`^\d+`).FindString(normalized); match != "" {
		components["number"] = match
	}

	// Extract zip code
	if match := regexp.MustCompile(`\b\d{5}(-\d{4})?\b`).FindString(normalized); match != "" {
		components["zip"] = match
	}

	// Remove zip and number for street name extraction
	streetName := regexp.MustCompile(`^\d+\s*`).ReplaceAllString(normalized, "")
	streetName = regexp.MustCompile(`\s+\d{5}(-\d{4})?\b.*$`).ReplaceAllString(streetName, "")
	components["street"] = strings.TrimSpace(streetName)

	return components
}

// CompareAddresses Compare addresses with fuzzy matching
func CompareAddresses(addr1, addr2 string) float64 {
	if addr1 == "" || addr2 == "" {
		return 0.0
	}

	// Exact match after normalization
	norm1 := NormalizeAddress(addr1)
	norm2 := NormalizeAddress(addr2)
	if norm1 == norm2 {
		return 1.0
	}

	// Component-wise comparison
	comp1 := ExtractAddressComponents(addr1)
	comp2 := ExtractAddressComponents(addr2)

	score := 0.0
	totalWeight := 0.0

	// Street number (weight: 30%)
	if comp1["number"] != "" && comp2["number"] != "" {
		if comp1["number"] == comp2["number"] {
			score += 0.3
		}
		totalWeight += 0.3
	}

	// Street name (weight: 60%)
	if comp1["street"] != "" && comp2["street"] != "" {
		streetSim := calculateStringSimilarity(comp1["street"], comp2["street"])
		score += 0.6 * streetSim
		totalWeight += 0.6
	}

	// Zip code (weight: 10%)
	if comp1["zip"] != "" && comp2["zip"] != "" {
		if comp1["zip"] == comp2["zip"] {
			score += 0.1
		}
		totalWeight += 0.1
	}

	if totalWeight > 0 {
		return score / totalWeight
	}

	// Fallback to string similarity
	return calculateStringSimilarity(norm1, norm2)
}

// NormalizedHours Opening hours normalization functions
type NormalizedHours struct {
	Monday    []TimeRange `json:"monday"`
	Tuesday   []TimeRange `json:"tuesday"`
	Wednesday []TimeRange `json:"wednesday"`
	Thursday  []TimeRange `json:"thursday"`
	Friday    []TimeRange `json:"friday"`
	Saturday  []TimeRange `json:"saturday"`
	Sunday    []TimeRange `json:"sunday"`
}

type TimeRange struct {
	Open  string `json:"open"`  // "HH:MM" format
	Close string `json:"close"` // "HH:MM" format
}

// ParseGoogleOpeningHours Parse Google Maps opening hours format
func ParseGoogleOpeningHours(periods []maps.OpeningHoursPeriod) NormalizedHours {
	hours := NormalizedHours{}
	dayMap := map[time.Weekday]*[]TimeRange{
		time.Sunday:    &hours.Sunday,
		time.Monday:    &hours.Monday,
		time.Tuesday:   &hours.Tuesday,
		time.Wednesday: &hours.Wednesday,
		time.Thursday:  &hours.Thursday,
		time.Friday:    &hours.Friday,
		time.Saturday:  &hours.Saturday,
	}

	for _, period := range periods {
		if period.Open.Day >= 0 && period.Open.Day <= 6 {
			day := time.Weekday(period.Open.Day)
			if dayHours, exists := dayMap[day]; exists {
				timeRange := TimeRange{
					Open:  formatGoogleTime(period.Open.Time),
					Close: formatGoogleTime(period.Close.Time),
				}
				*dayHours = append(*dayHours, timeRange)
			}
		}
	}

	// Sort time ranges for each day
	for _, dayHours := range dayMap {
		sort.Slice(*dayHours, func(i, j int) bool {
			return (*dayHours)[i].Open < (*dayHours)[j].Open
		})
	}

	return hours
}

// ParseHappyCowOpeningHours Parse HappyCow opening hours text format
func ParseHappyCowOpeningHours(hoursText string) NormalizedHours {
	hours := NormalizedHours{}
	if hoursText == "" {
		return hours
	}

	// Split by lines or semicolons
	lines := regexp.MustCompile(`[;\n\r]+`).Split(hoursText, -1)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse patterns like "Mon-Fri: 9:00-17:00" or "Monday 9am-5pm"
		if parsed := parseHoursLine(line); len(parsed) > 0 {
			for day, timeRanges := range parsed {
				switch strings.ToLower(day) {
				case "monday", "mon", "m":
					hours.Monday = append(hours.Monday, timeRanges...)
				case "tuesday", "tue", "tu", "t":
					hours.Tuesday = append(hours.Tuesday, timeRanges...)
				case "wednesday", "wed", "w":
					hours.Wednesday = append(hours.Wednesday, timeRanges...)
				case "thursday", "thu", "th", "r":
					hours.Thursday = append(hours.Thursday, timeRanges...)
				case "friday", "fri", "f":
					hours.Friday = append(hours.Friday, timeRanges...)
				case "saturday", "sat", "sa", "s":
					hours.Saturday = append(hours.Saturday, timeRanges...)
				case "sunday", "sun", "su":
					hours.Sunday = append(hours.Sunday, timeRanges...)
				}
			}
		}
	}

	return hours
}

// Helper function to parse individual hours line
func parseHoursLine(line string) map[string][]TimeRange {
	result := make(map[string][]TimeRange)

	// Handle day ranges like "Mon-Fri"
	dayPattern := regexp.MustCompile(`(?i)(mon|monday|tue|tuesday|wed|wednesday|thu|thursday|fri|friday|sat|saturday|sun|sunday)(?:day)?(?:\s*-\s*(mon|monday|tue|tuesday|wed|wednesday|thu|thursday|fri|friday|sat|saturday|sun|sunday)(?:day)?)?`)
	timePattern := regexp.MustCompile(`(\d{1,2}):?(\d{2})?\s*(am|pm)?\s*-\s*(\d{1,2}):?(\d{2})?\s*(am|pm)?`)

	dayMatch := dayPattern.FindStringSubmatch(line)
	timeMatch := timePattern.FindStringSubmatch(line)

	if len(dayMatch) > 1 && len(timeMatch) > 1 {
		startDay := normalizeDayName(dayMatch[1])
		endDay := startDay
		if len(dayMatch) > 2 && dayMatch[2] != "" {
			endDay = normalizeDayName(dayMatch[2])
		}

		// Parse time range
		openTime := normalizeTime(timeMatch[1], timeMatch[2], timeMatch[3])
		closeTime := normalizeTime(timeMatch[4], timeMatch[5], timeMatch[6])

		if openTime != "" && closeTime != "" {
			timeRange := []TimeRange{{Open: openTime, Close: closeTime}}

			// Apply to day range
			days := expandDayRange(startDay, endDay)
			for _, day := range days {
				result[day] = timeRange
			}
		}
	}

	return result
}

// Helper functions for opening hours parsing
func formatGoogleTime(googleTime string) string {
	if len(googleTime) == 4 {
		hour := googleTime[:2]
		minute := googleTime[2:]
		return hour + ":" + minute
	}
	return googleTime
}

func normalizeDayName(day string) string {
	day = strings.ToLower(day)
	switch day {
	case "mon", "monday":
		return "monday"
	case "tue", "tuesday":
		return "tuesday"
	case "wed", "wednesday":
		return "wednesday"
	case "thu", "thursday":
		return "thursday"
	case "fri", "friday":
		return "friday"
	case "sat", "saturday":
		return "saturday"
	case "sun", "sunday":
		return "sunday"
	}
	return day
}

func normalizeTime(hour, minute, ampm string) string {
	if hour == "" {
		return ""
	}

	h, _ := strconv.Atoi(hour)
	m := 0
	if minute != "" {
		m, _ = strconv.Atoi(minute)
	}

	// Convert to 24-hour format
	if ampm != "" {
		ampm = strings.ToLower(ampm)
		if ampm == "pm" && h != 12 {
			h += 12
		} else if ampm == "am" && h == 12 {
			h = 0
		}
	}

	return fmt.Sprintf("%02d:%02d", h, m)
}

func expandDayRange(startDay, endDay string) []string {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

	startIdx := findDayIndex(startDay)
	endIdx := findDayIndex(endDay)

	if startIdx == -1 || endIdx == -1 {
		return []string{startDay}
	}

	var result []string
	for i := startIdx; i != (endIdx+1)%7; i = (i + 1) % 7 {
		result = append(result, days[i])
		if i == endIdx {
			break
		}
	}

	return result
}

func findDayIndex(day string) int {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	for i, d := range days {
		if d == day {
			return i
		}
	}
	return -1
}

// CompareOpeningHours Compare opening hours
func CompareOpeningHours(hours1, hours2 NormalizedHours) float64 {
	dayScores := []float64{
		compareTimeRanges(hours1.Monday, hours2.Monday),
		compareTimeRanges(hours1.Tuesday, hours2.Tuesday),
		compareTimeRanges(hours1.Wednesday, hours2.Wednesday),
		compareTimeRanges(hours1.Thursday, hours2.Thursday),
		compareTimeRanges(hours1.Friday, hours2.Friday),
		compareTimeRanges(hours1.Saturday, hours2.Saturday),
		compareTimeRanges(hours1.Sunday, hours2.Sunday),
	}

	total := 0.0
	for _, score := range dayScores {
		total += score
	}

	return total / float64(len(dayScores))
}

func compareTimeRanges(ranges1, ranges2 []TimeRange) float64 {
	if len(ranges1) == 0 && len(ranges2) == 0 {
		return 1.0 // Both closed
	}

	if len(ranges1) == 0 || len(ranges2) == 0 {
		return 0.0 // One closed, one open
	}

	// Simple comparison - check if first range overlaps significantly
	r1 := ranges1[0]
	r2 := ranges2[0]

	// Calculate time difference in minutes
	openDiff := timeToMinutes(r2.Open) - timeToMinutes(r1.Open)
	closeDiff := timeToMinutes(r2.Close) - timeToMinutes(r1.Close)

	// Score based on time difference (within 30 minutes = 1.0, up to 2 hours = 0.5)
	openScore := 1.0 - (float64(abs(openDiff)) / 120.0)
	closeScore := 1.0 - (float64(abs(closeDiff)) / 120.0)

	if openScore < 0 {
		openScore = 0
	}
	if closeScore < 0 {
		closeScore = 0
	}

	return (openScore + closeScore) / 2.0
}

func timeToMinutes(timeStr string) int {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	return hours*60 + minutes
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// NormalizeURL URL/Website normalization functions
func NormalizeURL(url string) string {
	if url == "" {
		return ""
	}

	// Convert to lowercase
	normalized := strings.ToLower(strings.TrimSpace(url))

	// Add protocol if missing
	if !strings.HasPrefix(normalized, "http://") && !strings.HasPrefix(normalized, "https://") {
		normalized = "https://" + normalized
	}

	// Remove www prefix for comparison
	normalized = regexp.MustCompile(`^(https?://)www\.`).ReplaceAllString(normalized, "$1")

	// Remove trailing slash
	normalized = strings.TrimSuffix(normalized, "/")

	return normalized
}

// ExtractDomain Extract domain from URL
func ExtractDomain(url string) string {
	normalized := NormalizeURL(url)

	// Remove protocol
	domain := regexp.MustCompile(`^https?://`).ReplaceAllString(normalized, "")

	// Remove path
	domain = regexp.MustCompile(`/.*$`).ReplaceAllString(domain, "")

	return domain
}

// CompareURLs Compare URLs
func CompareURLs(url1, url2 string) float64 {
	if url1 == "" || url2 == "" {
		return 0.0
	}

	// Exact match after normalization
	norm1 := NormalizeURL(url1)
	norm2 := NormalizeURL(url2)
	if norm1 == norm2 {
		return 1.0
	}

	// Domain comparison
	domain1 := ExtractDomain(url1)
	domain2 := ExtractDomain(url2)
	if domain1 == domain2 {
		return 0.8
	}

	// String similarity fallback
	return calculateStringSimilarity(domain1, domain2)
}

// Utility function for string similarity (Levenshtein-based)
func calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if s1 == "" || s2 == "" {
		return 0.0
	}

	// Simple similarity based on common characters
	longer := s1
	shorter := s2
	if len(s2) > len(s1) {
		longer = s2
		shorter = s1
	}

	if len(longer) == 0 {
		return 1.0
	}

	commonChars := 0
	for _, char := range shorter {
		if strings.ContainsRune(longer, char) {
			commonChars++
		}
	}

	return float64(commonChars) / float64(len(longer))
}

// CompareVenueData compares HappyCow venue data with Google Places data using normalization
func CompareVenueData(happyCowVenue models.Venue, googleData models.GooglePlaceData) models.ValidationDetails {
	startTime := time.Now()

	var conflicts []models.DataConflict
	scoreBreakdown := models.ScoreBreakdown{}

	// 1. Venue Name Matching (25 points)
	nameScore := calculateStringSimilarity(strings.ToLower(happyCowVenue.Name),
		strings.ToLower(googleData.Name))
	scoreBreakdown.VenueNameMatch = int(nameScore * 25)

	if nameScore < 0.8 {
		conflicts = append(conflicts, models.DataConflict{
			Field:         "name",
			HappyCowValue: happyCowVenue.Name,
			GoogleValue:   googleData.Name,
			Resolution:    determineResolution(nameScore),
		})
	}

	// 2. Address Accuracy (20 points)
	addressScore := 0.0
	if happyCowVenue.Location != "" && googleData.FormattedAddress != "" {
		addressScore = CompareAddresses(happyCowVenue.Location, googleData.FormattedAddress)
	}
	scoreBreakdown.AddressAccuracy = int(addressScore * 20)

	if addressScore < 0.8 {
		conflicts = append(conflicts, models.DataConflict{
			Field:         "address",
			HappyCowValue: happyCowVenue.Location,
			GoogleValue:   googleData.FormattedAddress,
			Resolution:    determineResolution(addressScore),
		})
	}

	// 3. Geolocation Accuracy (15 points)
	geoScore := 0.0
	distanceMeters := 0.0
	if happyCowVenue.Lat != nil && happyCowVenue.Lng != nil {
		distanceMeters = calculateDistance(*happyCowVenue.Lat, *happyCowVenue.Lng,
			googleData.Geometry.Location.Lat, googleData.Geometry.Location.Lng)

		// Score based on distance (within 50m = full points, up to 500m scaled)
		if distanceMeters <= 50 {
			geoScore = 1.0
		} else if distanceMeters <= 500 {
			geoScore = 1.0 - (distanceMeters-50)/450
		}
	}
	scoreBreakdown.GeolocationAccuracy = int(geoScore * 15)

	// 4. Phone Number Verification (10 points)
	phoneScore := 0.0
	if happyCowVenue.Phone != nil && googleData.FormattedPhone != "" {
		phoneScore = ComparePhoneNumbers(*happyCowVenue.Phone, googleData.FormattedPhone)
	} else if happyCowVenue.Phone == nil && googleData.FormattedPhone == "" {
		phoneScore = 1.0 // Both missing is neutral
	} else {
		phoneScore = 0.5 // One missing
	}
	scoreBreakdown.PhoneVerification = int(phoneScore * 10)

	if happyCowVenue.Phone != nil && googleData.FormattedPhone != "" && phoneScore < 0.8 {
		conflicts = append(conflicts, models.DataConflict{
			Field:         "phone",
			HappyCowValue: *happyCowVenue.Phone,
			GoogleValue:   googleData.FormattedPhone,
			Resolution:    determineResolution(phoneScore),
		})
	}

	// 5. Website Verification (5 points)
	websiteScore := 0.0
	if happyCowVenue.URL != nil && googleData.Website != "" {
		websiteScore = CompareURLs(*happyCowVenue.URL, googleData.Website)
	} else if happyCowVenue.URL == nil && googleData.Website == "" {
		websiteScore = 1.0 // Both missing is neutral
	} else {
		websiteScore = 0.5 // One missing
	}
	scoreBreakdown.WebsiteVerification = int(websiteScore * 5)

	// 6. Business Hours Verification (10 points)
	hoursScore := 0.0
	if happyCowVenue.OpenHours != nil && googleData.OpeningHours != nil {
		happyCowHours := ParseHappyCowOpeningHours(*happyCowVenue.OpenHours)
		// Convert our model periods to maps periods for parsing
		var mapsPeriods []maps.OpeningHoursPeriod
		for _, period := range googleData.OpeningHours.Periods {
			mapsPeriods = append(mapsPeriods, maps.OpeningHoursPeriod{
				Open: maps.OpeningHoursOpenClose{
					Day:  time.Weekday(period.Open.Day),
					Time: period.Open.Time,
				},
				Close: maps.OpeningHoursOpenClose{
					Day:  time.Weekday(period.Close.Day),
					Time: period.Close.Time,
				},
			})
		}
		googleHours := ParseGoogleOpeningHours(mapsPeriods)
		hoursScore = CompareOpeningHours(happyCowHours, googleHours)
	} else if happyCowVenue.OpenHours == nil && googleData.OpeningHours == nil {
		hoursScore = 1.0 // Both missing is neutral
	} else {
		hoursScore = 0.5 // One missing
	}
	scoreBreakdown.BusinessHours = int(hoursScore * 10)

	// 7. Business Status (5 points)
	statusScore := 0.0
	switch googleData.BusinessStatus {
	case "OPERATIONAL":
		statusScore = 1.0
	case "TEMPORARILY_CLOSED":
		statusScore = 0.4
	case "PERMANENTLY_CLOSED":
		statusScore = 0.0
	default:
		statusScore = 0.4 // Unknown status
	}
	scoreBreakdown.BusinessStatus = int(statusScore * 5)

	// 8. Postal Code (5 points) - Extract from Google address components
	postalScore := 0.0
	googleZip := extractPostalCodeFromComponents(googleData.AddressComponents)
	if happyCowVenue.Zipcode != nil && googleZip != "" {
		if *happyCowVenue.Zipcode == googleZip {
			postalScore = 1.0
		} else {
			// Check if same area (first 3 digits for US)
			if len(*happyCowVenue.Zipcode) >= 3 && len(googleZip) >= 3 &&
				(*happyCowVenue.Zipcode)[:3] == googleZip[:3] {
				postalScore = 0.6
			}
		}
	} else if happyCowVenue.Zipcode == nil && googleZip == "" {
		postalScore = 1.0 // Both missing is neutral
	} else {
		postalScore = 0.4 // One missing
	}
	scoreBreakdown.PostalCode = int(postalScore * 5)

	// 9. Vegan/Vegetarian Relevance (5 points) - Basic relevance check
	veganScore := 1.0 // Assume relevant unless clear indicators suggest otherwise
	if happyCowVenue.AdditionalInfo != nil {
		additionalInfo := strings.ToLower(*happyCowVenue.AdditionalInfo)
		if strings.Contains(additionalInfo, "meat") || strings.Contains(additionalInfo, "non-veg") {
			veganScore = 0.2
		}
	}
	scoreBreakdown.VeganRelevance = int(veganScore * 5)

	// Calculate total score
	scoreBreakdown.Total = scoreBreakdown.VenueNameMatch + scoreBreakdown.AddressAccuracy +
		scoreBreakdown.GeolocationAccuracy + scoreBreakdown.PhoneVerification +
		scoreBreakdown.BusinessHours + scoreBreakdown.WebsiteVerification +
		scoreBreakdown.BusinessStatus + scoreBreakdown.PostalCode +
		scoreBreakdown.VeganRelevance

	// Determine auto-decision reason
	autoDecisionReason := ""
	if scoreBreakdown.Total >= 85 {
		autoDecisionReason = "High confidence match - auto-approved"
	} else if scoreBreakdown.Total >= 50 {
		autoDecisionReason = "Medium confidence - requires manual review"
	} else {
		autoDecisionReason = "Low confidence match - auto-rejected"
	}

	processingTime := time.Since(startTime).Milliseconds()

	return models.ValidationDetails{
		ScoreBreakdown:     scoreBreakdown,
		GooglePlaceFound:   true,
		DistanceMeters:     distanceMeters,
		Conflicts:          conflicts,
		AutoDecisionReason: autoDecisionReason,
		ProcessingTimeMs:   processingTime,
	}
}

// Helper function to determine conflict resolution strategy
func determineResolution(score float64) string {
	if score >= 0.8 {
		return "user_data" // Trust user data for minor differences
	} else if score >= 0.5 {
		return "manual_review" // Requires human decision
	} else {
		return "google_data" // Trust Google data for major differences
	}
}

// Calculate distance between two coordinates in meters using Haversine formula
func calculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000 // meters

	lat1Rad := lat1 * (3.14159265359 / 180)
	lat2Rad := lat2 * (3.14159265359 / 180)
	deltaLatRad := (lat2 - lat1) * (3.14159265359 / 180)
	deltaLngRad := (lng2 - lng1) * (3.14159265359 / 180)

	a := math.Sin(deltaLatRad/2)*math.Sin(deltaLatRad/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLngRad/2)*math.Sin(deltaLngRad/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// Extract postal code from Google address components
func extractPostalCodeFromComponents(components []models.AddressComponent) string {
	for _, component := range components {
		for _, typ := range component.Types {
			if typ == "postal_code" {
				return component.LongName
			}
		}
	}
	return ""
}

// EnhanceVenueWithValidation Enhanced venue enhancement method that includes validation details
func (s *GoogleMapsScraper) EnhanceVenueWithValidation(ctx context.Context, venue models.Venue) (*models.Venue, error) {
	// Get enhanced venue data
	enhanced, err := s.EnhanceVenue(ctx, venue)
	if err != nil {
		return &venue, err
	}

	// If no Google data found, return with minimal validation details
	if enhanced.PlaceDetails == nil {
		venue.ValidationDetails = &models.ValidationDetails{
			GooglePlaceFound:   false,
			AutoDecisionReason: "No matching Google Place found - requires manual review",
			ProcessingTimeMs:   0,
		}
		return &venue, nil
	}

	// Convert Google Places data to our model format
	googleData := convertToGooglePlaceData(*enhanced.PlaceDetails)

	// Perform detailed comparison
	validationDetails := CompareVenueData(venue, googleData)

	// Add Google data to venue
	venue.GoogleData = &googleData
	venue.GooglePlaceID = enhanced.PlaceDetails.PlaceID
	venue.ValidationDetails = &validationDetails

	// Fill missing venue data from Google where appropriate
	fillMissingVenueData(&venue, googleData)

	return &venue, nil
}

// Convert Google Places API response to our model format
func convertToGooglePlaceData(details maps.PlaceDetailsResult) models.GooglePlaceData {
	googleData := models.GooglePlaceData{
		PlaceID:          details.PlaceID,
		Name:             details.Name,
		FormattedAddress: details.FormattedAddress,
		FormattedPhone:   details.FormattedPhoneNumber,
		Website:          details.Website,
		Rating:           float64(details.Rating),
		UserRatingsTotal: details.UserRatingsTotal,
		Types:            details.Types,
	}

	googleData.BusinessStatus = details.BusinessStatus

	// Convert geometry
	googleData.Geometry = models.GoogleGeometry{
		Location: models.GoogleLatLng{
			Lat: details.Geometry.Location.Lat,
			Lng: details.Geometry.Location.Lng,
		},
	}

	// Set viewport bounds
	if details.Geometry.Viewport.NorthEast.Lat != 0 && details.Geometry.Viewport.NorthEast.Lng != 0 {
		googleData.Geometry.Viewport = models.GoogleBounds{
			Northeast: models.GoogleLatLng{
				Lat: details.Geometry.Viewport.NorthEast.Lat,
				Lng: details.Geometry.Viewport.NorthEast.Lng,
			},
			Southwest: models.GoogleLatLng{
				Lat: details.Geometry.Viewport.SouthWest.Lat,
				Lng: details.Geometry.Viewport.SouthWest.Lng,
			},
		}
	}

	// Convert opening hours
	if details.OpeningHours != nil {
		openingHours := &models.GoogleOpeningHours{
			OpenNow:     details.OpeningHours.OpenNow != nil && *details.OpeningHours.OpenNow,
			WeekdayText: details.OpeningHours.WeekdayText,
		}

		for _, period := range details.OpeningHours.Periods {
			convertedPeriod := models.GooglePeriod{
				Open: models.GoogleTime{
					Day:  int(period.Open.Day),
					Time: period.Open.Time,
				},
				Close: models.GoogleTime{
					Day:  int(period.Close.Day),
					Time: period.Close.Time,
				},
			}
			openingHours.Periods = append(openingHours.Periods, convertedPeriod)
		}

		googleData.OpeningHours = openingHours
	}

	// Convert address components
	for _, component := range details.AddressComponents {
		googleData.AddressComponents = append(googleData.AddressComponents, models.AddressComponent{
			LongName:  component.LongName,
			ShortName: component.ShortName,
			Types:     component.Types,
		})
	}

	return googleData
}

// Fill missing venue data from Google Places data
func fillMissingVenueData(venue *models.Venue, googleData models.GooglePlaceData) {
	// Fill missing phone
	if venue.Phone == nil && googleData.FormattedPhone != "" {
		venue.Phone = &googleData.FormattedPhone
	}

	// Fill missing website
	if venue.URL == nil && googleData.Website != "" {
		venue.URL = &googleData.Website
	}

	// Fill missing coordinates
	if venue.Lat == nil && venue.Lng == nil {
		venue.Lat = &googleData.Geometry.Location.Lat
		venue.Lng = &googleData.Geometry.Location.Lng
	}

	// Fill missing zipcode
	if venue.Zipcode == nil {
		if zipcode := extractPostalCodeFromComponents(googleData.AddressComponents); zipcode != "" {
			venue.Zipcode = &zipcode
		}
	}
}
