package approval

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// FormatOpenHoursFromCombined converts the Combined.Hours slice into the JSON structure
// expected by downstream systems. Returns an empty string when no parsable hours exist.
func FormatOpenHoursFromCombined(hours []string) (string, error) {
	if len(hours) == 0 {
		return "", nil
	}

	type openHoursFormat struct {
		OpenHours []string `json:"openhours"`
		Note      string   `json:"note"`
	}

	result := openHoursFormat{
		OpenHours: []string{},
		Note:      "",
	}

	dayMap := map[string]string{
		"Monday":    "Mon",
		"Tuesday":   "Tue",
		"Wednesday": "Wed",
		"Thursday":  "Thu",
		"Friday":    "Fri",
		"Saturday":  "Sat",
		"Sunday":    "Sun",
	}

	hoursRegex := regexp.MustCompile(`^(\w+):\s*(.+)$`)
	timeRegex := regexp.MustCompile(`(\d{1,2}):(\d{2})[\s\x{00A0}\x{202F}\x{2009}]*(AM|PM|am|pm)`)

	for _, line := range hours {
		line = strings.TrimSpace(line)
		matches := hoursRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		dayName := matches[1]
		hoursText := matches[2]

		shortDay, ok := dayMap[dayName]
		if !ok {
			continue
		}

		lowerHours := strings.ToLower(hoursText)
		// ok to keep as is.
		if strings.Contains(lowerHours, "closed") {
			continue
		}

		if strings.Contains(lowerHours, "24 hours") || strings.Contains(lowerHours, "open 24") {
			result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-00:00-24:00", shortDay))
			continue
		}

		times := timeRegex.FindAllStringSubmatch(hoursText, -1)
		if len(times) == 2 {
			openTime := convertTo24Hour(times[0][1], times[0][2], times[0][3])
			closeTime := convertTo24Hour(times[1][1], times[1][2], times[1][3])

			if openTime != "" && closeTime != "" {
				result.OpenHours = append(result.OpenHours, fmt.Sprintf("%s-%s-%s", shortDay, openTime, closeTime))
			}
		}
	}

	if len(result.OpenHours) == 0 {
		return "", nil
	}

	bytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal open hours: %w", err)
	}
	return string(bytes), nil
}

// convertTo24Hour converts 12-hour time components into HH:MM (24-hour) format.
func convertTo24Hour(hourStr, minuteStr, period string) string {
	hour := 0
	fmt.Sscanf(hourStr, "%d", &hour)

	minute := 0
	fmt.Sscanf(minuteStr, "%d", &minute)

	period = strings.ToUpper(strings.TrimSpace(period))
	if period == "PM" && hour != 12 {
		hour += 12
	} else if period == "AM" && hour == 12 {
		hour = 0
	}

	return fmt.Sprintf("%02d:%02d", hour, minute)
}
