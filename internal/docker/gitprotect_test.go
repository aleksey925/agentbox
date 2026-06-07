package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestProtectedPathsInDir__git_dir_inside_project(t *testing.T) {
	// arrange
	projectDir := t.TempDir()
	commonDir := filepath.Join(projectDir, ".git")
	hooks := filepath.Join(commonDir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(commonDir, "config")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	paths := protectedPathsInDir(projectDir, commonDir)

	// assert
	if !slices.Equal(paths, []string{hooks, configPath}) {
		t.Errorf("protectedPathsInDir = %v, want [%s %s]", paths, hooks, configPath)
	}
}

func TestProtectedPathsInDir__skips_missing_entries(t *testing.T) {
	// arrange
	projectDir := t.TempDir()
	commonDir := filepath.Join(projectDir, ".git")
	hooks := filepath.Join(commonDir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	paths := protectedPathsInDir(projectDir, commonDir)

	// assert
	if !slices.Equal(paths, []string{hooks}) {
		t.Errorf("protectedPathsInDir = %v, want [%s]", paths, hooks)
	}
}

func TestProtectedPathsInDir__git_dir_outside_project_returns_nil(t *testing.T) {
	// arrange
	base := t.TempDir()
	projectDir := filepath.Join(base, "project")
	commonDir := filepath.Join(base, "elsewhere", ".git")
	if err := os.MkdirAll(filepath.Join(commonDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	paths := protectedPathsInDir(projectDir, commonDir)

	// assert
	if paths != nil {
		t.Errorf("expected nil for git dir outside project, got %v", paths)
	}
}

func TestRenderGitProtectionCompose(t *testing.T) {
	// arrange
	paths := []string{"/home/u/p/.git/hooks", "/home/u/p/.git/config"}

	// act
	got := renderGitProtectionCompose(paths)

	// assert
	want := `services:
  agentbox:
    volumes:
      - type: bind
        source: "/home/u/p/.git/hooks"
        target: "/home/u/p/.git/hooks"
        read_only: true
      - type: bind
        source: "/home/u/p/.git/config"
        target: "/home/u/p/.git/config"
        read_only: true
`
	if got != want {
		t.Errorf("renderGitProtectionCompose mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestGitProtectedPaths__real_repo(t *testing.T) {
	// arrange
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	projectDir := t.TempDir()
	cmd := exec.Command("git", "-C", projectDir, "init") // #nosec G204 -- test command in a temp dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// act
	paths := gitProtectedPaths(projectDir)

	// assert
	want := []string{
		filepath.Join(projectDir, ".git", "hooks"),
		filepath.Join(projectDir, ".git", "config"),
	}
	if !slices.Equal(paths, want) {
		t.Errorf("gitProtectedPaths = %v, want %v", paths, want)
	}
}

func TestGitProtectedPaths__non_git_dir_returns_nil(t *testing.T) {
	// arrange
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// act
	paths := gitProtectedPaths(t.TempDir())

	// assert
	if paths != nil {
		t.Errorf("expected nil for non-git dir, got %v", paths)
	}
}
