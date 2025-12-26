package agents

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type CodexAgent struct {
	arch string
}

func NewCodexAgent() (*CodexAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, err
	}
	return &CodexAgent{arch: arch}, nil
}

func (c *CodexAgent) Name() string {
	return "codex"
}

func (c *CodexAgent) Variant() string {
	return "glibc"
}

func (c *CodexAgent) BinaryName() string {
	return "codex"
}

func (c *CodexAgent) FetchLatestVersion() (string, error) {
	tag, err := FetchLatestGitHubTag("openai", "codex")
	if err != nil {
		return "", err
	}
	// tag format: rust-v0.77.0
	return strings.TrimPrefix(tag, "rust-v"), nil
}

func (c *CodexAgent) rustArch() string {
	switch c.arch {
	case "arm64":
		return "aarch64"
	case "x64":
		return "x86_64"
	default:
		return c.arch
	}
}

func (c *CodexAgent) Download(version, destDir string, progress func(downloaded, total int64)) error {
	binaryName := fmt.Sprintf("codex-%s-unknown-linux-gnu", c.rustArch())
	assetName := binaryName + ".tar.gz"
	assetURL := fmt.Sprintf("https://github.com/openai/codex/releases/download/rust-v%s/%s", version, assetName)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	resp, err := httpClient.Get(assetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download asset: %s", resp.Status)
	}

	pr := &progressReader{
		reader:   resp.Body,
		total:    resp.ContentLength,
		progress: progress,
	}

	gzr, err := gzip.NewReader(pr)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Base(hdr.Name)
		if name == binaryName {
			destPath := filepath.Join(destDir, "codex")
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}

			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()

			if err := os.Chmod(destPath, 0755); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary '%s' not found in archive", binaryName)
}
