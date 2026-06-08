package agents

import (
	"context"
	"fmt"
	"strings"
)

type RalphexAgent struct {
	arch string
}

func NewRalphexAgent() (*RalphexAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &RalphexAgent{arch: arch}, nil
}

func (r *RalphexAgent) Name() string {
	return "ralphex"
}

func (r *RalphexAgent) BinaryName() string {
	return "ralphex"
}

func (r *RalphexAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := FetchLatestGitHubTag(ctx, "umputun", "ralphex")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// goArch maps the agentbox arch (x64/arm64) to the Go release arch (amd64/arm64)
// used by ralphex assets.
func (r *RalphexAgent) goArch() string {
	if r.arch == archX64 {
		return archAMD64
	}
	return r.arch
}

func (r *RalphexAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	// asset format: ralphex_<version>_linux_<amd64|arm64>.tar.gz
	assetName := fmt.Sprintf("ralphex_%s_linux_%s.tar.gz", version, r.goArch())
	baseURL := "https://github.com/umputun/ralphex/releases/download/v" + version

	checksumName := fmt.Sprintf("ralphex_%s_checksums.txt", version)
	checksum, err := fetchChecksum(ctx, baseURL+"/"+checksumName, assetName)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}

	return downloadAndExtractTarGz(ctx, baseURL+"/"+assetName, destDir, "ralphex", "ralphex", checksum, progress)
}
