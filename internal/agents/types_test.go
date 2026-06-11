package agents

import (
	"runtime"
	"testing"
)

func TestDetectArch(t *testing.T) {
	// act
	arch, err := DetectArch()

	// assert
	switch runtime.GOARCH {
	case "amd64":
		if err != nil {
			t.Fatalf("DetectArch() error = %v", err)
		}
		if arch != "x64" {
			t.Errorf("DetectArch() = %s, want x64", arch)
		}
	case "arm64":
		if err != nil {
			t.Fatalf("DetectArch() error = %v", err)
		}
		if arch != "arm64" {
			t.Errorf("DetectArch() = %s, want arm64", arch)
		}
	default:
		if err == nil {
			t.Errorf("DetectArch() should return error for unsupported arch %s", runtime.GOARCH)
		}
	}
}

func TestAllAgentNames(t *testing.T) {
	// act
	names := AllAgentNames()

	// assert
	expected := []string{"claude", "copilot", "codex", "cursor", "opencode", "ralphex"}
	if len(names) != len(expected) {
		t.Fatalf("len(AllAgentNames()) = %d, want %d", len(names), len(expected))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("AllAgentNames()[%d] = %s, want %s", i, name, expected[i])
		}
	}
}

func TestAgentConfigDirs__covers_all_agents(t *testing.T) {
	// act & assert
	for _, name := range AllAgentNames() {
		if _, ok := agentConfigDirs[name]; !ok {
			t.Errorf("agent %q missing from agentConfigDirs map", name)
		}
	}
}

func TestSuggestedFlags__covers_all_agents(t *testing.T) {
	// act
	suggested := SuggestedFlags()

	// assert
	for _, name := range AllAgentNames() {
		if _, ok := suggested[name]; !ok {
			t.Errorf("agent %q missing from SuggestedFlags map", name)
		}
	}
}
