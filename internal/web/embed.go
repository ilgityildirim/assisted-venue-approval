//go:build ignore

package web

import (
	"embed"
	"io/fs"
)

// Legacy placeholder; do not use. Real embedding is in pkg/webembed.
var templatesFS embed.FS
var staticFS embed.FS

// Templates returns a filesystem rooted at web/templates within the embedded FS.
func Templates() fs.FS {
	// If sub fails, return the raw FS which will error at read time; but this keeps API simple.
	if sub, err := fs.Sub(templatesFS, "../../web/templates"); err == nil {
		return sub
	}
	return templatesFS
}

// Static returns a filesystem rooted at web/static within the embedded FS.
func Static() fs.FS {
	if sub, err := fs.Sub(staticFS, "../../web/static"); err == nil {
		return sub
	}
	return staticFS
}
