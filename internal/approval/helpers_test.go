package approval

import (
	"encoding/json"
	"testing"
)

func TestFormatOpenHoursFromCombined(t *testing.T) {
	tests := []struct {
		name           string
		input          []string
		expectedHours  int
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
				t.Fatalf("FormatOpenHoursFromCombined() error = %v, expected nil", err)
			}

			if !tt.shouldHaveJSON {
				if result != "" {
					t.Fatalf("FormatOpenHoursFromCombined() = %q, expected empty string for %s", result, tt.description)
				}
				return
			}

			if result == "" {
				t.Fatalf("FormatOpenHoursFromCombined() returned empty string, expected JSON for %s", tt.description)
			}

			var parsed struct {
				OpenHours []string `json:"openhours"`
				Note      string   `json:"note"`
			}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Fatalf("FormatOpenHoursFromCombined() returned invalid JSON: %v\nJSON: %s", err, result)
			}

			if len(parsed.OpenHours) != tt.expectedHours {
				t.Fatalf("FormatOpenHoursFromCombined() parsed %d hours, expected %d\nJSON: %s\nInput: %v",
					len(parsed.OpenHours), tt.expectedHours, result, tt.input)
			}

			for i, hour := range parsed.OpenHours {
				if !isValidHourFormat(hour) {
					t.Fatalf("FormatOpenHoursFromCombined() hour[%d] = %q has invalid format (expected Day-HH:MM-HH:MM)",
						i, hour)
				}
			}

			if parsed.Note != "" {
				t.Fatalf("FormatOpenHoursFromCombined() note = %q, expected empty string", parsed.Note)
			}
		})
	}
}

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
				t.Fatalf("FormatOpenHoursFromCombined() error = %v", err)
			}

			var parsed struct {
				OpenHours []string `json:"openhours"`
			}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			if len(parsed.OpenHours) != 1 {
				t.Fatalf("Expected 1 hour entry, got %d", len(parsed.OpenHours))
			}

			if parsed.OpenHours[0] != tt.expected {
				t.Fatalf("FormatOpenHoursFromCombined() = %q, expected %q",
					parsed.OpenHours[0], tt.expected)
			}
		})
	}
}

func TestFormatOpenHours24HourVenues(t *testing.T) {
	input := []string{
		"Monday: Open 24 hours",
		"Tuesday: 24 hours",
	}

	result, err := FormatOpenHoursFromCombined(input)
	if err != nil {
		t.Fatalf("FormatOpenHoursFromCombined() error = %v", err)
	}

	var parsed struct {
		OpenHours []string `json:"openhours"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(parsed.OpenHours) != 2 {
		t.Fatalf("Expected 2 hour entries, got %d", len(parsed.OpenHours))
	}

	expected := []string{"Mon-00:00-24:00", "Tue-00:00-24:00"}
	for i, hour := range parsed.OpenHours {
		if hour != expected[i] {
			t.Fatalf("Hour[%d] = %q, expected %q", i, hour, expected[i])
		}
	}
}

func TestFormatOpenHoursWithUnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
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
				"Monday: 11:00 AM – 9:00 PM",
				"Tuesday: 11:00\u202fAM\u2009–\u20099:00\u202fPM",
			},
			expected: 2,
		},
		{
			name: "Non-breaking space",
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
				t.Fatalf("FormatOpenHoursFromCombined() error = %v", err)
			}
			if result == "" {
				t.Fatalf("FormatOpenHoursFromCombined() returned empty string, expected JSON")
			}

			var parsed struct {
				OpenHours []string `json:"openhours"`
			}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Fatalf("Failed to parse JSON: %v\nResult: %s", err, result)
			}

			if len(parsed.OpenHours) != tt.expected {
				t.Fatalf("Expected %d hour entries, got %d\nInput: %q\nParsed: %v",
					tt.expected, len(parsed.OpenHours), tt.input, parsed.OpenHours)
			}

			for i, hour := range parsed.OpenHours {
				if !isValidHourFormat(hour) {
					t.Fatalf("Hour[%d] = %q has invalid format", i, hour)
				}
			}
		})
	}
}

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
			if got := convertTo24Hour(tt.hour, tt.minute, tt.period); got != tt.expected {
				t.Fatalf("convertTo24Hour(%s, %s, %s) = %q, expected %q",
					tt.hour, tt.minute, tt.period, got, tt.expected)
			}
		})
	}
}

func isValidHourFormat(hour string) bool {
	if len(hour) < 11 {
		return false
	}

	parts := splitAfterDayName(hour)
	if len(parts) != 3 {
		return false
	}

	validDays := map[string]bool{
		"Mon": true, "Tue": true, "Wed": true, "Thu": true,
		"Fri": true, "Sat": true, "Sun": true,
	}
	if !validDays[parts[0]] {
		return false
	}

	for _, timePart := range parts[1:] {
		if len(timePart) != 5 || timePart[2] != ':' {
			return false
		}
	}
	return true
}

func splitAfterDayName(hour string) []string {
	if len(hour) < 4 {
		return nil
	}
	day := hour[:3]
	rest := hour[4:]
	return append([]string{day}, splitTimes(rest)...)
}

func splitTimes(s string) []string {
	var (
		result  []string
		current string
	)

	for i, ch := range s {
		if ch == '-' && len(current) == 5 {
			result = append(result, current)
			current = ""
			continue
		}
		current += string(ch)
		if i == len(s)-1 && current != "" {
			result = append(result, current)
		}
	}
	return result
}
