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

	"assisted-venue-approval/internal/constants"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/circuit"
	"assisted-venue-approval/pkg/utils"

	"googlemaps.github.io/maps"
)

type GoogleMapsScraper struct {
	client *maps.Client
	cb     *circuit.Breaker
}

func NewGoogleMapsScraper(apiKey string) (*GoogleMapsScraper, error) {
	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	// Circuit breaker tuned for Google Maps
	cb := circuit.New(circuit.Config{
		Name:              "googlemaps",
		OperationTimeout:  constants.GoogleMapsOperationTimeout,
		OpenFor:           constants.GoogleMapsOpenFor,
		MaxConsecFailures: 3,
		WindowSize:        20,
		FailureRate:       constants.CircuitFailureRate,
		SlowCallThreshold: constants.GoogleMapsSlowCallThreshold,
		SlowCallRate:      constants.CircuitSlowCallRate,
	}, nil)

	return &GoogleMapsScraper{client: client, cb: cb}, nil
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
	// Add per-request timeout; TODO: make configurable
	ctx, cancel := context.WithTimeout(ctx, constants.GoogleMapsRequestTimeout)
	defer cancel()

	// Respect cancellation early
	select {
	case <-ctx.Done():
		return &EnhancedVenueData{Venue: venue}, ctx.Err()
	default:
	}

	// Search for the place using name and location
	searchReq := &maps.TextSearchRequest{
		Query: venue.Name + " " + venue.Location,
	}

	var searchResp *maps.PlacesSearchResponse
	var err error
	// Use circuit breaker for TextSearch
	err = s.cb.Do(ctx, func(ctx context.Context) error {
		resp, e := s.client.TextSearch(ctx, searchReq)
		if e != nil {
			return e
		}
		searchResp = &resp
		return nil
	}, func(ctx context.Context, cause error) error {
		// Fallback: fail soft with minimal data
		searchResp = nil
		return nil
	})
	if err != nil || searchResp == nil || len(searchResp.Results) == 0 {
		return &EnhancedVenueData{Venue: venue}, nil
	}

	placeID := searchResp.Results[0].PlaceID

	// Get detailed information
	detailsReq := &maps.PlaceDetailsRequest{
		PlaceID: placeID,
		Fields: []maps.PlaceDetailsFieldMask{
			maps.PlaceDetailsFieldMaskName,
			maps.PlaceDetailsFieldMaskPlaceID,
			maps.PlaceDetailsFieldMaskFormattedAddress,
			maps.PlaceDetailsFieldMaskGeometry,
			maps.PlaceDetailsFieldMaskAddressComponent,
			maps.PlaceDetailsFieldMaskTypes,
			maps.PlaceDetailsFieldMaskFormattedPhoneNumber,
			maps.PlaceDetailsFieldMaskWebsite,
			// maps.PlaceDetailsFieldMaskRating, // Not available in current client; fallback to TextSearch rating
			maps.PlaceDetailsFieldMaskUserRatingsTotal,
			maps.PlaceDetailsFieldMaskBusinessStatus,
			maps.PlaceDetailsFieldMaskOpeningHours,
		},
	}

	var details maps.PlaceDetailsResult
	err = s.cb.Do(ctx, func(ctx context.Context) error {
		d, e := s.client.PlaceDetails(ctx, detailsReq)
		if e != nil {
			return e
		}
		details = d
		return nil
	}, func(ctx context.Context, cause error) error {
		// Fallback: return minimal data from search result
		return nil
	})
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

	// Fallback: if rating not returned in details but present in TextSearch results
	if (enhanced.Rating == 0 || math.IsNaN(float64(enhanced.Rating))) && enhanced.ReviewCount > 0 && len(searchResp.Results) > 0 {
		if searchResp.Results[0].Rating > 0 {
			enhanced.Rating = float32(searchResp.Results[0].Rating)
		}
	}

	if enhanced.Rating == 0 && enhanced.ReviewCount > 0 {
		fmt.Printf("[warn] EnhanceVenue: rating is 0 but user_ratings_total=%d for place_id=%s (may be missing field mask or permissions)\n", enhanced.ReviewCount, placeID)
	}

	if details.BusinessStatus == "OPERATIONAL" {
		enhanced.IsBusinessOpen = true
	}

	return enhanced, nil
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

// CompareVenueData compares HappyCow venue data with Google Places data using normalization
func CompareVenueData(happyCowVenue models.Venue, googleData models.GooglePlaceData) models.ValidationDetails {
	startTime := time.Now()

	var conflicts []models.DataConflict
	scoreBreakdown := models.ScoreBreakdown{}

	// 1. Venue Name Matching (25 points)
	nameScore := utils.CalculateStringSimilarity(strings.ToLower(happyCowVenue.Name),
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
		addressScore = utils.CompareAddresses(happyCowVenue.Location, googleData.FormattedAddress)
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
		phoneScore = utils.ComparePhoneNumbers(*happyCowVenue.Phone, googleData.FormattedPhone)
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
		websiteScore = utils.CompareURLs(*happyCowVenue.URL, googleData.Website)
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
	// Fallback: patch rating from TextSearch if details rating is absent
	if googleData.Rating == 0 && googleData.UserRatingsTotal > 0 && enhanced.Rating > 0 {
		googleData.Rating = float64(enhanced.Rating)
	}

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
		FetchedAt:        time.Now(),
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

	// Basic sanity logging for missing essential fields
	if googleData.PlaceID == "" || googleData.FormattedAddress == "" || (googleData.Geometry.Location.Lat == 0 && googleData.Geometry.Location.Lng == 0) {
		fmt.Printf("[warn] convertToGooglePlaceData: missing essential fields: place_id='%s' formatted_address='%s' lat=%f lng=%f\n", googleData.PlaceID, googleData.FormattedAddress, googleData.Geometry.Location.Lat, googleData.Geometry.Location.Lng)
	}
	// Warning if rating missing but reviews exist
	if googleData.Rating == 0 && googleData.UserRatingsTotal > 0 {
		fmt.Printf("[warn] convertToGooglePlaceData: rating is 0 but user_ratings_total=%d for place_id='%s'\n", googleData.UserRatingsTotal, googleData.PlaceID)
	}

	return googleData
}

// Fill missing venue data from Google Places data
func fillMissingVenueData(venue *models.Venue, googleData models.GooglePlaceData) {
	// Fill missing phone
	if venue.Phone == nil && googleData.FormattedPhone != "" {
		venue.Phone = &googleData.FormattedPhone
	}

	// Fill missing website - but check if it's actually social media first
	if venue.URL == nil && googleData.Website != "" {
		websiteURL := strings.ToLower(strings.TrimSpace(googleData.Website))

		// Check if Google website is Facebook
		if strings.Contains(websiteURL, "facebook.com/") ||
			strings.Contains(websiteURL, "fb.com/") ||
			strings.Contains(websiteURL, "m.facebook.com/") {
			// Move to FBUrl if empty
			if venue.FBUrl == nil || *venue.FBUrl == "" {
				venue.FBUrl = &googleData.Website
			}
		} else if strings.Contains(websiteURL, "instagram.com/") ||
			strings.Contains(websiteURL, "instagr.am/") {
			// Move to InstagramUrl if empty
			if venue.InstagramUrl == nil || *venue.InstagramUrl == "" {
				venue.InstagramUrl = &googleData.Website
			}
		} else {
			// Regular website - fill as normal
			venue.URL = &googleData.Website
		}
	}

	// Always override with Google Places coordinates - Google data is authoritative
	lat := googleData.Geometry.Location.Lat
	lng := googleData.Geometry.Location.Lng
	venue.Lat = &lat
	venue.Lng = &lng

	// Fill missing zipcode
	if venue.Zipcode == nil {
		if zipcode := extractPostalCodeFromComponents(googleData.AddressComponents); zipcode != "" {
			venue.Zipcode = &zipcode
		}
	}
}
