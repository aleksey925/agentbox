package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	HomeDir     string
	AgentboxDir string
	BinDir      string
	StateFile   string
}

func NewPaths() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	agentboxDir := filepath.Join(homeDir, ".agentbox")

	return &Paths{
		HomeDir:     homeDir,
		AgentboxDir: agentboxDir,
		BinDir:      filepath.Join(agentboxDir, "bin"),
		StateFile:   filepath.Join(agentboxDir, "state.json"),
	}, nil
}

func (p *Paths) AgentDir(agent string) string {
	return filepath.Join(p.BinDir, agent)
}

func (p *Paths) AgentVersionDir(agent, version string) string {
	return filepath.Join(p.BinDir, agent, version)
}

func (p *Paths) AgentCurrentFile(agent string) string {
	return filepath.Join(p.BinDir, agent, "current")
}

func (p *Paths) EnsureDirs() error {
	dirs := []string{
		p.AgentboxDir,
		p.BinDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return nil
}
