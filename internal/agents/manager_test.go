package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestCursorAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewCursorAgent()
	if err != nil {
		t.Fatalf("NewCursorAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "cursor" {
		t.Errorf("Name() = %s, want cursor", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "cursor-agent" {
		t.Errorf("BinaryName() = %s, want cursor-agent", agent.BinaryName())
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

	if agent.BinaryName() != "bundle/gemini.js" {
		t.Errorf("BinaryName() = %s, want bundle/gemini.js", agent.BinaryName())
	}
}

func TestGeminiAgent_FetchLatestVersion(t *testing.T) {
	// arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@google/gemini-cli/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"1.2.3","name":"@google/gemini-cli"}`))
	}))
	defer server.Close()

	prev := npmRegistryBaseURL
	npmRegistryBaseURL = server.URL
	defer func() { npmRegistryBaseURL = prev }()

	// act
	version, err := NewGeminiAgent().FetchLatestVersion(context.Background())

	// assert
	if err != nil {
		t.Fatalf("FetchLatestVersion() error = %v", err)
	}
	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
}

func TestGeminiAgent_FetchLatestVersion__missing_version(t *testing.T) {
	// arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	prev := npmRegistryBaseURL
	npmRegistryBaseURL = server.URL
	defer func() { npmRegistryBaseURL = prev }()

	// act
	_, err := NewGeminiAgent().FetchLatestVersion(context.Background())

	// assert
	if err == nil {
		t.Fatal("expected error for empty version field")
	}
}

func TestGeminiAgent_Download(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "package/", isDir: true},
		{name: "package/LICENSE", content: []byte("MIT"), mode: 0o644},
		{name: "package/bundle/", isDir: true},
		{name: "package/bundle/gemini.js", content: []byte("#!/usr/bin/env node"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/@google/gemini-cli/-/gemini-cli-1.2.3.tgz"
		if r.URL.Path != expected {
			t.Errorf("path = %q, want %q", r.URL.Path, expected)
		}
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	prev := npmRegistryBaseURL
	npmRegistryBaseURL = server.URL
	defer func() { npmRegistryBaseURL = prev }()

	destDir := t.TempDir()

	// act
	err := NewGeminiAgent().Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(destDir, "bundle", "gemini.js"))
	if err != nil {
		t.Fatalf("read bundle/gemini.js: %v", err)
	}
	if string(content) != "#!/usr/bin/env node" {
		t.Errorf("content = %q", content)
	}
	if _, err := os.Stat(filepath.Join(destDir, "LICENSE")); err != nil {
		t.Errorf("LICENSE not extracted: %v", err)
	}
}

func TestOpenCodeAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewOpenCodeAgent()
	if err != nil {
		t.Fatalf("NewOpenCodeAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "opencode" {
		t.Errorf("Name() = %s, want opencode", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "opencode" {
		t.Errorf("BinaryName() = %s, want opencode", agent.BinaryName())
	}
}

func TestRalphexAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewRalphexAgent()
	if err != nil {
		t.Fatalf("NewRalphexAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "ralphex" {
		t.Errorf("Name() = %s, want ralphex", agent.Name())
	}

	if agent.Variant() != "glibc" {
		t.Errorf("Variant() = %s, want glibc", agent.Variant())
	}

	if agent.BinaryName() != "ralphex" {
		t.Errorf("BinaryName() = %s, want ralphex", agent.BinaryName())
	}
}

func TestRalphexAgent_goArch(t *testing.T) {
	// arrange
	agent := &RalphexAgent{arch: "x64"}

	// act & assert
	if agent.goArch() != "amd64" {
		t.Errorf("goArch() = %s, want amd64", agent.goArch())
	}

	agent.arch = "arm64"
	if agent.goArch() != "arm64" {
		t.Errorf("goArch() = %s, want arm64", agent.goArch())
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

	// act
	manager, err := NewManager(paths)

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
}

func TestManager_GetAgent(t *testing.T) {
	// arrange
	manager, err := NewManager(&config.Paths{})
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
	manager, err := NewManager(&config.Paths{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// act
	agents := manager.AllAgents()

	// assert
	expectedNames := []string{"claude", "copilot", "codex", "cursor", "gemini", "opencode", "ralphex"}
	if len(agents) != len(expectedNames) {
		t.Fatalf("AllAgents() returned %d agents, want %d", len(agents), len(expectedNames))
	}

	for i, name := range expectedNames {
		if agents[i].Name() != name {
			t.Errorf("agents[%d].Name() = %s, want %s", i, agents[i].Name(), name)
		}
	}
}
