package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

// adminTemplates holds the parsed templates for the admin UI.
var adminTemplates *template.Template

// basePath holds the base path for URLs in templates
var basePath = "/"

// funcMap provides template helper functions used across templates.
var funcMap = template.FuncMap{
	"add": func(a, b interface{}) interface{} {
		switch a := a.(type) {
		case int:
			switch b := b.(type) {
			case int:
				return a + b
			case float64:
				return float64(a) + b
			}
		case float64:
			switch b := b.(type) {
			case int:
				return a + float64(b)
			case float64:
				return a + b
			}
		}
		return 0
	},
	"mul": func(a, b interface{}) interface{} {
		switch a := a.(type) {
		case int:
			switch b := b.(type) {
			case int:
				return a * b
			case float64:
				return float64(a) * b
			}
		case float64:
			switch b := b.(type) {
			case int:
				return a * float64(b)
			case float64:
				return a * b
			}
		}
		return 0
	},
	"div": func(a, b interface{}) interface{} {
		switch a := a.(type) {
		case int:
			switch b := b.(type) {
			case int:
				if b == 0 {
					return float64(0)
				}
				return float64(a) / float64(b)
			case float64:
				if b == 0 {
					return float64(0)
				}
				return float64(a) / b
			}
		case float64:
			switch b := b.(type) {
			case int:
				if b == 0 {
					return float64(0)
				}
				return a / float64(b)
			case float64:
				if b == 0 {
					return float64(0)
				}
				return a / b
			}
		}
		return 0
	},
	"seq": func(start, end int) []int {
		var s []int
		for i := start; i <= end; i++ {
			s = append(s, i)
		}
		return s
	},
	"intVal": func(i interface{}, def int) int {
		switch v := i.(type) {
		case *int:
			if v == nil {
				return def
			}
			return *v
		case int:
			return v
		case int64:
			return int(v)
		case *int64:
			if v == nil {
				return def
			}
			return int(*v)
		default:
			return def
		}
	},
	"fmtFloat": func(f *float64) string {
		if f == nil {
			return ""
		}
		return fmt.Sprintf("%.6f", *f)
	},
	"basePath": func() string {
		return basePath
	},
	"parseOpenHoursJSON": func(input *string) map[string]interface{} {
		if input == nil || *input == "" {
			return nil
		}

		var parsed struct {
			OpenHours []string `json:"openhours"`
			Note      string   `json:"note"`
		}

		if err := json.Unmarshal([]byte(*input), &parsed); err != nil {
			return nil // fallback to raw display
		}

		// Convert "Mon-09:00-17:00" to "Monday: 9:00 AM - 5:00 PM"
		formatted := make([]string, len(parsed.OpenHours))
		for i, h := range parsed.OpenHours {
			formatted[i] = formatHourEntry(h)
		}

		return map[string]interface{}{
			"Hours": formatted,
			"Note":  parsed.Note,
		}
	},
}

// formatHourEntry converts "Mon-09:00-17:00" to "Monday: 9:00 AM - 5:00 PM"
func formatHourEntry(entry string) string {
	parts := strings.Split(entry, "-")
	if len(parts) != 3 {
		return entry // return as-is if format is unexpected
	}

	day := expandDay(parts[0])
	start := formatTime(parts[1])
	end := formatTime(parts[2])

	return fmt.Sprintf("%s: %s - %s", day, start, end)
}

// expandDay converts day abbreviation to full name
func expandDay(abbr string) string {
	days := map[string]string{
		"Mon": "Monday",
		"Tue": "Tuesday",
		"Wed": "Wednesday",
		"Thu": "Thursday",
		"Fri": "Friday",
		"Sat": "Saturday",
		"Sun": "Sunday",
	}
	if full, ok := days[abbr]; ok {
		return full
	}
	return abbr
}

// formatTime converts 24-hour time to 12-hour with AM/PM
func formatTime(time24 string) string {
	parts := strings.Split(time24, ":")
	if len(parts) != 2 {
		return time24
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time24
	}

	minute := parts[1]
	period := "AM"

	if hour == 0 {
		hour = 12
	} else if hour == 12 {
		period = "PM"
	} else if hour > 12 {
		hour -= 12
		period = "PM"
	}

	return fmt.Sprintf("%d:%s %s", hour, minute, period)
}

// LoadTemplates parses all admin templates from the provided filesystem. It should be called at application startup.
func LoadTemplates(fsys fs.FS) error {
	t, err := template.New("").Funcs(funcMap).ParseFS(fsys, "*.tmpl")
	if err != nil {
		return err
	}
	adminTemplates = t
	return nil
}

// SetBasePath sets the base path for URLs in templates.
func SetBasePath(path string) {
	basePath = path
}

// ExecuteTemplate renders a named template to the ResponseWriter.
func ExecuteTemplate(w http.ResponseWriter, name string, data interface{}) error {
	if adminTemplates == nil {
		return fmt.Errorf("templates not loaded: call admin.LoadTemplates at startup")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return adminTemplates.ExecuteTemplate(w, name, data)
}

// RenderUnauthorized renders the unauthorized access page
func RenderUnauthorized(w http.ResponseWriter, ip string) {
	data := struct {
		IP string
	}{
		IP: ip,
	}
	w.WriteHeader(http.StatusForbidden)
	if err := ExecuteTemplate(w, "unauthorized.tmpl", data); err != nil {
		http.Error(w, "Unauthorized", http.StatusForbidden)
	}
}
