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
// a git dir elsewhere is not reachable from inside the sandbox.
func gitProtectedPaths(projectDir string) []string {
	commonDir, err := gitCommonDir(projectDir)
	if err != nil {
		return nil
	}
	return protectedPathsInDir(projectDir, commonDir)
}

func protectedPathsInDir(projectDir, commonDir string) []string {
	rel, err := filepath.Rel(projectDir, commonDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}

	var paths []string
	for _, name := range gitExecSurface {
		p := filepath.Join(commonDir, name)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

// gitCommonDir resolves the shared git dir (where config and hooks live, even
// for worktrees, where the working tree's `.git` is a file pointing elsewhere).
func gitCommonDir(projectDir string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "--git-common-dir")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
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

	paths := gitProtectedPaths(projectDir)
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
