package agents

import (
	"context"
	"fmt"
	"strings"
)

type CopilotAgent struct {
	arch string
}

func NewCopilotAgent() (*CopilotAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &CopilotAgent{arch: arch}, nil
}

func (c *CopilotAgent) Name() string {
	return "copilot"
}

func (c *CopilotAgent) BinaryName() string {
	return "copilot"
}

func (c *CopilotAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := FetchLatestGitHubTag(ctx, "github", "copilot-cli")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (c *CopilotAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	assetName := fmt.Sprintf("copilot-linux-%s.tar.gz", c.arch)
	baseURL := "https://github.com/github/copilot-cli/releases/download/v" + version

	checksum, err := fetchChecksum(ctx, baseURL+"/SHA256SUMS.txt", assetName)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}

	return downloadAndExtractTarGz(ctx, baseURL+"/"+assetName, destDir, "copilot", "copilot", checksum, progress)
}
