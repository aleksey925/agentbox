package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPaths(t *testing.T) {
	// arrange
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// act
	paths, err := NewPaths()

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if paths.HomeDir != homeDir {
		t.Errorf("HomeDir = %s, want %s", paths.HomeDir, homeDir)
	}

	expectedAgentboxDir := filepath.Join(homeDir, ".agentbox")
	if paths.AgentboxDir != expectedAgentboxDir {
		t.Errorf("AgentboxDir = %s, want %s", paths.AgentboxDir, expectedAgentboxDir)
	}

	expectedBinDir := filepath.Join(expectedAgentboxDir, "bin")
	if paths.BinDir != expectedBinDir {
		t.Errorf("BinDir = %s, want %s", paths.BinDir, expectedBinDir)
	}
}

func TestPaths_AgentDir(t *testing.T) {
	// arrange
	paths := &Paths{
		BinDir: "/home/user/.agentbox/bin",
	}

	// act
	result := paths.AgentDir("claude")

	// assert
	expected := "/home/user/.agentbox/bin/claude"
	if result != expected {
		t.Errorf("AgentDir = %s, want %s", result, expected)
	}
}

func TestPaths_AgentVersionDir(t *testing.T) {
	// arrange
	paths := &Paths{
		BinDir: "/home/user/.agentbox/bin",
	}

	// act
	result := paths.AgentVersionDir("claude", "2.0.76")

	// assert
	expected := "/home/user/.agentbox/bin/claude/2.0.76"
	if result != expected {
		t.Errorf("AgentVersionDir = %s, want %s", result, expected)
	}
}

func TestPaths_AgentCurrentLink(t *testing.T) {
	// arrange
	paths := &Paths{
		BinDir: "/home/user/.agentbox/bin",
	}

	// act
	result := paths.AgentCurrentLink("claude")

	// assert
	expected := "/home/user/.agentbox/bin/claude/current"
	if result != expected {
		t.Errorf("AgentCurrentLink = %s, want %s", result, expected)
	}
}

func TestPaths_EnsureDirs(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	paths := &Paths{
		AgentboxDir: filepath.Join(tmpDir, ".agentbox"),
		BinDir:      filepath.Join(tmpDir, ".agentbox", "bin"),
	}

	// act
	err := paths.EnsureDirs()

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDirs := []string{paths.AgentboxDir, paths.BinDir}
	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %s was not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestPaths_EnsureDirs__already_exists(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	paths := &Paths{
		AgentboxDir: filepath.Join(tmpDir, ".agentbox"),
		BinDir:      filepath.Join(tmpDir, ".agentbox", "bin"),
	}

	if err := os.MkdirAll(paths.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	err := paths.EnsureDirs()

	// assert
	if err != nil {
		t.Fatalf("unexpected error when dirs already exist: %v", err)
	}
}
