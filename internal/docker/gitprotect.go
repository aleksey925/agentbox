package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitExecSurface are the entries inside a git dir that the host runs on ordinary
// git operations (hooks directly; config via core.hooksPath, filters, fsmonitor).
// They are mounted read-only so a hijacked agent cannot plant host-executed code
// - see CLAUDE.md "The agent cannot rewrite its own sandbox".
var gitExecSurface = []string{"hooks", "config"}

// gitProtectedPaths returns the absolute paths of the git exec surface that the
// agent could otherwise write. It is empty when the project is not a git repo,
// or when the git dir lives outside the project: only the project is mounted, so
// a git dir elsewhere is not reachable from inside the sandbox. A `.git` that
// exists but cannot be resolved (git missing from PATH, safe.directory refusal)
// is an error, not a skip - launching anyway would silently drop the protection.
func gitProtectedPaths(projectDir string) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(projectDir, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect .git: %w", err)
	}

	commonDir, err := gitCommonDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve git dir: %w", err)
	}
	return protectedPathsInDir(projectDir, commonDir)
}

func protectedPathsInDir(projectDir, commonDir string) ([]string, error) {
	rel, err := filepath.Rel(projectDir, commonDir)
	if err != nil {
		// unrelatable paths mean the git dir cannot be inside the mounted
		// project, so there is nothing reachable to protect
		return nil, nil //nolint:nilerr // see comment above
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, nil
	}
	return gitDirExecSurface(commonDir)
}

// gitDirExecSurface collects the host-executed entries of one git dir, then the
// per-worktree configs and submodule git dirs nested under it - those live
// inside the mounted project too, and a hook planted in a submodule's git dir
// runs on the host just like one in the top-level repo.
func gitDirExecSurface(gitDir string) ([]string, error) {
	paths := make([]string, 0, len(gitExecSurface))
	for _, name := range gitExecSurface {
		p := filepath.Join(gitDir, name)
		if err := ensureProtectable(p, name == "hooks"); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}

	// config.worktree can carry core.hooksPath when extensions.worktreeConfig is
	// enabled, so it is exec surface too
	worktrees, err := subdirs(filepath.Join(gitDir, "worktrees"))
	if err != nil {
		return nil, err
	}
	for _, wt := range worktrees {
		p := filepath.Join(wt, "config.worktree")
		if ensureErr := ensureProtectable(p, false); ensureErr != nil {
			return nil, ensureErr
		}
		paths = append(paths, p)
	}

	subs, err := submoduleGitDirs(filepath.Join(gitDir, "modules"))
	if err != nil {
		return nil, err
	}
	for _, sub := range subs {
		subPaths, err := gitDirExecSurface(sub)
		if err != nil {
			return nil, err
		}
		paths = append(paths, subPaths...)
	}
	return paths, nil
}

// submoduleGitDirs finds git dirs under a modules/ tree. A submodule at path
// "a/b" keeps its git dir at modules/a/b, so intermediate components are plain
// directories; an actual git dir is recognized by its HEAD file. Recognized git
// dirs are not descended into here - gitDirExecSurface recurses into their own
// modules/ - which keeps the scan out of heavyweight subtrees like objects/.
func submoduleGitDirs(dir string) ([]string, error) {
	dirs, err := subdirs(dir)
	if err != nil {
		return nil, err
	}
	var gitDirs []string
	for _, sub := range dirs {
		if _, err := os.Stat(filepath.Join(sub, "HEAD")); err == nil {
			gitDirs = append(gitDirs, sub)
			continue
		}
		nested, err := submoduleGitDirs(sub)
		if err != nil {
			return nil, err
		}
		gitDirs = append(gitDirs, nested...)
	}
	return gitDirs, nil
}

func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
	}
	return dirs, nil
}

// ensureProtectable creates a missing exec-surface entry empty so it can be
// mounted read-only; left absent, the agent could create it from inside the
// sandbox and the host would execute it on the next git operation.
func ensureProtectable(path string, isDir bool) error {
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if isDir {
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

// gitCommonDir resolves the shared git dir (where config and hooks live, even
// for worktrees, where the working tree's `.git` is a file pointing elsewhere).
func gitCommonDir(projectDir string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "--git-common-dir")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git rev-parse --git-common-dir: %w: %s", err, msg)
		}
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}

	dir := strings.TrimSpace(out.String())
	if dir == "" {
		return "", errors.New("empty git common dir")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(projectDir, dir)
	}
	return filepath.Clean(dir), nil
}

// renderGitProtectionCompose builds a compose fragment that mounts each path
// read-only at the same in-container path (mirror scheme, see CLAUDE.md "Live,
// not baked"). Long bind syntax avoids the colon-delimited short form choking on
// paths that contain spaces or colons.
func renderGitProtectionCompose(paths []string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  agentbox:\n")
	b.WriteString("    volumes:\n")
	for _, p := range paths {
		b.WriteString("      - type: bind\n")
		fmt.Fprintf(&b, "        source: %q\n", p)
		fmt.Fprintf(&b, "        target: %q\n", p)
		b.WriteString("        read_only: true\n")
	}
	return b.String()
}

// writeGitProtectionFragment writes the read-only git overlay as a temporary
// compose file to append to the run. It returns ("", noop) when there is nothing
// to protect, so callers can append unconditionally.
func writeGitProtectionFragment(projectDir string) (path string, cleanup func(), err error) {
	noop := func() {}

	paths, err := gitProtectedPaths(projectDir)
	if err != nil {
		return "", noop, err
	}
	if len(paths) == 0 {
		return "", noop, nil
	}

	f, err := os.CreateTemp("", "agentbox-gitprotect-*.yml")
	if err != nil {
		return "", noop, fmt.Errorf("create git protection fragment: %w", err)
	}
	if _, err := f.WriteString(renderGitProtectionCompose(paths)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", noop, fmt.Errorf("write git protection fragment: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", noop, fmt.Errorf("close git protection fragment: %w", err)
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}
