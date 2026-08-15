package agents

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCodexAgent_Name(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)

	// act & assert
	if agent.Name() != "codex" {
		t.Errorf("Name() = %s, want codex", agent.Name())
	}

	if agent.BinaryName() != "codex" {
		t.Errorf("BinaryName() = %s, want codex", agent.BinaryName())
	}
}

func TestCodexAgent_rustArch(t *testing.T) {
	// arrange
	agent := &CodexAgent{arch: "arm64"}

	// act & assert
	if agent.rustArch() != "aarch64" {
		t.Errorf("rustArch() = %s, want aarch64", agent.rustArch())
	}

	agent.arch = "x64"
	if agent.rustArch() != "x86_64" {
		t.Errorf("rustArch() = %s, want x86_64", agent.rustArch())
	}
}

func TestCodexAgent_Download(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, codexBundleEntries())

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	want := []string{agent.BinaryName(), codexCodeModeHost}
	if got := listNames(t, destDir); !slices.Equal(got, want) {
		t.Errorf("install listing = %v, want %v", got, want)
	}
	for _, name := range want {
		path := filepath.Join(destDir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatalf("lstat %s: %v", name, statErr)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s mode = %v, want a regular executable file", name, info.Mode())
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		if string(content) != codexBundleBinDir+"/"+name {
			t.Errorf("%s content = %q, want the archive's %s entry", name, content, codexBundleBinDir+"/"+name)
		}
	}
}

func TestCodexAgent_Download__promotes_unknown_helper(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, append(codexBundleEntries(), codexFileEntry("bin/codex-zz-helper")))

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	want := []string{agent.BinaryName(), codexCodeModeHost, "codex-zz-helper"}
	if got := listNames(t, destDir); !slices.Equal(got, want) {
		t.Errorf("install listing = %v, want %v", got, want)
	}
}

func TestCodexAgent_Download__replaces_pre_fix_layout(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, codexBundleEntries())
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("create dest dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, agent.BinaryName()), []byte("stale"), 0o755); err != nil {
		t.Fatalf("write stale binary: %v", err)
	}

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	want := []string{agent.BinaryName(), codexCodeModeHost}
	if got := listNames(t, destDir); !slices.Equal(got, want) {
		t.Errorf("install listing = %v, want %v", got, want)
	}
	content, err := os.ReadFile(filepath.Join(destDir, agent.BinaryName()))
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}
	if string(content) != codexBundleBinDir+"/"+agent.BinaryName() {
		t.Errorf("entrypoint content = %q, want the archive's %s entry", content, codexBundleBinDir+"/"+agent.BinaryName())
	}
}

func TestCodexAgent_Download__missing_entrypoint(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, []codexPackageEntry{
		codexDirEntry("bin"),
		codexFileEntry("bin/codex-code-mode-host"),
		codexFileEntry("codex-package.json"),
	})

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive has no bin/codex")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, agent.BinaryName())); !os.IsNotExist(statErr) {
		t.Errorf("entrypoint must not exist (lstat err = %v)", statErr)
	}
}

func TestCodexAgent_Download__missing_code_mode_host(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, []codexPackageEntry{
		codexDirEntry("bin"),
		codexFileEntry("bin/codex"),
		codexFileEntry("codex-package.json"),
	})

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive has no bin/codex-code-mode-host")
	}
}

func TestCodexAgent_Download__non_executable_entrypoint(t *testing.T) {
	// arrange
	// the extractor takes the mode from the archive, so a release that ships
	// bin/codex without an execute bit must be rejected, not installed
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, []codexPackageEntry{
		codexDirEntry("bin"),
		{name: "bin/codex", mode: 0o644, content: "bin/codex"},
		codexFileEntry("bin/codex-code-mode-host"),
	})

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive's entrypoint is not executable")
	}
}

func TestCodexAgent_Download__archive_checksum_mismatch(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	archive := createCodexPackage(t, codexBundleEntries())
	serveCodexRelease(t, agent, "1.2.3", archive, fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), codexAssetName(agent)))
	destDir := filepath.Join(t.TempDir(), "1.2.3")

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive does not match its published checksum")
	}
	if got := listNames(t, destDir); len(got) != 0 {
		t.Errorf("install listing = %v, want an empty directory", got)
	}
}

func TestCodexAgent_Download__no_bin_dir(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, []codexPackageEntry{codexFileEntry("codex-package.json")})

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive has no bin/ at all")
	}
	if got := listNames(t, destDir); len(got) != 0 {
		t.Errorf("install listing = %v, want an empty directory", got)
	}
}

func TestCodexAgent_Download__rejects_a_non_regular_bin_entry(t *testing.T) {
	// both entries are contained where the archive puts them and only escape
	// destDir once promotion rebases them one level up
	cases := map[string][]codexPackageEntry{
		"symlink": {codexLinkEntry(codexBundleBinDir+"/rg", "../codex-path/rg")},
		"directory": {
			codexDirEntry(codexBundleBinDir + "/helpers"),
			codexFileEntry(codexBundleBinDir + "/helpers/rg"),
		},
	}

	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			// arrange
			agent := newCodexAgent(t)
			destDir := serveCodexBundle(t, agent, append(codexBundleEntries(), extra...))

			// act
			err := agent.Download(context.Background(), "1.2.3", destDir, nil)

			// assert
			if err == nil {
				t.Fatalf("expected error when %s holds a %s", codexBundleBinDir, name)
			}
			if got := listNames(t, destDir); !slices.Equal(got, []string{codexBundleBinDir}) {
				t.Errorf("install listing = %v, want nothing promoted out of %s", got, codexBundleBinDir)
			}
		})
	}
}

func TestCodexAgent_Download__checksum_entry_missing(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	archive := createCodexPackage(t, codexBundleEntries())
	serveCodexRelease(t, agent, "1.2.3", archive, "0000  codex-package-other-target.tar.gz\n")
	destDir := filepath.Join(t.TempDir(), "1.2.3")

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the checksums file has no entry for the asset")
	}
	if _, statErr := os.Stat(filepath.Join(destDir, codexBundleBinDir)); !os.IsNotExist(statErr) {
		t.Errorf("archive must not be extracted without a checksum (stat err = %v)", statErr)
	}
}

func TestCodexAgent_IsInstalled(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	complete := codexLayout(t, map[string]os.FileMode{agent.BinaryName(): 0o755, codexCodeModeHost: 0o755})
	entrypointOnly := codexLayout(t, map[string]os.FileMode{agent.BinaryName(): 0o755})
	helperOnly := codexLayout(t, map[string]os.FileMode{codexCodeModeHost: 0o755})
	notExecutable := codexLayout(t, map[string]os.FileMode{agent.BinaryName(): 0o644, codexCodeModeHost: 0o755})
	entrypointDir := codexLayout(t, map[string]os.FileMode{codexCodeModeHost: 0o755})
	if err := os.Mkdir(filepath.Join(entrypointDir, agent.BinaryName()), 0o755); err != nil {
		t.Fatalf("create entrypoint dir: %v", err)
	}
	entrypointLink := codexLayout(t, map[string]os.FileMode{codexCodeModeHost: 0o755})
	if err := os.Symlink(codexCodeModeHost, filepath.Join(entrypointLink, agent.BinaryName())); err != nil {
		t.Fatalf("create entrypoint symlink: %v", err)
	}

	// act & assert
	if !agent.IsInstalled(complete) {
		t.Error("the entrypoint plus the helper must count as installed")
	}
	if agent.IsInstalled(entrypointOnly) {
		t.Error("the pre-fix single-binary layout must not count as installed")
	}
	if agent.IsInstalled(helperOnly) {
		t.Error("a layout without the entrypoint must not count as installed")
	}
	if agent.IsInstalled(notExecutable) {
		t.Error("a non-executable entrypoint must not count as installed")
	}
	if agent.IsInstalled(entrypointDir) {
		t.Error("an entrypoint that is a directory must not count as installed")
	}
	if agent.IsInstalled(entrypointLink) {
		t.Error("an entrypoint that is a symlink must not count as installed")
	}
	if agent.IsInstalled(filepath.Join(t.TempDir(), "absent")) {
		t.Error("a missing directory must not count as installed")
	}
}

func TestCodexAgent_flattenPackage__rejects_a_bundle_without_the_entrypoint(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := t.TempDir()
	binDir := filepath.Join(destDir, codexBundleBinDir)
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, codexCodeModeHost), []byte("x"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	// act
	err := agent.flattenPackage(destDir)

	// assert
	if err == nil {
		t.Fatal("expected error when bin/ holds no entrypoint")
	}
	if !strings.Contains(err.Error(), "promote "+agent.BinaryName()) {
		t.Errorf("error = %v, want it to name the entrypoint promotion", err)
	}
}

func newCodexAgent(t *testing.T) *CodexAgent {
	t.Helper()

	agent, err := NewCodexAgent()
	if err != nil {
		t.Fatalf("NewCodexAgent() error = %v", err)
	}
	return agent
}

func codexLayout(t *testing.T, files map[string]os.FileMode) string {
	t.Helper()

	dir := t.TempDir()
	for name, mode := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), mode); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func codexAssetName(agent *CodexAgent) string {
	return fmt.Sprintf("codex-package-%s-unknown-linux-musl.tar.gz", agent.rustArch())
}

func codexChecksums(agent *CodexAgent, archive []byte) string {
	sum := sha256.Sum256(archive)
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), codexAssetName(agent))
}

// serveCodexBundle publishes entries as the release's package asset and returns
// the version directory to install into.
func serveCodexBundle(t *testing.T, agent *CodexAgent, entries []codexPackageEntry) string {
	t.Helper()

	archive := createCodexPackage(t, entries)
	serveCodexRelease(t, agent, "1.2.3", archive, codexChecksums(agent, archive))
	return filepath.Join(t.TempDir(), "1.2.3")
}

func serveCodexRelease(t *testing.T, agent *CodexAgent, version string, archive []byte, checksums string) {
	t.Helper()

	base := "/rust-v" + version
	mux := http.NewServeMux()
	mux.HandleFunc(base+"/codex-package_SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, checksums)
	})
	mux.HandleFunc(base+"/"+codexAssetName(agent), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	agent.releaseBaseURL = server.URL
}

type codexPackageEntry struct {
	name    string
	isDir   bool
	mode    int64
	content string
	link    string
}

func codexFileEntry(name string) codexPackageEntry {
	return codexPackageEntry{name: name, mode: 0o755, content: name}
}

func codexDirEntry(name string) codexPackageEntry {
	return codexPackageEntry{name: name, isDir: true, mode: 0o755}
}

func codexLinkEntry(name, target string) codexPackageEntry {
	return codexPackageEntry{name: name, mode: 0o777, link: target}
}

// codexBundleEntries mirrors the real rust-v0.147.0 package asset, directory
// headers included.
func codexBundleEntries() []codexPackageEntry {
	return []codexPackageEntry{
		codexDirEntry("bin"),
		codexFileEntry("bin/codex"),
		codexFileEntry("bin/codex-code-mode-host"),
		codexFileEntry("codex-package.json"),
		codexDirEntry("codex-path"),
		codexFileEntry("codex-path/rg"),
		codexDirEntry("codex-resources"),
		codexFileEntry("codex-resources/bwrap"),
		codexDirEntry("codex-resources/zsh"),
		codexDirEntry("codex-resources/zsh/bin"),
		codexFileEntry("codex-resources/zsh/bin/zsh"),
	}
}

func createCodexPackage(t *testing.T, entries []codexPackageEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.content)),
			Typeflag: tar.TypeReg,
		}
		if entry.isDir {
			header.Name += "/"
			header.Size = 0
			header.Typeflag = tar.TypeDir
		}
		if entry.link != "" {
			header.Size = 0
			header.Typeflag = tar.TypeSymlink
			header.Linkname = entry.link
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if entry.isDir || entry.link != "" {
			continue
		}
		if _, err := tw.Write([]byte(entry.content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}
