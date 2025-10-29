package validation

import (
	"fmt"
	"regexp"
	"strings"

	"assisted-venue-approval/internal/drafts"
)

var (
	// pathRegex allows alphanumeric, hyphens, underscores, pipes for URL slugs
	pathRegex = regexp.MustCompile(`^[a-zA-Z0-9_|\-]+$`)
)

// ValidateName validates venue name
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 {
		return fmt.Errorf("name must be at least 2 characters")
	}
	if len(name) > 200 {
		return fmt.Errorf("name must be less than 200 characters")
	}
	return nil
}

// ValidateAddress validates venue address
func ValidateAddress(addr string) error {
	addr = strings.TrimSpace(addr)
	if len(addr) < 5 {
		return fmt.Errorf("address must be at least 5 characters")
	}
	if len(addr) > 500 {
		return fmt.Errorf("address must be less than 500 characters")
	}
	return nil
}

// ValidatePhone validates phone number (flexible international format)
func ValidatePhone(phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil // Optional field
	}
	if len(phone) > 50 {
		return fmt.Errorf("phone must be less than 50 characters")
	}
	// Basic validation: allow digits, spaces, +, -, (, )
	validChars := regexp.MustCompile(`^[0-9+\-() ]+$`)
	if !validChars.MatchString(phone) {
		return fmt.Errorf("phone contains invalid characters")
	}
	return nil
}

// ValidateLatitude validates latitude coordinate
func ValidateLatitude(lat float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	return nil
}

// ValidateLongitude validates longitude coordinate
func ValidateLongitude(lng float64) error {
	if lng < -180 || lng > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	return nil
}

// ValidatePath validates URL slug/path
func ValidatePath(path string) error {
	path = strings.TrimSpace(path)
	if len(path) < 1 {
		return fmt.Errorf("path cannot be empty")
	}
	if len(path) > 255 {
		return fmt.Errorf("path must be less than 255 characters")
	}
	if !pathRegex.MatchString(path) {
		return fmt.Errorf("path can only contain letters, numbers, hyphens, underscores, and pipes")
	}
	return nil
}

// ValidateDescription validates venue description
func ValidateDescription(desc string) error {
	if len(desc) > 5000 {
		return fmt.Errorf("description must be less than 5000 characters")
	}
	return nil
}

// ValidateOpenHours validates open hours array
func ValidateOpenHours(hours []string) error {
	if len(hours) == 0 {
		return nil // Optional field
	}
	if len(hours) > 20 {
		return fmt.Errorf("too many open hours entries (max 20)")
	}
	for _, h := range hours {
		if len(h) > 200 {
			return fmt.Errorf("individual hours entry too long (max 200 chars)")
		}
	}
	return nil
}

// ValidateHoursNote validates hours note/closed days
func ValidateHoursNote(note string) error {
	if len(note) > 500 {
		return fmt.Errorf("hours note must be less than 500 characters")
	}
	return nil
}

// ValidateType validates venue entry type
func ValidateType(t int) error {
	// EntryType: 1=Restaurant, 2=Store, 3=Catering, 4=Organization, 5=Professional
	if t < 1 || t > 5 {
		return fmt.Errorf("invalid venue type (must be 1-5)")
	}
	return nil
}

// ValidateVegan validates vegan status
func ValidateVegan(v int) error {
	// 0=Vegan-friendly, 1=Vegan, 2=Veg-options
	if v < 0 || v > 2 {
		return fmt.Errorf("invalid vegan status (must be 0-2)")
	}
	return nil
}

// ValidateCategory validates venue category
func ValidateCategory(c int) error {
	// 0=Restaurant (generic), 1=Bakery, 2=Ice Cream, 3=Juice Bar, 4=Coffee/Tea
	// Allow 0-10 for future categories
	if c < 0 || c > 10 {
		return fmt.Errorf("invalid category (must be 0-10)")
	}
	return nil
}

// ValidateVenueDraft validates all fields in a draft
// Returns a map of field names to error messages
func ValidateVenueDraft(fields map[string]drafts.DraftField) map[string]string {
	errors := make(map[string]string)

	for fieldName, field := range fields {
		var err error

		switch fieldName {
		case "name":
			if val, ok := field.Value.(string); ok {
				err = ValidateName(val)
			} else {
				err = fmt.Errorf("invalid type for name")
			}

		case "address":
			if val, ok := field.Value.(string); ok {
				err = ValidateAddress(val)
			} else {
				err = fmt.Errorf("invalid type for address")
			}

		case "phone":
			if val, ok := field.Value.(string); ok {
				err = ValidatePhone(val)
			} else {
				err = fmt.Errorf("invalid type for phone")
			}

		case "lat":
			if val, ok := field.Value.(float64); ok {
				err = ValidateLatitude(val)
			} else {
				err = fmt.Errorf("invalid type for latitude")
			}

		case "lng":
			if val, ok := field.Value.(float64); ok {
				err = ValidateLongitude(val)
			} else {
				err = fmt.Errorf("invalid type for longitude")
			}

		case "path":
			if val, ok := field.Value.(string); ok {
				err = ValidatePath(val)
			} else {
				err = fmt.Errorf("invalid type for path")
			}

		case "description":
			if val, ok := field.Value.(string); ok {
				err = ValidateDescription(val)
			} else {
				err = fmt.Errorf("invalid type for description")
			}

		case "hours_note":
			if val, ok := field.Value.(string); ok {
				err = ValidateHoursNote(val)
			} else {
				err = fmt.Errorf("invalid type for hours_note")
			}

		case "type":
			// Handle both int and float64 from JSON
			var intVal int
			switch v := field.Value.(type) {
			case int:
				intVal = v
			case float64:
				intVal = int(v)
			default:
				err = fmt.Errorf("invalid type for venue type")
				break
			}
			if err == nil {
				err = ValidateType(intVal)
			}

		case "vegan":
			var intVal int
			switch v := field.Value.(type) {
			case int:
				intVal = v
			case float64:
				intVal = int(v)
			default:
				err = fmt.Errorf("invalid type for vegan status")
				break
			}
			if err == nil {
				err = ValidateVegan(intVal)
			}

		case "category":
			var intVal int
			switch v := field.Value.(type) {
			case int:
				intVal = v
			case float64:
				intVal = int(v)
			default:
				err = fmt.Errorf("invalid type for category")
				break
			}
			if err == nil {
				err = ValidateCategory(intVal)
			}

		case "open_hours":
			if val, ok := field.Value.([]interface{}); ok {
				hours := make([]string, len(val))
				for i, h := range val {
					if str, ok := h.(string); ok {
						hours[i] = str
					} else {
						err = fmt.Errorf("invalid type in open_hours array")
						break
					}
				}
				if err == nil {
					err = ValidateOpenHours(hours)
				}
			} else {
				err = fmt.Errorf("invalid type for open_hours")
			}
		}

		if err != nil {
			errors[fieldName] = err.Error()
		}
	}

	return errors
}
