package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aleksey925/agentbox/internal/agentflags"
	"github.com/aleksey925/agentbox/internal/agents"
	"github.com/aleksey925/agentbox/internal/config"
	"github.com/aleksey925/agentbox/internal/docker"
	"github.com/aleksey925/agentbox/internal/download"
	"github.com/aleksey925/agentbox/internal/skeleton"
	"github.com/charmbracelet/huh"
)

// exit codes
const (
	exitOK       = 0
	exitError    = 1
	exitCanceled = -1 // user canceled operation, exit gracefully
)

// toShellExit converts internal exit codes to shell-compatible codes.
func toShellExit(code int) int {
	if code == exitCanceled {
		return exitOK
	}
	return code
}

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
  - .agentbox/Dockerfile.v*.agentbox Dockerfile for the sandbox
  - .agentbox/local.yml             Project-specific overrides (not overwritten)
  - mise.toml (if not exists)       Tool versions configuration

On first run, you'll set up the base sandbox configuration.
Run 'agentbox init skeleton --force' to reconfigure.

Use "agentbox init skeleton --help" for more information.
`, CommandDesc("init"), SubcommandDesc("init", "skeleton"))
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	return toShellExit(a.doInit())
}

func (a *App) cmdInitSkeleton(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox init skeleton [flags]

Flags:
  -f, --force                       Force reinitialize even if skeleton exists

Reinitializes the base sandbox configuration (~/.agentbox/skeleton/).
This configuration is used for project initialization with 'agentbox init'.

Use this to:
  - Change enabled presets (Go, Python)
  - Reset to defaults

Without --force, skeleton will only be initialized if it doesn't exist.
With --force, existing skeleton is deleted and recreated after confirmation.
`, SubcommandDesc("init", "skeleton"))
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, InitSkeletonFlags()); code != 0 {
		return code
	}

	// parse --force flag
	force := false
	for _, arg := range args {
		if arg == "-f" || arg == "--force" {
			force = true
		}
	}

	paths, err := a.Paths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	manager := skeleton.NewManager(paths)

	// get previously enabled presets BEFORE any changes (for pre-selection in TUI)
	previousPresets, _ := manager.GetEnabledPresets()

	// check if skeleton already exists
	if skeleton.HasRealFiles(paths.SkeletonDir) && !force {
		fmt.Println("Skeleton already exists at ~/.agentbox/skeleton/")
		fmt.Println("Use --force to recreate it, or remove/move manually.")
		return 1
	}

	// run preset selection with pre-selected presets
	selectedPresets, canceled := a.selectPresets(paths.HomeDir, previousPresets)
	if canceled {
		fmt.Println("Canceled")
		return 0
	}

	// create new skeleton (only after user confirmation via TUI)
	if err := manager.CreateSkeleton(selectedPresets); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating skeleton: %v\n", err)
		return 1
	}

	fmt.Println()
	if force {
		fmt.Println("Recreated ~/.agentbox/skeleton/")
	} else {
		fmt.Println("Created ~/.agentbox/skeleton/")
	}

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

	// create skeleton if missing
	if code := a.ensureSkeletonReady(paths, manager); code != exitOK {
		return code
	}

	// only guard a working config: an empty or local.yml-only .agentbox/ is not
	// something the user would mind reseeding, and CopyToProject keeps local.yml.
	agentboxDir := filepath.Join(cwd, ".agentbox")
	if skeleton.ProjectInitialized(agentboxDir) {
		fmt.Println("Warning: .agentbox/ already exists and will be overwritten (except local.yml)")
		if !a.confirmAction("Continue?") {
			fmt.Println("Aborted")
			return 0
		}
	}

	// a pre-existing local.yml is kept as is and silently customizes the sandbox
	// (extra mounts, env) - in a freshly cloned repo it may come from the repo
	// author, not the user, so point at it explicitly.
	localYmlPath := filepath.Join(agentboxDir, "local.yml")
	_, localYmlErr := os.Lstat(localYmlPath)
	keptLocalYml := localYmlErr == nil

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
	if keptLocalYml {
		fmt.Println("Warning: kept existing .agentbox/local.yml - it adds mounts and")
		fmt.Println("environment to the sandbox; review it if you did not create it.")
	}

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

// ensureSkeletonReady creates the skeleton if missing, and refuses to seed a
// project from a stale one: init must not produce a project the run gate would
// immediately reject.
func (a *App) ensureSkeletonReady(paths *config.Paths, manager *skeleton.Manager) int {
	if !paths.SkeletonExists() {
		return a.createInitialSkeleton(paths, manager)
	}

	status, err := skeleton.CheckVersion(paths.SkeletonDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}
	switch status {
	case skeleton.VersionCurrent:
		return exitOK
	case skeleton.VersionOutdated:
		fmt.Fprintln(os.Stderr, "Error: the global skeleton is older than this agentbox.")
		fmt.Fprintln(os.Stderr, "Run 'agentbox init skeleton --force' to refresh it, then re-run init.")
		return exitError
	case skeleton.VersionAhead:
		fmt.Fprintln(os.Stderr, "Error: the global skeleton is newer than this agentbox.")
		fmt.Fprintln(os.Stderr, "Update agentbox with 'agentbox self update'.")
		return exitError
	}
	return exitOK
}

func (a *App) createInitialSkeleton(paths *config.Paths, manager *skeleton.Manager) int {
	fmt.Println("No skeleton found. Let's set it up...")
	fmt.Println()
	fmt.Println("Detecting environment...")
	fmt.Println()

	selectedPresets, canceled := a.selectPresets(paths.HomeDir, nil)
	if canceled {
		fmt.Println("Canceled")
		return exitCanceled
	}

	if err := manager.CreateSkeleton(selectedPresets); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating skeleton: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Println("Created ~/.agentbox/skeleton/")
	return 0
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
	build     bool
	noCache   bool
	forceNew  bool
	container string
}

func (a *App) cmdRun(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox run [flags]

Flags:
  --build                           Rebuild image before running (implies --new)
  --build-no-cache                  Rebuild image without Docker cache (implies --new)
  --new                             Force a new container even if one is running
  --container <name|id>             Attach to a specific container by name or ID

By default run attaches to a running sandbox for this project if one exists,
otherwise it starts a new one.
`, CommandDesc("run"))
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, CommandFlags()["run"]); code != 0 {
		return code
	}

	opts := a.parseRunFlags(args)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if opts.container != "" {
		return a.attachToContainer(opts.container)
	}

	// --build implies --new: a live container can't pick up a freshly built image
	if !opts.forceNew && !opts.build {
		containers, err := docker.ListContainers(cwd, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if len(containers) > 0 {
			return a.attachToContainer(containers[0].ID)
		}
	}

	return a.startNewContainer(cwd, opts)
}

func (a *App) startNewContainer(cwd string, opts runOptions) int {
	if code := a.ensureProjectReady(cwd); code != exitOK {
		return toShellExit(code)
	}

	// ensure shared volumes exist (prevents "volume created for different project" warning)
	if err := docker.EnsureSharedVolumes(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating shared volumes: %v\n", err)
		return 1
	}

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
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--build":
			opts.build = true
		case "--build-no-cache":
			opts.build = true
			opts.noCache = true
		case "--new":
			opts.forceNew = true
		case "--container":
			if i+1 < len(args) {
				opts.container = args[i+1]
				i++
			}
		}
	}
	return opts
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
	if !skeleton.ProjectInitialized(agentboxDir) {
		fmt.Println("Warning: not initialized or incomplete, running init first...")
		if code := a.doInit(); code != 0 {
			return code
		}
		fmt.Println()
	} else if code := checkConfigVersion(agentboxDir); code != exitOK {
		return code
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

	if err := agentflags.EnsureFile(paths.FlagsFile()); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent flags file: %v\n", err)
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

// checkConfigVersion fails loudly when a project's sandbox config does not match
// the schema this binary ships. Migration is explicit (CLAUDE.md "Updates are
// explicit"): the new binary cannot run an old compose, so it refuses and points
// at 'agentbox upgrade' instead of silently mounting a mismatched sandbox.
func checkConfigVersion(agentboxDir string) int {
	status, err := skeleton.CheckVersion(agentboxDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	switch status {
	case skeleton.VersionCurrent:
		return exitOK
	case skeleton.VersionOutdated:
		fmt.Fprintln(os.Stderr, "Error: this project's sandbox config is older than this agentbox.")
		fmt.Fprintln(os.Stderr, "Run 'agentbox upgrade' to migrate it.")
		return exitError
	case skeleton.VersionAhead:
		fmt.Fprintln(os.Stderr, "Error: this project's sandbox config is newer than this agentbox.")
		fmt.Fprintln(os.Stderr, "Update agentbox with 'agentbox self update'.")
		return exitError
	}
	return exitOK
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
  flags                             Edit flags agents are launched with

Available agents: %s

Examples:
  agentbox agent                    Show status of all agents
  agentbox agent update             Update all agents
  agentbox agent update claude      Update only Claude
  agentbox agent use claude 1.0.0   Switch Claude to version 1.0.0
  agentbox agent flags              Edit agent launch flags

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

	var failed []agents.DownloadResult
	for _, result := range results {
		if result.Error != nil {
			failed = append(failed, result)
		}
	}

	if len(failed) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, result := range failed {
			fmt.Fprintf(os.Stderr, "%s: %v\n", result.Agent, result.Error)
		}
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

var agentFlagsAllowedFlags = []string{"--show", "--path"}

func (a *App) cmdAgentFlags(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox agent flags [flags]

Flags:
  --show                            Print current flags instead of opening editor
  --path                            Print path to the flags file

Opens the global agent flags file in $VISUAL (falls back to $EDITOR, then vi).
Edits apply to the next agent launch — even inside a running sandbox — with no
image rebuild.

Examples:
  agentbox agent flags              Open the flags file in your editor
  agentbox agent flags --show       Print current flags
  agentbox agent flags --path       Print the flags file path
`, SubcommandDesc("agent", "flags"))
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, agentFlagsAllowedFlags); code != 0 {
		return code
	}

	paths, err := a.Paths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	flagsFile := paths.FlagsFile()
	if err := agentflags.EnsureFile(flagsFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	for _, arg := range args {
		switch arg {
		case "--path":
			fmt.Println(flagsFile)
			return 0
		case "--show":
			content, err := os.ReadFile(flagsFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			fmt.Print(string(content))
			return 0
		}
	}

	return openInEditor(flagsFile)
}

// openInEditor opens path in the user's editor, resolved from $VISUAL, then
// $EDITOR, then vi. The editor value may contain arguments (e.g. "code --wait").
func openInEditor(path string) int {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}

	// strings.Fields handles both unset and whitespace-only env values: an empty
	// result falls back to vi, so parts[0] below never indexes an empty slice.
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	cmdArgs := make([]string, 0, len(parts))
	cmdArgs = append(cmdArgs, parts[1:]...)
	cmdArgs = append(cmdArgs, path)
	cmd := exec.CommandContext(context.Background(), parts[0], cmdArgs...) // #nosec G204 -- editor comes from user's own env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening editor: %v\n", err)
		return 1
	}
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
	fmt.Println("Cleaned successfully")
	return 0
}

var upgradeAllowedFlags = []string{"--depth"}

const defaultScanDepth = 1

func (a *App) cmdUpgrade(args []string) int {
	if hasHelpFlag(args) {
		fmt.Printf(`%s

Usage:
  agentbox upgrade [path] [flags]

Flags:
  --depth <n>                       Directory levels to scan under <path> (default %d)

Regenerates the global skeleton at this agentbox version (keeping your enabled
presets) and reseeds project configs (.agentbox/, local.yml preserved).

Without a path, only the current project is upgraded and its image is rebuilt.
With a path, agentbox scans it for projects and reseeds each, then drops the
shared sandbox image so every project rebuilds on its next 'agentbox run'.

Examples:
  agentbox upgrade                  Upgrade the current project
  agentbox upgrade ~/CodeProjects   Upgrade every project found there
  agentbox upgrade ~/Code --depth 2 Scan two levels deep
`, CommandDesc("upgrade"), defaultScanDepth)
		return 0
	}

	if code := RejectUnknownFlagsWithAllowed(args, upgradeAllowedFlags); code != 0 {
		return code
	}

	scanPath, depth, code := parseUpgradeArgs(args)
	if code != exitOK {
		return code
	}

	paths, err := a.Paths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// upgrade derives presets from the existing skeleton; regenerating from a
	// missing one would silently drop every project's presets on reseed.
	if !paths.SkeletonExists() {
		fmt.Fprintln(os.Stderr, "Error: no skeleton found. Run 'agentbox init skeleton' first.")
		return exitError
	}

	manager := skeleton.NewManager(paths)
	if scanPath == "" {
		return a.upgradeCurrentProject(manager)
	}
	return a.upgradeScan(manager, scanPath, depth)
}

// parseUpgradeArgs extracts the optional scan path and --depth value.
func parseUpgradeArgs(args []string) (path string, depth, code int) {
	depth = defaultScanDepth
	depthGiven := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--depth":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --depth requires a value")
				return "", 0, exitError
			}
			d, err := strconv.Atoi(args[i+1])
			if err != nil || d < 1 {
				fmt.Fprintf(os.Stderr, "Error: invalid --depth %q (want a positive integer)\n", args[i+1])
				return "", 0, exitError
			}
			depth, depthGiven = d, true
			i++
		case strings.HasPrefix(args[i], "-"):
			// --depth is consumed above; any other flag is rejected upstream, so a
			// flag-shaped token reaching here is never a path - skip it.
		case path != "":
			fmt.Fprintln(os.Stderr, "Error: upgrade takes at most one path")
			return "", 0, exitError
		default:
			path = args[i]
		}
	}
	if depthGiven && path == "" {
		fmt.Fprintln(os.Stderr, "Error: --depth applies only when scanning a path")
		return "", 0, exitError
	}
	return path, depth, exitOK
}

// upgradeCurrentProject reseeds the cwd project and rebuilds its image so it is
// immediately runnable, without disturbing other projects' images.
func (a *App) upgradeCurrentProject(manager *skeleton.Manager) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if !skeleton.ProjectInitialized(filepath.Join(cwd, ".agentbox")) {
		fmt.Fprintln(os.Stderr, "Error: current directory is not an initialized agentbox project.")
		fmt.Fprintln(os.Stderr, "Run 'agentbox init' here, or pass a path: 'agentbox upgrade <path>'.")
		return exitError
	}

	if code := regenerateSkeleton(manager); code != exitOK {
		return code
	}
	if _, err := manager.CopyToProject(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Error reseeding project: %v\n", err)
		return 1
	}
	fmt.Printf("Reseeded %s\n", cwd)

	fmt.Println("Building image...")
	if err := buildProjectImage(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: build failed: %v\n", err)
		fmt.Println("Run 'agentbox run --build' to retry.")
		return 0
	}

	fmt.Println("\nDone. Run 'agentbox run' to start.")
	return 0
}

// upgradeScan reseeds every project found under root and drops the shared image
// so each rebuilds lazily on its next run.
func (a *App) upgradeScan(manager *skeleton.Manager, root string, depth int) int {
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", root)
		return exitError
	}

	if code := regenerateSkeleton(manager); code != exitOK {
		return code
	}

	projects := scanProjects(root, depth)
	if len(projects) == 0 {
		fmt.Printf("No agentbox projects found under %s (depth %d).\n", root, depth)
	}
	for _, dir := range projects {
		if _, err := manager.CopyToProject(dir); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", dir, err)
			continue
		}
		fmt.Printf("  reseeded %s\n", dir)
	}

	if err := docker.RemoveSandboxImage(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not drop sandbox image: %v\n", err)
	}

	fmt.Println("\nDone. Each project rebuilds on its next 'agentbox run'.")
	return 0
}

func regenerateSkeleton(manager *skeleton.Manager) int {
	presets, err := manager.GetEnabledPresets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading skeleton presets: %v\n", err)
		return exitError
	}
	if err := manager.CreateSkeleton(presets); err != nil {
		fmt.Fprintf(os.Stderr, "Error regenerating skeleton: %v\n", err)
		return exitError
	}
	fmt.Println("Regenerated ~/.agentbox/skeleton/")
	return exitOK
}

func buildProjectImage(cwd string) error {
	if err := docker.EnsureSharedVolumes(); err != nil {
		return fmt.Errorf("ensure shared volumes: %w", err)
	}
	composeFiles, err := docker.DiscoverComposeFiles(cwd)
	if err != nil {
		return fmt.Errorf("discover compose files: %w", err)
	}
	if err := docker.Build(cwd, composeFiles, false); err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	return nil
}

// scanProjects walks root up to maxDepth levels deep and returns directories that
// are initialized agentbox projects. It does not descend into a found project,
// follow symlinks, or enter hidden directories.
func scanProjects(root string, maxDepth int) []string {
	var projects []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if skeleton.ProjectInitialized(filepath.Join(dir, ".agentbox")) {
			projects = append(projects, dir)
			return
		}
		if depth >= maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			walk(filepath.Join(dir, e.Name()), depth+1)
		}
	}
	walk(root, 0)
	return projects
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

	// create ~/.claude.sandbox.json if not exists (prevents Docker from creating it as directory)
	// sandbox uses separate config to avoid conflicts with host's ~/.claude.json
	claudeSandboxJSON := filepath.Join(home, ".claude.sandbox.json")
	if _, err := os.Stat(claudeSandboxJSON); os.IsNotExist(err) {
		if err := os.WriteFile(claudeSandboxJSON, []byte("{}"), 0o644); err != nil {
			return fmt.Errorf("write claude.sandbox.json: %w", err)
		}
	}

	// create ~/.gitconfig if not exists (prevents Docker from creating it as directory)
	// if user has global git config, this file already exists; otherwise create empty placeholder
	gitconfig := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(gitconfig); os.IsNotExist(err) {
		if err := os.WriteFile(gitconfig, []byte{}, 0o644); err != nil {
			return fmt.Errorf("write .gitconfig: %w", err)
		}
	}

	// create config directories if not exist
	for _, dirName := range agents.AgentConfigDirs() {
		dir := filepath.Join(home, dirName)
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

	if guardHomebrewManaged("upgrade") {
		return 1
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

	if err := config.ValidateVersion(targetVersion); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid version %q\n", targetVersion)
		return 1
	}

	if targetVersion == a.Build.Version {
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

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/v%s", githubRepo, targetVersion)
	assetName := fmt.Sprintf("agentbox_%s_%s_%s.tar.gz", targetVersion, runtime.GOOS, runtime.GOARCH)
	downloadURL := baseURL + "/" + assetName

	fmt.Printf("Downloading from %s\n", downloadURL)

	tmpDir, err := os.MkdirTemp("", "agentbox-update-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	checksum, err := download.FetchChecksum(ctx, baseURL+"/checksums.txt", assetName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching checksum: %v\n", err)
		fmt.Fprintf(os.Stderr, "Version %s may not exist for %s/%s\n", targetVersion, runtime.GOOS, runtime.GOARCH)
		return 1
	}

	if err := download.DownloadAndExtractTarGz(ctx, downloadURL, tmpDir, "agentbox", "agentbox", checksum, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading update: %v\n", err)
		return 1
	}

	newBinaryPath := filepath.Join(tmpDir, "agentbox")

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

	if guardHomebrewManaged("uninstall") {
		return 1
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

const httpTimeout = 30 * time.Second

// httpGet performs a GET request with standard timeout.
func httpGet(url string) (resp *http.Response, cancel context.CancelFunc, err error) {
	return httpGetWithTimeout(url, httpTimeout)
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
