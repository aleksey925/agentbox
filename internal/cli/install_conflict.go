package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolvedExecPath returns the canonical path of the running binary, following
// symlinks — Homebrew symlinks its bin entry into the versioned Cellar directory.
func resolvedExecPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return resolved, nil
}

// isHomebrewManaged reports whether the binary at path is managed by Homebrew.
// Homebrew keeps the real binary under a versioned Cellar directory and symlinks
// it onto PATH, so the resolved path always contains a /Cellar/ segment.
func isHomebrewManaged(path string) bool {
	sep := string(filepath.Separator)
	return strings.Contains(path, sep+"Cellar"+sep)
}

// duplicateBinaries returns canonical paths of every other agentbox executable
// found on PATH, excluding the currently running one.
func duplicateBinaries(current string) []string {
	entries := filepath.SplitList(os.Getenv("PATH"))
	seen := map[string]bool{current: true}
	dups := make([]string, 0, len(entries))
	for _, dir := range entries {
		if dir == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(dir, "agentbox"))
		if err != nil || seen[resolved] {
			continue
		}
		seen[resolved] = true
		dups = append(dups, resolved)
	}
	return dups
}

// guardHomebrewManaged blocks self-management when the binary was installed via
// Homebrew; updating or removing it outside brew would desync brew's state.
// Returns true if the caller must abort.
func guardHomebrewManaged(brewCmd string) bool {
	path, err := resolvedExecPath()
	if err != nil {
		return false
	}
	if !isHomebrewManaged(path) {
		return false
	}
	fmt.Fprintf(os.Stderr, "agentbox was installed via Homebrew — use `brew %s agentbox` instead\n", brewCmd)
	return true
}

// warnDuplicateInstalls prints a warning when several agentbox binaries shadow
// each other on PATH, since the one that runs then depends on PATH order.
func warnDuplicateInstalls() {
	current, err := resolvedExecPath()
	if err != nil {
		return
	}
	dups := duplicateBinaries(current)
	if len(dups) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "\nWarning: multiple agentbox binaries found on PATH (the one that runs depends on PATH order):")
	fmt.Fprintf(os.Stderr, "  running:      %s\n", current)
	for _, d := range dups {
		fmt.Fprintf(os.Stderr, "  also on PATH: %s\n", d)
	}
	fmt.Fprintln(os.Stderr, "Keep only one to avoid version confusion.")
}
