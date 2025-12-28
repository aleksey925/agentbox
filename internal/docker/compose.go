package docker

import (
	"os"
	"os/exec"
)

func Run(projectDir string) error {
	args := []string{
		"compose",
		"-f", "docker-compose.agentbox.yml",
		"-f", "docker-compose.agentbox.local.yml",
		"run", "--rm",
		"agentbox",
	}

	cmd := exec.Command("docker", args...)
	cmd.Dir = projectDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func Attach(containerID string) error {
	cmd := exec.Command("docker", "exec", "-it", containerID, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func Build(projectDir string, noCache bool) error {
	args := []string{
		"compose",
		"-f", "docker-compose.agentbox.yml",
		"-f", "docker-compose.agentbox.local.yml",
		"build",
	}

	if noCache {
		args = append(args, "--no-cache")
	}

	cmd := exec.Command("docker", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
