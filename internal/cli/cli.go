package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/aleksey925/agentbox/internal/agents"
	"github.com/aleksey925/agentbox/internal/config"
)

// App holds CLI state and provides lazy-cached resources.
type App struct {
	Version string

	// lazy-cached resources
	paths        *config.Paths
	agentManager *agents.Manager
}

// Paths returns config paths (lazy-initialized and cached).
func (a *App) Paths() (*config.Paths, error) {
	if a.paths != nil {
		return a.paths, nil
	}
	var err error
	a.paths, err = config.NewPaths()
	if err != nil {
		return nil, fmt.Errorf("create paths: %w", err)
	}
	return a.paths, nil
}

// AgentManager returns agent manager (lazy-initialized and cached).
func (a *App) AgentManager() (*agents.Manager, error) {
	if a.agentManager != nil {
		return a.agentManager, nil
	}
	paths, err := a.Paths()
	if err != nil {
		return nil, err
	}
	a.agentManager, err = agents.NewManager(paths)
	if err != nil {
		return nil, fmt.Errorf("create agent manager: %w", err)
	}
	return a.agentManager, nil
}

// Run is the main entry point for CLI.
func Run(args []string, version string) int {
	app := &App{Version: version}
	return app.dispatch(args)
}

// dispatch routes commands based on commandTree.
func (a *App) dispatch(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return 0
	}

	cmdName := args[0]
	cmdArgs := args[1:]

	// Handle global flags/aliases
	switch cmdName {
	case "-h", "--help":
		a.printHelp()
		return 0
	case "-v", "--version":
		fmt.Println(a.Version)
		return 0
	}

	cmd := FindCommand(commandTree(), cmdName)
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmdName)
		a.printHelp()
		return 1
	}

	// Special handling for help and version commands
	if cmd.Name == "help" {
		a.printHelp()
		return 0
	}
	if cmd.Name == "version" {
		fmt.Println(a.Version)
		return 0
	}

	return a.executeCommand(cmd, cmdArgs)
}

// executeCommand runs a command, handling subcommand dispatch.
func (a *App) executeCommand(cmd *Command, args []string) int {
	// Check for subcommand first (before help flag check)
	if len(cmd.Subcommands) > 0 && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if sub := FindCommand(cmd.Subcommands, args[0]); sub != nil {
			return a.executeCommand(sub, args[1:])
		}
	}

	// Handle help for commands without handler (parent-only commands like "self")
	if cmd.Handler == nil {
		if hasHelpFlag(args) {
			a.printParentCommandHelp(cmd)
			return 0
		}
		// Command requires subcommand but none provided
		fmt.Fprintf(os.Stderr, "Usage: agentbox %s <command>\n", cmd.Name)
		fmt.Fprintf(os.Stderr, "Run 'agentbox %s --help' for available commands.\n", cmd.Name)
		return 1
	}

	return cmd.Handler(a, args)
}

// printParentCommandHelp prints help for commands that only have subcommands.
func (a *App) printParentCommandHelp(cmd *Command) {
	fmt.Printf("%s\n\nUsage:\n  agentbox %s <command>\n\nCommands:\n", cmd.Description, cmd.Name)
	for _, sub := range cmd.Subcommands {
		fmt.Printf("  %-36s%s\n", sub.Name, sub.Description)
	}
	fmt.Printf("\nUse \"agentbox %s <command> --help\" for more information about a command.\n", cmd.Name)
}

func (a *App) printHelp() {
	fmt.Printf(`agentbox %s - CLI tool for running AI agents in Docker sandbox

Usage:
  agentbox <command> [options]

Commands:
`, a.Version)

	for _, cmd := range commandTree() {
		// Skip help and version from commands list (they're in Global Flags)
		if cmd.Name == "help" || cmd.Name == "version" {
			continue
		}
		fmt.Printf("  %-36s%s\n", cmd.Name, cmd.Description)
	}

	fmt.Print(`
Global Flags:
  -h, --help                        Show help
  -v, --version                     Show version

Use "agentbox <command> --help" for more information about a command.
`)
}
