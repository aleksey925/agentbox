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
	"runtime"
	"testing"
)

func TestDetectArch(t *testing.T) {
	// act
	arch, err := DetectArch()

	// assert
	switch runtime.GOARCH {
	case "amd64":
		if err != nil {
			t.Fatalf("DetectArch() error = %v", err)
		}
		if arch != "x64" {
			t.Errorf("DetectArch() = %s, want x64", arch)
		}
	case "arm64":
		if err != nil {
			t.Fatalf("DetectArch() error = %v", err)
		}
		if arch != "arm64" {
			t.Errorf("DetectArch() = %s, want arm64", arch)
		}
	default:
		if err == nil {
			t.Errorf("DetectArch() should return error for unsupported arch %s", runtime.GOARCH)
		}
	}
}

func TestAllAgentNames(t *testing.T) {
	// act
	names := AllAgentNames()

	// assert
	expected := []string{"claude", "copilot", "codex", "cursor", "opencode", "ralphex"}
	if len(names) != len(expected) {
		t.Fatalf("len(AllAgentNames()) = %d, want %d", len(names), len(expected))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("AllAgentNames()[%d] = %s, want %s", i, name, expected[i])
		}
	}
}

func TestAgentConfigDirs__covers_all_agents(t *testing.T) {
	// act & assert
	for _, name := range AllAgentNames() {
		if _, ok := agentConfigDirs[name]; !ok {
			t.Errorf("agent %q missing from agentConfigDirs map", name)
		}
	}
}

func TestSuggestedFlags__covers_all_agents(t *testing.T) {
	// act
	suggested := SuggestedFlags()

	// assert
	for _, name := range AllAgentNames() {
		if _, ok := suggested[name]; !ok {
			t.Errorf("agent %q missing from SuggestedFlags map", name)
		}
	}
}

func TestDownloadAndExtractTarGz(t *testing.T) {
	// arrange
	binaryContent := []byte("#!/bin/bash\necho hello")
	tarGzData := createTarGz(t, "testbin", binaryContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGz(context.Background(), server.URL, destDir, "testbin", "output", "", nil)

	// assert
	if err != nil {
		t.Fatalf("downloadAndExtractTarGz() error = %v", err)
	}

	destPath := filepath.Join(destDir, "output")
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}

	if !bytes.Equal(content, binaryContent) {
		t.Errorf("extracted content = %q, want %q", content, binaryContent)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat extracted file: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("file permissions = %o, want 755", info.Mode().Perm())
	}
}

func TestDownloadAndExtractTarGz__binary_not_found(t *testing.T) {
	// arrange
	tarGzData := createTarGz(t, "otherfile", []byte("hello"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	// act
	err := downloadAndExtractTarGz(context.Background(), server.URL, t.TempDir(), "missing", "output", "", nil)

	// assert
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestDownloadAndExtractTarGz__checksum_match(t *testing.T) {
	// arrange
	binaryContent := []byte("#!/bin/bash\necho hello")
	tarGzData := createTarGz(t, "testbin", binaryContent)
	sum := sha256.Sum256(tarGzData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGz(
		context.Background(), server.URL, destDir, "testbin", "output", hex.EncodeToString(sum[:]), nil,
	)

	// assert
	if err != nil {
		t.Fatalf("downloadAndExtractTarGz() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destDir, "output"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if !bytes.Equal(content, binaryContent) {
		t.Errorf("extracted content = %q, want %q", content, binaryContent)
	}
}

func TestDownloadAndExtractTarGz__checksum_mismatch(t *testing.T) {
	// arrange
	tarGzData := createTarGz(t, "testbin", []byte("#!/bin/bash\necho hello"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGz(
		context.Background(), server.URL, destDir, "testbin", "output", "deadbeef", nil,
	)

	// assert
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "output")); !os.IsNotExist(statErr) {
		t.Errorf("binary must not be extracted on checksum mismatch (stat err = %v)", statErr)
	}
}

func TestFetchChecksum(t *testing.T) {
	// arrange
	body := "111  other-asset.tar.gz\n" +
		"222  *binary-mode-asset.tar.gz\n" +
		"333  wanted-asset.tar.gz\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	// act
	got, err := fetchChecksum(context.Background(), server.URL, "wanted-asset.tar.gz")
	binaryMode, binaryErr := fetchChecksum(context.Background(), server.URL, "binary-mode-asset.tar.gz")
	_, missingErr := fetchChecksum(context.Background(), server.URL, "absent.tar.gz")

	// assert
	if err != nil {
		t.Fatalf("fetchChecksum() error = %v", err)
	}
	if got != "333" {
		t.Errorf("fetchChecksum() = %q, want %q", got, "333")
	}
	if binaryErr != nil {
		t.Fatalf("fetchChecksum() binary-mode error = %v", binaryErr)
	}
	if binaryMode != "222" {
		t.Errorf("fetchChecksum() binary-mode = %q, want %q", binaryMode, "222")
	}
	if missingErr == nil {
		t.Error("expected error for asset missing from checksums file")
	}
}

func TestStripPathComponents(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"dist-package/cursor-agent", 1, "cursor-agent"},
		{"dist-package/sub/file.js", 1, "sub/file.js"},
		{"dist-package", 1, ""},
		{"file.txt", 1, ""},
		{"a/b/c", 2, "c"},
		{"a/b/c", 3, ""},
		{"/leading/slash/file", 1, "slash/file"},
		{"keep/all", 0, "keep/all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			got := stripPathComponents(tt.name, tt.n)

			// assert
			if got != tt.want {
				t.Errorf("stripPathComponents(%q, %d) = %q, want %q", tt.name, tt.n, got, tt.want)
			}
		})
	}
}

func TestDownloadAndExtractTarGzAll(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/main", content: []byte("#!/bin/bash\necho hi"), mode: 0o755},
		{name: "pkg/sub/lib.js", content: []byte("console.log(1)"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("downloadAndExtractTarGzAll() error = %v", err)
	}

	mainPath := filepath.Join(destDir, "main")
	mainContent, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	if string(mainContent) != "#!/bin/bash\necho hi" {
		t.Errorf("main content = %q", mainContent)
	}

	mainInfo, err := os.Stat(mainPath)
	if err != nil {
		t.Fatalf("stat main: %v", err)
	}
	if mainInfo.Mode().Perm() != 0o755 {
		t.Errorf("main perms = %o, want 755", mainInfo.Mode().Perm())
	}

	libContent, err := os.ReadFile(filepath.Join(destDir, "sub", "lib.js"))
	if err != nil {
		t.Fatalf("read lib.js: %v", err)
	}
	if string(libContent) != "console.log(1)" {
		t.Errorf("lib.js content = %q", libContent)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_escaping_symlink(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "../../../etc/passwd"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for escaping symlink")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("escaping symlink should not have been created (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_absolute_symlink(t *testing.T) {
	// arrange — absolute targets bypass the join-based prefix check, so they
	// must be rejected explicitly.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "/etc/passwd"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for absolute symlink target")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("absolute symlink should not have been created (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__allows_internal_symlink(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/real", content: []byte("hi"), mode: 0o644},
		{name: "pkg/link", rawType: tar.TypeSymlink, linkname: "real"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil)

	// assert
	if err != nil {
		t.Fatalf("downloadAndExtractTarGzAll() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(destDir, "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "real" {
		t.Errorf("symlink target = %q, want %q", target, "real")
	}
}

func TestDownloadAndExtractTarGzAll__rejects_hardlink(t *testing.T) {
	// arrange — hard links aren't expected in upstream archives, extractor
	// must fail loudly rather than silently skip and produce a broken install.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/real", content: []byte("hi"), mode: 0o644},
		{name: "pkg/link", rawType: tar.TypeLink, linkname: "pkg/real"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for hard link entry")
	}
}

func TestDownloadAndExtractTarGzAll__strips_setuid(t *testing.T) {
	// arrange — archive declares mode with setuid bit; extractor must strip it.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/bin", content: []byte("x"), mode: 0o4755},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	if err := downloadAndExtractTarGzAll(context.Background(), server.URL, destDir, nil); err != nil {
		t.Fatalf("downloadAndExtractTarGzAll() error = %v", err)
	}

	// assert
	info, err := os.Stat(filepath.Join(destDir, "bin"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&os.ModeSetuid != 0 {
		t.Errorf("setuid bit not stripped (mode = %o)", info.Mode())
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("perms = %o, want 755", info.Mode().Perm())
	}
}

type tarFile struct {
	name     string
	content  []byte
	mode     int64
	isDir    bool
	rawType  byte // overrides regular-file default; required for symlinks, hard links, etc.
	linkname string
}

func createMultiFileTarGz(t *testing.T, files []tarFile) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, f := range files {
		hdr := &tar.Header{Name: f.name, Mode: f.mode}
		switch {
		case f.rawType != 0:
			hdr.Typeflag = f.rawType
			hdr.Linkname = f.linkname
		case f.isDir:
			hdr.Typeflag = tar.TypeDir
		default:
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(f.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write(f.content); err != nil {
				t.Fatalf("write tar content: %v", err)
			}
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

func createTarGz(t *testing.T, filename string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if err := tw.WriteHeader(&tar.Header{
		Name:     filename,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}
