package cli

import (
	"strings"
	"testing"
)

func TestSanitizeFuncName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"agentbox", "agentbox"},
		{"my-tool", "my_tool"},
		{"cmd.exe", "cmd_exe"},
		{"my-tool.sh", "my_tool_sh"},
		{"already_valid", "already_valid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// act
			result := sanitizeFuncName(tt.input)

			// assert
			if result != tt.expected {
				t.Errorf("sanitizeFuncName(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateBashCompletion(t *testing.T) {
	// act
	result := generateBashCompletion("agentbox")

	// assert
	expectedSubstrings := []string{
		"__agentbox()",
		"complete -F __agentbox agentbox",
		"commands=\"init run agents clean help completions version\"",
	}

	for _, expected := range expectedSubstrings {
		if !strings.Contains(result, expected) {
			t.Errorf("bash completion missing: %s", expected)
		}
	}
}

func TestGenerateBashCompletion__custom_name(t *testing.T) {
	// act
	result := generateBashCompletion("abox")

	// assert
	if !strings.Contains(result, "__abox()") {
		t.Error("bash completion should use custom function name")
	}
	if !strings.Contains(result, "complete -F __abox abox") {
		t.Error("bash completion should register custom command")
	}
}

func TestGenerateZshCompletion(t *testing.T) {
	// act
	result := generateZshCompletion("agentbox")

	// assert
	expectedSubstrings := []string{
		"_agentbox()",
		"compdef _agentbox agentbox",
		"'init:Initialize sandbox in current directory'",
	}

	for _, expected := range expectedSubstrings {
		if !strings.Contains(result, expected) {
			t.Errorf("zsh completion missing: %s", expected)
		}
	}
}

func TestGenerateZshCompletion__custom_name(t *testing.T) {
	// act
	result := generateZshCompletion("abox")

	// assert
	if !strings.Contains(result, "compdef _agentbox abox") {
		t.Error("zsh completion should add alias for custom command name")
	}
}
