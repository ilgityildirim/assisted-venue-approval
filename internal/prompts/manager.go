package prompts

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	errs "assisted-venue-approval/pkg/errors"
)

// Manager loads, compiles and renders prompt templates.
// Templates are compiled once at startup for performance.
// Simple and extensible: variants can be added as new files (e.g., unified_user@v2.tmpl).
type Manager struct {
	mu   sync.RWMutex
	tpls map[string]*template.Template
}

// NewManager parses all embedded templates.
func NewManager() (*Manager, error) {
	m := &Manager{tpls: make(map[string]*template.Template)}

	// Walk embedded FS and parse .tmpl files
	err := fs.WalkDir(FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".txt.tmpl") {
			return nil
		}
		b, rerr := fs.ReadFile(FS(), p)
		if rerr != nil {
			return fmt.Errorf("read template %s: %w", p, rerr)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".txt.tmpl")
		tpl, perr := template.New(name).Parse(string(b))
		if perr != nil {
			return fmt.Errorf("parse template %s: %w", p, perr)
		}
		m.tpls[name] = tpl
		return nil
	})
	if err != nil {
		return nil, errs.NewBiz("prompts.NewManager", "failed to load prompts", err)
	}
	return m, nil
}

// Render executes a named template with data and returns the result string.
func (m *Manager) Render(name string, data any) (string, error) {
	m.mu.RLock()
	tpl, ok := m.tpls[name]
	m.mu.RUnlock()
	if !ok {
		return "", errs.NewValidation("prompts.Render", fmt.Sprintf("prompt template not found: %s", name), nil)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		return "", errs.NewBiz("prompts.Render", fmt.Sprintf("execute template %s", name), err)
	}
	return sb.String(), nil
}
