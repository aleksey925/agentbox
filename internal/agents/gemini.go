package agents

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type GeminiAgent struct{}

func NewGeminiAgent() *GeminiAgent {
	return &GeminiAgent{}
}

func (g *GeminiAgent) Name() string {
	return "gemini"
}

func (g *GeminiAgent) Variant() string {
	return "js"
}

func (g *GeminiAgent) BinaryName() string {
	return "gemini.js"
}

func (g *GeminiAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	tag, err := FetchLatestGitHubTag(ctx, "google-gemini", "gemini-cli")
	if err != nil {
		return "", fmt.Errorf("fetch github tag: %w", err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (g *GeminiAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	assetURL := fmt.Sprintf(
		"https://github.com/google-gemini/gemini-cli/releases/download/v%s/gemini-cli-bundle.zip",
		version,
	)

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

	// download zip to a temp file (zip requires random access)
	tmpPath := filepath.Join(destDir, "bundle.zip.tmp")
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpPath)

	pr := &progressReader{
		reader:   resp.Body,
		total:    resp.ContentLength,
		progress: progress,
	}

	if _, copyErr := io.Copy(out, pr); copyErr != nil {
		out.Close()
		return fmt.Errorf("download zip: %w", copyErr)
	}
	out.Close()

	// extract all files from the zip
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	cleanDestDir := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range zr.File {
		destPath := filepath.Join(destDir, f.Name) //nolint:gosec // zip slip is checked below
		if !strings.HasPrefix(destPath, cleanDestDir) {
			return fmt.Errorf("invalid file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create file: %w", err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("extract file: %w", err)
		}

		outFile.Close()
		rc.Close()
	}

	// make gemini.js executable
	if err := os.Chmod(filepath.Join(destDir, "gemini.js"), 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	return nil
}
