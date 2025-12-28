package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const claudeBucketURL = "https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/claude-code-releases"

type ClaudeAgent struct {
	arch string
}

type claudeManifest struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	Platforms map[string]struct {
		Checksum string `json:"checksum"`
		Size     int64  `json:"size"`
	} `json:"platforms"`
}

func NewClaudeAgent() (*ClaudeAgent, error) {
	arch, err := DetectArch()
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	return &ClaudeAgent{arch: arch}, nil
}

func (c *ClaudeAgent) Name() string {
	return "claude"
}

func (c *ClaudeAgent) Variant() string {
	return "glibc"
}

func (c *ClaudeAgent) BinaryName() string {
	return "claude"
}

func (c *ClaudeAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeBucketURL+"/latest", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch stable version: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	return string(body), nil
}

func (c *ClaudeAgent) fetchManifest(ctx context.Context, version string) (*claudeManifest, error) {
	url := fmt.Sprintf("%s/%s/manifest.json", claudeBucketURL, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch manifest: %s", resp.Status)
	}

	var manifest claudeManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	return &manifest, nil
}

func (c *ClaudeAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	manifest, err := c.fetchManifest(ctx, version)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	platform := "linux-" + c.arch
	platformInfo, ok := manifest.Platforms[platform]
	if !ok {
		return fmt.Errorf("platform %s not found in manifest", platform)
	}

	binaryURL := fmt.Sprintf("%s/%s/%s/claude", claudeBucketURL, version, platform)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	destPath := filepath.Join(destDir, "claude")
	tmpPath := destPath + ".tmp"

	if err := c.downloadAndVerify(ctx, binaryURL, tmpPath, platformInfo.Checksum, platformInfo.Size, progress); err != nil {
		return fmt.Errorf("download and verify: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func (c *ClaudeAgent) downloadAndVerify(
	ctx context.Context,
	url, tmpPath, expectedChecksum string,
	totalSize int64,
	progress func(downloaded, total int64),
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download binary: %s", resp.Status)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)

	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
				out.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("write to file: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("read response: %w", readErr)
		}
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	if checksum != expectedChecksum {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}

	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}
