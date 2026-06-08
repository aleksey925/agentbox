package agents

import (
	"context"
	"fmt"
	"strings"
)

type CodexAgent struct {
	arch string
}

func NewCodexAgent() (*CodexAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &CodexAgent{arch: arch}, nil
}

func (c *CodexAgent) Name() string {
	return "codex"
}

func (c *CodexAgent) BinaryName() string {
	return "codex"
}

func (c *CodexAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := FetchLatestGitHubTag(ctx, "openai", "codex")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	// tag format: rust-v0.77.0
	return strings.TrimPrefix(tag, "rust-v"), nil
}

func (c *CodexAgent) rustArch() string {
	switch c.arch {
	case archARM64:
		return "aarch64"
	case archX64:
		return "x86_64"
	default:
		return c.arch
	}
}

func (c *CodexAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	// upstream dropped linux-gnu; static musl is the only linux build now.
	binaryName := fmt.Sprintf("codex-%s-unknown-linux-musl", c.rustArch())
	assetName := binaryName + ".tar.gz"
	assetURL := fmt.Sprintf("https://github.com/openai/codex/releases/download/rust-v%s/%s", version, assetName)

	// unverified, see CLAUDE.md "Download integrity"
	return downloadAndExtractTarGz(ctx, assetURL, destDir, binaryName, "codex", "", progress)
}
