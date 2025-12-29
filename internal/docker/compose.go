package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Run(projectDir string) error {
	ctx := context.Background()
	args := []string{
		"compose",
		"-f", "docker-compose.agentbox.yml",
		"-f", "docker-compose.agentbox.local.yml",
		"run", "--rm",
		"agentbox",
	}

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

func Build(projectDir string, noCache bool) error {
	ctx := context.Background()
	args := []string{
		"compose",
		"-f", "docker-compose.agentbox.yml",
		"-f", "docker-compose.agentbox.local.yml",
		"build",
	}

	if noCache {
		args = append(args, "--no-cache")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose build: %w", err)
	}
	return nil
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
