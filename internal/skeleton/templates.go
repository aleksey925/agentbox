package skeleton

import (
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/compose/*.yml templates/*.agentbox templates/local.yml
var embeddedTemplates embed.FS

// Template represents a skeleton template file with version information.
type Template struct {
	Name    string // base name without version (e.g., "core", "go", "python")
	Version int    // parsed from filename, 0 if no version, -1 for special files like local.yml
	Content []byte
	// original filename as stored in templates/ (e.g., "core.vN.yml")
	Filename string
}

// versionRegex matches "name.vN.ext" format
var versionRegex = regexp.MustCompile(`^(.+)\.v(\d+)\.(.+)$`)

// ParseTemplateName extracts name and version from a template filename.
//
// Examples:
//
//	"core.v2.yml"    → ("core", 2)
//	"python.v3.yml"  → ("python", 3)
//	"python.yml"     → ("python", 0)    // no version = 0
//	"local.yml"      → ("local", -1)    // special case, excluded from updates
//	"my-custom.yml"  → ("my-custom", 0) // user file, no version
func ParseTemplateName(filename string) (name string, version int) {
	// special case for local.yml
	if filename == "local.yml" {
		return "local", -1
	}

	matches := versionRegex.FindStringSubmatch(filename)
	if matches == nil {
		// no version in filename, extract base name
		ext := filepath.Ext(filename)
		return strings.TrimSuffix(filename, ext), 0
	}

	name = matches[1]
	version, _ = strconv.Atoi(matches[2])
	return name, version
}

// FormatTemplateFilename creates a filename from name, version, and extension.
func FormatTemplateFilename(name string, version int, ext string) string {
	if version <= 0 {
		return name + ext
	}
	return fmt.Sprintf("%s.v%d%s", name, version, ext)
}

// GetEmbeddedComposeTemplates returns all embedded compose templates.
func GetEmbeddedComposeTemplates() ([]Template, error) {
	entries, err := embeddedTemplates.ReadDir("templates/compose")
	if err != nil {
		return nil, fmt.Errorf("read templates/compose: %w", err)
	}

	templates := make([]Template, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		content, err := embeddedTemplates.ReadFile("templates/compose/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", e.Name(), err)
		}

		name, version := ParseTemplateName(e.Name())
		templates = append(templates, Template{
			Name:     name,
			Version:  version,
			Content:  content,
			Filename: e.Name(),
		})
	}

	return templates, nil
}

// GetEmbeddedDockerfile returns the embedded Dockerfile template.
func GetEmbeddedDockerfile() (Template, error) {
	entries, err := embeddedTemplates.ReadDir("templates")
	if err != nil {
		return Template{}, fmt.Errorf("read templates: %w", err)
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".agentbox") {
			content, err := embeddedTemplates.ReadFile("templates/" + e.Name())
			if err != nil {
				return Template{}, fmt.Errorf("read Dockerfile template: %w", err)
			}

			name, version := ParseTemplateName(e.Name())
			return Template{
				Name:     name,
				Version:  version,
				Content:  content,
				Filename: e.Name(),
			}, nil
		}
	}

	return Template{}, errors.New("dockerfile template not found")
}

// GetEmbeddedLocalYml returns the embedded local.yml template.
func GetEmbeddedLocalYml() ([]byte, error) {
	content, err := embeddedTemplates.ReadFile("templates/local.yml")
	if err != nil {
		return nil, fmt.Errorf("read local.yml template: %w", err)
	}
	return content, nil
}

// GetCoreTemplate returns the core compose template.
func GetCoreTemplate() (Template, error) {
	templates, err := GetEmbeddedComposeTemplates()
	if err != nil {
		return Template{}, err
	}

	for _, t := range templates {
		if t.Name == "core" {
			return t, nil
		}
	}

	return Template{}, errors.New("core template not found")
}

// GetPresetTemplate returns a preset-specific compose template.
func GetPresetTemplate(presetTemplate string) (Template, error) {
	templates, err := GetEmbeddedComposeTemplates()
	if err != nil {
		return Template{}, err
	}

	for _, t := range templates {
		if t.Name == presetTemplate {
			return t, nil
		}
	}

	return Template{}, fmt.Errorf("template %s not found", presetTemplate)
}

// SortComposeFiles sorts compose files in the correct order for Docker Compose.
// Order: core first, then alphabetically, local.yml always last.
func SortComposeFiles(files []string) {
	sort.Slice(files, func(i, j int) bool {
		nameI := filepath.Base(files[i])
		nameJ := filepath.Base(files[j])

		iIsCore := strings.HasPrefix(nameI, "core.")
		jIsCore := strings.HasPrefix(nameJ, "core.")

		// core.* always first
		if iIsCore && !jIsCore {
			return true
		}
		if jIsCore && !iIsCore {
			return false
		}
		// local.yml always last
		if nameI == "local.yml" {
			return false
		}
		if nameJ == "local.yml" {
			return true
		}
		// rest alphabetically
		return nameI < nameJ
	})
}
