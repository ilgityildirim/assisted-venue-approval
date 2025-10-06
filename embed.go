package main

import (
	"embed"
	"io/fs"
)

//go:embed web/templates/*.tmpl
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

//go:embed config/*.md
var configFS embed.FS

// Templates returns a filesystem rooted at web/templates within the embedded FS.
func Templates() fs.FS {
	if sub, err := fs.Sub(templatesFS, "web/templates"); err == nil {
		return sub
	}
	return templatesFS
}

// Static returns a filesystem rooted at web/static within the embedded FS.
func Static() fs.FS {
	if sub, err := fs.Sub(staticFS, "web/static"); err == nil {
		return sub
	}
	return staticFS
}

// ConfigFiles returns a filesystem rooted at config within the embedded FS.
func ConfigFiles() fs.FS {
	if sub, err := fs.Sub(configFS, "config"); err == nil {
		return sub
	}
	return configFS
}
