package cli

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aleksey925/agentbox/internal/agents"
	"github.com/aleksey925/agentbox/internal/config"
	"github.com/aleksey925/agentbox/internal/docker"
	"github.com/aleksey925/agentbox/internal/skeleton"
	"github.com/charmbracelet/huh"
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
		fmt.Printf(`%s

Usage:
  agentbox init [command]

Commands:
  skeleton                          %s

Copies sandbox configurations into the project.

Files created:
  - .agentbox/core.v*.yml           Core Docker Compose configuration
  - .agentbox/<preset>.v*.yml       Environment preset configurations (Go, Python)
  - .agentbox/Dockerfile.agentbox   Dockerfile for the sandbox
  - .agentbox/local.yml             Project-specific overrides (not overwritten)
  - mise.toml (if not exists)       Tool versions configuration

On first run, you'll set up the base sandbox configuration.
Run 'agentbox init skeleton' to reconfigure.

Use "agentbox init skeleton --help" for more information.
`, CommandDesc("init"), SubcommandDesc("init", "skeleton"))
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	return a.doInit()
}

func (a *App) cmdInitSkeleton(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox init skeleton

Reinitializes the base sandbox configuration (~/.agentbox/skeleton/).
This configuration is used for project initialization with 'agentbox init'.

Use this to:
  - Change enabled presets (Go, Python)
  - Reset to defaults

Note: skeleton/ is fully recreated. Your custom presets in skeleton.local/ are preserved.
`, SubcommandDesc("init", "skeleton"))
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	paths, err := a.Paths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	manager := skeleton.NewManager(paths)

	// get previously enabled presets to pre-select them in TUI
	previousPresets, _ := manager.GetEnabledPresets()

	// run preset selection
	selectedPresets, canceled := a.selectPresets(paths.HomeDir, previousPresets)
	if canceled {
		fmt.Println("Canceled")
		return 0
	}

	// create new skeleton
	if err := manager.CreateSkeleton(selectedPresets); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating skeleton: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Println("Updated ~/.agentbox/skeleton/")

	return 0
}

func (a *App) doInit() int {
	paths, err := a.Paths()
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

	manager := skeleton.NewManager(paths)

	// create skeleton or auto-update if needed
	if code := a.ensureSkeletonReady(paths, manager); code != 0 {
		if code < 0 {
			return 0 // user canceled - exit gracefully
		}
		return code
	}

	// check if .agentbox/ already exists in project
	agentboxDir := filepath.Join(cwd, ".agentbox")
	if _, statErr := os.Stat(agentboxDir); statErr == nil {
		fmt.Println("Warning: .agentbox/ already exists and will be overwritten (except local.yml)")
		if !a.confirmAction("Continue?") {
			fmt.Println("Aborted")
			return 0
		}
	}

	// copy skeleton to project
	copiedFiles, err := manager.CopyToProject(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying skeleton to project: %v\n", err)
		return 1
	}
	fmt.Println("Created .agentbox/ (from skeleton)")
	for _, name := range copiedFiles {
		fmt.Printf("  %s\n", name)
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
	fmt.Println("Run 'agentbox run' to start the sandbox.")

	return 0
}

// ensureSkeletonReady creates skeleton if missing, or auto-updates if embedded versions are newer.
func (a *App) ensureSkeletonReady(paths *config.Paths, manager *skeleton.Manager) int {
	if !paths.SkeletonExists() {
		return a.createInitialSkeleton(paths, manager)
	}
	a.autoUpdateSkeleton(manager)
	return 0
}

func (a *App) createInitialSkeleton(paths *config.Paths, manager *skeleton.Manager) int {
	fmt.Println("No skeleton found. Let's set it up...")
	fmt.Println()
	fmt.Println("Detecting environment...")
	fmt.Println()

	selectedPresets, canceled := a.selectPresets(paths.HomeDir, nil)
	if canceled {
		fmt.Println("Canceled")
		return -1 // special code to indicate early exit without error
	}

	if err := manager.CreateSkeleton(selectedPresets); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating skeleton: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Println("Created ~/.agentbox/skeleton/")
	return 0
}

func (a *App) autoUpdateSkeleton(manager *skeleton.Manager) {
	updates, err := manager.CheckUpdates()
	if err != nil || len(updates) == 0 {
		return
	}

	currentPresets, _ := manager.GetEnabledPresets()
	if err := manager.CreateSkeleton(currentPresets); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-update skeleton: %v\n", err)
		return
	}

	fmt.Println("Updated skeleton:")
	for _, u := range updates {
		fmt.Printf("  %s: v%d -> v%d\n", u.Name, u.CurrentVersion, u.LatestVersion)
	}
	fmt.Println()
}

type presetSelectState struct {
	detections  []skeleton.DetectionResult
	names       map[string]string
	labels      map[string]string
	selectedSet map[string]bool
}

func newPresetSelectState(homeDir string, preSelected []string) *presetSelectState {
	detections := skeleton.DetectPresets(homeDir)
	preSelectedSet := sliceToSet(preSelected)

	state := &presetSelectState{
		detections:  detections,
		names:       make(map[string]string),
		labels:      make(map[string]string),
		selectedSet: make(map[string]bool),
	}

	for _, det := range detections {
		state.names[det.Preset.TemplateName] = det.Preset.Name
		state.labels[det.Preset.TemplateName] = buildLabel(det, preSelectedSet)
		state.selectedSet[det.Preset.TemplateName] = shouldSelect(det, preSelectedSet)
	}

	return state
}

func buildLabel(det skeleton.DetectionResult, preSelectedSet map[string]bool) string {
	label := det.Preset.Name
	if preSelectedSet[det.Preset.TemplateName] {
		label += " (current)"
	} else if det.Detected && det.Reason != "" {
		label += fmt.Sprintf(" (detected: %s)", det.Reason)
	}
	return label
}

func shouldSelect(det skeleton.DetectionResult, preSelectedSet map[string]bool) bool {
	if len(preSelectedSet) > 0 {
		return preSelectedSet[det.Preset.TemplateName]
	}
	return det.Detected
}

func (s *presetSelectState) buildOptions() []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(s.detections))

	// selected items first
	for _, det := range s.detections {
		if s.selectedSet[det.Preset.TemplateName] {
			opt := huh.NewOption(s.labels[det.Preset.TemplateName], det.Preset.TemplateName).Selected(true)
			options = append(options, opt)
		}
	}
	// then unselected
	for _, det := range s.detections {
		if !s.selectedSet[det.Preset.TemplateName] {
			opt := huh.NewOption(s.labels[det.Preset.TemplateName], det.Preset.TemplateName)
			options = append(options, opt)
		}
	}
	return options
}

func (s *presetSelectState) updateSelection(selected []string) {
	s.selectedSet = sliceToSet(selected)
}

func (s *presetSelectState) printSelection(selected []string) {
	fmt.Println()
	if len(selected) == 0 {
		fmt.Println("No presets enabled.")
	} else {
		fmt.Println("Enabled presets:")
		for _, preset := range selected {
			fmt.Printf("  - %s\n", s.names[preset])
		}
	}
	fmt.Println()
}

// selectPresets shows interactive preset selection UI.
// Returns selected presets and canceled flag (true if user pressed Ctrl+C).
func (a *App) selectPresets(homeDir string, preSelected []string) ([]string, bool) {
	state := newPresetSelectState(homeDir, preSelected)

	for {
		var selected []string
		err := huh.NewMultiSelect[string]().
			Title("Configure sandbox").
			Description("Select your development tools — sandbox will mount their caches and configs").
			Options(state.buildOptions()...).
			Value(&selected).
			Run()

		if err != nil {
			return nil, true
		}

		state.updateSelection(selected)

		var confirm bool
		err = huh.NewConfirm().
			Title("Continue with this selection?").
			Affirmative("Yes, continue").
			Negative("No, edit selection").
			Value(&confirm).
			Run()

		if err != nil {
			return nil, true
		}

		if confirm {
			state.printSelection(selected)
			return selected, false
		}
	}
}

func sliceToSet(slice []string) map[string]bool {
	set := make(map[string]bool)
	for _, s := range slice {
		set[s] = true
	}
	return set
}

func (a *App) confirmAction(prompt string) bool {
	fmt.Printf("%s [Y/n] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		// EOF or read error - don't auto-confirm
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
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
		fmt.Printf(`%s

Usage:
  agentbox run [flags]

Flags:
  --build                           Rebuild image before running
  --build-no-cache                  Rebuild image without Docker cache
`, CommandDesc("run"))
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

	// discover compose files
	composeFiles, err := docker.DiscoverComposeFiles(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering compose files: %v\n", err)
		return 1
	}

	if opts.build {
		fmt.Println("Building Docker image...")
		if err := docker.Build(cwd, composeFiles, opts.noCache); err != nil {
			fmt.Fprintf(os.Stderr, "Error building image: %v\n", err)
			return 1
		}
	}

	fmt.Println("Starting agentbox...")
	if err := docker.Run(cwd, composeFiles); err != nil {
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
		fmt.Printf(`%s

Usage:
  agentbox attach [container-id]

Arguments:
  container-id                      Container ID (optional, auto-select if only one)

If multiple sandboxes are running, you will be prompted to select one.
`, CommandDesc("attach"))
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
		fmt.Println("No running sandboxes in this project")
		return 1
	}

	if len(containers) == 1 {
		return a.attachToContainer(containers[0].ID)
	}

	return a.selectAndAttach(containers)
}

func (a *App) selectAndAttach(containers []docker.Container) int {
	fmt.Println("Multiple running sandboxes found:")
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
	agentboxDir := filepath.Join(cwd, ".agentbox")
	if _, err := os.Stat(agentboxDir); os.IsNotExist(err) {
		fmt.Println("Warning: not initialized, running init first...")
		if code := a.doInit(); code != 0 {
			return code
		}
		fmt.Println()
	}

	paths, err := a.Paths()
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

func (a *App) cmdAgentStatus(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

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
`, CommandDesc("agent"), availableAgentsStr())
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	manager, err := a.AgentManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	return a.showAgentStatus(manager)
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

func (a *App) cmdAgentUpdate(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox agent update [agent...]

Arguments:
  agent                             Agent name(s) to update (optional, all if omitted)

Available agents: %s

Examples:
  agentbox agent update             Update all agents
  agentbox agent update claude      Update only Claude
  agentbox agent update claude copilot  Update Claude and Copilot
`, SubcommandDesc("agent", "update"), availableAgentsStr())
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	manager, err := a.AgentManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
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

func (a *App) cmdAgentUse(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox agent use <agent> <version>

Arguments:
  agent                             Agent name
  version                           Version to switch to

Available agents: %s

Examples:
  agentbox agent use claude 1.0.0
  agentbox agent use copilot 0.5.0
`, SubcommandDesc("agent", "use"), availableAgentsStr())
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: agentbox agent use <agent> <version>\n")
		return 1
	}

	manager, err := a.AgentManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		fmt.Printf(`%s

Usage:
  agentbox ps [flags]

Flags:
  -a, --all                         Show sandboxes from all projects

By default, only sandboxes from the current project are shown.
`, CommandDesc("ps"))
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
			fmt.Println("No running sandboxes")
		} else {
			fmt.Println("No running sandboxes in this project")
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
		fmt.Printf(`%s

Usage:
  agentbox clean

This command removes the .agentbox/ directory from the current project.
`, CommandDesc("clean"))
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

	agentboxDir := filepath.Join(cwd, ".agentbox")
	if _, err := os.Stat(agentboxDir); os.IsNotExist(err) {
		fmt.Println("No .agentbox/ directory found")
		return 0
	}

	if err := os.RemoveAll(agentboxDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing .agentbox/: %v\n", err)
		return 1
	}

	fmt.Println("Removed: .agentbox/")

	// remove from .git/info/exclude
	for _, entry := range skeleton.GitExcludeEntries() {
		if err := removeFromGitExclude(cwd, entry); err == nil {
			fmt.Printf("Removed from .git/info/exclude: %s\n", entry)
		}
	}

	fmt.Println("Cleaned successfully")
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
	entries := skeleton.GitExcludeEntries()

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
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

	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
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

func removeFromGitExclude(projectDir, entry string) error {
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")

	content, err := os.ReadFile(excludePath)
	if err != nil {
		return fmt.Errorf("read exclude file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))
	found := false

	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
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

const githubRepo = "aleksey925/agentbox"

// Note: cmdSelf is not used directly - "self" command requires subcommand
// The help is shown via dispatcher when no subcommand is provided

func (a *App) cmdSelfUpdate(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox self update [version]

Arguments:
  version                           Target version (default: latest)

Examples:
  agentbox self update              Update to latest version
  agentbox self update 1.2.0        Update to version 1.2.0
`, SubcommandDesc("self", "update"))
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	var targetVersion string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			targetVersion = arg
			break
		}
	}

	if targetVersion == "" {
		fmt.Println("Fetching latest version...")
		latest, err := fetchLatestVersion()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching latest version: %v\n", err)
			return 1
		}
		targetVersion = latest
	}

	targetVersion = strings.TrimPrefix(targetVersion, "v")

	if targetVersion == a.Version {
		fmt.Printf("Already at version %s\n", targetVersion)
		return 0
	}

	fmt.Printf("Updating to version %s...\n", targetVersion)

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		return 1
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		return 1
	}

	downloadURL := fmt.Sprintf(
		"https://github.com/%s/releases/download/v%s/agentbox_%s_%s_%s.tar.gz",
		githubRepo, targetVersion, targetVersion, runtime.GOOS, runtime.GOARCH,
	)

	fmt.Printf("Downloading from %s\n", downloadURL)

	tmpDir, err := os.MkdirTemp("", "agentbox-update-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	resp, cancel, err := httpDownload(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading: %v\n", err)
		return 1
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		fmt.Fprintf(os.Stderr, "Error: download failed with status %d\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "Version %s may not exist for %s/%s\n", targetVersion, runtime.GOOS, runtime.GOARCH)
		return 1
	}

	newBinaryPath := filepath.Join(tmpDir, "agentbox")
	err = extractBinaryFromTarGz(resp.Body, newBinaryPath)
	resp.Body.Close()
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting archive: %v\n", err)
		return 1
	}

	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error backing up current binary: %v\n", err)
		return 1
	}

	if err := copyFile(newBinaryPath, execPath); err != nil {
		// restore backup
		_ = os.Rename(backupPath, execPath)
		fmt.Fprintf(os.Stderr, "Error replacing binary: %v\n", err)
		return 1
	}

	_ = os.Remove(backupPath)

	fmt.Printf("Successfully updated to version %s\n", targetVersion)
	return 0
}

func (a *App) cmdSelfUninstall(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox self uninstall [flags]

Flags:
  --purge                           Also remove ~/.agentbox directory
`, SubcommandDesc("self", "uninstall"))
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, SelfUninstallFlags()); code != 0 {
		return code
	}

	purge := false
	for _, arg := range args {
		if arg == "--purge" {
			purge = true
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		return 1
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		return 1
	}

	fmt.Printf("This will remove: %s\n", execPath)
	if purge {
		home, _ := os.UserHomeDir()
		fmt.Printf("This will also remove: %s\n", filepath.Join(home, ".agentbox"))
	}

	fmt.Print("Continue? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted")
		return 0
	}

	if purge {
		home, err := os.UserHomeDir()
		if err == nil {
			agentboxDir := filepath.Join(home, ".agentbox")
			if err := os.RemoveAll(agentboxDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", agentboxDir, err)
			} else {
				fmt.Printf("Removed %s\n", agentboxDir)
			}
		}
	}

	if err := os.Remove(execPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing binary: %v\n", err)
		return 1
	}

	fmt.Println("agentbox has been uninstalled")
	return 0
}

func (a *App) cmdSelfVersions(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox self versions
`, SubcommandDesc("self", "versions"))
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	versions, err := fetchVersions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching versions: %v\n", err)
		return 1
	}

	for _, v := range versions {
		fmt.Println(v)
	}
	return 0
}

func fetchVersions() ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=30", githubRepo)
	resp, cancel, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	versions := make([]string, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, strings.TrimPrefix(r.TagName, "v"))
	}
	return versions, nil
}

func fetchLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, cancel, err := httpGet(url)
	if err != nil {
		return "", fmt.Errorf("fetch releases: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

const (
	httpTimeout         = 30 * time.Second
	httpDownloadTimeout = 5 * time.Minute
)

// httpGet performs a GET request with standard timeout.
func httpGet(url string) (resp *http.Response, cancel context.CancelFunc, err error) {
	return httpGetWithTimeout(url, httpTimeout)
}

// httpDownload performs a GET request with extended timeout for file downloads.
func httpDownload(url string) (resp *http.Response, cancel context.CancelFunc, err error) {
	return httpGetWithTimeout(url, httpDownloadTimeout)
}

func httpGetWithTimeout(url string, timeout time.Duration) (resp *http.Response, cancel context.CancelFunc, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("execute request: %w", err)
	}
	return resp, cancel, nil
}

func extractBinaryFromTarGz(r io.Reader, destPath string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "agentbox" {
			if err := extractFile(tr, destPath); err != nil {
				return err
			}
			return nil
		}
	}

	return errors.New("agentbox binary not found in archive")
}

func extractFile(r io.Reader, destPath string) error {
	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, r); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	if err := dstFile.Chmod(0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	return nil
}
