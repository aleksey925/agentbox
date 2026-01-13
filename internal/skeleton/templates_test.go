package skeleton

import (
	"testing"
)

func TestParseTemplateName(t *testing.T) {
	tests := []struct {
		filename        string
		expectedName    string
		expectedVersion int
	}{
		{"core.v2.yml", "core", 2},
		{"python.v3.yml", "python", 3},
		{"go.v1.yml", "go", 1},
		{"python.yml", "python", 0},
		{"local.yml", "local", -1},
		{"my-custom.yml", "my-custom", 0},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			// act
			name, version := ParseTemplateName(tt.filename)

			// assert
			if name != tt.expectedName {
				t.Errorf("name = %s, want %s", name, tt.expectedName)
			}
			if version != tt.expectedVersion {
				t.Errorf("version = %d, want %d", version, tt.expectedVersion)
			}
		})
	}
}

func TestFormatTemplateFilename(t *testing.T) {
	tests := []struct {
		name     string
		version  int
		ext      string
		expected string
	}{
		{"core", 2, ".yml", "core.v2.yml"},
		{"python", 3, ".yml", "python.v3.yml"},
		{"local", 0, ".yml", "local.yml"},
		{"Dockerfile", 1, ".agentbox", "Dockerfile.v1.agentbox"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			// act
			result := FormatTemplateFilename(tt.name, tt.version, tt.ext)

			// assert
			if result != tt.expected {
				t.Errorf("result = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGetEmbeddedComposeTemplates(t *testing.T) {
	// act
	templates, err := GetEmbeddedComposeTemplates()

	// assert
	if err != nil {
		t.Fatalf("GetEmbeddedComposeTemplates error: %v", err)
	}

	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	// check that core template exists
	var hasCoreTemplate bool
	for _, tmpl := range templates {
		if tmpl.Name == "core" {
			hasCoreTemplate = true
			if tmpl.Version == 0 {
				t.Error("core template should have version > 0")
			}
			if len(tmpl.Content) == 0 {
				t.Error("core template content is empty")
			}
		}
	}
	if !hasCoreTemplate {
		t.Error("core template not found")
	}
}

func TestGetEmbeddedDockerfile(t *testing.T) {
	// act
	dockerfile, err := GetEmbeddedDockerfile()

	// assert
	if err != nil {
		t.Fatalf("GetEmbeddedDockerfile error: %v", err)
	}

	if len(dockerfile.Content) == 0 {
		t.Error("Dockerfile content is empty")
	}

	if dockerfile.Version == 0 {
		t.Error("Dockerfile should have version > 0")
	}
}

func TestGetEmbeddedLocalYml(t *testing.T) {
	// act
	content, err := GetEmbeddedLocalYml()

	// assert
	if err != nil {
		t.Fatalf("GetEmbeddedLocalYml error: %v", err)
	}

	if len(content) == 0 {
		t.Error("local.yml content is empty")
	}
}

func TestSortComposeFiles(t *testing.T) {
	// arrange
	files := []string{
		"/path/python.v1.yml",
		"/path/local.yml",
		"/path/go.v1.yml",
		"/path/core.v1.yml",
	}

	// act
	SortComposeFiles(files)

	// assert
	expected := []string{
		"/path/core.v1.yml",
		"/path/go.v1.yml",
		"/path/python.v1.yml",
		"/path/local.yml",
	}

	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %s, want %s", i, f, expected[i])
		}
	}
}
