package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	HomeDir     string
	AgentboxDir string
	BinDir      string
	SkeletonDir string
	StateFile   string
}

func NewPaths() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	agentboxDir := filepath.Join(homeDir, ".agentbox")

	return &Paths{
		HomeDir:     homeDir,
		AgentboxDir: agentboxDir,
		BinDir:      filepath.Join(agentboxDir, "bin"),
		SkeletonDir: filepath.Join(agentboxDir, "skeleton"),
		StateFile:   filepath.Join(agentboxDir, "state.json"),
	}, nil
}

func (p *Paths) AgentDir(agent string) string {
	return filepath.Join(p.BinDir, agent)
}

func (p *Paths) AgentVersionDir(agent, version string) string {
	return filepath.Join(p.BinDir, agent, version)
}

func (p *Paths) AgentCurrentLink(agent string) string {
	return filepath.Join(p.BinDir, agent, "current")
}

func (p *Paths) EnsureDirs() error {
	dirs := []string{
		p.AgentboxDir,
		p.BinDir,
		p.SkeletonDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
