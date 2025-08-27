package admin

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

// adminTemplates holds the parsed templates for the admin UI.
var adminTemplates *template.Template

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
	// classify type based on entrytype
	"hcType": func(entrytype int) string {
		if entrytype == 2 {
			return "Store"
		}
		return "Restaurant"
	},
	// vegan label according to entrytype, vegonly, vegan
	"hcVeganLabel": func(entrytype, vegonly, vegan int) string {
		if entrytype == 2 {
			return "—" // stores: exclude vegan/non-veg labels
		}
		if vegonly == 1 && vegan == 1 {
			return "Vegan"
		}
		if vegonly == 1 && vegan == 0 {
			return "Vegetarian"
		}
		// vegonly 0 (or vegan 0/1 with vegonly 0) => veg-options
		return "Vegan Options"
	},
	// category name mapping, depends on entrytype; 0 => false/empty
	"hcCategory": func(entrytype, category int) string {
		if category == 0 {
			return ""
		}
		if entrytype == 1 {
			// restaurants usually uncategorized; ignore
			return ""
		}
		switch category {
		case 1:
			return "Health Store"
		case 2:
			return "Veg Store"
		case 3:
			return "Bakery"
		case 4:
			return "B&B"
		case 5:
			return "Delivery"
		case 6:
			return "Catering"
		case 7:
			return "Organization"
		case 8:
			return "Farmer's Market"
		case 10:
			return "Food Truck"
		case 11:
			return "Market Vendor"
		case 12:
			return "Ice Cream"
		case 13:
			return "Juice Bar"
		case 14:
			return "Professional"
		case 15:
			return "Coffee & Tea"
		case 16:
			return "Spa"
		case 99:
			return "Other"
		default:
			return ""
		}
	},
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

// ExecuteTemplate renders a named template to the ResponseWriter.
func ExecuteTemplate(w http.ResponseWriter, name string, data interface{}) error {
	if adminTemplates == nil {
		return fmt.Errorf("templates not loaded: call admin.LoadTemplates at startup")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return adminTemplates.ExecuteTemplate(w, name, data)
}
