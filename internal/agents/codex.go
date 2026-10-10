package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksey925/agentbox/internal/download"
)

const codexReleaseBaseURL = "https://github.com/openai/codex/releases/download"

const (
	// codexBundleBinDir is where the package archive keeps its executables,
	// relative to the archive root.
	codexBundleBinDir = "bin"
	// codexPackageManifest marks the directory above bin/ as a codex package.
	codexPackageManifest = "codex-package.json"
)

type CodexAgent struct {
	arch           string
	releaseBaseURL string
}

func NewCodexAgent() (*CodexAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &CodexAgent{arch: arch, releaseBaseURL: codexReleaseBaseURL}, nil
}

func (c *CodexAgent) Name() string {
	return "codex"
}

func (c *CodexAgent) BinaryName() string {
	return "codex"
}

func (c *CodexAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := download.FetchLatestGitHubTag(ctx, "openai", "codex")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	// tag format: rust-v0.77.0
	return strings.TrimPrefix(tag, "rust-v"), nil
}

// IsInstalled states the layout Download produces: the vendor package kept
// whole, plus the entrypoint link the launcher execs. It rejects the layouts
// agentbox installed before, which have no package manifest and so cannot
// start the codex background server.
func (c *CodexAgent) IsInstalled(destDir string) bool {
	manifest, err := os.Lstat(filepath.Join(destDir, codexPackageManifest))
	if err != nil || !manifest.Mode().IsRegular() {
		return false
	}
	// only what agentbox itself relies on - which other files the package must
	// hold is codex's own contract, and a copy of it here would go stale
	entrypoint, err := os.Lstat(filepath.Join(destDir, c.entrypointTarget()))
	if err != nil || !entrypoint.Mode().IsRegular() || entrypoint.Mode().Perm()&0o111 == 0 {
		return false
	}
	link, err := os.Readlink(filepath.Join(destDir, c.BinaryName()))
	return err == nil && link == c.entrypointTarget()
}

func (c *CodexAgent) entrypointTarget() string {
	return filepath.Join(codexBundleBinDir, c.BinaryName())
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
	assetName := fmt.Sprintf("codex-package-%s-unknown-linux-musl.tar.gz", c.rustArch())
	releaseURL := fmt.Sprintf("%s/rust-v%s", c.releaseBaseURL, version)

	checksum, err := download.FetchChecksum(ctx, releaseURL+"/codex-package_SHA256SUMS", assetName)
	if err != nil {
		return fmt.Errorf("codex: fetch checksum: %w", err)
	}

	// the package archive, not the bare binary: it is the only codex asset with a
	// published checksum, and codex refuses to run outside its package layout.
	// the archive has no wrapping directory, so nothing is stripped.
	if err := download.DownloadAndExtractTarGzAll(ctx, releaseURL+"/"+assetName, destDir, checksum, 0, progress); err != nil {
		return fmt.Errorf("codex: %w", err)
	}

	// the link goes last, after the whole package is on disk: IsInstalled reads
	// it as the mark of a finished install
	if err := os.Symlink(c.entrypointTarget(), filepath.Join(destDir, c.BinaryName())); err != nil {
		return fmt.Errorf("codex: link entrypoint: %w", err)
	}

	if !c.IsInstalled(destDir) {
		return fmt.Errorf("codex: package archive did not yield %s and an executable %s", codexPackageManifest, c.entrypointTarget())
	}
	return nil
}
