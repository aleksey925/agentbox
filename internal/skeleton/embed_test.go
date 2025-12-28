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
		"docker-compose.agentbox.local.yml",
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

func TestOverwriteFiles(t *testing.T) {
	// act
	files := OverwriteFiles()

	// assert
	expected := []string{
		"Dockerfile.agentbox",
		"docker-compose.agentbox.yml",
	}

	if len(files) != len(expected) {
		t.Fatalf("len(OverwriteFiles()) = %d, want %d", len(files), len(expected))
	}

	for i, f := range files {
		if f != expected[i] {
			t.Errorf("OverwriteFiles()[%d] = %s, want %s", i, f, expected[i])
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

	for _, name := range OverwriteFiles() {
		path := filepath.Join(tmpDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("file %s not copied", name)
		}
	}
}

func TestCopyUserFilesIfMissing__creates_file(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// act
	created, err := CopyUserFilesIfMissing(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("CopyUserFilesIfMissing error: %v", err)
	}

	expected := []string{"docker-compose.agentbox.local.yml"}
	if len(created) != len(expected) {
		t.Fatalf("created = %v, want %v", created, expected)
	}

	for _, name := range expected {
		path := filepath.Join(tmpDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("file %s not created", name)
		}
	}
}

func TestCopyUserFilesIfMissing__skips_existing(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "docker-compose.agentbox.local.yml")
	customContent := []byte("# custom user content")
	if err := os.WriteFile(existingFile, customContent, 0644); err != nil {
		t.Fatal(err)
	}

	// act
	created, err := CopyUserFilesIfMissing(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("CopyUserFilesIfMissing error: %v", err)
	}

	if len(created) != 0 {
		t.Errorf("created = %v, want empty", created)
	}

	content, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(customContent) {
		t.Errorf("file was overwritten, content = %s", content)
	}
}
