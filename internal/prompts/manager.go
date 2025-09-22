package prompts

import (
	"fmt"
	"io/fs"
	"log"
	"os"
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

// NewManager loads templates from an optional external directory first, then fills missing ones from embedded templates.
func NewManager(templatesDir string) (*Manager, error) {
	m := &Manager{tpls: make(map[string]*template.Template)}

	// 1) Try external directory (optional)
	if td := strings.TrimSpace(templatesDir); td != "" {
		fi, err := os.Stat(td)
		if err != nil {
			log.Printf("prompts: external templates dir '%s' not accessible: %v (using embedded)", td, err)
		} else if !fi.IsDir() {
			log.Printf("prompts: path '%s' is not a directory (using embedded)", td)
		} else {
			for _, name := range []string{"system", "unified_user"} {
				path := filepath.Join(td, PathFor(name))
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					log.Printf("prompts: external template not loaded '%s': %v (will try embedded)", path, rerr)
					continue
				}
				tpl, perr := template.New(name).Parse(string(b))
				if perr != nil {
					log.Printf("prompts: parse error in external template '%s': %v (will use embedded fallback)", path, perr)
					continue
				}
				m.tpls[name] = tpl
				log.Printf("prompts: loaded external template '%s' from %s", name, path)
			}
		}
	}

	// 2) Embedded templates as fallback (do not override external)
	_ = fs.WalkDir(FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("prompts: walk embedded fs error: %v", err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".txt.tmpl") {
			return nil
		}
		b, rerr := fs.ReadFile(FS(), p)
		if rerr != nil {
			log.Printf("prompts: read embedded template %s failed: %v", p, rerr)
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(p), ".txt.tmpl")
		if _, exists := m.tpls[name]; exists {
			return nil // external already present
		}
		tpl, perr := template.New(name).Parse(string(b))
		if perr != nil {
			log.Printf("prompts: parse embedded template %s failed: %v", p, perr)
			return nil
		}
		m.tpls[name] = tpl
		log.Printf("prompts: loaded embedded template '%s'", name)
		return nil
	})

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
