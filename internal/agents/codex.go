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
	// codexCodeModeHost is the helper codex spawns from the directory of its own
	// resolved executable, failing closed on code mode when it is missing.
	codexCodeModeHost = "codex-code-mode-host"
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

// IsInstalled states the layout Download produces: the binary the launcher execs
// and the helper codex spawns beside it, both real executables. It rejects the
// single-binary layout agentbox installed before the package archive, which has
// no codex-code-mode-host and so fails closed on code mode.
func (c *CodexAgent) IsInstalled(destDir string) bool {
	for _, name := range []string{c.BinaryName(), codexCodeModeHost} {
		info, err := os.Lstat(filepath.Join(destDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return false
		}
	}
	return true
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
	// published checksum, and it carries codex-code-mode-host. its layout has no
	// wrapping directory, so nothing is stripped.
	if err := download.DownloadAndExtractTarGzAll(ctx, releaseURL+"/"+assetName, destDir, checksum, 0, progress); err != nil {
		return fmt.Errorf("codex: %w", err)
	}

	if err := flattenPackage(destDir, c.BinaryName()); err != nil {
		return fmt.Errorf("codex: %w", err)
	}

	if !c.IsInstalled(destDir) {
		return fmt.Errorf("codex: package archive did not yield executable %s and %s", c.BinaryName(), codexCodeModeHost)
	}
	return nil
}

// flattenPackage promotes everything under bin/ to destDir and drops the rest of
// the bundle, so <version>/codex is the file the launcher execs and the helpers
// sit next to it - see CLAUDE.md "Agent install layout".
func flattenPackage(destDir, entrypoint string) error {
	roots, readErr := os.ReadDir(destDir)
	if readErr != nil {
		return fmt.Errorf("read package root: %w", readErr)
	}
	// the extras go before the promotion, so a bundle that ever ships bin/<name>
	// colliding with a root <name> cannot have its promoted file deleted after
	for _, entry := range roots {
		if entry.Name() == codexBundleBinDir {
			continue
		}
		if err := os.RemoveAll(filepath.Join(destDir, entry.Name())); err != nil {
			return fmt.Errorf("drop %s: %w", entry.Name(), err)
		}
	}

	binDir := filepath.Join(destDir, codexBundleBinDir)
	payload, readErr := os.ReadDir(binDir)
	if readErr != nil {
		return fmt.Errorf("read package %s: %w", codexBundleBinDir, readErr)
	}
	for _, entry := range payload {
		if entry.Name() == entrypoint {
			continue
		}
		if err := promoteEntry(binDir, destDir, entry.Name()); err != nil {
			return err
		}
	}
	// the entrypoint goes last, after every helper: IsInstalled reads it as the
	// mark of a finished install, so an interrupted flatten must never leave it at
	// the root ahead of something else
	if err := promoteEntry(binDir, destDir, entrypoint); err != nil {
		return err
	}

	// os.Remove, not RemoveAll: a bin/ that is still non-empty means an entry was
	// not promoted, and silently deleting it would ship a half-installed codex
	if err := os.Remove(binDir); err != nil {
		return fmt.Errorf("drop %s: %w", codexBundleBinDir, err)
	}
	return nil
}

func promoteEntry(binDir, destDir, name string) error {
	if err := os.Rename(filepath.Join(binDir, name), filepath.Join(destDir, name)); err != nil {
		return fmt.Errorf("promote %s: %w", name, err)
	}
	return nil
}
