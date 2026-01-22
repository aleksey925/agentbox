package agents

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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
	expected := []string{"claude", "copilot", "codex", "gemini", "opencode"}
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
	err := downloadAndExtractTarGz(context.Background(), server.URL, destDir, "testbin", "output", nil)

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
	err := downloadAndExtractTarGz(context.Background(), server.URL, t.TempDir(), "missing", "output", nil)

	// assert
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
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
