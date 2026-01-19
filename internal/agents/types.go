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
	"runtime"
	"strings"
	"time"
)

const (
	userAgent    = "agentbox/1.0"
	variantGlibc = "glibc"
)

var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
}

// FetchLatestGitHubTag gets latest release tag via redirect (bypasses API rate limit)
func FetchLatestGitHubTag(ctx context.Context, owner, repo string) (string, error) {
	url := "https://github.com/" + owner + "/" + repo + "/releases/latest"

	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", errors.New("no redirect location")
	}

	// extract tag from URL like: https://github.com/owner/repo/releases/tag/v1.2.3
	parts := strings.Split(location, "/tag/")
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected redirect URL: %s", location)
	}

	return parts[1], nil
}

type Agent interface {
	Name() string
	Variant() string
	FetchLatestVersion(ctx context.Context) (string, error)
	Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error
	BinaryName() string
}

type DownloadResult struct {
	Agent   string
	Version string
	Variant string
	Error   error
}

func DetectArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

func AllAgentNames() []string {
	return []string{"claude", "copilot", "codex", "gemini", "opencode"}
}

// AgentConfigDirs returns config directory names for all agents (e.g., ".claude", ".copilot").
// Used by CLI to create directories before Docker mounts them.
func AgentConfigDirs() []string {
	names := AllAgentNames()
	dirs := make([]string, len(names))
	for i, name := range names {
		dirs[i] = "." + name
	}
	return dirs
}

// AgentDescriptions returns short descriptions for all agents.
func AgentDescriptions() map[string]string {
	return map[string]string{
		"claude":   "Claude Code by Anthropic",
		"copilot":  "GitHub Copilot",
		"codex":    "OpenAI Codex",
		"gemini":   "Google Gemini",
		"opencode": "Open Source AI Coding Agent",
	}
}

type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	progress   func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.progress != nil {
			pr.progress(pr.downloaded, pr.total)
		}
	}
	switch err {
	case nil:
		return n, nil
	case io.EOF:
		// don't wrap io.EOF - it breaks gzip/tar readers that check for io.EOF
		return n, io.EOF
	default:
		return n, fmt.Errorf("read: %w", err)
	}
}

// downloadAndExtractTarGz downloads a tar.gz archive and extracts a specific binary.
// binaryInArchive is the name of the binary to look for inside the archive.
// destBinaryName is the name to save the binary as in destDir.
func downloadAndExtractTarGz(
	ctx context.Context,
	assetURL, destDir, binaryInArchive, destBinaryName string,
	progress func(downloaded, total int64),
) error {
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
		if name == binaryInArchive {
			destPath := filepath.Join(destDir, destBinaryName)
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

	return fmt.Errorf("binary '%s' not found in archive", binaryInArchive)
}
