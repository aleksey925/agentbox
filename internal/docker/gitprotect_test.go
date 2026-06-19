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
	paths, err := protectedPathsInDir(projectDir, commonDir)

	// assert
	if err != nil {
		t.Fatalf("protectedPathsInDir error: %v", err)
	}
	if !slices.Equal(paths, []string{hooks, configPath}) {
		t.Errorf("protectedPathsInDir = %v, want [%s %s]", paths, hooks, configPath)
	}
}

func TestProtectedPathsInDir__creates_missing_entries(t *testing.T) {
	// arrange - an absent config must be created and protected, or the agent
	// could create it from inside the sandbox after launch.
	projectDir := t.TempDir()
	commonDir := filepath.Join(projectDir, ".git")
	hooks := filepath.Join(commonDir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(commonDir, "config")

	// act
	paths, err := protectedPathsInDir(projectDir, commonDir)

	// assert
	if err != nil {
		t.Fatalf("protectedPathsInDir error: %v", err)
	}
	if !slices.Equal(paths, []string{hooks, configPath}) {
		t.Errorf("protectedPathsInDir = %v, want [%s %s]", paths, hooks, configPath)
	}
	info, statErr := os.Stat(configPath)
	if statErr != nil {
		t.Fatalf("missing config must be created: %v", statErr)
	}
	if info.Size() != 0 {
		t.Errorf("created config must be empty, got %d bytes", info.Size())
	}
}

func TestProtectedPathsInDir__covers_submodules_and_worktrees(t *testing.T) {
	// arrange - submodule git dirs under modules/ and per-worktree configs are
	// host-executed surface too; a hook planted there runs on the host on the
	// next `git submodule update` or worktree operation.
	projectDir := t.TempDir()
	commonDir := filepath.Join(projectDir, ".git")
	hooks := filepath.Join(commonDir, "hooks")
	configPath := filepath.Join(commonDir, "config")
	subGitDir := filepath.Join(commonDir, "modules", "a", "b")
	worktreeDir := filepath.Join(commonDir, "worktrees", "wt1")
	for _, dir := range []string{hooks, subGitDir, worktreeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{configPath, filepath.Join(subGitDir, "HEAD")} {
		if err := os.WriteFile(file, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// act
	paths, err := protectedPathsInDir(projectDir, commonDir)

	// assert
	if err != nil {
		t.Fatalf("protectedPathsInDir error: %v", err)
	}
	want := []string{
		hooks,
		configPath,
		filepath.Join(worktreeDir, "config.worktree"),
		filepath.Join(subGitDir, "hooks"),
		filepath.Join(subGitDir, "config"),
	}
	if !slices.Equal(paths, want) {
		t.Errorf("protectedPathsInDir = %v, want %v", paths, want)
	}
	for _, p := range want {
		if _, statErr := os.Lstat(p); statErr != nil {
			t.Errorf("protected path %s must exist so it can be mounted: %v", p, statErr)
		}
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
	paths, err := protectedPathsInDir(projectDir, commonDir)

	// assert
	if err != nil {
		t.Fatalf("protectedPathsInDir error: %v", err)
	}
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
	paths, err := gitProtectedPaths(projectDir)

	// assert
	if err != nil {
		t.Fatalf("gitProtectedPaths error: %v", err)
	}
	want := []string{
		filepath.Join(projectDir, ".git", "hooks"),
		filepath.Join(projectDir, ".git", "config"),
	}
	if !slices.Equal(paths, want) {
		t.Errorf("gitProtectedPaths = %v, want %v", paths, want)
	}
}

func TestGitProtectedPaths__non_git_dir_returns_nil(t *testing.T) {
	// act
	paths, err := gitProtectedPaths(t.TempDir())

	// assert
	if err != nil {
		t.Fatalf("gitProtectedPaths error: %v", err)
	}
	if paths != nil {
		t.Errorf("expected nil for non-git dir, got %v", paths)
	}
}

func TestGitProtectedPaths__unresolvable_git_errors(t *testing.T) {
	// arrange - `.git` exists but git cannot resolve it; launching anyway would
	// silently run the sandbox without the read-only overlay, so it must fail.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	projectDir := t.TempDir()
	gitFile := filepath.Join(projectDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /nonexistent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := gitProtectedPaths(projectDir)

	// assert
	if err == nil {
		t.Fatal("expected error for unresolvable .git")
	}
}
