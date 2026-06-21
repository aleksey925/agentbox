package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aleksey925/agentbox/internal/agents"
	"github.com/aleksey925/agentbox/internal/skeleton"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestHasHelpFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"empty args", []string{}, false},
		{"no help flag", []string{"update", "claude"}, false},
		{"short help flag", []string{"-h"}, true},
		{"long help flag", []string{"--help"}, true},
		{"help in middle", []string{"update", "--help"}, true},
		{"help at end", []string{"update", "claude", "-h"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			result := hasHelpFlag(tt.args)

			// assert
			if result != tt.expected {
				t.Errorf("hasHelpFlag(%v) = %v, want %v", tt.args, result, tt.expected)
			}
		})
	}
}

// Test that subcommand --help shows subcommand help, not parent help.
// This prevents regression where `agent update --help` showed `agent` help.
func TestAgentSubcommandHelp(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		shouldContain    string
		shouldNotContain string
	}{
		{
			name:             "agent --help shows agent help",
			args:             []string{"agent", "--help"},
			shouldContain:    "agentbox agent [command]",
			shouldNotContain: "",
		},
		{
			name:             "agent update --help shows update help",
			args:             []string{"agent", "update", "--help"},
			shouldContain:    "agentbox agent update",
			shouldNotContain: "agentbox agent [command]",
		},
		{
			name:             "agent use --help shows use help",
			args:             []string{"agent", "use", "--help"},
			shouldContain:    "agentbox agent use <agent> <version>",
			shouldNotContain: "agentbox agent [command]",
		},
		{
			name:             "agent flags --help shows flags help",
			args:             []string{"agent", "flags", "--help"},
			shouldContain:    "agentbox agent flags [flags]",
			shouldNotContain: "agentbox agent [command]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			output := captureOutput(func() {
				Run(tt.args, BuildInfo{Version: "test"})
			})

			// assert
			if !strings.Contains(output, tt.shouldContain) {
				t.Errorf("output should contain %q, got:\n%s", tt.shouldContain, output)
			}
			if tt.shouldNotContain != "" && strings.Contains(output, tt.shouldNotContain) {
				t.Errorf("output should NOT contain %q, got:\n%s", tt.shouldNotContain, output)
			}
		})
	}
}

// Test that self subcommand --help shows subcommand help, not parent help.
func TestSelfSubcommandHelp(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		shouldContain    string
		shouldNotContain string
	}{
		{
			name:             "self update --help shows update help",
			args:             []string{"self", "update", "--help"},
			shouldContain:    "agentbox self update",
			shouldNotContain: "",
		},
		{
			name:             "self uninstall --help shows uninstall help",
			args:             []string{"self", "uninstall", "--help"},
			shouldContain:    "agentbox self uninstall",
			shouldNotContain: "",
		},
		{
			name:             "self versions --help shows versions help",
			args:             []string{"self", "versions", "--help"},
			shouldContain:    "agentbox self versions",
			shouldNotContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			output := captureOutput(func() {
				Run(tt.args, BuildInfo{Version: "test"})
			})

			// assert
			if !strings.Contains(output, tt.shouldContain) {
				t.Errorf("output should contain %q, got:\n%s", tt.shouldContain, output)
			}
			if tt.shouldNotContain != "" && strings.Contains(output, tt.shouldNotContain) {
				t.Errorf("output should NOT contain %q, got:\n%s", tt.shouldNotContain, output)
			}
		})
	}
}

func TestParseRunFlags(t *testing.T) {
	app := &App{}

	tests := []struct {
		name string
		args []string
		want runOptions
	}{
		{"no flags", nil, runOptions{}},
		{"build", []string{"--build"}, runOptions{build: true}},
		{"build no cache", []string{"--build-no-cache"}, runOptions{build: true, noCache: true}},
		{"new", []string{"--new"}, runOptions{forceNew: true}},
		{"container", []string{"--container", "abc123"}, runOptions{container: "abc123"}},
		{"container without value", []string{"--container"}, runOptions{}},
		{"combined", []string{"--new", "--container", "box-1"}, runOptions{forceNew: true, container: "box-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			got := app.parseRunFlags(tt.args)

			// assert
			if got != tt.want {
				t.Errorf("parseRunFlags(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

// Test that unknown flags are rejected with proper error messages.
// This prevents regression where unknown flags were silently ignored.
func TestUnknownFlagRejection(t *testing.T) {
	app := &App{Build: BuildInfo{Version: "test"}}

	tests := []struct {
		name    string
		command func([]string) int
		args    []string
	}{
		{"ps rejects unknown flag", app.cmdPs, []string{"--unknown"}},
		{"ps rejects unknown short flag", app.cmdPs, []string{"-x"}},
		{"run rejects unknown flag", app.cmdRun, []string{"--unknown"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			var exitCode int
			stderr := captureStderr(func() {
				exitCode = tt.command(tt.args)
			})

			// assert
			if exitCode != 1 {
				t.Errorf("expected exit code 1, got %d", exitCode)
			}
			if !strings.Contains(stderr, "Unknown flag") {
				t.Errorf("expected 'Unknown flag' in stderr, got: %s", stderr)
			}
		})
	}
}

// Test that agent update rejects unknown flags (not agent names).
func TestAgentUpdateUnknownFlagRejection(t *testing.T) {
	// act
	var exitCode int
	stderr := captureStderr(func() {
		exitCode = Run([]string{"agent", "update", "--unknown"}, BuildInfo{Version: "test"})
	})

	// assert
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "Unknown flag") {
		t.Errorf("expected 'Unknown flag' in stderr, got: %s", stderr)
	}
}

// Test that help commands return exit code 0.
func TestHelpExitCode(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"init --help", []string{"init", "--help"}},
		{"init skeleton --help", []string{"init", "skeleton", "--help"}},
		{"run --help", []string{"run", "--help"}},
		{"ps --help", []string{"ps", "--help"}},
		{"agent --help", []string{"agent", "--help"}},
		{"agent update --help", []string{"agent", "update", "--help"}},
		{"agent use --help", []string{"agent", "use", "--help"}},
		{"clean --help", []string{"clean", "--help"}},
		{"self update --help", []string{"self", "update", "--help"}},
		{"self uninstall --help", []string{"self", "uninstall", "--help"}},
		{"self versions --help", []string{"self", "versions", "--help"}},
		{"completion --help", []string{"completion", "--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			var exitCode int
			captureOutput(func() {
				exitCode = Run(tt.args, BuildInfo{Version: "test"})
			})

			// assert
			if exitCode != 0 {
				t.Errorf("expected exit code 0, got %d", exitCode)
			}
		})
	}
}

// =============================================================================
// Synchronization tests - ensure completions match actual CLI structure
// =============================================================================

// TestBashCompletionContainsAllCommands verifies that bash completion
// includes all commands defined in AllCommands().
func TestBashCompletionContainsAllCommands(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, cmd := range AllCommands() {
		if !strings.Contains(completion, cmd) {
			t.Errorf("bash completion missing command: %s", cmd)
		}
	}
}

// TestBashCompletionContainsAllFlags verifies that bash completion
// includes all flags defined in CommandFlags().
func TestBashCompletionContainsAllFlags(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for cmd, flags := range CommandFlags() {
		for _, flag := range flags {
			if !strings.Contains(completion, flag) {
				t.Errorf("bash completion missing flag %s for command %s", flag, cmd)
			}
		}
	}
}

// TestBashCompletionContainsAllAgentSubcommands verifies that bash completion
// includes all agent subcommands.
func TestBashCompletionContainsAllAgentSubcommands(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, sub := range AgentSubcommands() {
		if !strings.Contains(completion, sub) {
			t.Errorf("bash completion missing agent subcommand: %s", sub)
		}
	}
}

// TestBashCompletionContainsAllInitSubcommands verifies that bash completion
// includes all init subcommands.
func TestBashCompletionContainsAllInitSubcommands(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, sub := range InitSubcommands() {
		if !strings.Contains(completion, sub) {
			t.Errorf("bash completion missing init subcommand: %s", sub)
		}
	}
}

// TestBashCompletionContainsAllSelfSubcommands verifies that bash completion
// includes all self subcommands.
func TestBashCompletionContainsAllSelfSubcommands(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, sub := range SelfSubcommands() {
		if !strings.Contains(completion, sub) {
			t.Errorf("bash completion missing self subcommand: %s", sub)
		}
	}
}

// TestBashCompletionContainsAllSelfUninstallFlags verifies that bash completion
// includes all self uninstall flags.
func TestBashCompletionContainsAllSelfUninstallFlags(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, flag := range SelfUninstallFlags() {
		if !strings.Contains(completion, flag) {
			t.Errorf("bash completion missing self uninstall flag: %s", flag)
		}
	}
}

// TestBashCompletionContainsAllAgentFlagsFlags verifies that bash completion
// includes all agent flags subcommand flags.
func TestBashCompletionContainsAllAgentFlagsFlags(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, flag := range AgentFlagsFlags() {
		if !strings.Contains(completion, flag) {
			t.Errorf("bash completion missing agent flags flag: %s", flag)
		}
	}
}

// TestBashCompletionContainsAllAgentNames verifies that bash completion
// includes all agent names from agents package.
func TestBashCompletionContainsAllAgentNames(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, name := range agents.AllAgentNames() {
		if !strings.Contains(completion, name) {
			t.Errorf("bash completion missing agent name: %s", name)
		}
	}
}

// TestBashCompletionContainsAllShells verifies that bash completion
// includes all shells defined in CompletionShells().
func TestBashCompletionContainsAllShells(t *testing.T) {
	// act
	completion := generateBashCompletion("agentbox")

	// assert
	for _, shell := range CompletionShells() {
		if !strings.Contains(completion, shell) {
			t.Errorf("bash completion missing shell: %s", shell)
		}
	}
}

// TestZshCompletionContainsAllCommands verifies that zsh completion
// includes all commands.
func TestZshCompletionContainsAllCommands(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, cmd := range AllCommands() {
		if !strings.Contains(completion, "'"+cmd+":") {
			t.Errorf("zsh completion missing command: %s", cmd)
		}
	}
}

// TestZshCompletionContainsAllAgentNames verifies that zsh completion
// includes all agent names.
func TestZshCompletionContainsAllAgentNames(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, name := range agents.AllAgentNames() {
		if !strings.Contains(completion, "'"+name+":") {
			t.Errorf("zsh completion missing agent name: %s", name)
		}
	}
}

// TestZshCompletionContainsAllFlags verifies that zsh completion
// includes all flags defined in CommandFlags().
func TestZshCompletionContainsAllFlags(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for cmd, flags := range CommandFlags() {
		for _, flag := range flags {
			if !strings.Contains(completion, "'"+flag+":") {
				t.Errorf("zsh completion missing flag %s for command %s", flag, cmd)
			}
		}
	}
}

// TestZshCompletionContainsAllAgentSubcommands verifies that zsh completion
// includes all agent subcommands.
func TestZshCompletionContainsAllAgentSubcommands(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, sub := range AgentSubcommands() {
		if !strings.Contains(completion, "'"+sub+":") {
			t.Errorf("zsh completion missing agent subcommand: %s", sub)
		}
	}
}

// TestZshCompletionContainsAllInitSubcommands verifies that zsh completion
// includes all init subcommands.
func TestZshCompletionContainsAllInitSubcommands(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, sub := range InitSubcommands() {
		if !strings.Contains(completion, "'"+sub+":") {
			t.Errorf("zsh completion missing init subcommand: %s", sub)
		}
	}
}

// TestZshCompletionContainsAllSelfSubcommands verifies that zsh completion
// includes all self subcommands.
func TestZshCompletionContainsAllSelfSubcommands(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, sub := range SelfSubcommands() {
		if !strings.Contains(completion, "'"+sub+":") {
			t.Errorf("zsh completion missing self subcommand: %s", sub)
		}
	}
}

// TestZshCompletionContainsAllSelfUninstallFlags verifies that zsh completion
// includes all self uninstall flags.
func TestZshCompletionContainsAllSelfUninstallFlags(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, flag := range SelfUninstallFlags() {
		if !strings.Contains(completion, "'"+flag+":") {
			t.Errorf("zsh completion missing self uninstall flag: %s", flag)
		}
	}
}

// TestZshCompletionContainsAllAgentFlagsFlags verifies that zsh completion
// includes all agent flags subcommand flags.
func TestZshCompletionContainsAllAgentFlagsFlags(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, flag := range AgentFlagsFlags() {
		if !strings.Contains(completion, "'"+flag+":") {
			t.Errorf("zsh completion missing agent flags flag: %s", flag)
		}
	}
}

// TestZshCompletionContainsAllShells verifies that zsh completion
// includes all shells defined in CompletionShells().
func TestZshCompletionContainsAllShells(t *testing.T) {
	// act
	completion := generateZshCompletion("agentbox")

	// assert
	for _, shell := range CompletionShells() {
		if !strings.Contains(completion, "'"+shell+":") {
			t.Errorf("zsh completion missing shell: %s", shell)
		}
	}
}

// TestCliRouterHandlesAllCommands verifies that cli.go router
// handles all commands defined in AllCommands().
func TestCliRouterHandlesAllCommands(t *testing.T) {
	// act
	for _, cmd := range AllCommands() {
		exitCode := Run([]string{cmd, "--help"}, BuildInfo{Version: "test"})

		// assert - help should return 0 for all commands
		if exitCode != 0 {
			t.Errorf("command %s --help returned %d, want 0", cmd, exitCode)
		}
	}
}

// TestAllCommandsRejectUnknownFlags verifies that all top-level commands
// reject unknown flags. Uses Run() to test via the real dispatcher.
func TestAllCommandsRejectUnknownFlags(t *testing.T) {
	// Commands that should reject unknown flags
	// Note: help and version are special and handled before flag validation
	commands := []string{"init", "run", "ps", "clean", "upgrade", "agent", "completion"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			// act
			var exitCode int
			stderr := captureStderr(func() {
				exitCode = Run([]string{cmd, "--unknown-flag-xyz"}, BuildInfo{Version: "test"})
			})

			// assert
			if exitCode != 1 {
				t.Errorf("%s should reject unknown flag, got exit code %d", cmd, exitCode)
			}
			if !strings.Contains(stderr, "Unknown flag") {
				t.Errorf("%s should print 'Unknown flag', got: %s", cmd, stderr)
			}
		})
	}
}

// TestAllSubcommandsRejectUnknownFlags verifies that ALL subcommands
// reject unknown flags. Test cases are generated from AllSubcommandPaths()
// to ensure complete coverage - adding a new subcommand path automatically
// adds a new test case.
func TestAllSubcommandsRejectUnknownFlags(t *testing.T) {
	for _, path := range AllSubcommandPaths() {
		// generate test name from path (e.g., "agent update")
		testName := strings.Join(path[:2], " ")

		// insert --unknown-flag at position 2 (right after subcommand name)
		args := make([]string, 0, len(path)+1)
		args = append(args, path[:2]...)
		args = append(args, "--unknown-flag")
		args = append(args, path[2:]...)

		t.Run(testName, func(t *testing.T) {
			// act
			var exitCode int
			stderr := captureStderr(func() {
				exitCode = Run(args, BuildInfo{Version: "test"})
			})

			// assert
			if exitCode != 1 {
				t.Errorf("%s should reject unknown flag, got exit code %d", testName, exitCode)
			}
			if !strings.Contains(stderr, "Unknown flag") {
				t.Errorf("%s should print 'Unknown flag', got: %s", testName, stderr)
			}
		})
	}
}

func TestToShellExit(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"exitOK stays 0", exitOK, 0},
		{"exitError stays 1", exitError, 1},
		{"exitCanceled becomes 0", exitCanceled, 0},
		{"other positive codes unchanged", 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			result := toShellExit(tt.code)

			// assert
			if result != tt.expected {
				t.Errorf("toShellExit(%d) = %d, want %d", tt.code, result, tt.expected)
			}
		})
	}
}

func TestExitCanceled__stops_execution_chain(t *testing.T) {
	// This test documents the expected behavior:
	// when a function returns exitCanceled, callers should stop execution
	// and not proceed with subsequent operations (like downloading agents)

	// arrange
	executionLog := []string{}

	mockStep1 := func() int {
		executionLog = append(executionLog, "step1")
		return exitCanceled // user canceled
	}

	mockStep2 := func() int {
		executionLog = append(executionLog, "step2")
		return exitOK
	}

	// act: simulate the pattern used in doInit
	code := mockStep1()
	if code == exitOK {
		mockStep2()
	}

	// assert
	expected := []string{"step1"}
	if !slices.Equal(executionLog, expected) {
		t.Fatalf("executionLog = %v, want %v", executionLog, expected)
	}

	// verify exitCanceled is non-zero (so code != exitOK works)
	if exitCanceled == exitOK {
		t.Error("exitCanceled must not equal exitOK")
	}
}

func TestCommandDesc(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected string
	}{
		{"init", "init", "Initialize sandbox in current directory"},
		{"run", "run", "Start sandbox or attach if running"},
		{"nonexistent", "nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			result := CommandDesc(tt.cmd)

			// assert
			if result != tt.expected {
				t.Errorf("CommandDesc(%q) = %q, want %q", tt.cmd, result, tt.expected)
			}
		})
	}
}

func TestSubcommandDesc(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		sub      string
		expected string
	}{
		{"init skeleton", "init", "skeleton", "(Re)init global sandbox configs"},
		{"agent update", "agent", "update", "Update agents to latest version"},
		{"self uninstall", "self", "uninstall", "Remove agentbox from system"},
		{"unknown parent", "unknown", "sub", ""},
		{"nonexistent sub", "init", "nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			result := SubcommandDesc(tt.parent, tt.sub)

			// assert
			if result != tt.expected {
				t.Errorf("SubcommandDesc(%q, %q) = %q, want %q", tt.parent, tt.sub, result, tt.expected)
			}
		})
	}
}

func TestCheckConfigVersion(t *testing.T) {
	core, err := skeleton.GetCoreTemplate()
	if err != nil {
		t.Fatal(err)
	}
	df, err := skeleton.GetEmbeddedDockerfile()
	if err != nil {
		t.Fatal(err)
	}
	cv, dv := core.Version, df.Version

	tests := []struct {
		name  string
		files []string
		want  int
	}{
		{name: "current", files: []string{coreFile(cv), dockerfileName(dv)}, want: exitOK},
		{name: "older", files: []string{coreFile(cv - 1), dockerfileName(dv)}, want: exitError},
		{name: "newer", files: []string{coreFile(cv + 1), dockerfileName(dv)}, want: exitError},
		{name: "empty", files: nil, want: exitOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			dir := t.TempDir()
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			// act
			got := checkConfigVersion(dir)

			// assert
			if got != tt.want {
				t.Errorf("checkConfigVersion(%v) = %d, want %d", tt.files, got, tt.want)
			}
		})
	}
}

func coreFile(v int) string       { return fmt.Sprintf("core.v%d.yml", v) }
func dockerfileName(v int) string { return fmt.Sprintf("Dockerfile.v%d.agentbox", v) }

func TestParseUpgradeArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantPath  string
		wantDepth int
		wantCode  int
	}{
		{name: "empty", args: nil, wantPath: "", wantDepth: 1, wantCode: exitOK},
		{name: "path_only", args: []string{"/x"}, wantPath: "/x", wantDepth: 1, wantCode: exitOK},
		{name: "depth_then_path", args: []string{"--depth", "2", "/x"}, wantPath: "/x", wantDepth: 2, wantCode: exitOK},
		{name: "path_then_depth", args: []string{"/x", "--depth", "3"}, wantPath: "/x", wantDepth: 3, wantCode: exitOK},
		{name: "depth_no_value", args: []string{"--depth"}, wantCode: exitError},
		{name: "depth_not_int", args: []string{"--depth", "abc"}, wantCode: exitError},
		{name: "depth_zero", args: []string{"--depth", "0"}, wantCode: exitError},
		{name: "depth_without_path", args: []string{"--depth", "2"}, wantCode: exitError},
		{name: "two_paths", args: []string{"/a", "/b"}, wantCode: exitError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			path, depth, code := captureStderrValue(func() (string, int, int) {
				return parseUpgradeArgs(tt.args)
			})

			// assert
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
			if code == exitOK && (path != tt.wantPath || depth != tt.wantDepth) {
				t.Errorf("parseUpgradeArgs(%v) = (%q, %d), want (%q, %d)", tt.args, path, depth, tt.wantPath, tt.wantDepth)
			}
		})
	}
}

func TestScanProjects(t *testing.T) {
	root := t.TempDir()
	mkProject(t, filepath.Join(root, "direct"))
	mkProject(t, filepath.Join(root, "org", "nested"))

	// depth 1 finds only the direct child
	got := scanProjects(root, 1)
	if len(got) != 1 || got[0] != filepath.Join(root, "direct") {
		t.Errorf("depth 1 = %v, want [%s]", got, filepath.Join(root, "direct"))
	}

	// depth 2 also finds the nested one
	got = scanProjects(root, 2)
	want := []string{filepath.Join(root, "direct"), filepath.Join(root, "org", "nested")}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("depth 2 = %v, want %v", got, want)
	}
}

func mkProject(t *testing.T, dir string) {
	t.Helper()
	agentboxDir := filepath.Join(dir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"core.v4.yml", "Dockerfile.v4.agentbox"} {
		if err := os.WriteFile(filepath.Join(agentboxDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func captureStderrValue(f func() (string, int, int)) (string, int, int) {
	old := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old }()
	p, d, c := f()
	w.Close()
	return p, d, c
}
