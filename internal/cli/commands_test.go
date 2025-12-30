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
	app := &App{Version: "test"}

	tests := []struct {
		name           string
		args           []string
		shouldContain  string
		shouldNotContain string
	}{
		{
			name:           "agent --help shows agent help",
			args:           []string{"--help"},
			shouldContain:  "agentbox agent [command]",
			shouldNotContain: "",
		},
		{
			name:           "agent update --help shows update help",
			args:           []string{"update", "--help"},
			shouldContain:  "agentbox agent update",
			shouldNotContain: "agentbox agent [command]",
		},
		{
			name:           "agent use --help shows use help",
			args:           []string{"use", "--help"},
			shouldContain:  "agentbox agent use <agent> <version>",
			shouldNotContain: "agentbox agent [command]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			output := captureOutput(func() {
				app.cmdAgent(tt.args)
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
	// arrange
	app := &App{Version: "test"}

	// act
	var exitCode int
	stderr := captureStderr(func() {
		exitCode = app.cmdAgent([]string{"update", "--unknown"})
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
	app := &App{Version: "test"}

	tests := []struct {
		name    string
		command func([]string) int
		args    []string
	}{
		{"init --help", app.cmdInit, []string{"--help"}},
		{"run --help", app.cmdRun, []string{"--help"}},
		{"attach --help", app.cmdAttach, []string{"--help"}},
		{"ps --help", app.cmdPs, []string{"--help"}},
		{"agent --help", app.cmdAgent, []string{"--help"}},
		{"agent update --help", app.cmdAgent, []string{"update", "--help"}},
		{"agent use --help", app.cmdAgent, []string{"use", "--help"}},
		{"clean --help", app.cmdClean, []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			var exitCode int
			captureOutput(func() {
				exitCode = tt.command(tt.args)
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
// reject unknown flags.
func TestAllCommandsRejectUnknownFlags(t *testing.T) {
	app := &App{Version: "test"}

	commandFuncs := map[string]func([]string) int{
		"init":       app.cmdInit,
		"run":        app.cmdRun,
		"attach":     app.cmdAttach,
		"ps":         app.cmdPs,
		"clean":      app.cmdClean,
		"agent":      app.cmdAgent,
		"completion": app.cmdCompletion,
	}

	for cmd, fn := range commandFuncs {
		t.Run(cmd, func(t *testing.T) {
			// act
			var exitCode int
			stderr := captureStderr(func() {
				exitCode = fn([]string{"--unknown-flag-xyz"})
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
