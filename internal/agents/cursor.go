package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/aleksey925/agentbox/internal/download"
)

const cursorInstallScriptURL = "https://cursor.com/install"

var cursorVersionRegex = regexp.MustCompile(`\d{4}\.\d{2}\.\d{2}-[0-9a-f]+`)

type CursorAgent struct {
	arch string
}

func NewCursorAgent() (*CursorAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &CursorAgent{arch: arch}, nil
}

func (c *CursorAgent) Name() string {
	return "cursor"
}

func (c *CursorAgent) BinaryName() string {
	return "cursor-agent"
}

func (c *CursorAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cursorInstallScriptURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", download.UserAgent)

	resp, err := download.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch install script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch install script: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read install script: %w", err)
	}

	version := cursorVersionRegex.FindString(string(body))
	if version == "" {
		return "", errors.New("version not found in install script")
	}

	return version, nil
}

func (c *CursorAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	assetURL := fmt.Sprintf(
		"https://downloads.cursor.com/lab/%s/linux/%s/agent-cli-package.tar.gz",
		version, c.arch,
	)

	// cursor-agent is a bash wrapper that resolves sibling files by relative
	// path, so the whole archive has to be extracted, not just the binary.
	if err := download.DownloadAndExtractTarGzAll(ctx, assetURL, destDir, progress); err != nil {
		return fmt.Errorf("cursor: %w", err)
	}
	return nil
}
