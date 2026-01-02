package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksey925/agentbox/internal/agents"
	"github.com/aleksey925/agentbox/internal/config"
	"github.com/aleksey925/agentbox/internal/docker"
	"github.com/aleksey925/agentbox/internal/skeleton"
)

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func availableAgentsStr() string {
	return strings.Join(agents.AllAgentNames(), ", ")
}

func (a *App) cmdInit(args []string) int {
	if hasHelpFlag(args) {
		fmt.Print(`Initialize sandbox in current directory

Usage:
  agentbox init

This command creates Docker configuration files and downloads AI agent binaries.
Files created:
  - Dockerfile.agentbox
  - docker-compose.agentbox.yml
  - docker-compose.agentbox.local.yml
  - mise.toml (if not exists)
`)
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	return a.doInit(true)
}

func (a *App) doInit(interactive bool) int {
	paths, err := config.NewPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err = paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if interactive && !a.confirmOverwrite(cwd) {
		return 1
	}

	if code := a.copySkeletonFiles(cwd); code != 0 {
		return code
	}

	a.setupGitExclude(cwd)
	a.createMiseToml(cwd)

	if code := a.ensureAgentsInstalled(paths); code != 0 {
		return code
	}

	if err := ensureAgentConfigs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent configs: %v\n", err)
		return 1
	}

	fmt.Println("\nSandbox initialized successfully!")
	fmt.Println("Run 'agentbox run' to start the container.")

	return 0
}

func (a *App) confirmOverwrite(cwd string) bool {
	var existing []string
	for _, name := range skeleton.OverwriteFiles() {
		path := filepath.Join(cwd, name)
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, name)
		}
	}

	if len(existing) == 0 {
		return true
	}

	fmt.Printf("Warning: files already exist: %s\n", strings.Join(existing, ", "))
	fmt.Print("Overwrite? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted")
		return false
	}
	return true
}

func (a *App) copySkeletonFiles(cwd string) int {
	fmt.Println("Initializing agentbox...")

	if err := skeleton.CopyTo(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying skeleton files: %v\n", err)
		return 1
	}
	for _, name := range skeleton.OverwriteFiles() {
		fmt.Printf("  Created: %s\n", name)
	}

	createdUserFiles, err := skeleton.CopyUserFilesIfMissing(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying user files: %v\n", err)
		return 1
	}
	for _, name := range createdUserFiles {
		fmt.Printf("  Created: %s\n", name)
	}

	return 0
}

func (a *App) setupGitExclude(cwd string) {
	added, err := addToGitExcludeVerbose(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not update .git/info/exclude: %v\n", err)
		return
	}
	for _, name := range added {
		fmt.Printf("  Added to .git/info/exclude: %s\n", name)
	}
}

func (a *App) createMiseToml(cwd string) {
	misePath := filepath.Join(cwd, "mise.toml")
	if _, err := os.Stat(misePath); os.IsNotExist(err) {
		if err := createMiseTomlIfNotExists(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create mise.toml: %v\n", err)
		} else {
			fmt.Println("  Created: mise.toml")
		}
	}
}

type runOptions struct {
	build   bool
	noCache bool
}

var runAllowedFlags = []string{"--build", "--build-no-cache"}

func (a *App) cmdRun(args []string) int {
	if hasHelpFlag(args) {
		fmt.Print(`Start a new container

Usage:
  agentbox run [flags]

Flags:
  --build                           Rebuild image before running
  --build-no-cache                  Rebuild image without Docker cache
`)
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, runAllowedFlags); code != 0 {
		return code
	}

	opts := a.parseRunFlags(args)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if code := a.ensureProjectReady(cwd); code != 0 {
		return code
	}

	if opts.build {
		fmt.Println("Building Docker image...")
		if err := docker.Build(cwd, opts.noCache); err != nil {
			fmt.Fprintf(os.Stderr, "Error building image: %v\n", err)
			return 1
		}
	}

	fmt.Println("Starting agentbox...")
	if err := docker.Run(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Error running container: %v\n", err)
		return 1
	}

	return 0
}

// parseRunFlags parses run command flags.
// Assumes validation was already done by RejectUnknownFlagsWithAllowed.
func (a *App) parseRunFlags(args []string) runOptions {
	var opts runOptions
	for _, arg := range args {
		switch arg {
		case "--build":
			opts.build = true
		case "--build-no-cache":
			opts.build = true
			opts.noCache = true
		}
	}
	return opts
}

func (a *App) cmdAttach(args []string) int {
	if hasHelpFlag(args) {
		fmt.Print(`Attach to running container

Usage:
  agentbox attach [container-id]

Arguments:
  container-id                      Container ID (optional, interactive if omitted)

If no container ID is provided and multiple containers are running,
you will be prompted to select one.
`)
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	if len(args) > 0 {
		return a.attachToContainer(args[0])
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	containers, err := docker.ListContainers(cwd, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(containers) == 0 {
		fmt.Println("No running agentbox containers in this project")
		return 1
	}

	if len(containers) == 1 {
		return a.attachToContainer(containers[0].ID)
	}

	return a.selectAndAttach(containers)
}

func (a *App) selectAndAttach(containers []docker.Container) int {
	fmt.Println("Multiple running containers found:")
	for i, c := range containers {
		fmt.Printf("  %d) %s (started %s)\n", i+1, c.ID, c.Started)
	}
	fmt.Printf("Select [1-%d]: ", len(containers))

	var selection int
	if _, err := fmt.Scanf("%d", &selection); err != nil || selection < 1 || selection > len(containers) {
		fmt.Fprintln(os.Stderr, "Invalid selection")
		return 1
	}

	return a.attachToContainer(containers[selection-1].ID)
}

func (a *App) attachToContainer(containerID string) int {
	if err := docker.Attach(containerID); err != nil {
		fmt.Fprintf(os.Stderr, "Error attaching to container: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) ensureProjectReady(cwd string) int {
	composePath := filepath.Join(cwd, "docker-compose.agentbox.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		fmt.Println("Warning: not initialized, running init first...")
		if code := a.doInit(false); code != 0 {
			return code
		}
		fmt.Println()
	}

	paths, err := config.NewPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err = paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		return 1
	}

	if code := a.ensureAgentsInstalled(paths); code != 0 {
		return code
	}

	if err := ensureAgentConfigs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent configs: %v\n", err)
		return 1
	}

	return 0
}

func (a *App) cmdAgent(args []string) int {
	if len(args) > 0 && hasHelpFlag(args[:1]) {
		fmt.Printf(`Manage AI agents

Usage:
  agentbox agent [command]

Commands:
  (none)                            Show agent status (installed vs latest)
  update [agent...]                 Update agents (all or specified)
  use <agent> <version>             Switch agent to specific version

Available agents: %s

Examples:
  agentbox agent                    Show status of all agents
  agentbox agent update             Update all agents
  agentbox agent update claude      Update only Claude
  agentbox agent use claude 1.0.0   Switch Claude to version 1.0.0

Use "agentbox agent <command> --help" for more information about a command.
`, availableAgentsStr())
		return 0
	}

	// check for unknown flags at agent level (before subcommand dispatch)
	if len(args) > 0 {
		if code := RejectUnknownFlags(args[:1]); code != 0 {
			return code
		}
	}

	paths, err := config.NewPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	manager, err := agents.NewManager(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(args) == 0 {
		return a.showAgentStatus(manager)
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "update":
		return a.agentUpdate(manager, subargs)
	case "use":
		return a.agentUse(manager, subargs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown agent subcommand: %s\n", subcmd)
		return 1
	}
}

func (a *App) showAgentStatus(manager *agents.Manager) int {
	fmt.Println("\nFetching agent versions...")
	statuses := manager.GetStatus()

	table := NewTable("Agent", "Installed", "Latest", "Status")

	for _, status := range statuses {
		installed := status.Installed
		if installed == "" {
			installed = "-"
		}

		latest := status.Latest
		if status.Error != nil {
			latest = "error"
		}

		var statusStr string
		switch {
		case status.Error != nil:
			statusStr = "error fetching"
		case installed == "-":
			statusStr = "not installed"
		case status.UpToDate:
			statusStr = "up to date"
		default:
			statusStr = "update available"
		}

		table.AddRow(status.Name, installed, latest, statusStr)
	}

	fmt.Println()
	table.Render()
	fmt.Println()
	return 0
}

func (a *App) agentUpdate(manager *agents.Manager, args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`Update agents to latest version

Usage:
  agentbox agent update [agent...]

Arguments:
  agent                             Agent name(s) to update (optional, all if omitted)

Available agents: %s

Examples:
  agentbox agent update             Update all agents
  agentbox agent update claude      Update only Claude
  agentbox agent update claude copilot  Update Claude and Copilot
`, availableAgentsStr())
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	// all remaining args are agent names (flags already validated)
	agentsToUpdate := args

	fmt.Println("Updating agents...")

	results, err := manager.Update(agentsToUpdate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating agents: %v\n", err)
		return 1
	}

	fmt.Println()
	var failedCount int
	for _, result := range results {
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "  %s: error - %v\n", result.Agent, result.Error)
			failedCount++
		} else {
			fmt.Printf("  %s: updated to %s\n", result.Agent, result.Version)
		}
	}

	if failedCount > 0 {
		fmt.Fprintf(os.Stderr, "\nWarning: %d agent(s) failed to update\n", failedCount)
	}

	// cleanup old versions
	totalRemoved := 0
	for _, name := range agents.AllAgentNames() {
		removed, _ := manager.Cleanup(name)
		totalRemoved += removed
	}

	if totalRemoved > 0 {
		fmt.Printf("\nCleanup: removed %d old version(s)\n", totalRemoved)
	}

	return 0
}

func (a *App) agentUse(manager *agents.Manager, args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`Switch agent to specific version

Usage:
  agentbox agent use <agent> <version>

Arguments:
  agent                             Agent name
  version                           Version to switch to

Available agents: %s

Examples:
  agentbox agent use claude 1.0.0
  agentbox agent use copilot 0.5.0
`, availableAgentsStr())
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: agentbox agent use <agent> <version>\n")
		return 1
	}

	agentName := args[0]
	version := args[1]

	if err := manager.SwitchVersion(agentName, version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("%s switched to %s\n", agentName, version)
	return 0
}

var psAllowedFlags = []string{"-a", "--all"}

func (a *App) cmdPs(args []string) int {
	if hasHelpFlag(args) {
		fmt.Print(`List running agentbox containers

Usage:
  agentbox ps [flags]

Flags:
  -a, --all                         Show containers from all projects

By default, only containers from the current project directory are shown.
`)
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, psAllowedFlags); code != 0 {
		return code
	}

	showAll := false
	for _, arg := range args {
		if arg == "-a" || arg == "--all" {
			showAll = true
		}
	}

	var projectDir string
	if !showAll {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		projectDir = cwd
	}

	containers, err := docker.ListContainers(projectDir, showAll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(containers) == 0 {
		if showAll {
			fmt.Println("No running agentbox containers")
		} else {
			fmt.Println("No running agentbox containers in this project")
		}
		return 0
	}

	table := NewTable("CONTAINER ID", "NAME", "STARTED")
	for _, c := range containers {
		table.AddRow(c.ID, c.Name, c.Started)
	}
	table.Render()

	return 0
}

func (a *App) cmdClean(args []string) int {
	if hasHelpFlag(args) {
		fmt.Print(`Remove sandbox files from project

Usage:
  agentbox clean

This command removes all agentbox-generated files from the current directory:
  - Dockerfile.agentbox
  - docker-compose.agentbox.yml
  - docker-compose.agentbox.local.yml
`)
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Println("Cleaning agentbox files...")

	files := skeleton.Files()
	removed := 0
	for _, name := range files {
		path := filepath.Join(cwd, name)
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", name, err)
			}
		} else {
			fmt.Printf("Removed: %s\n", name)
			removed++
		}

		// remove from .git/info/exclude
		if err := removeFromGitExclude(cwd, name); err == nil {
			fmt.Printf("Removed from .git/info/exclude: %s\n", name)
		}
	}

	if removed == 0 {
		fmt.Println("No files to remove")
	} else {
		fmt.Printf("Cleaned %d file(s)\n", removed)
	}
	return 0
}

func addToGitExcludeVerbose(projectDir string) ([]string, error) {
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")

	if _, err := os.Stat(filepath.Join(projectDir, ".git")); os.IsNotExist(err) {
		return nil, nil
	}

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read exclude file: %w", err)
	}

	content := string(existing)
	files := skeleton.Files()

	var toAdd []string
	for _, name := range files {
		if !strings.Contains(content, name) {
			toAdd = append(toAdd, name)
		}
	}

	if len(toAdd) == 0 {
		return nil, nil
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open exclude file: %w", err)
	}
	defer f.Close()

	for _, name := range toAdd {
		if _, err := f.WriteString(name + "\n"); err != nil {
			return nil, fmt.Errorf("write to exclude file: %w", err)
		}
	}

	return toAdd, nil
}

func createMiseTomlIfNotExists(projectDir string) error {
	misePath := filepath.Join(projectDir, "mise.toml")

	if _, err := os.Stat(misePath); err == nil {
		return nil
	}

	if err := os.WriteFile(misePath, []byte{}, 0o644); err != nil {
		return fmt.Errorf("write mise.toml: %w", err)
	}
	return nil
}

func removeFromGitExclude(projectDir, filename string) error {
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")

	content, err := os.ReadFile(excludePath)
	if err != nil {
		return fmt.Errorf("read exclude file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))
	found := false

	for _, line := range lines {
		if strings.TrimSpace(line) == filename {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}

	if !found {
		return errors.New("not found")
	}

	if err := os.WriteFile(excludePath, []byte(strings.Join(newLines, "\n")), 0o644); err != nil {
		return fmt.Errorf("write exclude file: %w", err)
	}
	return nil
}

func (a *App) ensureAgentsInstalled(paths *config.Paths) int {
	manager, err := agents.NewManager(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if manager.HasInstalledAgents() {
		return 0
	}

	fmt.Println()
	fmt.Println("No agents installed. Downloading all agents...")

	results, err := manager.Update(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating agents: %v\n", err)
		return 1
	}

	fmt.Println()
	var failedCount int
	for _, result := range results {
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "  %s: error - %v\n", result.Agent, result.Error)
			failedCount++
		} else {
			fmt.Printf("  %s: %s installed\n", result.Agent, result.Version)
		}
	}

	if failedCount > 0 {
		fmt.Fprintf(os.Stderr, "\nWarning: %d agent(s) failed to download\n", failedCount)
	}

	fmt.Println()
	return 0
}

func ensureAgentConfigs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	// create ~/.claude.json if not exists (prevents Docker from creating it as directory)
	claudeJSON := filepath.Join(home, ".claude.json")
	if _, err := os.Stat(claudeJSON); os.IsNotExist(err) {
		if err := os.WriteFile(claudeJSON, []byte("{}"), 0o644); err != nil {
			return fmt.Errorf("write claude.json: %w", err)
		}
	}

	// create config directories if not exist
	dirs := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".copilot"),
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".gemini"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return nil
}
