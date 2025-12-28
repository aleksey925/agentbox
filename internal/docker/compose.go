package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
