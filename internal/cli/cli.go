package cli

import (
	"fmt"
	"os"
)

type App struct {
	Version string
}

func Run(args []string, version string) int {
	app := &App{Version: version}

	if len(args) == 0 {
		app.printHelp()
		return 0
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "init":
		return app.cmdInit(cmdArgs)
	case "run":
		return app.cmdRun(cmdArgs)
	case "agents":
		return app.cmdAgents(cmdArgs)
	case "clean":
		return app.cmdClean(cmdArgs)
	case "completions":
		return app.cmdCompletions(cmdArgs)
	case "help", "-h", "--help":
		app.printHelp()
		return 0
	case "version", "-v", "--version":
		fmt.Println(version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		app.printHelp()
		return 1
	}
}

func (a *App) printHelp() {
	fmt.Printf(`agentbox %s - CLI tool for running AI agents in Docker sandbox

Usage:
  agentbox <command> [options]

Commands:
  init                    Initialize sandbox in current directory
  run [container-id]      Start the container or attach to existing
    --build               Rebuild image before running
    --build-no-cache      Rebuild image without Docker cache
  agents                  Show agents status (installed vs latest)
    update [agent...]     Update agents (all or specified)
    use <agent> <version> Switch agent to specific version
  clean                   Remove sandbox files from project
  completions <shell>     Output shell completions (bash/zsh)
  help                    Show this help
  version                 Show version

Examples:
  agentbox init                          # Initialize sandbox in current directory
  agentbox run                           # Start the container
  agentbox run 9179da2caec4              # Attach to running container
  agentbox run --build                   # Rebuild image and start
  agentbox run --build-no-cache          # Rebuild from scratch
  agentbox agents                        # Show agents status
  agentbox agents update                 # Update all agents
  agentbox agents update claude copilot  # Update specific agents
  agentbox agents use claude 2.0.67      # Switch claude to specific version
`, a.Version)
}
