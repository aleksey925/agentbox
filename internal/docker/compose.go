package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aleksey925/agentbox/internal/skeleton"
)

// SharedVolumes are Docker volumes shared between all agentbox projects.
var SharedVolumes = []string{
	"agentbox-mise-data",
	"agentbox-mise-cache",
	"agentbox-opencode-cache",
	"agentbox-go-cache",
	"agentbox-uv-cache",
}

// sandboxImage is the tag every project builds its sandbox as; it must match the
// "image:" field in the core compose template. The tag is shared, so removing it
// forces the next "compose run" to rebuild from the current Dockerfile.
const sandboxImage = "agentbox:local"

// RemoveSandboxImage drops the shared sandbox image so the next run rebuilds it
// from the current config. A missing image is not an error.
func RemoveSandboxImage() error {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "image", "rm", "-f", sandboxImage)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "No such image") {
			return nil
		}
		// docker missing entirely leaves stderr empty; surface the exec error.
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("remove image %s: %s", sandboxImage, msg)
	}
	return nil
}

// EnsureSharedVolumes creates shared volumes if they don't exist.
func EnsureSharedVolumes() error {
	for _, vol := range SharedVolumes {
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "docker", "volume", "create", vol) // #nosec G204 -- vol comes from hardcoded SharedVolumes
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("create volume %s: %w", vol, err)
		}
	}
	return nil
}

// containerProjectPath returns the in-container path a project is mounted at.
// mirror scheme (see CLAUDE.md "Live, not baked"): identical to the host path.
func containerProjectPath(hostProjectDir string) string {
	return hostProjectDir
}

// buildRunEnv builds the environment for docker compose invocations, exporting
// the per-project mount path consumed by ${AGENTBOX_PROJECT_PATH} in the core compose file.
func buildRunEnv(projectDir string) []string {
	env := slices.DeleteFunc(os.Environ(), func(e string) bool {
		return strings.HasPrefix(e, "AGENTBOX_PROJECT_PATH=")
	})
	return append(env, "AGENTBOX_PROJECT_PATH="+containerProjectPath(projectDir))
}

// buildRunArgs builds docker compose run arguments.
func buildRunArgs(projectDir string, composeFiles []string) []string {
	args := []string{"compose", "--project-directory", projectDir}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "run", "--rm", "agentbox")
	return args
}

// Run starts a container using compose files from .agentbox/ directory.
func Run(projectDir string, composeFiles []string) error {
	ctx := context.Background()

	fragment, cleanup, err := writeGitProtectionFragment(projectDir)
	if err != nil {
		return fmt.Errorf("prepare git protection: %w", err)
	}
	defer cleanup()
	if fragment != "" {
		composeFiles = append(composeFiles, fragment)
	}

	args := buildRunArgs(projectDir, composeFiles)

	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- args built from validated compose files
	cmd.Dir = projectDir
	cmd.Env = buildRunEnv(projectDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose run: %w", err)
	}
	return nil
}

func Attach(containerID string) error {
	ctx := context.Background()

	args := buildExecArgs(containerID, containerWorkingDir(containerID))

	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- fixed argv; containerID follows "--" so docker can't read it as a flag
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker exec: %w", err)
	}
	return nil
}

// buildExecArgs builds the docker exec argv. The "--" terminator stops docker from
// reading a containerID that starts with "-" as a flag (e.g. --privileged, -u, -e):
// a value from `agentbox run --container` reaches docker only as the target container.
func buildExecArgs(containerID, workingDir string) []string {
	args := []string{"exec", "-it"}
	// -w is explicit because the project path is set live via working_dir at
	// compose-run time, so it isn't baked into the image WORKDIR exec defaults to.
	if workingDir != "" {
		args = append(args, "-w", workingDir)
	}
	return append(args, "--", containerID, "/bin/bash")
}

func containerWorkingDir(containerID string) string {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Config.WorkingDir}}", "--", containerID)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// buildBuildArgs builds docker compose build arguments.
func buildBuildArgs(projectDir string, composeFiles []string, noCache bool) []string {
	args := []string{"compose", "--project-directory", projectDir}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "build")
	if noCache {
		args = append(args, "--no-cache")
	}
	return args
}

// Build builds the container image using compose files from .agentbox/ directory.
func Build(projectDir string, composeFiles []string, noCache bool) error {
	ctx := context.Background()
	args := buildBuildArgs(projectDir, composeFiles, noCache)

	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- args built from validated compose files
	cmd.Dir = projectDir
	cmd.Env = buildRunEnv(projectDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose build: %w", err)
	}
	return nil
}

// DiscoverComposeFiles finds all compose files in .agentbox/ directory and sorts them.
// Order: core first, then alphabetically, local.yml always last.
func DiscoverComposeFiles(projectDir string) ([]string, error) {
	agentboxDir := filepath.Join(projectDir, ".agentbox")
	entries, err := os.ReadDir(agentboxDir)
	if err != nil {
		return nil, fmt.Errorf("read .agentbox directory: %w", err)
	}

	files := make([]string, 0, len(entries))
	hasCore := false
	for _, e := range entries {
		name := e.Name()
		if !skeleton.IsManagedComposeFile(name) {
			continue
		}
		files = append(files, filepath.Join(agentboxDir, name))
		if baseName, _ := skeleton.ParseTemplateName(name); baseName == "core" {
			hasCore = true
		}
	}

	if len(files) == 0 {
		return nil, errors.New("no compose files found in .agentbox/. Run 'agentbox init' to fix")
	}

	// a lone local.yml satisfies len>0 but has no service or build section; the
	// core file is what actually defines the sandbox, so require it explicitly.
	if !hasCore {
		return nil, errors.New("core compose file missing in .agentbox/. Run 'agentbox init' to fix")
	}

	skeleton.SortComposeFiles(files)
	return files, nil
}

type Container struct {
	ID      string
	Name    string
	Started string
}

func ListContainers(projectDir string, all bool) ([]Container, error) {
	ctx := context.Background()

	args := []string{
		"ps",
		"--filter", "label=com.docker.compose.service=agentbox",
		"--format", "{{.ID}}\t{{.Names}}\t{{.RunningFor}}",
	}

	if !all && projectDir != "" {
		args = append(args, "--filter", "label=com.docker.compose.project.working_dir="+projectDir)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("docker ps: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	return parseContainersOutput(out.String()), nil
}

func parseContainersOutput(output string) []Container {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	containers := make([]Container, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		containers = append(containers, Container{
			ID:      parts[0],
			Name:    parts[1],
			Started: parts[2],
		})
	}
	return containers
}
