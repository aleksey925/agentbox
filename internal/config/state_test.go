package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadState__file_not_exists__returns_empty_state(t *testing.T) {
	// arrange
	path := "/nonexistent/path/state.json"

	// act
	state, err := LoadState(path)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state.Agents == nil {
		t.Error("Agents should not be nil")
	}

	if len(state.Agents) != 0 {
		t.Errorf("Agents length = %d, want 0", len(state.Agents))
	}
}

func TestState_SetAgent(t *testing.T) {
	// arrange
	state := &State{
		Agents: make(map[string]*AgentState),
	}

	// act
	state.SetAgent("claude", "2.0.76", "musl")

	// assert
	agent, ok := state.Agents["claude"]
	if !ok {
		t.Fatal("claude agent not found")
	}

	if agent.Version != "2.0.76" {
		t.Errorf("Version = %s, want 2.0.76", agent.Version)
	}

	if agent.Variant != "musl" {
		t.Errorf("Variant = %s, want musl", agent.Variant)
	}

	if agent.InstalledAt.IsZero() {
		t.Error("InstalledAt should not be zero")
	}
}

func TestState_GetAgentVersion(t *testing.T) {
	// arrange
	state := &State{
		Agents: map[string]*AgentState{
			"claude": {Version: "2.0.76"},
		},
	}

	// act & assert
	if v := state.GetAgentVersion("claude"); v != "2.0.76" {
		t.Errorf("GetAgentVersion(claude) = %s, want 2.0.76", v)
	}

	if v := state.GetAgentVersion("unknown"); v != "" {
		t.Errorf("GetAgentVersion(unknown) = %s, want empty", v)
	}
}

func TestState_SaveAndLoad(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	state := &State{
		Arch:   "arm64",
		Agents: make(map[string]*AgentState),
	}
	state.SetAgent("claude", "2.0.76", "musl")

	// act
	if err := state.Save(path); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadState(path)

	// assert
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}

	if loaded.Arch != "arm64" {
		t.Errorf("Arch = %s, want arm64", loaded.Arch)
	}

	if v := loaded.GetAgentVersion("claude"); v != "2.0.76" {
		t.Errorf("claude version = %s, want 2.0.76", v)
	}
}

func TestLoadState__invalid_json__returns_error(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// act
	_, err := LoadState(path)

	// assert
	if err == nil {
		t.Error("expected error for invalid json")
	}
}
