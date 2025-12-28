package agents

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func (c *CopilotAgent) Variant() string {
	return "glibc"
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
	assetURL := fmt.Sprintf("https://github.com/github/copilot-cli/releases/download/v%s/%s", version, assetName)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
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
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Base(hdr.Name)
		if name == "copilot" {
			destPath := filepath.Join(destDir, "copilot")
			out, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}

			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("copy to file: %w", err)
			}
			out.Close()

			if err := os.Chmod(destPath, 0o755); err != nil {
				return fmt.Errorf("chmod: %w", err)
			}
			return nil
		}
	}

	return errors.New("binary 'copilot' not found in archive")
}
