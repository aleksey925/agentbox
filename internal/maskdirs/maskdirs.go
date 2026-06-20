// Package maskdirs manages the user-editable file that lists project
// sub-directories to hide from the sandbox.
//
// Each listed directory is replaced inside the container by its own isolated,
// initially-empty Docker volume; the host directory is never mounted. The file
// lives in .agentbox/ (mounted read-only into the container, so the agent
// cannot edit it and un-mask itself) and is generated, not a skeleton template,
// because its content depends on per-project detection - see CLAUDE.md
// "Masking project sub-directories".
package maskdirs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// candidateNames are the directory names auto-detected at the project root and
// one level deep, written active when found. They are host-built artifacts that
// break inside the Linux container. A build artifact a lockfile regenerates
// (Go's vendor/) is deliberately excluded - see CLAUDE.md "Masking project
// sub-directories".
var candidateNames = []string{".venv", "venv", ".tox", "node_modules"}

const fileHeader = `# Project sub-directories to hide from the sandbox.
#
# Each listed directory is replaced inside the container by its own isolated,
# empty Docker volume. The host directory is never mounted and never touched -
# the agent works in its own copy. This hides host-built artifacts that do not
# work in the Linux container (a macOS .venv, a platform node_modules).
#
# Format: one path per line, relative to the project root. A line starting with
# '#' is a comment (inline '#' is not); blank lines are ignored. Nested paths
# are allowed (e.g. frontend/node_modules). Edits apply on the next 'agentbox run'.

`

// DetectMaskDirs returns the sorted relative paths of maskable directories
// found in the project: the candidate names at the root, plus the same names
// inside direct sub-directories (one level deep). It skips .agentbox and .git,
// and does not descend into a dir already masked at the root (a candidate name)
// - its contents live inside that mask, so a nested entry would be redundant.
func DetectMaskDirs(projectDir string) []string {
	noDescend := map[string]bool{".agentbox": true, ".git": true}
	for _, name := range candidateNames {
		noDescend[name] = true
	}

	var found []string
	for _, name := range candidateNames {
		if dirExists(filepath.Join(projectDir, name)) {
			found = append(found, name)
		}
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		sort.Strings(found)
		return found
	}
	for _, e := range entries {
		if !e.IsDir() || noDescend[e.Name()] {
			continue
		}
		for _, name := range candidateNames {
			if dirExists(filepath.Join(projectDir, e.Name(), name)) {
				found = append(found, filepath.ToSlash(filepath.Join(e.Name(), name)))
			}
		}
	}

	sort.Strings(found)
	return found
}

// DefaultFileContent renders the seed content. Detected directories are written
// as active lines; the remaining candidate names are written as commented
// examples so the user sees what else is available.
func DefaultFileContent(detected []string) []byte {
	var b strings.Builder
	b.WriteString(fileHeader)

	active := make(map[string]bool, len(detected))
	for _, d := range detected {
		active[d] = true
		fmt.Fprintf(&b, "%s\n", d)
	}

	for _, name := range candidateNames {
		if !active[name] {
			fmt.Fprintf(&b, "# %s\n", name)
		}
	}

	return []byte(b.String())
}

// EnsureFile creates the file with default content if it does not exist. An
// existing file is left untouched - it is owned by the user.
func EnsureFile(path string, detected []string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat masked-dirs file: %w", err)
	}

	if err := os.WriteFile(path, DefaultFileContent(detected), 0o644); err != nil {
		return fmt.Errorf("write masked-dirs file: %w", err)
	}

	return nil
}

// ParseFile returns the non-blank, non-comment lines of the file, trimmed. A
// missing file returns (nil, nil).
func ParseFile(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- path is a fixed name under the project's .agentbox
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open masked-dirs file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read masked-dirs file: %w", err)
	}
	return lines, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
