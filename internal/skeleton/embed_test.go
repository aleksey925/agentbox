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

func TestCopyTo(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// act
	err := CopyTo(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("CopyTo error: %v", err)
	}

	for _, name := range Files() {
		path := filepath.Join(tmpDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("file %s not copied", name)
		}
	}
}
