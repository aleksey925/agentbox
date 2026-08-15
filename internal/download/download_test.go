package download

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
	"strings"
	"testing"
)

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
	err := DownloadAndExtractTarGz(context.Background(), server.URL, destDir, "testbin", "output", "", nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGz() error = %v", err)
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

func TestDownloadAndExtractTarGz__rejects_oversize(t *testing.T) {
	// arrange — lower the cap so a small archive trips it without a multi-GB file.
	original := MaxArtifactBytes
	MaxArtifactBytes = 1024
	defer func() { MaxArtifactBytes = original }()

	tarGzData := createTarGz(t, "testbin", make([]byte, 8192))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGz(context.Background(), server.URL, destDir, "testbin", "output", "", nil)

	// assert
	if err == nil {
		t.Fatal("expected error when decompressed output exceeds the cap")
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
	err := DownloadAndExtractTarGz(context.Background(), server.URL, t.TempDir(), "missing", "output", "", nil)

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
	err := DownloadAndExtractTarGz(
		context.Background(), server.URL, destDir, "testbin", "output", hex.EncodeToString(sum[:]), nil,
	)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGz() error = %v", err)
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
	err := DownloadAndExtractTarGz(
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
	got, err := FetchChecksum(context.Background(), server.URL, "wanted-asset.tar.gz")
	binaryMode, binaryErr := FetchChecksum(context.Background(), server.URL, "binary-mode-asset.tar.gz")
	_, missingErr := FetchChecksum(context.Background(), server.URL, "absent.tar.gz")

	// assert
	if err != nil {
		t.Fatalf("FetchChecksum() error = %v", err)
	}
	if got != "333" {
		t.Errorf("FetchChecksum() = %q, want %q", got, "333")
	}
	if binaryErr != nil {
		t.Fatalf("FetchChecksum() binary-mode error = %v", binaryErr)
	}
	if binaryMode != "222" {
		t.Errorf("FetchChecksum() binary-mode = %q, want %q", binaryMode, "222")
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
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
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

func TestDownloadAndExtractTarGzAll__keeps_layout_without_stripping(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "bin/", isDir: true},
		{name: "bin/main", content: []byte("hi"), mode: 0o755},
		{name: "resources/lib", content: []byte("x"), mode: 0o644},
		{name: "meta.json", content: []byte("{}"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 0, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	for _, rel := range []string{"bin/main", "resources/lib", "meta.json"} {
		if _, statErr := os.Stat(filepath.Join(destDir, rel)); statErr != nil {
			t.Errorf("expected %s to be extracted, stat err = %v", rel, statErr)
		}
	}
}

func TestDownloadAndExtractTarGzAll__rejects_escaping_path(t *testing.T) {
	// arrange
	// codex extracts with no stripping, so an entry name reaches the destination
	// path verbatim and the containment check is all that keeps it inside destDir
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "../evil", content: []byte("x"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 0, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for an entry escaping the archive root")
	}
	if _, statErr := os.Lstat(filepath.Join(filepath.Dir(destDir), "evil")); !os.IsNotExist(statErr) {
		t.Errorf("escaping entry should not have been written (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__checksum_match(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/main", content: []byte("hi"), mode: 0o755},
	})
	sum := sha256.Sum256(tarGzData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, hex.EncodeToString(sum[:]), 1, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "main")); statErr != nil {
		t.Errorf("expected extracted file, stat err = %v", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__checksum_mismatch(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/main", content: []byte("hi"), mode: 0o755},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "deadbeef", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "main")); !os.IsNotExist(statErr) {
		t.Errorf("archive must not be extracted on checksum mismatch (stat err = %v)", statErr)
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
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

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
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for absolute symlink target")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("absolute symlink should not have been created (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_write_through_a_symlinked_parent(t *testing.T) {
	// arrange — every entry passes the lexical checks on its own: `x/up -> ..`
	// resolves to destDir, and `x/up/esc -> ..` resolves to destDir/x. But
	// `x/up` really *is* destDir, so `esc` lands as destDir/esc pointing at
	// destDir's parent, and the file below it is written outside.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/x/", isDir: true},
		{name: "pkg/x/up", rawType: tar.TypeSymlink, linkname: ".."},
		{name: "pkg/x/up/esc", rawType: tar.TypeSymlink, linkname: ".."},
		{name: "pkg/x/up/esc/leaked", content: []byte("owned"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for an entry written through a symlinked parent")
	}
	if _, statErr := os.Lstat(filepath.Join(filepath.Dir(destDir), "leaked")); !os.IsNotExist(statErr) {
		t.Errorf("entry must not be written outside destDir (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_a_file_overwriting_an_escaping_symlink(t *testing.T) {
	// arrange — `leak -> x/up/../evil` cleans to destDir/x/evil lexically, so the
	// target check passes, but `x/up` is a link to destDir, so it really points at
	// destDir's parent. The regular entry of the same name is then written through it.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/x/", isDir: true},
		{name: "pkg/x/up", rawType: tar.TypeSymlink, linkname: ".."},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "x/up/../evil"},
		{name: "pkg/leak", content: []byte("owned"), mode: 0o644},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for a file written through an escaping symlink")
	}
	if _, statErr := os.Lstat(filepath.Join(filepath.Dir(destDir), "evil")); !os.IsNotExist(statErr) {
		t.Errorf("entry must not be written outside destDir (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_a_symlink_escaping_through_a_symlinked_parent(t *testing.T) {
	// arrange — `leak -> x/up/../evil` cleans to destDir/x/evil, so the lexical
	// target check passes, but `x/up` is a link to destDir, so the stored link
	// really resolves to destDir's parent. Nothing writes through it here: the
	// escaping link itself must not survive extraction.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/x/", isDir: true},
		{name: "pkg/x/up", rawType: tar.TypeSymlink, linkname: ".."},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "x/up/../evil"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for a symlink resolving outside destDir")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("escaping symlink should not have been left behind (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__rejects_a_symlinked_parent_shipped_after_the_link(t *testing.T) {
	// arrange — same escape with the entries swapped: when `leak` is created
	// `x/up` does not exist yet, so the link is still dangling and only the
	// finished tree shows where it points.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "x/up/../evil"},
		{name: "pkg/x/", isDir: true},
		{name: "pkg/x/up", rawType: tar.TypeSymlink, linkname: ".."},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for a symlink resolving outside destDir")
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("escaping symlink should not have been left behind (lstat err = %v)", statErr)
	}
}

func TestDownloadAndExtractTarGzAll__removes_an_escaping_symlink_when_a_later_entry_fails(t *testing.T) {
	// arrange — the same escape, followed by an entry the extractor rejects, so
	// the tar loop aborts before the tree is complete.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/x/", isDir: true},
		{name: "pkg/x/up", rawType: tar.TypeSymlink, linkname: ".."},
		{name: "pkg/leak", rawType: tar.TypeSymlink, linkname: "x/up/../evil"},
		{name: "pkg/fifo", rawType: tar.TypeFifo},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "install")

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for the unsupported entry")
	}
	if !strings.Contains(err.Error(), "unsupported tar entry type") {
		t.Errorf("error must report the entry that failed, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "leak")); !os.IsNotExist(statErr) {
		t.Errorf("escaping symlink should not have been left behind (lstat err = %v)", statErr)
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
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(destDir, "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "real" {
		t.Errorf("symlink target = %q, want %q", target, "real")
	}
}

func TestDownloadAndExtractTarGzAll__allows_a_symlink_climbing_to_a_sibling_directory(t *testing.T) {
	// arrange — the `bin/x -> ../lib/x` shape vendor bundles use, with the target
	// shipped after the link so it is dangling at creation time.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/bin/", isDir: true},
		{name: "pkg/bin/tool", rawType: tar.TypeSymlink, linkname: "../lib/tool"},
		{name: "pkg/lib/", isDir: true},
		{name: "pkg/lib/tool", content: []byte("hi"), mode: 0o755},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destDir, "bin", "tool"))
	if err != nil {
		t.Fatalf("read through symlink: %v", err)
	}
	if string(content) != "hi" {
		t.Errorf("content through symlink = %q, want %q", content, "hi")
	}
}

func TestDownloadAndExtractTarGzAll__allows_a_symlink_to_a_target_the_archive_never_ships(t *testing.T) {
	// arrange
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pkg/", isDir: true},
		{name: "pkg/link", rawType: tar.TypeSymlink, linkname: "never-shipped"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(destDir, "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "never-shipped" {
		t.Errorf("symlink target = %q, want %q", target, "never-shipped")
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
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil)

	// assert
	if err == nil {
		t.Fatal("expected error for hard link entry")
	}
}

func TestDownloadAndExtractTarGzAll__skips_a_pax_global_header(t *testing.T) {
	// arrange — a global header is a single-component name, so stripping hides it;
	// codex extracts with no stripping and the entry reaches the extractor.
	tarGzData := createMultiFileTarGz(t, []tarFile{
		{name: "pax_global_header", rawType: tar.TypeXGlobalHeader},
		{name: "bin/codex", content: []byte("hi"), mode: 0o755},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarGzData)
	}))
	defer server.Close()

	destDir := t.TempDir()

	// act
	err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 0, nil)

	// assert
	if err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "bin", "codex")); statErr != nil {
		t.Errorf("expected bin/codex to be extracted, stat err = %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(destDir, "pax_global_header")); !os.IsNotExist(statErr) {
		t.Errorf("global header should not land on disk (lstat err = %v)", statErr)
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
	if err := DownloadAndExtractTarGzAll(context.Background(), server.URL, destDir, "", 1, nil); err != nil {
		t.Fatalf("DownloadAndExtractTarGzAll() error = %v", err)
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
