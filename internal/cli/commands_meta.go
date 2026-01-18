package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// =============================================================================
// CLI METADATA - Single Source of Truth
// =============================================================================
//
// When adding a new command/subcommand:
// 1. Add entry to commandTree below
// 2. Implement handler method on *App
//
// Everything else (router, help, completions, tests) derives from commandTree.
// =============================================================================

// Command represents a CLI command with optional subcommands.
type Command struct {
	Name        string
	Description string
	Handler     func(app *App, args []string) int
	Subcommands []Command
	Flags       []Flag
}

// Flag represents a command flag with description.
type Flag struct {
	Name        string // e.g., "-a, --all" or "--build"
	Description string
}

// commandTree returns the single source of truth for CLI structure.
// All routing, help, completions, and tests derive from this.
// Note: This is a function to avoid initialization cycles with handlers
// that reference metadata functions.
func commandTree() []Command {
	return []Command{
	{
		Name:        "init",
		Description: "Initialize sandbox in current directory",
		Handler:     (*App).cmdInit,
		Subcommands: []Command{
			{Name: "skeleton", Description: "(Re)init global sandbox configs", Handler: (*App).cmdInitSkeleton, Flags: []Flag{
				{"-f, --force", "Force reinitialize even if skeleton exists"},
			}},
		},
	},
	{
		Name:        "run",
		Description: "Start sandbox",
		Handler:     (*App).cmdRun,
		Flags: []Flag{
			{"--build", "Rebuild image before running"},
			{"--build-no-cache", "Rebuild image without Docker cache"},
		},
	},
	{
		Name:        "attach",
		Description: "Attach to running sandbox",
		Handler:     (*App).cmdAttach,
	},
	{
		Name:        "ps",
		Description: "List running sandboxes",
		Handler:     (*App).cmdPs,
		Flags: []Flag{
			{"-a, --all", "Show sandboxes from all projects"},
		},
	},
	{
		Name:        "agent",
		Description: "Manage AI agents",
		Handler:     (*App).cmdAgentStatus,
		Subcommands: []Command{
			{Name: "update", Description: "Update agents to latest version", Handler: (*App).cmdAgentUpdate},
			{Name: "use", Description: "Switch agent to specific version", Handler: (*App).cmdAgentUse},
		},
	},
	{
		Name:        "self",
		Description: "Update or uninstall agentbox",
		Handler:     nil, // requires subcommand
		Subcommands: []Command{
			{Name: "update", Description: "Update to latest or specified version", Handler: (*App).cmdSelfUpdate},
			{Name: "uninstall", Description: "Remove agentbox from system", Handler: (*App).cmdSelfUninstall, Flags: []Flag{
				{"--purge", "Also remove ~/.agentbox directory"},
			}},
			{Name: "versions", Description: "List available versions", Handler: (*App).cmdSelfVersions},
		},
	},
	{
		Name:        "clean",
		Description: "Remove sandbox files from project",
		Handler:     (*App).cmdClean,
	},
	{
		Name:        "completion",
		Description: "Generate shell completion script",
		Handler:     (*App).cmdCompletion,
	},
	{
		Name:        "help",
		Description: "Show help",
		Handler:     nil, // handled specially in dispatcher
	},
	{
		Name:        "version",
		Description: "Show version",
		Handler:     nil, // handled specially in dispatcher
	},
	}
}

// =============================================================================
// Command tree access
// =============================================================================

// CommandTree returns the command tree (for completions and tests).
func CommandTree() []Command {
	return commandTree()
}

// FindCommand finds a command by name in a command slice.
func FindCommand(commands []Command, name string) *Command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}

// =============================================================================
// Derived metadata (for backward compatibility and convenience)
// =============================================================================

// AllCommands returns all top-level command names.
func AllCommands() []string {
	tree := commandTree()
	names := make([]string, len(tree))
	for i, cmd := range tree {
		names[i] = cmd.Name
	}
	return names
}

// AllCommandsWithDesc returns all top-level commands with descriptions.
func AllCommandsWithDesc() []Subcommand {
	tree := commandTree()
	subs := make([]Subcommand, len(tree))
	for i, cmd := range tree {
		subs[i] = Subcommand{cmd.Name, cmd.Description}
	}
	return subs
}

// Subcommand is a legacy type for backward compatibility.
type Subcommand struct {
	Name        string
	Description string
}

// CommandFlags returns flag names for each command.
func CommandFlags() map[string][]string {
	result := make(map[string][]string)
	for _, cmd := range commandTree() {
		result[cmd.Name] = extractFlagNames(cmd.Flags)
	}
	return result
}

// SubcommandFlags returns flag names for a subcommand.
func SubcommandFlags(parent, sub string) []string {
	parentCmd := FindCommand(commandTree(), parent)
	if parentCmd == nil {
		return nil
	}
	subCmd := FindCommand(parentCmd.Subcommands, sub)
	if subCmd == nil {
		return nil
	}
	return extractFlagNames(subCmd.Flags)
}

func extractFlagNames(flags []Flag) []string {
	names := make([]string, 0, len(flags)*2)
	for _, f := range flags {
		// Handle combined flags like "-a, --all"
		for part := range strings.SplitSeq(f.Name, ", ") {
			names = append(names, strings.TrimSpace(part))
		}
	}
	return names
}

// InitSubcommands returns init subcommand names.
func InitSubcommands() []string {
	cmd := FindCommand(commandTree(), "init")
	if cmd == nil {
		return nil
	}
	return extractSubcommandNames(cmd.Subcommands)
}

// AgentSubcommands returns agent subcommand names.
func AgentSubcommands() []string {
	cmd := FindCommand(commandTree(), "agent")
	if cmd == nil {
		return nil
	}
	return extractSubcommandNames(cmd.Subcommands)
}

// SelfSubcommands returns self subcommand names.
func SelfSubcommands() []string {
	cmd := FindCommand(commandTree(), "self")
	if cmd == nil {
		return nil
	}
	return extractSubcommandNames(cmd.Subcommands)
}

// SelfUninstallFlags returns self uninstall flag names.
func SelfUninstallFlags() []string {
	return SubcommandFlags("self", "uninstall")
}

// InitSkeletonFlags returns init skeleton flag names.
func InitSkeletonFlags() []string {
	return SubcommandFlags("init", "skeleton")
}

// InitSkeletonFlagsWithDesc returns init skeleton flags with descriptions.
func InitSkeletonFlagsWithDesc() []Subcommand {
	parentCmd := FindCommand(commandTree(), "init")
	if parentCmd == nil {
		return nil
	}
	subCmd := FindCommand(parentCmd.Subcommands, "skeleton")
	if subCmd == nil {
		return nil
	}
	return flagsToSubcommands(subCmd.Flags)
}

func extractSubcommandNames(subs []Command) []string {
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.Name
	}
	return names
}

// InitSubcommandsWithDesc returns init subcommands with descriptions.
func InitSubcommandsWithDesc() []Subcommand {
	cmd := FindCommand(commandTree(), "init")
	if cmd == nil {
		return nil
	}
	return commandsToSubcommands(cmd.Subcommands)
}

// AgentSubcommandsWithDesc returns agent subcommands with descriptions.
func AgentSubcommandsWithDesc() []Subcommand {
	cmd := FindCommand(commandTree(), "agent")
	if cmd == nil {
		return nil
	}
	return commandsToSubcommands(cmd.Subcommands)
}

// SelfSubcommandsWithDesc returns self subcommands with descriptions.
func SelfSubcommandsWithDesc() []Subcommand {
	cmd := FindCommand(commandTree(), "self")
	if cmd == nil {
		return nil
	}
	return commandsToSubcommands(cmd.Subcommands)
}

// SelfUninstallFlagsWithDesc returns self uninstall flags with descriptions.
func SelfUninstallFlagsWithDesc() []Subcommand {
	parentCmd := FindCommand(commandTree(), "self")
	if parentCmd == nil {
		return nil
	}
	subCmd := FindCommand(parentCmd.Subcommands, "uninstall")
	if subCmd == nil {
		return nil
	}
	return flagsToSubcommands(subCmd.Flags)
}

// RunFlagsWithDesc returns run flags with descriptions.
func RunFlagsWithDesc() []Subcommand {
	cmd := FindCommand(commandTree(), "run")
	if cmd == nil {
		return nil
	}
	return flagsToSubcommands(cmd.Flags)
}

// PsFlagsWithDesc returns ps flags with descriptions.
func PsFlagsWithDesc() []Subcommand {
	cmd := FindCommand(commandTree(), "ps")
	if cmd == nil {
		return nil
	}
	return flagsToSubcommands(cmd.Flags)
}

func commandsToSubcommands(cmds []Command) []Subcommand {
	subs := make([]Subcommand, len(cmds))
	for i, c := range cmds {
		subs[i] = Subcommand{c.Name, c.Description}
	}
	return subs
}

func flagsToSubcommands(flags []Flag) []Subcommand {
	// Expand combined flags like "-a, --all" into separate entries
	var subs []Subcommand
	for _, f := range flags {
		for part := range strings.SplitSeq(f.Name, ", ") {
			name := strings.TrimSpace(part)
			subs = append(subs, Subcommand{name, f.Description})
		}
	}
	return subs
}

// CompletionShells returns valid shell names.
func CompletionShells() []string {
	return []string{"bash", "zsh"}
}

// CompletionShellsWithDesc returns shells with descriptions.
func CompletionShellsWithDesc() []Subcommand {
	return []Subcommand{
		{"bash", "Bash shell"},
		{"zsh", "Zsh shell"},
	}
}

// CommandDesc returns the description for a command.
func CommandDesc(name string) string {
	cmd := FindCommand(commandTree(), name)
	if cmd == nil {
		return ""
	}
	return cmd.Description
}

// SubcommandDesc returns the description for a subcommand.
func SubcommandDesc(parent, name string) string {
	parentCmd := FindCommand(commandTree(), parent)
	if parentCmd == nil {
		return ""
	}
	subCmd := FindCommand(parentCmd.Subcommands, name)
	if subCmd == nil {
		return ""
	}
	return subCmd.Description
}

// AllSubcommandPaths returns all subcommand paths for testing.
func AllSubcommandPaths() [][]string {
	var paths [][]string
	for _, cmd := range commandTree() {
		for _, sub := range cmd.Subcommands {
			path := []string{cmd.Name, sub.Name}
			// Add required args for commands that need them
			if cmd.Name == "agent" && sub.Name == "use" {
				path = append(path, "dummy-agent", "1.0.0")
			}
			paths = append(paths, path)
		}
	}
	return paths
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

// RejectUnknownFlags validates args and prints error if unknown flag found.
func RejectUnknownFlags(args []string) int {
	return RejectUnknownFlagsWithAllowed(args, nil)
}

// RejectUnknownFlagsWithAllowed validates args against allowed flags.
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
