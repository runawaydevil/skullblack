package web

import (
	"html/template"
	"path/filepath"
)

// LoadTemplates parses all HTML templates from the templates directory.
func LoadTemplates(dir string) (*template.Template, error) {
	pattern := filepath.Join(dir, "*.html")
	return template.ParseGlob(pattern)
}
