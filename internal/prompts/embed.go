package prompts

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed templates/*.txt.tmpl
var promptsFS embed.FS

// FS returns the embedded filesystem for prompts in this package.
func FS() fs.FS {
	if sub, err := fs.Sub(promptsFS, "templates"); err == nil {
		return sub
	}
	return promptsFS
}

// PathFor returns canonical template path from logical name.
// Supports variants via name like "unified_user@v2" -> unified_user@v2.txt.tmpl
func PathFor(name string) string {
	return fmt.Sprintf("%s.txt.tmpl", name)
}
