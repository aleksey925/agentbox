package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Command metadata for consistency between help, completions, and validation.
// This is the single source of truth for CLI structure.

// =============================================================================
// Core types and helpers
// =============================================================================

// Subcommand represents a subcommand with its description for completions.
type Subcommand struct {
	Name        string
	Description string
}

// extractNames extracts just the names from a slice of Subcommands.
func extractNames(subs []Subcommand) []string {
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.Name
	}
	return names
}

// =============================================================================
// Top-level commands
// =============================================================================

// AllCommandsWithDesc returns all available top-level commands with descriptions.
// This is the single source of truth for top-level commands.
func AllCommandsWithDesc() []Subcommand {
	return []Subcommand{
		{"init", "Initialize sandbox in current directory"},
		{"run", "Start sandbox"},
		{"attach", "Attach to running sandbox"},
		{"ps", "List running sandboxes"},
		{"agent", "Manage AI agents"},
		{"self", "Update or uninstall agentbox"},
		{"clean", "Remove sandbox files from project"},
		{"completion", "Generate shell completion script"},
		{"help", "Show help"},
		{"version", "Show version"},
	}
}

// AllCommands returns all available top-level command names.
func AllCommands() []string {
	return extractNames(AllCommandsWithDesc())
}

// =============================================================================
// Command flags
// =============================================================================

// RunFlagsWithDesc returns run command flags with descriptions.
// This is the single source of truth for run flags.
func RunFlagsWithDesc() []Subcommand {
	return []Subcommand{
		{"--build", "Rebuild image before running"},
		{"--build-no-cache", "Rebuild image without Docker cache"},
	}
}

// PsFlagsWithDesc returns ps command flags with descriptions.
// This is the single source of truth for ps flags.
func PsFlagsWithDesc() []Subcommand {
	return []Subcommand{
		{"-a", "Show sandboxes from all projects"},
		{"--all", "Show sandboxes from all projects"},
	}
}

// CommandFlags returns valid flags for each command.
// Empty slice means no flags (except global -h/--help).
func CommandFlags() map[string][]string {
	return map[string][]string{
		"init":       {}, // no flags
		"run":        extractNames(RunFlagsWithDesc()),
		"attach":     {}, // no flags, only positional args
		"ps":         extractNames(PsFlagsWithDesc()),
		"agent":      {}, // has subcommands, not flags
		"self":       {}, // has subcommands, not flags
		"clean":      {}, // no flags
		"completion": {}, // no flags, only positional args
	}
}

// =============================================================================
// Subcommands
// =============================================================================

// InitSubcommandsWithDesc returns init subcommands with descriptions.
// This is the single source of truth for init subcommands.
func InitSubcommandsWithDesc() []Subcommand {
	return []Subcommand{
		{"skeleton", "(Re)init global sandbox configs"},
	}
}

// InitSubcommands returns valid init subcommand names.
func InitSubcommands() []string {
	return extractNames(InitSubcommandsWithDesc())
}

// AgentSubcommandsWithDesc returns agent subcommands with descriptions.
// This is the single source of truth for agent subcommands.
func AgentSubcommandsWithDesc() []Subcommand {
	return []Subcommand{
		{"update", "Update agents to latest version"},
		{"use", "Switch agent to specific version"},
	}
}

// AgentSubcommands returns valid agent subcommand names.
func AgentSubcommands() []string {
	return extractNames(AgentSubcommandsWithDesc())
}

// SelfSubcommandsWithDesc returns self subcommands with descriptions.
// This is the single source of truth for self subcommands.
func SelfSubcommandsWithDesc() []Subcommand {
	return []Subcommand{
		{"update", "Update to latest or specified version"},
		{"uninstall", "Remove agentbox from system"},
		{"versions", "List available versions"},
	}
}

// SelfSubcommands returns valid self subcommand names.
func SelfSubcommands() []string {
	return extractNames(SelfSubcommandsWithDesc())
}

// SelfUninstallFlagsWithDesc returns self uninstall flags with descriptions.
// This is the single source of truth for self uninstall flags.
func SelfUninstallFlagsWithDesc() []Subcommand {
	return []Subcommand{
		{"--purge", "Also remove ~/.agentbox directory"},
	}
}

// SelfUninstallFlags returns valid flags for self uninstall subcommand.
func SelfUninstallFlags() []string {
	return extractNames(SelfUninstallFlagsWithDesc())
}

// CompletionShellsWithDesc returns valid shells with descriptions.
// This is the single source of truth for completion shells.
func CompletionShellsWithDesc() []Subcommand {
	return []Subcommand{
		{"bash", "Bash shell"},
		{"zsh", "Zsh shell"},
	}
}

// CompletionShells returns valid shells for completion command.
func CompletionShells() []string {
	return extractNames(CompletionShellsWithDesc())
}

// AllSubcommandPaths returns all subcommand paths for testing.
// Each path is a slice of arguments to reach the subcommand.
// This ensures tests cover ALL entry points, not just top-level commands.
func AllSubcommandPaths() [][]string {
	return [][]string{
		{"init", "skeleton"},
		{"agent", "update"},
		{"agent", "use", "dummy-agent", "1.0.0"}, // need args to pass validation
		{"self", "update"},
		{"self", "uninstall"},
		{"self", "versions"},
	}
}

// =============================================================================
// Flag validation
// =============================================================================

// UnknownFlagError is returned when an unknown flag is encountered.
type UnknownFlagError struct {
	Flag string
}

func (e UnknownFlagError) Error() string {
	return "unknown flag: " + e.Flag
}

// ValidateNoUnknownFlags checks that args contain no unknown flags.
// Returns UnknownFlagError if unknown flag found.
// Help flags (-h, --help) are allowed and should be handled before calling this.
func ValidateNoUnknownFlags(args, allowedFlags []string) error {
	allowed := make(map[string]bool)
	for _, f := range allowedFlags {
		allowed[f] = true
	}
	// help flags are always allowed
	allowed["-h"] = true
	allowed["--help"] = true

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if !allowed[arg] {
				return UnknownFlagError{Flag: arg}
			}
		}
	}
	return nil
}

// RejectUnknownFlags validates args and prints error to stderr if unknown flag found.
// Returns exit code: 0 if valid, 1 if unknown flag found.
// This is a convenience wrapper for commands that don't accept any flags.
func RejectUnknownFlags(args []string) int {
	return RejectUnknownFlagsWithAllowed(args, nil)
}

// RejectUnknownFlagsWithAllowed validates args against allowed flags.
// Returns exit code: 0 if valid, 1 if unknown flag found.
func RejectUnknownFlagsWithAllowed(args, allowedFlags []string) int {
	if err := ValidateNoUnknownFlags(args, allowedFlags); err != nil {
		var flagErr UnknownFlagError
		if errors.As(err, &flagErr) {
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", flagErr.Flag)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		if len(allowedFlags) > 0 {
			fmt.Fprintf(os.Stderr, "Available flags: %s\n", strings.Join(allowedFlags, ", "))
		}
		return 1
	}
	return 0
}
