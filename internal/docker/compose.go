package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aleksey925/agentbox/internal/skeleton"
)

// SharedVolumes are Docker volumes shared between all agentbox projects.
var SharedVolumes = []string{
	"agentbox-mise-data",
	"agentbox-mise-cache",
	"agentbox-opencode-cache",
}

// EnsureSharedVolumes creates shared volumes if they don't exist.
func EnsureSharedVolumes() error {
	for _, vol := range SharedVolumes {
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "docker", "volume", "create", vol)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("create volume %s: %w", vol, err)
		}
	}
	return nil
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
	args := buildRunArgs(projectDir, composeFiles)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = projectDir
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
	cmd := exec.CommandContext(ctx, "docker", "exec", "-it", containerID, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker exec: %w", err)
	}
	return nil
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

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = projectDir
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

	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			files = append(files, filepath.Join(agentboxDir, name))
		}
	}

	if len(files) == 0 {
		return nil, errors.New("no compose files found in .agentbox/. Run 'agentbox init' to fix")
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
