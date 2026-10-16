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
	want := []string{agent.BinaryName() + " -> " + filepath.ToSlash(agent.entrypointTarget())}
	for _, entry := range codexBundleEntries() {
		if !entry.isDir {
			want = append(want, entry.name)
		}
	}
	slices.Sort(want)
	got := listTree(t, destDir)
	slices.Sort(got)
	if !slices.Equal(got, want) {
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
	for _, missing := range codexRequiredFiles(newCodexAgent(t)) {
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
			if err == nil || !strings.Contains(err.Error(), codexIncompleteMessage) {
				t.Fatalf("Download() error = %v, want %q when the archive has no %s", err, codexIncompleteMessage, missing)
			}
		})
	}
}

func TestCodexAgent_Download__rejects_an_archive_that_ships_another_entry_as_the_entrypoint_link(t *testing.T) {
	linkName := newCodexAgent(t).BinaryName()
	cases := []struct {
		name  string
		entry codexPackageEntry
	}{
		{name: "regular file", entry: codexFileEntry(linkName)},
		{name: "foreign link", entry: codexPackageEntry{name: linkName, mode: 0o777, linkTarget: "codex-path/rg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			agent := newCodexAgent(t)
			destDir := serveCodexBundle(t, agent, append(codexBundleEntries(), tc.entry))

			// act
			err := agent.Download(context.Background(), "1.2.3", destDir, nil)

			// assert
			if err == nil || !strings.Contains(err.Error(), "link entrypoint") {
				t.Fatalf("Download() error = %v, want a link entrypoint error", err)
			}
		})
	}
}

func TestCodexAgent_Download__accepts_an_archive_that_ships_the_same_entrypoint_link(t *testing.T) {
	// arrange
	agent := newCodexAgent(t)
	link := codexPackageEntry{name: agent.BinaryName(), mode: 0o777, linkTarget: agent.entrypointTarget()}
	destDir := serveCodexBundle(t, agent, append(codexBundleEntries(), link))

	// act
	err := agent.Download(context.Background(), "1.2.3", destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if !agent.IsInstalled(destDir) {
		t.Error("a package that ships the entrypoint link itself must count as installed")
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
	if err == nil || !strings.Contains(err.Error(), codexIncompleteMessage) {
		t.Fatalf("Download() error = %v, want %q", err, codexIncompleteMessage)
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
	if got := listTree(t, destDir); len(got) != 0 {
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
	type seedFunc func(t *testing.T, agent *CodexAgent, dir string)
	cases := []struct {
		name string
		seed seedFunc
		want bool
	}{
		{
			name: "whole package plus the entrypoint link",
			seed: seedCodexPackage,
			want: true,
		},
		{
			name: "flattened layout of older agentbox versions",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				writeStubFile(t, filepath.Join(dir, agent.BinaryName()))
				writeStubFile(t, filepath.Join(dir, codexHostBinary))
			},
		},
		{
			name: "no entrypoint link",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, agent.BinaryName())
			},
		},
		{
			name: "entrypoint link to another file",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, agent.BinaryName())
				if err := os.Symlink(filepath.Join("codex-path", "rg"), filepath.Join(dir, agent.BinaryName())); err != nil {
					t.Fatalf("create foreign link: %v", err)
				}
			},
		},
		{
			name: "regular file in place of the entrypoint link",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, agent.BinaryName())
				writeStubFile(t, filepath.Join(dir, agent.BinaryName()))
			},
		},
		{
			name: "non-executable entrypoint",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				if err := os.Chmod(filepath.Join(dir, agent.entrypointTarget()), 0o644); err != nil {
					t.Fatalf("chmod entrypoint: %v", err)
				}
			},
		},
		{
			name: "manifest that is not a regular file",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, codexPackageManifest)
				if err := os.Mkdir(filepath.Join(dir, codexPackageManifest), 0o755); err != nil {
					t.Fatalf("create manifest dir: %v", err)
				}
			},
		},
		{
			name: "entrypoint target that is a symlink",
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, agent.entrypointTarget())
				writeStubFile(t, filepath.Join(dir, "real-codex"))
				if err := os.Symlink(filepath.Join("..", "real-codex"), filepath.Join(dir, agent.entrypointTarget())); err != nil {
					t.Fatalf("link entrypoint target: %v", err)
				}
			},
		},
		{
			name: "missing directory",
			seed: func(*testing.T, *CodexAgent, string) {},
		},
	}
	for _, missing := range codexRequiredFiles(newCodexAgent(t)) {
		cases = append(cases, struct {
			name string
			seed seedFunc
			want bool
		}{
			name: "package without " + missing,
			seed: func(t *testing.T, agent *CodexAgent, dir string) {
				seedCodexPackage(t, agent, dir)
				removeCodexFile(t, dir, missing)
			},
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			agent := newCodexAgent(t)
			dir := filepath.Join(t.TempDir(), "1.2.3")
			tc.seed(t, agent, dir)

			// act
			got := agent.IsInstalled(dir)

			// assert
			if got != tc.want {
				t.Errorf("IsInstalled() = %v, want %v", got, tc.want)
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

const (
	codexIncompleteMessage = "package archive did not yield"
	codexHostBinary        = "codex-code-mode-host"
)

func codexRequiredFiles(agent *CodexAgent) []string {
	return []string{agent.entrypointTarget(), codexPackageManifest}
}

// seedCodexPackage writes the minimal layout IsInstalled accepts into dir.
func seedCodexPackage(t *testing.T, agent *CodexAgent, dir string) {
	t.Helper()

	for _, name := range codexRequiredFiles(agent) {
		writeStubFile(t, filepath.Join(dir, name))
	}
	if err := os.Symlink(agent.entrypointTarget(), filepath.Join(dir, agent.BinaryName())); err != nil {
		t.Fatalf("link entrypoint: %v", err)
	}
}

func writeStubFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func removeCodexFile(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatalf("remove %s: %v", name, err)
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
	name       string
	isDir      bool
	mode       int64
	content    string
	linkTarget string
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
		codexFileEntry("bin/" + codexHostBinary),
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
		if entry.linkTarget != "" {
			header.Linkname = entry.linkTarget
			header.Typeflag = tar.TypeSymlink
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if entry.isDir || entry.linkTarget != "" {
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
