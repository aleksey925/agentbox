package skeleton

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFiles(t *testing.T) {
	// act
	files := Files()

	// assert
	expected := []string{
		"Dockerfile.agentbox",
		"docker-compose.agentbox.yml",
	}

	if len(files) != len(expected) {
		t.Fatalf("len(Files()) = %d, want %d", len(files), len(expected))
	}

	for i, f := range files {
		if f != expected[i] {
			t.Errorf("Files()[%d] = %s, want %s", i, f, expected[i])
		}
	}
}

func TestExtract(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// act
	err := Extract(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}

	for _, name := range Files() {
		path := filepath.Join(tmpDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("file %s not extracted", name)
		}
	}
}

func TestIsExtracted__all_files_exist__returns_true(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	_ = Extract(tmpDir)

	// act
	result := IsExtracted(tmpDir)

	// assert
	if !result {
		t.Error("IsExtracted should return true")
	}
}

func TestIsExtracted__missing_file__returns_false(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// act
	result := IsExtracted(tmpDir)

	// assert
	if result {
		t.Error("IsExtracted should return false")
	}
}
