package template

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

// Template defines a project folder structure
type Template struct {
	Name string
	Dirs []string // Use {ProjectName} and {SessionName} as placeholders
}

// builtinTemplates holds the predefined project structures
var builtinTemplates = map[string]*Template{
	"film": {
		Name: "film",
		Dirs: []string{
			// Project root (created once per project)
			"{ProjectName}/EDIT/LUTS",
			"{ProjectName}/EDIT/DRP",
			"{ProjectName}/EDIT/OUTPUTS/V01/RATIO",
			"{ProjectName}/DOCS",
			// Session dirs (created once per shooting day)
			"{ProjectName}/FILM-DATAS/{SessionName}/RUSHES",
			"{ProjectName}/FILM-DATAS/{SessionName}/SOUND",
			"{ProjectName}/FILM-DATAS/{SessionName}/ASSETS",
			"{ProjectName}/PROXIES/{SessionName}_OFF/RUSHES",
			"{ProjectName}/PROXIES/{SessionName}_OFF/DRP",
		},
	},
	"photo": {
		Name: "photo",
		Dirs: []string{
			"{ProjectName}/RAW/{SessionName}",
			"{ProjectName}/EDIT",
			"{ProjectName}/EXPORT",
			"{ProjectName}/DOCS",
		},
	},
	"generic": {
		Name: "generic",
		Dirs: []string{
			"{ProjectName}/SOURCE/{SessionName}",
			"{ProjectName}/OUTPUT",
			"{ProjectName}/DOCS",
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

// Apply creates the folder structure on the remote host via SFTP.
// basePath is the root destination (e.g. /data/projects).
// projectName and sessionName are substituted into directory paths.
func (t *Template) Apply(client *sftp.Client, basePath, projectName, sessionName string) error {
	replacer := strings.NewReplacer(
		"{ProjectName}", projectName,
		"{SessionName}", sessionName,
	)

	for _, dir := range t.Dirs {
		resolved := replacer.Replace(dir)
		fullPath := filepath.Join(basePath, resolved)
		if err := client.MkdirAll(fullPath); err != nil {
			return fmt.Errorf("create remote dir %s: %w", fullPath, err)
		}
	}
	return nil
}
