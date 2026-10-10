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
	want := []string{
		"bin/codex",
		"bin/codex-code-mode-host",
		"codex -> bin/codex",
		"codex-package.json",
		"codex-path/rg",
		"codex-resources/bwrap",
		"codex-resources/zsh/bin/zsh",
	}
	if got := listTree(t, destDir); !slices.Equal(got, want) {
		t.Errorf("install tree = %v, want %v", got, want)
	}
	content, err := os.ReadFile(filepath.Join(destDir, agent.BinaryName()))
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}
	if string(content) != agent.entrypointTarget() {
		t.Errorf("entrypoint content = %q, want the archive's %s entry", content, agent.entrypointTarget())
	}
}

func TestCodexAgent_Download__rejects_an_incomplete_package(t *testing.T) {
	for _, missing := range codexRequiredFiles() {
		t.Run(missing, func(t *testing.T) {
			// arrange
			agent := newCodexAgent(t)
			entries := slices.DeleteFunc(codexBundleEntries(), func(entry codexPackageEntry) bool {
				return entry.name == missing
			})
			destDir := serveCodexBundle(t, agent, entries)

			// act
			err := agent.Download(context.Background(), "1.2.3", destDir, nil)

			// assert
			if err == nil {
				t.Fatalf("expected error when the archive has no %s", missing)
			}
		})
	}
}

func TestCodexAgent_Download__rejects_an_archive_that_ships_the_entrypoint_link_name(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	destDir := serveCodexBundle(t, agent, append(codexBundleEntries(), codexFileEntry(agent.BinaryName())))

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error when the archive root already holds the entrypoint name")
	}
}

func TestCodexAgent_Download__non_executable_entrypoint(t *testing.T) {
	// arrange
	// the extractor takes the mode from the archive, so a release that ships
	// bin/codex without an execute bit must be rejected, not installed
	agent := newCodexAgent(t)
	entries := codexBundleEntries()
	for i := range entries {
		if entries[i].name == agent.entrypointTarget() {
			entries[i].mode = 0o644
		}
	}
	destDir := serveCodexBundle(t, agent, entries)

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
	complete := t.TempDir()
	seedCodexPackage(t, complete)

	flat := t.TempDir()
	writeExecutable(t, filepath.Join(flat, agent.BinaryName()))
	writeExecutable(t, filepath.Join(flat, "codex-code-mode-host"))

	noLink := t.TempDir()
	seedCodexPackage(t, noLink)
	if err := os.Remove(filepath.Join(noLink, agent.BinaryName())); err != nil {
		t.Fatalf("remove entrypoint link: %v", err)
	}

	foreignLink := t.TempDir()
	seedCodexPackage(t, foreignLink)
	if err := os.Remove(filepath.Join(foreignLink, agent.BinaryName())); err != nil {
		t.Fatalf("remove entrypoint link: %v", err)
	}
	if err := os.Symlink(filepath.Join("codex-path", "rg"), filepath.Join(foreignLink, agent.BinaryName())); err != nil {
		t.Fatalf("create foreign link: %v", err)
	}

	notExecutable := t.TempDir()
	seedCodexPackage(t, notExecutable)
	if err := os.Chmod(filepath.Join(notExecutable, agent.entrypointTarget()), 0o644); err != nil {
		t.Fatalf("chmod entrypoint: %v", err)
	}

	// act & assert
	if !agent.IsInstalled(complete) {
		t.Error("the whole package plus the entrypoint link must count as installed")
	}
	if agent.IsInstalled(flat) {
		t.Error("the flattened layout of older agentbox versions must not count as installed")
	}
	if agent.IsInstalled(noLink) {
		t.Error("a package without the entrypoint link must not count as installed")
	}
	if agent.IsInstalled(foreignLink) {
		t.Error("an entrypoint link to another file must not count as installed")
	}
	if agent.IsInstalled(notExecutable) {
		t.Error("a non-executable entrypoint must not count as installed")
	}
	if agent.IsInstalled(filepath.Join(t.TempDir(), "absent")) {
		t.Error("a missing directory must not count as installed")
	}
}

func TestCodexAgent_IsInstalled__rejects_a_package_missing_a_required_file(t *testing.T) {
	for _, missing := range codexRequiredFiles() {
		t.Run(missing, func(t *testing.T) {
			// arrange
			agent := newCodexAgent(t)
			dir := t.TempDir()
			seedCodexPackage(t, dir)
			if err := os.Remove(filepath.Join(dir, missing)); err != nil {
				t.Fatalf("remove %s: %v", missing, err)
			}

			// act & assert
			if agent.IsInstalled(dir) {
				t.Errorf("a package without %s must not count as installed", missing)
			}
		})
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

func codexRequiredFiles() []string {
	return []string{filepath.Join(codexBundleBinDir, "codex"), codexPackageManifest}
}

// seedCodexPackage writes the layout Download produces into dir.
func seedCodexPackage(t *testing.T, dir string) {
	t.Helper()

	for _, name := range codexRequiredFiles() {
		writeExecutable(t, filepath.Join(dir, name))
	}
	if err := os.Symlink(filepath.Join(codexBundleBinDir, "codex"), filepath.Join(dir, "codex")); err != nil {
		t.Fatalf("link entrypoint: %v", err)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// listTree lists every non-directory under dir by slash path, a symlink as
// "name -> target".
func listTree(t *testing.T, dir string) []string {
	t.Helper()

	var tree []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, dir+string(filepath.Separator)))
		if entry.Type()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				t.Fatalf("readlink %s: %v", path, linkErr)
			}
			rel += " -> " + filepath.ToSlash(target)
		}
		tree = append(tree, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return tree
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
}

func codexFileEntry(name string) codexPackageEntry {
	return codexPackageEntry{name: name, mode: 0o755, content: name}
}

func codexDirEntry(name string) codexPackageEntry {
	return codexPackageEntry{name: name, isDir: true, mode: 0o755}
}

// codexBundleEntries mirrors the real rust-v0.147.0 package asset, directory
// headers included; later releases add more under codex-resources/.
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
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if entry.isDir {
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
