package agents

import (
	"context"
	"fmt"
	"strings"
)

type OpenCodeAgent struct {
	arch string
}

func NewOpenCodeAgent() (*OpenCodeAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &OpenCodeAgent{arch: arch}, nil
}

func (o *OpenCodeAgent) Name() string {
	return "opencode"
}

func (o *OpenCodeAgent) BinaryName() string {
	return "opencode"
}

func (o *OpenCodeAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := FetchLatestGitHubTag(ctx, "anomalyco", "opencode")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (o *OpenCodeAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	// asset format: opencode-linux-x64.tar.gz or opencode-linux-arm64.tar.gz
	assetName := fmt.Sprintf("opencode-linux-%s.tar.gz", o.arch)
	assetURL := fmt.Sprintf("https://github.com/anomalyco/opencode/releases/download/v%s/%s", version, assetName)

	return downloadAndExtractTarGz(ctx, assetURL, destDir, "opencode", "opencode", progress)
}
