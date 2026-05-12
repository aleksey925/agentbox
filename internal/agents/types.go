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
	userAgent = "agentbox/1.0"

	archAMD64 = "amd64"
	archARM64 = "arm64"
	archX64   = "x64"
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
	FetchLatestVersion(ctx context.Context) (string, error)
	Download(ctx context.Context, version, destDir string, progress func(downloaded, total int64)) error
	BinaryName() string
}

type DownloadResult struct {
	Agent   string
	Version string
	Error   error
}

func DetectArch() (string, error) {
	switch runtime.GOARCH {
	case archAMD64:
		return archX64, nil
	case archARM64:
		return archARM64, nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

func AllAgentNames() []string {
	return []string{"claude", "copilot", "codex", "cursor", "gemini", "opencode", "ralphex"}
}

// agentConfigDirs maps agent name to its config directories (relative to $HOME).
// Most agents use simple format (.claude), but opencode and ralphex use XDG paths.
var agentConfigDirs = map[string][]string{
	"claude":   {".claude"},
	"copilot":  {".copilot"},
	"codex":    {".codex"},
	"cursor":   {".cursor"},
	"gemini":   {".gemini"},
	"opencode": {".config/opencode", ".local/share/opencode", ".local/state/opencode"},
	"ralphex":  {".config/ralphex"},
}

// AgentConfigDirs returns all config directory paths (relative to $HOME) for all agents.
// Used by CLI to create directories before Docker mounts them.
func AgentConfigDirs() []string {
	var dirs []string
	for _, name := range AllAgentNames() {
		dirs = append(dirs, agentConfigDirs[name]...)
	}
	return dirs
}

// AgentDescriptions returns short descriptions for all agents.
func AgentDescriptions() map[string]string {
	return map[string]string{
		"claude":   "Claude Code by Anthropic",
		"copilot":  "GitHub Copilot",
		"codex":    "OpenAI Codex",
		"cursor":   "Cursor CLI",
		"gemini":   "Google Gemini",
		"opencode": "Open Source AI Coding Agent",
		"ralphex":  "Autonomous plan execution tool by umputun",
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

// downloadAndExtractTarGzAll extracts the whole archive into destDir while
// dropping the leading path component (mirrors `tar --strip-components=1`),
// which is how vendor archives like cursor's `dist-package/...` are shaped.
func downloadAndExtractTarGzAll(
	ctx context.Context,
	assetURL, destDir string,
	progress func(downloaded, total int64),
) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

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
	cleanDestDir := filepath.Clean(destDir) + string(os.PathSeparator)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		if err := extractTarEntry(tr, hdr, destDir, cleanDestDir); err != nil {
			return err
		}
	}

	return nil
}

func extractTarEntry(tr *tar.Reader, hdr *tar.Header, destDir, cleanDestDir string) error {
	stripped := stripPathComponents(hdr.Name, 1)
	if stripped == "" {
		return nil
	}

	destPath := filepath.Join(destDir, stripped)
	if !strings.HasPrefix(destPath, cleanDestDir) && filepath.Clean(destPath) != filepath.Clean(destDir) {
		return fmt.Errorf("invalid file path in archive: %s", hdr.Name)
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(destPath, 0o755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
	case tar.TypeReg:
		return writeTarFile(tr, hdr, destPath)
	case tar.TypeSymlink:
		// absolute targets resolve outside destDir at runtime; filepath.Join
		// would swallow the leading "/" and make the relative-path check below
		// pass even though os.Symlink stores the original absolute target.
		if filepath.IsAbs(hdr.Linkname) {
			return fmt.Errorf("symlink has absolute target: %s -> %s", hdr.Name, hdr.Linkname)
		}
		resolvedTarget := filepath.Join(filepath.Dir(destPath), hdr.Linkname) //nolint:gosec // checked next line
		if !strings.HasPrefix(resolvedTarget, cleanDestDir) && filepath.Clean(resolvedTarget) != filepath.Clean(destDir) {
			return fmt.Errorf("symlink target escapes archive root: %s -> %s", hdr.Name, hdr.Linkname)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
		if err := os.Symlink(hdr.Linkname, destPath); err != nil {
			return fmt.Errorf("create symlink: %w", err)
		}
	default:
		// hard links, device nodes, fifos — fail loudly so partial extraction
		// doesn't silently produce a broken install if upstream archive changes.
		return fmt.Errorf("unsupported tar entry type %d for %s", hdr.Typeflag, hdr.Name)
	}
	return nil
}

func writeTarFile(tr *tar.Reader, hdr *tar.Header, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if _, err := io.Copy(out, tr); err != nil {
		out.Close()
		return fmt.Errorf("copy to file: %w", err)
	}
	out.Close()

	// mask drops setuid/setgid/sticky bits — extracted files should never be
	// privileged regardless of what the archive declares.
	mode := os.FileMode(uint32(hdr.Mode) & 0o777) //nolint:gosec // value bounded by mask
	if err := os.Chmod(destPath, mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	return nil
}

func stripPathComponents(name string, n int) string {
	if n <= 0 {
		return name
	}
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	if len(parts) <= n {
		return ""
	}
	return strings.Join(parts[n:], "/")
}
