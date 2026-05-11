package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// upstream has shipped github release tags without the bundle asset
// (e.g. v0.41.2), so we pull from npm where the tarball is always present.
const geminiNpmPackage = "@google/gemini-cli"

var npmRegistryBaseURL = "https://registry.npmjs.org"

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
	return "bundle/gemini.js"
}

func (g *GeminiAgent) FetchLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/%s/latest", npmRegistryBaseURL, geminiNpmPackage)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch npm metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var meta struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", fmt.Errorf("decode npm metadata: %w", err)
	}
	if meta.Version == "" {
		return "", errors.New("npm metadata missing version")
	}
	return meta.Version, nil
}

func (g *GeminiAgent) Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error {
	assetURL := fmt.Sprintf(
		"%s/%s/-/gemini-cli-%s.tgz",
		npmRegistryBaseURL, geminiNpmPackage, version,
	)
	return downloadAndExtractTarGzAll(ctx, assetURL, destDir, progress)
}
