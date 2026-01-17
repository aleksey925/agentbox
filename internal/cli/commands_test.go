package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aleksey925/agentbox/internal/agents"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			output := captureOutput(func() {
				Run(tt.args, "test")
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
				Run(tt.args, "test")
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

// Test that unknown flags are rejected with proper error messages.
// This prevents regression where unknown flags were silently ignored.
func TestUnknownFlagRejection(t *testing.T) {
	app := &App{Version: "test"}

	tests := []struct {
		name    string
		command func([]string) int
		args    []string
	}{
		{"ps rejects unknown flag", app.cmdPs, []string{"--unknown"}},
		{"ps rejects unknown short flag", app.cmdPs, []string{"-x"}},
		{"run rejects unknown flag", app.cmdRun, []string{"--unknown"}},
		{"attach rejects flag-like arg", app.cmdAttach, []string{"--unknown"}},
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
		exitCode = Run([]string{"agent", "update", "--unknown"}, "test")
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
		{"attach --help", []string{"attach", "--help"}},
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
				exitCode = Run(tt.args, "test")
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
		exitCode := Run([]string{cmd, "--help"}, "test")

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
	commands := []string{"init", "run", "attach", "ps", "clean", "agent", "completion"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			// act
			var exitCode int
			stderr := captureStderr(func() {
				exitCode = Run([]string{cmd, "--unknown-flag-xyz"}, "test")
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
				exitCode = Run(args, "test")
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
	if len(executionLog) != len(expected) {
		t.Fatalf("execution log = %v, want %v", executionLog, expected)
	}
	for i, step := range expected {
		if executionLog[i] != step {
			t.Errorf("executionLog[%d] = %s, want %s", i, executionLog[i], step)
		}
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
		{"run", "run", "Start sandbox"},
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
