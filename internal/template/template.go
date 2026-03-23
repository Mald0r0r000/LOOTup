package template

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/sftp"
)

// Template defines a project folder structure
type Template struct {
	Name string
	Dirs []string
}

// builtinTemplates holds the predefined project structures
var builtinTemplates = map[string]*Template{
	"film": {
		Name: "film",
		Dirs: []string{
			"FILM/Rushes",
			"FILM/Audio",
			"EDIT/Output",
			"EDIT/Exports",
			"DOCS",
		},
	},
	"photo": {
		Name: "photo",
		Dirs: []string{
			"RAW",
			"EDIT",
			"EXPORT",
			"DOCS",
		},
	},
	"generic": {
		Name: "generic",
		Dirs: []string{
			"SOURCE",
			"OUTPUT",
			"DOCS",
		},
	},
}

// Get returns a template by name, or an error if not found
func Get(name string) (*Template, error) {
	t, ok := builtinTemplates[name]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s (available: %s)", name, ListNames())
	}
	return t, nil
}

// ListNames returns a comma-separated list of available template names
func ListNames() string {
	names := ""
	for k := range builtinTemplates {
		if names != "" {
			names += ", "
		}
		names += k
	}
	return names
}

// List returns all available templates
func List() []*Template {
	var result []*Template
	for _, t := range builtinTemplates {
		result = append(result, t)
	}
	return result
}

// Apply creates the folder structure on the remote host via SFTP
func (t *Template) Apply(client *sftp.Client, basePath string) error {
	for _, dir := range t.Dirs {
		fullPath := filepath.Join(basePath, dir)
		if err := client.MkdirAll(fullPath); err != nil {
			return fmt.Errorf("create remote dir %s: %w", fullPath, err)
		}
	}
	return nil
}
