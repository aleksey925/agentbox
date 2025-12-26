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

type CopilotAgent struct {
	arch string
}

func NewCopilotAgent() (*CopilotAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, err
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

func (c *CopilotAgent) FetchLatestVersion() (string, error) {
	tag, err := FetchLatestGitHubTag("github", "copilot-cli")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (c *CopilotAgent) Download(version, destDir string, progress func(downloaded, total int64)) error {
	assetName := fmt.Sprintf("copilot-linux-%s.tar.gz", c.arch)
	assetURL := fmt.Sprintf("https://github.com/github/copilot-cli/releases/download/v%s/%s", version, assetName)

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
		if name == "copilot" {
			destPath := filepath.Join(destDir, "copilot")
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

	return fmt.Errorf("binary 'copilot' not found in archive")
}
