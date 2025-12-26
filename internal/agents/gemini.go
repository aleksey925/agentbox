package agents

import (
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

func (g *GeminiAgent) FetchLatestVersion() (string, error) {
	tag, err := FetchLatestGitHubTag("google-gemini", "gemini-cli")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func (g *GeminiAgent) Download(version, destDir string, progress func(downloaded, total int64)) error {
	assetURL := fmt.Sprintf("https://github.com/google-gemini/gemini-cli/releases/download/v%s/gemini.js", version)

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

	destPath := filepath.Join(destDir, "gemini.js")
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				out.Close()
				os.Remove(tmpPath)
				return writeErr
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}
