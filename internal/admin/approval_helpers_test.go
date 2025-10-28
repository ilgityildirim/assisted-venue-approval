package admin

import (
	"encoding/json"
	"testing"
)

// TestFormatOpenHoursFromCombined tests the hours formatting function with various inputs
func TestFormatOpenHoursFromCombined(t *testing.T) {
	tests := []struct {
		name           string
		input          []string
		expectedHours  int // number of expected hour entries
		shouldHaveJSON bool
		description    string
	}{
		{
			name: "Standard Google Places format",
			input: []string{
				"Monday: 11:00 AM – 9:00 PM",
				"Tuesday: 11:00 AM – 9:00 PM",
				"Wednesday: 11:00 AM – 9:00 PM",
				"Thursday: 11:00 AM – 9:00 PM",
				"Friday: 11:00 AM – 9:00 PM",
				"Saturday: 11:00 AM – 9:00 PM",
				"Sunday: 11:00 AM – 9:00 PM",
			},
			expectedHours:  7,
			shouldHaveJSON: true,
			description:    "All days open with standard AM/PM format",
		},
		{
			name: "With closed day",
			input: []string{
				"Monday: 11:00 AM – 9:00 PM",
				"Tuesday: Closed",
				"Wednesday: 11:00 AM – 9:00 PM",
				"Thursday: 11:00 AM – 9:00 PM",
				"Friday: 11:00 AM – 9:00 PM",
				"Saturday: 11:00 AM – 9:00 PM",
				"Sunday: 11:00 AM – 9:00 PM",
			},
			expectedHours:  6,
			shouldHaveJSON: true,
			description:    "One closed day should be skipped",
		},
		{
			name: "24-hour venue",
			input: []string{
				"Monday: Open 24 hours",
				"Tuesday: Open 24 hours",
				"Wednesday: Open 24 hours",
				"Thursday: Open 24 hours",
				"Friday: Open 24 hours",
				"Saturday: Open 24 hours",
				"Sunday: Open 24 hours",
			},
			expectedHours:  7,
			shouldHaveJSON: true,
			description:    "24-hour venues should be formatted correctly",
		},
		{
			name: "Mixed case AM/PM",
			input: []string{
				"Monday: 7:30 am – 10:00 pm",
				"Tuesday: 7:30 AM – 10:00 PM",
			},
			expectedHours:  2,
			shouldHaveJSON: true,
			description:    "Should handle both lowercase and uppercase AM/PM",
		},
		{
			name: "Different time formats",
			input: []string{
				"Monday: 7:30 AM – 10:00 PM",
				"Tuesday: 12:00 PM – 8:00 PM",
				"Wednesday: 6:00 AM – 2:00 PM",
			},
			expectedHours:  3,
			shouldHaveJSON: true,
			description:    "Various time combinations",
		},
		{
			name:           "Empty input",
			input:          []string{},
			expectedHours:  0,
			shouldHaveJSON: false,
			description:    "Empty input should return empty string",
		},
		{
			name: "All closed days",
			input: []string{
				"Monday: Closed",
				"Tuesday: Closed",
				"Wednesday: Closed",
				"Thursday: Closed",
				"Friday: Closed",
				"Saturday: Closed",
				"Sunday: Closed",
			},
			expectedHours:  0,
			shouldHaveJSON: false,
			description:    "All closed days should return empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatOpenHoursFromCombined(tt.input)

			if err != nil {
				t.Errorf("FormatOpenHoursFromCombined() error = %v, expected nil", err)
				return
			}

			if !tt.shouldHaveJSON {
				if result != "" {
					t.Errorf("FormatOpenHoursFromCombined() = %q, expected empty string for %s", result, tt.description)
				}
				return
			}

			if result == "" {
				t.Errorf("FormatOpenHoursFromCombined() returned empty string, expected JSON for %s", tt.description)
				return
			}

			// Parse the JSON to verify structure
			var parsed struct {
				OpenHours []string `json:"openhours"`
				Note      string   `json:"note"`
			}

			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Errorf("FormatOpenHoursFromCombined() returned invalid JSON: %v\nJSON: %s", err, result)
				return
			}

			if len(parsed.OpenHours) != tt.expectedHours {
				t.Errorf("FormatOpenHoursFromCombined() parsed %d hours, expected %d\nJSON: %s\nInput: %v",
					len(parsed.OpenHours), tt.expectedHours, result, tt.input)
			}

			// Verify format of each hour entry (should be "Day-HH:MM-HH:MM")
			for i, hour := range parsed.OpenHours {
				if !isValidHourFormat(hour) {
					t.Errorf("FormatOpenHoursFromCombined() hour[%d] = %q has invalid format (expected Day-HH:MM-HH:MM)",
						i, hour)
				}
			}

			// Verify note is empty
			if parsed.Note != "" {
				t.Errorf("FormatOpenHoursFromCombined() note = %q, expected empty string", parsed.Note)
			}
		})
	}
}

// TestFormatOpenHours12To24HourConversion specifically tests time conversion
func TestFormatOpenHours12To24HourConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Monday: 12:00 AM – 11:59 PM", "Mon-00:00-23:59"},
		{"Monday: 12:00 PM – 11:59 PM", "Mon-12:00-23:59"},
		{"Monday: 1:00 AM – 1:00 PM", "Mon-01:00-13:00"},
		{"Monday: 11:00 AM – 9:00 PM", "Mon-11:00-21:00"},
		{"Monday: 7:30 AM – 10:30 PM", "Mon-07:30-22:30"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := FormatOpenHoursFromCombined([]string{tt.input})
			if err != nil {
				t.Errorf("FormatOpenHoursFromCombined() error = %v", err)
				return
			}

			var parsed struct {
				OpenHours []string `json:"openhours"`
			}

			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Errorf("Failed to parse JSON: %v", err)
				return
			}

			if len(parsed.OpenHours) != 1 {
				t.Errorf("Expected 1 hour entry, got %d", len(parsed.OpenHours))
				return
			}

			if parsed.OpenHours[0] != tt.expected {
				t.Errorf("FormatOpenHoursFromCombined() = %q, expected %q",
					parsed.OpenHours[0], tt.expected)
			}
		})
	}
}

// TestFormatOpenHours24HourVenues tests 24-hour venue formatting
func TestFormatOpenHours24HourVenues(t *testing.T) {
	input := []string{
		"Monday: Open 24 hours",
		"Tuesday: 24 hours",
	}

	result, err := FormatOpenHoursFromCombined(input)
	if err != nil {
		t.Errorf("FormatOpenHoursFromCombined() error = %v", err)
		return
	}

	var parsed struct {
		OpenHours []string `json:"openhours"`
	}

	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
		return
	}

	if len(parsed.OpenHours) != 2 {
		t.Errorf("Expected 2 hour entries, got %d", len(parsed.OpenHours))
		return
	}

	for i, hour := range parsed.OpenHours {
		expected := []string{"Mon-00:00-24:00", "Tue-00:00-24:00"}[i]
		if hour != expected {
			t.Errorf("Hour[%d] = %q, expected %q", i, hour, expected)
		}
	}
}

// isValidHourFormat checks if a hour string matches the expected format
// Expected formats: "Day-HH:MM-HH:MM" or "Day-00:00-24:00"
func isValidHourFormat(hour string) bool {
	// Simple validation: should have format like "Mon-11:00-21:00"
	if len(hour) < 11 {
		return false
	}

	// Should contain exactly 2 dashes after the day abbreviation
	parts := splitAfterDayName(hour)
	if len(parts) != 3 {
		return false
	}

	// Day should be 3 letters
	day := parts[0]
	validDays := map[string]bool{
		"Mon": true, "Tue": true, "Wed": true, "Thu": true,
		"Fri": true, "Sat": true, "Sun": true,
	}

	if !validDays[day] {
		return false
	}

	// Times should be in HH:MM format
	for _, timePart := range parts[1:] {
		if len(timePart) != 5 { // HH:MM
			return false
		}
		if timePart[2] != ':' {
			return false
		}
	}

	return true
}

// splitAfterDayName splits "Mon-11:00-21:00" into ["Mon", "11:00", "21:00"]
func splitAfterDayName(hour string) []string {
	if len(hour) < 4 {
		return nil
	}
	day := hour[:3]
	rest := hour[4:] // skip the dash after day
	parts := []string{day}

	// Split remaining by dash
	timeParts := splitTimes(rest)
	parts = append(parts, timeParts...)

	return parts
}

// splitTimes splits "11:00-21:00" into ["11:00", "21:00"]
func splitTimes(s string) []string {
	var result []string
	var current string

	for i, ch := range s {
		if ch == '-' && len(current) == 5 { // HH:MM is 5 chars
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}

		// Add the last time part
		if i == len(s)-1 && current != "" {
			result = append(result, current)
		}
	}

	return result
}

// TestFormatOpenHoursWithUnicodeWhitespace tests handling of Unicode whitespace characters
// Google Places API returns narrow no-break spaces (\u202f) and thin spaces (\u2009)
func TestFormatOpenHoursWithUnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int // number of expected hour entries
	}{
		{
			name: "Real Google Places format with Unicode whitespace",
			input: []string{
				"Monday: 11:00\u202fAM\u2009–\u20099:00\u202fPM",
				"Tuesday: 11:00\u202fAM\u2009–\u20099:00\u202fPM",
				"Wednesday: 11:00\u202fAM\u2009–\u20099:00\u202fPM",
			},
			expected: 3,
		},
		{
			name: "Mixed Unicode and regular spaces",
			input: []string{
				"Monday: 11:00 AM – 9:00 PM",                      // regular spaces
				"Tuesday: 11:00\u202fAM\u2009–\u20099:00\u202fPM", // unicode spaces
			},
			expected: 2,
		},
		{
			name: "Non-breaking space (\\u00A0)",
			input: []string{
				"Monday: 11:00\u00A0AM\u00A0–\u00A09:00\u00A0PM",
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatOpenHoursFromCombined(tt.input)
			if err != nil {
				t.Errorf("FormatOpenHoursFromCombined() error = %v", err)
				return
			}

			if result == "" {
				t.Errorf("FormatOpenHoursFromCombined() returned empty string, expected JSON")
				return
			}

			var parsed struct {
				OpenHours []string `json:"openhours"`
			}

			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Errorf("Failed to parse JSON: %v\nResult: %s", err, result)
				return
			}

			if len(parsed.OpenHours) != tt.expected {
				t.Errorf("Expected %d hour entries, got %d\nInput: %q\nParsed: %v",
					tt.expected, len(parsed.OpenHours), tt.input, parsed.OpenHours)
			}

			// Verify the format is correct
			for i, hour := range parsed.OpenHours {
				if !isValidHourFormat(hour) {
					t.Errorf("Hour[%d] = %q has invalid format", i, hour)
				}
			}
		})
	}
}

// TestConvertTo24Hour tests the time conversion helper function
func TestConvertTo24Hour(t *testing.T) {
	tests := []struct {
		hour     string
		minute   string
		period   string
		expected string
	}{
		{"12", "00", "AM", "00:00"},
		{"12", "00", "PM", "12:00"},
		{"1", "00", "AM", "01:00"},
		{"1", "00", "PM", "13:00"},
		{"11", "59", "AM", "11:59"},
		{"11", "59", "PM", "23:59"},
		{"7", "30", "AM", "07:30"},
		{"10", "30", "PM", "22:30"},
	}

	for _, tt := range tests {
		t.Run(tt.hour+":"+tt.minute+" "+tt.period, func(t *testing.T) {
			result := convertTo24Hour(tt.hour, tt.minute, tt.period)
			if result != tt.expected {
				t.Errorf("convertTo24Hour(%s, %s, %s) = %q, expected %q",
					tt.hour, tt.minute, tt.period, result, tt.expected)
			}
		})
	}
}
