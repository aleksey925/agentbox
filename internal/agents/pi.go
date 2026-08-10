package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksey925/agentbox/internal/download"
)

type PiAgent struct {
	arch string
}

func NewPiAgent() (*PiAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &PiAgent{arch: arch}, nil
}

func (p *PiAgent) Name() string {
	return "pi"
}

func (p *PiAgent) BinaryName() string {
	return "pi"
}

func (p *PiAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := download.FetchLatestGitHubTag(ctx, "earendil-works", "pi")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (p *PiAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	// asset format: pi-linux-x64.tar.gz or pi-linux-arm64.tar.gz
	assetName := fmt.Sprintf("pi-linux-%s.tar.gz", p.arch)
	baseURL := "https://github.com/earendil-works/pi/releases/download/v" + version

	checksum, err := download.FetchChecksum(ctx, baseURL+"/SHA256SUMS", assetName)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}

	// the pi binary loads sibling files (package.json, node_modules, assets,
	// wasm) relative to its own path, so the whole archive must be extracted,
	// not just the binary - same shape as cursor, but pi publishes a checksum.
	if err := download.DownloadAndExtractTarGzAll(ctx, baseURL+"/"+assetName, destDir, checksum, 1, progress); err != nil {
		return fmt.Errorf("pi: %w", err)
	}
	return nil
}
