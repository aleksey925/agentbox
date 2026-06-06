// Package agentflags manages the user-editable file that defines which flags
// each agent (harness) is launched with inside the sandbox.
//
// The file is global (~/.agentbox/flags/agent-flags) and is read live by the
// in-container launcher on every agent invocation, so edits apply to the next
// launch without an image rebuild. Because the launcher is a plain bash script,
// the format is deliberately line-based (one harness per line) rather than YAML.
package agentflags

import (
	"fmt"
	"os"
	"strings"

	"github.com/aleksey925/agentbox/internal/agents"
)

const fileHeader = `# Flags each agent is launched with inside the sandbox.
#
# Format: <agent> <flags...>      (one agent per line)
#   *      <flags...>             applies to any agent without its own line
#
# By default no flags are imposed — every agent below is commented out.
# Uncomment a line and add flags (or add your own line) to opt in. Edits apply
# to the next agent launch — even inside a running sandbox — with no rebuild.
# A specific agent line takes precedence over the "*" line.
#
# Available agents:

`

// DefaultFileContent renders the seed content for the flags file. Every agent
// is written as a commented-out line so nothing is imposed by default. Any flags
// SuggestedFlags grows in the future are rendered (still commented) alongside the
// agent name, ready to uncomment.
func DefaultFileContent() []byte {
	var b strings.Builder
	b.WriteString(fileHeader)

	suggested := agents.SuggestedFlags()
	for _, name := range agents.AllAgentNames() {
		if flags := suggested[name]; flags != "" {
			fmt.Fprintf(&b, "# %s %s\n", name, flags)
		} else {
			fmt.Fprintf(&b, "# %s\n", name)
		}
	}

	return []byte(b.String())
}

// EnsureFile creates the flags file with default content if it does not exist.
// An existing file is left untouched — it is owned by the user.
func EnsureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat flags file: %w", err)
	}

	if err := os.WriteFile(path, DefaultFileContent(), 0o644); err != nil {
		return fmt.Errorf("write flags file: %w", err)
	}

	return nil
}
