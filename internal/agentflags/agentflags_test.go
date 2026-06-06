package agentflags

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksey925/agentbox/internal/agents"
)

func TestDefaultFileContent__imposes_no_flags(t *testing.T) {
	// act
	content := string(DefaultFileContent())

	// assert - every non-blank line is a comment, so nothing is imposed
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			t.Errorf("seed file imposes a flag via non-comment line: %q", line)
		}
	}
}

func TestDefaultFileContent__lists_every_agent(t *testing.T) {
	// act
	content := string(DefaultFileContent())

	// assert
	for _, name := range agents.AllAgentNames() {
		if !strings.Contains(content, "# "+name) {
			t.Errorf("seed file is missing agent %q", name)
		}
	}
}

func TestEnsureFile__creates_when_absent(t *testing.T) {
	// arrange
	path := filepath.Join(t.TempDir(), "agent-flags")

	// act
	err := EnsureFile(path)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was not created: %v", err)
	}
	if !bytes.Equal(content, DefaultFileContent()) {
		t.Errorf("file content = %q, want default content", string(content))
	}
}

func TestEnsureFile__leaves_existing_untouched(t *testing.T) {
	// arrange
	path := filepath.Join(t.TempDir(), "agent-flags")
	existing := []byte("claude --my-own-flag\n")
	if err := os.WriteFile(path, existing, 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	err := EnsureFile(path)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, existing) {
		t.Errorf("existing file was modified: got %q, want %q", string(content), string(existing))
	}
}
