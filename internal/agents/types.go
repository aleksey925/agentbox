package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const userAgent = "agentbox/1.0"

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
	return []string{"claude", "copilot", "codex", "gemini"}
}

// AgentDescriptions returns short descriptions for all agents.
func AgentDescriptions() map[string]string {
	return map[string]string{
		"claude":  "Claude Code by Anthropic",
		"copilot": "GitHub Copilot",
		"codex":   "OpenAI Codex",
		"gemini":  "Google Gemini",
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
