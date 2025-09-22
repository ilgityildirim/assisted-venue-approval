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
