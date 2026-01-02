package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Command metadata for consistency between help, completions, and validation.
// This is the single source of truth for CLI structure.

// AllCommands returns all available top-level commands.
func AllCommands() []string {
	return []string{
		"init",
		"run",
		"attach",
		"ps",
		"agent",
		"self",
		"clean",
		"completion",
		"help",
		"version",
	}
}

// CommandFlags returns valid flags for each command.
// Empty slice means no flags (except global -h/--help).
func CommandFlags() map[string][]string {
	return map[string][]string{
		"init":       {}, // no flags
		"run":        {"--build", "--build-no-cache"},
		"attach":     {}, // no flags, only positional args
		"ps":         {"-a", "--all"},
		"agent":      {}, // has subcommands, not flags
		"self":       {}, // has subcommands, not flags
		"clean":      {}, // no flags
		"completion": {}, // no flags, only positional args
	}
}

// AgentSubcommands returns valid agent subcommands.
func AgentSubcommands() []string {
	return []string{"update", "use"}
}

// SelfSubcommands returns valid self subcommands.
func SelfSubcommands() []string {
	return []string{"update", "uninstall", "versions"}
}

// SelfUninstallFlags returns valid flags for self uninstall subcommand.
func SelfUninstallFlags() []string {
	return []string{"--purge"}
}

// CompletionShells returns valid shells for completion command.
func CompletionShells() []string {
	return []string{"bash", "zsh"}
}

// AllSubcommandPaths returns all subcommand paths for testing.
// Each path is a slice of arguments to reach the subcommand.
// This ensures tests cover ALL entry points, not just top-level commands.
func AllSubcommandPaths() [][]string {
	return [][]string{
		{"agent", "update"},
		{"agent", "use", "dummy-agent", "1.0.0"}, // need args to pass validation
		{"self", "update"},
		{"self", "uninstall"},
		{"self", "versions"},
	}
}

// =============================================================================
// Flag validation - single source of truth for all commands
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
