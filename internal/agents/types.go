package agents

import (
	"context"
	"fmt"
	"runtime"
)

const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
	archX64   = "x64"
)

type Agent interface {
	Name() string
	FetchLatestVersion(ctx context.Context) (string, error)
	Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error
	BinaryName() string
}

type DownloadResult struct {
	Agent   string
	Version string
	Error   error
}

func DetectArch() (string, error) {
	switch runtime.GOARCH {
	case archAMD64:
		return archX64, nil
	case archARM64:
		return archARM64, nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

func AllAgentNames() []string {
	return []string{"claude", "copilot", "codex", "cursor", "opencode", "pi", "ralphex"}
}

// agentConfigDirs maps agent name to its config directories (relative to $HOME).
// Most agents use simple format (.claude), but opencode and ralphex use XDG paths.
var agentConfigDirs = map[string][]string{
	"claude":   {".claude"},
	"copilot":  {".copilot"},
	"codex":    {".codex"},
	"cursor":   {".cursor"},
	"opencode": {".config/opencode", ".local/share/opencode", ".local/state/opencode"},
	"pi":       {".pi"},
	"ralphex":  {".config/ralphex"},
}

// AgentConfigDirs returns all config directory paths (relative to $HOME) for all agents.
// Used by CLI to create directories before Docker mounts them.
func AgentConfigDirs() []string {
	var dirs []string
	for _, name := range AllAgentNames() {
		dirs = append(dirs, agentConfigDirs[name]...)
	}
	return dirs
}

// SuggestedFlags returns per-agent launch flags to seed the flags file with
// (~/.agentbox/flags/agent-flags), written as commented-out lines. It is the
// hook for shipping recommended defaults later, but is intentionally empty for
// every agent now: out of the box agentbox imposes no flags, so cautious users
// are never surprised by an agent running in a permissive mode they didn't pick.
func SuggestedFlags() map[string]string {
	return map[string]string{
		"claude":   "",
		"copilot":  "",
		"codex":    "",
		"cursor":   "",
		"opencode": "",
		"pi":       "",
		"ralphex":  "",
	}
}

// AgentDescriptions returns short descriptions for all agents.
func AgentDescriptions() map[string]string {
	return map[string]string{
		"claude":   "Claude Code by Anthropic",
		"copilot":  "GitHub Copilot",
		"codex":    "OpenAI Codex",
		"cursor":   "Cursor CLI",
		"opencode": "Open Source AI Coding Agent",
		"pi":       "Pi Coding Agent by earendil-works",
		"ralphex":  "Autonomous plan execution tool by umputun",
	}
}
