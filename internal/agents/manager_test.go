package agents

import (
	"testing"

	"github.com/aleksey925/agentbox/internal/config"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
		{"2.0.76", "2.0.67", 1},
		{"0.0.372", "0.0.371", 1},
		{"10.0.0", "9.0.0", 1},
		{"1.10.0", "1.9.0", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			// act
			result := compareVersions(tt.a, tt.b)

			// assert
			if result != tt.expected {
				t.Errorf("compareVersions(%s, %s) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestClaudeAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewClaudeAgent()
	if err != nil {
		t.Fatalf("NewClaudeAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "claude" {
		t.Errorf("Name() = %s, want claude", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "claude" {
		t.Errorf("BinaryName() = %s, want claude", agent.BinaryName())
	}
}

func TestCopilotAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewCopilotAgent()
	if err != nil {
		t.Fatalf("NewCopilotAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "copilot" {
		t.Errorf("Name() = %s, want copilot", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "copilot" {
		t.Errorf("BinaryName() = %s, want copilot", agent.BinaryName())
	}
}

func TestCodexAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewCodexAgent()
	if err != nil {
		t.Fatalf("NewCodexAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "codex" {
		t.Errorf("Name() = %s, want codex", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "codex" {
		t.Errorf("BinaryName() = %s, want codex", agent.BinaryName())
	}
}

func TestGeminiAgent_Name(t *testing.T) {
	// arrange
	agent := NewGeminiAgent()

	// act & assert
	if agent.Name() != "gemini" {
		t.Errorf("Name() = %s, want gemini", agent.Name())
	}

	if agent.Variant() != "js" {
		t.Errorf("Variant() = %s, want js", agent.Variant())
	}

	if agent.BinaryName() != "gemini.js" {
		t.Errorf("BinaryName() = %s, want gemini.js", agent.BinaryName())
	}
}

func TestCodexAgent_rustArch(t *testing.T) {
	// arrange
	agent := &CodexAgent{arch: "arm64"}

	// act & assert
	if agent.rustArch() != "aarch64" {
		t.Errorf("rustArch() = %s, want aarch64", agent.rustArch())
	}

	agent.arch = "x64"
	if agent.rustArch() != "x86_64" {
		t.Errorf("rustArch() = %s, want x86_64", agent.rustArch())
	}
}

func TestNewManager(t *testing.T) {
	// arrange
	paths := &config.Paths{BinDir: "/tmp/test"}
	state := &config.State{}

	// act
	manager, err := NewManager(paths, state)

	// assert
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if manager == nil {
		t.Fatal("NewManager returned nil")
	}
	if manager.paths != paths {
		t.Error("paths not set correctly")
	}
	if manager.state != state {
		t.Error("state not set correctly")
	}
}

func TestManager_GetAgent(t *testing.T) {
	// arrange
	manager, err := NewManager(&config.Paths{}, &config.State{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// act & assert
	agent, ok := manager.GetAgent("claude")
	if !ok {
		t.Fatal("GetAgent(claude) returned false")
	}
	if agent.Name() != "claude" {
		t.Errorf("agent.Name() = %s, want claude", agent.Name())
	}

	_, ok = manager.GetAgent("unknown")
	if ok {
		t.Error("GetAgent(unknown) should return false")
	}
}

func TestManager_AllAgents(t *testing.T) {
	// arrange
	manager, err := NewManager(&config.Paths{}, &config.State{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// act
	agents := manager.AllAgents()

	// assert
	expectedNames := []string{"claude", "copilot", "codex", "gemini"}
	if len(agents) != len(expectedNames) {
		t.Fatalf("AllAgents() returned %d agents, want %d", len(agents), len(expectedNames))
	}

	for i, name := range expectedNames {
		if agents[i].Name() != name {
			t.Errorf("agents[%d].Name() = %s, want %s", i, agents[i].Name(), name)
		}
	}
}
