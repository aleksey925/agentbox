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
	case "attach":
		return app.cmdAttach(cmdArgs)
	case "ps":
		return app.cmdPs(cmdArgs)
	case "agent":
		return app.cmdAgent(cmdArgs)
	case "self":
		return app.cmdSelf(cmdArgs)
	case "clean":
		return app.cmdClean(cmdArgs)
	case "completion":
		return app.cmdCompletion(cmdArgs)
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
  init                              Initialize sandbox in current directory
  run                               Start a new container
  attach                            Attach to running container
  ps                                List running agentbox containers
  agent                             Manage AI agents
  self                              Update or uninstall agentbox
  clean                             Remove sandbox files from project
  completion                        Generate shell completion script

Global Flags:
  -h, --help                        Show help
  -v, --version                     Show version

Use "agentbox <command> --help" for more information about a command.
`, a.Version)
}
