package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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

	if agent.BinaryName() != "copilot" {
		t.Errorf("BinaryName() = %s, want copilot", agent.BinaryName())
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

	if agent.BinaryName() != "cursor-agent" {
		t.Errorf("BinaryName() = %s, want cursor-agent", agent.BinaryName())
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

	if agent.BinaryName() != "opencode" {
		t.Errorf("BinaryName() = %s, want opencode", agent.BinaryName())
	}
}

func TestPiAgent_Name(t *testing.T) {
	// arrange
	agent, err := NewPiAgent()
	if err != nil {
		t.Fatalf("NewPiAgent() error = %v", err)
	}

	// act & assert
	if agent.Name() != "pi" {
		t.Errorf("Name() = %s, want pi", agent.Name())
	}

	if agent.BinaryName() != "pi" {
		t.Errorf("BinaryName() = %s, want pi", agent.BinaryName())
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

func TestManager_SwitchVersion__rejects_unsafe_version(t *testing.T) {
	// arrange — ".." resolves to BinDir itself, which exists, so without the
	// version check SwitchVersion would pass os.Stat and succeed; the check is
	// what must reject it.
	manager, err := NewManager(&config.Paths{BinDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// act
	err = manager.SwitchVersion("claude", "..")

	// assert
	if err == nil || !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("SwitchVersion must reject a path-traversing version, got %v", err)
	}
}

func TestManager_hasCurrentInstall(t *testing.T) {
	// arrange
	manager := newTestManager(t)
	plain := manager.agents["claude"]
	installDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(installDir, "claude"), []byte("x"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	preFixCodexDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(preFixCodexDir, "codex"), []byte("x"), 0o755); err != nil {
		t.Fatalf("write pre-fix codex binary: %v", err)
	}

	// act & assert
	if !manager.hasCurrentInstall(plain, installDir) {
		t.Error("an existing directory must count as installed for a single-binary agent")
	}
	if manager.hasCurrentInstall(plain, filepath.Join(installDir, "absent")) {
		t.Error("a missing directory must not count as installed")
	}
	if manager.hasCurrentInstall(manager.agents["codex"], preFixCodexDir) {
		t.Error("a directory rejected by the agent must not count as installed")
	}
	if _, statErr := os.Stat(preFixCodexDir); statErr != nil {
		t.Errorf("the predicate must leave a rejected directory in place (stat err = %v)", statErr)
	}
}

func TestManager_HasCurrentLayout(t *testing.T) {
	// arrange
	selected := newTestManager(t)
	seedAgentVersion(t, selected, "claude", "1.0.0", "claude")
	writeCurrentVersion(t, selected, "claude", "1.0.0")
	seedAgentVersion(t, selected, "codex", "1.0.0", "codex")
	writeCurrentVersion(t, selected, "codex", "1.0.0")

	unselected := newTestManager(t)
	seedAgentVersion(t, unselected, "codex", "1.0.0", "codex", codexCodeModeHost)

	fresh := newTestManager(t)

	gone := newTestManager(t)
	seedAgentVersion(t, gone, "claude", "1.0.0", "claude")
	writeCurrentVersion(t, gone, "claude", "2.0.0")

	// act & assert
	if !selected.HasCurrentLayout("claude") {
		t.Error("a single-binary agent has no layout to reject")
	}
	if gone.HasCurrentLayout("claude") {
		t.Error("a current naming a version directory that no longer exists must be reported as stale")
	}
	if selected.HasCurrentLayout("codex") {
		t.Error("a selected codex install without the helper must be reported as stale")
	}
	if unselected.HasCurrentLayout("codex") {
		t.Error("a version on disk with nothing selected must be reported as stale")
	}
	if !fresh.HasCurrentLayout("codex") {
		t.Error("an agent that was never installed must not be reported as stale")
	}
	if !selected.HasCurrentLayout("unknown") {
		t.Error("an unknown agent must not be reported as stale")
	}
}

func TestManager_HasCurrentLayout__a_current_naming_no_version_is_not_a_fresh_install(t *testing.T) {
	// arrange
	manager := newTestManager(t)
	writeCurrentVersion(t, manager, "codex", "")

	// act & assert
	if !manager.HasInstalledAgents() {
		t.Fatal("an agent dir holding a current file must count as installed, or the state is unreachable")
	}
	if manager.HasCurrentLayout("codex") {
		t.Error("a current that names no version must be reported as stale")
	}
}

func TestDedupeNames(t *testing.T) {
	// act
	names := dedupeNames([]string{"codex", "claude", "codex", "pi", "claude"})

	// assert
	if want := []string{"codex", "claude", "pi"}; !slices.Equal(names, want) {
		t.Errorf("dedupeNames() = %v, want %v", names, want)
	}
}

func TestManager_Update__repeated_name_installs_once_into_an_empty_dir(t *testing.T) {
	// arrange
	manager := newTestManager(t)
	agent := &fakeAgent{version: "1.0.0", leftover: "leftover"}
	manager.agents[agent.Name()] = agent
	seedAgentVersion(t, manager, agent.Name(), agent.version, agent.leftover)

	// act
	results, err := manager.Update([]string{agent.Name(), agent.Name()})

	// assert
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := []DownloadResult{{Agent: agent.Name(), Version: agent.version}}
	if !slices.Equal(results, want) {
		t.Errorf("Update() = %v, want %v", results, want)
	}
	if downloads := agent.downloads.Load(); downloads != 1 {
		t.Errorf("Download called %d times, want 1", downloads)
	}
	if agent.leftoverSeen.Load() {
		t.Error("the install dir must be cleared before Download runs")
	}
}

func TestManager_Update__keeps_an_install_the_agent_accepts(t *testing.T) {
	// arrange
	manager := newTestManager(t)
	agent := &fakeAgent{version: "1.0.0", leftover: "fake", installed: true}
	manager.agents[agent.Name()] = agent
	seedAgentVersion(t, manager, agent.Name(), agent.version, agent.leftover)

	// act
	results, err := manager.Update([]string{agent.Name()})

	// assert
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := []DownloadResult{{Agent: agent.Name(), Version: agent.version}}
	if !slices.Equal(results, want) {
		t.Errorf("Update() = %v, want %v", results, want)
	}
	if downloads := agent.downloads.Load(); downloads != 0 {
		t.Errorf("Download called %d times, want 0 - the install is already current", downloads)
	}
	installed := filepath.Join(manager.paths.AgentVersionDir(agent.Name(), agent.version), agent.leftover)
	if _, statErr := os.Stat(installed); statErr != nil {
		t.Errorf("the accepted install must survive (stat err = %v)", statErr)
	}
	current, err := os.ReadFile(manager.paths.AgentCurrentFile(agent.Name()))
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if strings.TrimSpace(string(current)) != agent.version {
		t.Errorf("current = %q, want %s", current, agent.version)
	}
}

// fakeAgent installs without a network and reports what Update did to its
// version directory before calling it.
type fakeAgent struct {
	version      string
	leftover     string
	installed    bool
	downloads    atomic.Int64
	leftoverSeen atomic.Bool
}

func (f *fakeAgent) Name() string       { return "fake" }
func (f *fakeAgent) BinaryName() string { return "fake" }

func (f *fakeAgent) FetchLatestVersion(context.Context) (string, error) { return f.version, nil }

func (f *fakeAgent) IsInstalled(string) bool { return f.installed }

func (f *fakeAgent) Download(_ context.Context, _, destDir string, _ func(downloaded, total int64)) error {
	f.downloads.Add(1)
	if _, err := os.Stat(filepath.Join(destDir, f.leftover)); err == nil {
		f.leftoverSeen.Store(true)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	return nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	manager, err := NewManager(&config.Paths{BinDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func seedAgentVersion(t *testing.T, manager *Manager, name, version string, files ...string) {
	t.Helper()

	versionDir := manager.paths.AgentVersionDir(name, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("create version dir: %v", err)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(versionDir, file), []byte("x"), 0o755); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
}

func writeCurrentVersion(t *testing.T, manager *Manager, name, version string) {
	t.Helper()

	if err := os.MkdirAll(manager.paths.AgentDir(name), 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	if err := os.WriteFile(manager.paths.AgentCurrentFile(name), []byte(version+"\n"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
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
	expectedNames := []string{"claude", "copilot", "codex", "cursor", "opencode", "pi", "ralphex"}
	if len(agents) != len(expectedNames) {
		t.Fatalf("AllAgents() returned %d agents, want %d", len(agents), len(expectedNames))
	}

	for i, name := range expectedNames {
		if agents[i].Name() != name {
			t.Errorf("agents[%d].Name() = %s, want %s", i, agents[i].Name(), name)
		}
	}
}
