package agents

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const userAgent = "agentbox/1.0"

var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
}

// GitHubRequest creates an HTTP request with proper headers for GitHub API
func GitHubRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// use token if available
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return req, nil
}

// FetchLatestGitHubTag gets latest release tag via redirect (bypasses API rate limit)
func FetchLatestGitHubTag(owner, repo string) (string, error) {
	url := "https://github.com/" + owner + "/" + repo + "/releases/latest"

	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no redirect location")
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
	FetchLatestVersion() (string, error)
	Download(version, destDir string, progress func(downloaded, total int64)) error
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
	return n, err
}
