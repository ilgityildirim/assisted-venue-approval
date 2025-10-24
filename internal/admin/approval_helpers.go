package admin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"assisted-venue-approval/internal/models"
)

// AISuggestions contains AI-suggested improvements extracted from validation history
type AISuggestions struct {
	Name        string  // Suggested venue name (if needs correction)
	Description string  // Suggested description (always provided)
	ClosedDays  *string // Formatted closed days string
}

// GoogleVenueData contains validated data from Google Maps
type GoogleVenueData struct {
	Lat          *float64 // Latitude from Google
	Lng          *float64 // Longitude from Google
	Phone        string   // Formatted phone number
	OpeningHours []string // Weekday text array
}

// ExtractAISuggestions safely extracts AI quality suggestions from validation history
// Returns nil suggestions if AIOutputData is missing or malformed (not an error condition)
func ExtractAISuggestions(history *models.ValidationHistory) (*AISuggestions, error) {
	if history == nil {
		return nil, fmt.Errorf("validation history is nil")
	}

	// If no AI output data, return empty suggestions (not an error)
	if history.AIOutputData == nil || *history.AIOutputData == "" {
		return &AISuggestions{}, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(*history.AIOutputData), &raw); err != nil {
		return &AISuggestions{}, nil // Malformed JSON, return empty suggestions
	}

	suggestions := &AISuggestions{}

	// Extract quality suggestions
	if qualityMap, ok := raw["quality"].(map[string]interface{}); ok {
		// Name suggestion (only if correction needed)
		if name, ok := qualityMap["name"].(string); ok && strings.TrimSpace(name) != "" {
			suggestions.Name = strings.TrimSpace(name)
		}

		// Description suggestion (always provided)
		if desc, ok := qualityMap["description"].(string); ok && strings.TrimSpace(desc) != "" {
			suggestions.Description = strings.TrimSpace(desc)
		}

		// Closed days suggestion
		if closedDays, ok := qualityMap["closed_days"].(string); ok && strings.TrimSpace(closedDays) != "" {
			cd := strings.TrimSpace(closedDays)
			suggestions.ClosedDays = &cd
		}
	}

	return suggestions, nil
}

// ExtractGoogleData safely extracts validated Google Maps data from validation history
// Returns nil data if Google data is missing or incomplete (not an error condition)
func ExtractGoogleData(history *models.ValidationHistory) (*GoogleVenueData, error) {
	if history == nil {
		return nil, fmt.Errorf("validation history is nil")
	}

	// If no Google data, return empty data (not an error)
	if history.GooglePlaceData == nil {
		return &GoogleVenueData{}, nil
	}

	gd := history.GooglePlaceData
	data := &GoogleVenueData{}

	// Extract coordinates (validate they're not zero)
	if gd.Geometry.Location.Lat != 0 || gd.Geometry.Location.Lng != 0 {
		lat := gd.Geometry.Location.Lat
		lng := gd.Geometry.Location.Lng
		data.Lat = &lat
		data.Lng = &lng
	}

	// Extract phone (formatted)
	if strings.TrimSpace(gd.FormattedPhone) != "" {
		data.Phone = strings.TrimSpace(gd.FormattedPhone)
	}

	// Extract opening hours
	if gd.OpeningHours != nil && len(gd.OpeningHours.WeekdayText) > 0 {
		data.OpeningHours = gd.OpeningHours.WeekdayText
	}

	return data, nil
}

// FormatOpenHoursJSON converts Google weekday text format to required JSON structure
// Input: ["Monday: 7:30 AM – 10:00 PM", "Tuesday: 7:30 AM – 10:00 PM", ...]
// Output: {"openhours":["Mon-07:30-22:00","Tue-07:30-22:00",...],"note":""}
// Returns empty string if hours cannot be parsed
func FormatOpenHoursJSON(googleHours []string) (string, error) {
	if len(googleHours) == 0 {
		return "", nil
	}

	type OpenHoursFormat struct {
		OpenHours []string `json:"openhours"`
		Note      string   `json:"note"`
	}

	result := OpenHoursFormat{
		OpenHours: []string{},
		Note:      "",
	}

	// Day name mapping
	dayMap := map[string]string{
		"Monday":    "Mon",
		"Tuesday":   "Tue",
		"Wednesday": "Wed",
		"Thursday":  "Thu",
		"Friday":    "Fri",
		"Saturday":  "Sat",
		"Sunday":    "Sun",
	}

	// Regex to parse Google hours format: "Monday: 7:30 AM – 10:00 PM"
	// Also handles: "Monday: Closed", "Monday: Open 24 hours"
	hoursRegex := regexp.MustCompile(`^(\w+):\s*(.+)$`)
	timeRegex := regexp.MustCompile(`(\d{1,2}):(\d{2})\s*(AM|PM)`)

	for _, line := range googleHours {
		matches := hoursRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 3 {
			continue
		}

		dayName := matches[1]
		hoursText := matches[2]

		shortDay, ok := dayMap[dayName]
		if !ok {
			continue
		}

		// Handle closed days
		if strings.Contains(strings.ToLower(hoursText), "closed") {
			continue // Skip closed days
		}

		// Handle 24-hour venues
		if strings.Contains(strings.ToLower(hoursText), "24 hours") || strings.Contains(strings.ToLower(hoursText), "open 24") {
			result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-00:00-24:00", shortDay))
			continue
		}

		// Parse regular hours (e.g., "7:30 AM – 10:00 PM")
		times := timeRegex.FindAllStringSubmatch(hoursText, -1)
		if len(times) == 2 {
			// Convert to 24-hour format
			openTime := convertTo24Hour(times[0][1], times[0][2], times[0][3])
			closeTime := convertTo24Hour(times[1][1], times[1][2], times[1][3])

			if openTime != "" && closeTime != "" {
				result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-%s-%s", shortDay, openTime, closeTime))
			}
		}
	}

	// If no valid hours parsed, return empty string
	if len(result.OpenHours) == 0 {
		return "", nil
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal open hours: %w", err)
	}

	return string(jsonBytes), nil
}

// convertTo24Hour converts 12-hour time format to 24-hour format
// Input: hour="7", minute="30", period="AM" -> Output: "07:30"
// Input: hour="10", minute="00", period="PM" -> Output: "22:00"
func convertTo24Hour(hourStr, minuteStr, period string) string {
	hour := 0
	fmt.Sscanf(hourStr, "%d", &hour)

	minute := 0
	fmt.Sscanf(minuteStr, "%d", &minute)

	// Convert to 24-hour format
	if period == "PM" && hour != 12 {
		hour += 12
	} else if period == "AM" && hour == 12 {
		hour = 0
	}

	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// FormatOpenHoursFromCombined converts Combined.Hours format to required JSON structure
// Input: Combined.Hours array (e.g., ["Monday: 7:30 AM – 10:00 PM", "Tuesday: 7:30 AM – 10:00 PM", ...])
// Output: {"openhours":["Mon-07:30-22:00","Tue-07:30-22:00",...],"note":""}
func FormatOpenHoursFromCombined(hours []string) (string, error) {
	if len(hours) == 0 {
		return "", nil
	}

	type OpenHoursFormat struct {
		OpenHours []string `json:"openhours"`
		Note      string   `json:"note"`
	}

	result := OpenHoursFormat{
		OpenHours: []string{},
		Note:      "",
	}

	// Day name mapping
	dayMap := map[string]string{
		"Monday":    "Mon",
		"Tuesday":   "Tue",
		"Wednesday": "Wed",
		"Thursday":  "Thu",
		"Friday":    "Fri",
		"Saturday":  "Sat",
		"Sunday":    "Sun",
	}

	// Regex to parse hours format: "Monday: 7:30 AM – 10:00 PM"
	// Also handles: "Monday: Closed", "Monday: Open 24 hours"
	hoursRegex := regexp.MustCompile(`^(\w+):\s*(.+)$`)
	timeRegex := regexp.MustCompile(`(\d{1,2}):(\d{2})\s*(AM|PM)`)

	for _, line := range hours {
		matches := hoursRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 3 {
			continue
		}

		dayName := matches[1]
		hoursText := matches[2]

		shortDay, ok := dayMap[dayName]
		if !ok {
			continue
		}

		// Handle closed days
		if strings.Contains(strings.ToLower(hoursText), "closed") {
			continue // Skip closed days
		}

		// Handle 24-hour venues
		if strings.Contains(strings.ToLower(hoursText), "24 hours") || strings.Contains(strings.ToLower(hoursText), "open 24") {
			result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-00:00-24:00", shortDay))
			continue
		}

		// Parse regular hours (e.g., "7:30 AM – 10:00 PM")
		times := timeRegex.FindAllStringSubmatch(hoursText, -1)
		if len(times) == 2 {
			// Convert to 24-hour format
			openTime := convertTo24Hour(times[0][1], times[0][2], times[0][3])
			closeTime := convertTo24Hour(times[1][1], times[1][2], times[1][3])

			if openTime != "" && closeTime != "" {
				result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-%s-%s", shortDay, openTime, closeTime))
			}
		}
	}

	// If no valid hours parsed, return empty string
	if len(result.OpenHours) == 0 {
		return "", nil
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal open hours: %w", err)
	}

	return string(jsonBytes), nil
}
