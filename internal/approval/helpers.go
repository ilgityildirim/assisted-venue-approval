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

	dayAliases := map[string]string{
		"mon":       "Mon",
		"monday":    "Mon",
		"tue":       "Tue",
		"tues":      "Tue",
		"tuesday":   "Tue",
		"wed":       "Wed",
		"weds":      "Wed",
		"wednesday": "Wed",
		"thu":       "Thu",
		"thur":      "Thu",
		"thurs":     "Thu",
		"thursday":  "Thu",
		"fri":       "Fri",
		"friday":    "Fri",
		"sat":       "Sat",
		"saturday":  "Sat",
		"sun":       "Sun",
		"sunday":    "Sun",
	}

	for _, line := range hours {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if normalized := parseNormalizedHourLine(line, dayAliases); normalized != "" {
			result.OpenHours = append(result.OpenHours, normalized)
			continue
		}

		if normalized := parseLegacyHourLine(line, dayAliases); normalized != "" {
			result.OpenHours = append(result.OpenHours, normalized)
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

func parseNormalizedHourLine(line string, dayAliases map[string]string) string {
	normalizedPattern := regexp.MustCompile(`(?i)^([A-Za-z]{3})\s*-\s*(\d{1,2}):(\d{2})\s*-\s*(\d{1,2}):(\d{2})$`)
	matches := normalizedPattern.FindStringSubmatch(line)
	if len(matches) != 6 {
		return ""
	}

	shortDay := normalizeDay(matches[1], dayAliases)
	if shortDay == "" {
		return ""
	}

	openTime := normalize24Hour(matches[2], matches[3])
	closeTime := normalize24Hour(matches[4], matches[5])
	if openTime == "" || closeTime == "" {
		return ""
	}

	return fmt.Sprintf("%s-%s-%s", shortDay, openTime, closeTime)
}

func parseLegacyHourLine(line string, dayAliases map[string]string) string {
	hoursRegex := regexp.MustCompile(`^([\p{L}]+):\s*(.+)$`)
	matches := hoursRegex.FindStringSubmatch(line)
	if len(matches) != 3 {
		return ""
	}

	shortDay := normalizeDay(matches[1], dayAliases)
	if shortDay == "" {
		return ""
	}

	hoursText := strings.TrimSpace(matches[2])
	lowered := strings.ToLower(hoursText)
	if strings.Contains(lowered, "closed") {
		return ""
	}
	if strings.Contains(lowered, "24 hours") || strings.Contains(lowered, "open 24") {
		return fmt.Sprintf("%s-00:00-24:00", shortDay)
	}

	openTime, closeTime := parseTimeRange(hoursText)
	if openTime == "" || closeTime == "" {
		return ""
	}

	return fmt.Sprintf("%s-%s-%s", shortDay, openTime, closeTime)
}

func normalizeDay(day string, dayAliases map[string]string) string {
	if day == "" {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(day))
	if short, ok := dayAliases[key]; ok {
		return short
	}
	if len(day) >= 3 {
		if short, ok := dayAliases[key[:3]]; ok {
			return short
		}
	}
	return ""
}

func normalize24Hour(hourStr, minuteStr string) string {
	hour := 0
	minute := 0
	if _, err := fmt.Sscanf(hourStr, "%d", &hour); err != nil {
		return ""
	}
	if _, err := fmt.Sscanf(minuteStr, "%d", &minute); err != nil {
		return ""
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func parseTimeRange(text string) (string, string) {
	ampmRegex := regexp.MustCompile(`(?i)(\d{1,2}):(\d{2})\s*(am|pm)`)
	if matches := ampmRegex.FindAllStringSubmatch(text, -1); len(matches) >= 2 {
		open := convertTo24Hour(matches[0][1], matches[0][2], matches[0][3])
		close := convertTo24Hour(matches[1][1], matches[1][2], matches[1][3])
		return open, close
	}

	twentyFourRegex := regexp.MustCompile(`(\d{1,2}):(\d{2})`)
	if matches := twentyFourRegex.FindAllStringSubmatch(text, -1); len(matches) >= 2 {
		open := normalize24Hour(matches[0][1], matches[0][2])
		close := normalize24Hour(matches[1][1], matches[1][2])
		return open, close
	}

	return "", ""
}
