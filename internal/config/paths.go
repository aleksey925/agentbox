package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	HomeDir     string
	AgentboxDir string
	BinDir      string
	SkeletonDir string
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
		SkeletonDir: filepath.Join(agentboxDir, "skeleton"),
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

// SkeletonExists checks if the skeleton directory exists and has template files.
func (p *Paths) SkeletonExists() bool {
	// check if there's at least core.*.yml file in skeleton dir (flat structure)
	entries, err := os.ReadDir(p.SkeletonDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "core.") {
			return true
		}
	}
	return false
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
