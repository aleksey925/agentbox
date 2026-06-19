// Package download provides generic helpers to fetch, verify, and extract
// release artifacts (tarballs + checksums) over HTTP. It is intentionally free
// of any agent-specific knowledge so both the agent installer and self-update
// can share the same hardened path (see CLAUDE.md "Download integrity").
package download

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserAgent identifies agentbox on every outbound request.
const UserAgent = "agentbox/1.0"

// Client is the shared HTTP client for all artifact fetching.
var Client = &http.Client{
	Timeout: 5 * time.Minute,
}

// MaxArtifactBytes bounds both the buffered raw download and the decompressed
// output, so a gzip bomb or an endless stream can't exhaust disk. Far above any
// real agent/self-update artifact (tens to a few hundred MB). A var, not a const,
// so tests can lower it without producing a real multi-GB archive.
var MaxArtifactBytes int64 = 1 << 30 // 1 GiB

// maxChecksumsBytes bounds a checksums/manifest-style text response; an endless
// stream would otherwise exhaust memory in io.ReadAll.
const maxChecksumsBytes = 1 << 20 // 1 MiB

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
	req.Header.Set("User-Agent", UserAgent)

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

// FetchChecksum downloads a SHA256SUMS-style file (each line "<hex>  <name>",
// the name optionally "*"-prefixed for binary mode) and returns the hash
// recorded for assetName. A missing entry is an error rather than a silent
// skip, so a truncated or wrong-version checksums file can't downgrade a
// verified download to an unverified one.
func FetchChecksum(ctx context.Context, checksumsURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch checksums: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumsBytes))
	if err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}

	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == assetName {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("checksum for %s not found", assetName)
}

// downloadArchive fetches an asset for extraction. With a non-empty expectedSHA256
// it buffers the archive to a temp file and verifies the hash before returning, so a
// tampered archive never reaches the extractor; with an empty one it streams the body
// straight through (see CLAUDE.md "Download integrity"). cleanup must always be called.
func downloadArchive(
	ctx context.Context,
	assetURL, expectedSHA256 string,
	progress func(downloaded, total int64),
) (io.Reader, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, http.NoBody)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := Client.Do(req) //nolint:bodyclose // closed via the returned cleanup (streaming) or the defer below (verified)
	if err != nil {
		return nil, nil, fmt.Errorf("http get: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("failed to download asset: %s", resp.Status)
	}

	pr := &progressReader{
		reader:   resp.Body,
		total:    resp.ContentLength,
		progress: progress,
	}

	if expectedSHA256 == "" {
		return pr, func() { resp.Body.Close() }, nil
	}

	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "agentbox-download-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp file: %w", err)
	}
	cleanup := func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}

	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(pr, MaxArtifactBytes+1))
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("download asset: %w", err)
	}
	if n > MaxArtifactBytes {
		cleanup()
		return nil, nil, fmt.Errorf("asset exceeds %d bytes", MaxArtifactBytes)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, expectedSHA256) {
		cleanup()
		return nil, nil, fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, got)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("seek temp file: %w", err)
	}

	return tmp, cleanup, nil
}

// DownloadAndExtractTarGz downloads a tar.gz archive and extracts a specific binary.
// binaryInArchive is the name of the binary to look for inside the archive.
// destBinaryName is the name to save the binary as in destDir.
// expectedSHA256 verifies the archive before extraction (empty = unverified).
func DownloadAndExtractTarGz(
	ctx context.Context,
	assetURL, destDir, binaryInArchive, destBinaryName, expectedSHA256 string,
	progress func(downloaded, total int64),
) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	archive, cleanup, err := downloadArchive(ctx, assetURL, expectedSHA256, progress)
	if err != nil {
		return err
	}
	defer cleanup()

	gzr, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	// cap total decompressed bytes: a gzip bomb is small on the wire but expands
	// without bound, so the limit goes on the gzip output, not the archive.
	tr := tar.NewReader(io.LimitReader(gzr, MaxArtifactBytes+1))

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
				os.Remove(destPath)
				return fmt.Errorf("copy to file: %w", err)
			}
			out.Close()

			if err := os.Chmod(destPath, 0o755); err != nil {
				os.Remove(destPath)
				return fmt.Errorf("chmod: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("binary '%s' not found in archive", binaryInArchive)
}

// DownloadAndExtractTarGzAll extracts the whole archive into destDir while
// dropping the leading path component (mirrors `tar --strip-components=1`),
// which is how vendor archives like cursor's `dist-package/...` are shaped.
// It takes no checksum: archives extracted this way are unverified (see
// CLAUDE.md "Download integrity").
func DownloadAndExtractTarGzAll(
	ctx context.Context,
	assetURL, destDir string,
	progress func(downloaded, total int64),
) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	archive, cleanup, err := downloadArchive(ctx, assetURL, "", progress)
	if err != nil {
		return err
	}
	defer cleanup()

	gzr, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	// cap total decompressed bytes against a gzip bomb (see DownloadAndExtractTarGz).
	tr := tar.NewReader(io.LimitReader(gzr, MaxArtifactBytes+1))
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
		os.Remove(destPath)
		return fmt.Errorf("copy to file: %w", err)
	}
	out.Close()

	// mask drops setuid/setgid/sticky bits — extracted files should never be
	// privileged regardless of what the archive declares.
	mode := os.FileMode(uint32(hdr.Mode) & 0o777) //nolint:gosec // value bounded by mask
	if err := os.Chmod(destPath, mode); err != nil {
		os.Remove(destPath)
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
