package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aleksey925/agentbox/internal/maskdirs"
)

// maskVolumePrefix is the fixed prefix of every mask volume name. The
// per-project hash follows it, which is what orphan cleanup filters on.
const maskVolumePrefix = "agentbox-mask-"

// maskEntry is one masked sub-directory: the relative path from the file, the
// absolute in-container target (mirror scheme, same path as the host), and the
// named Docker volume that overrides the project bind-mount at that target.
type maskEntry struct {
	sub    string
	target string
	volume string
}

// shortHash returns the first 12 hex chars of sha256(s) - enough to make a
// volume name collision-safe while keeping it short.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// projectMaskPrefix is the name prefix shared by every mask volume of one
// project; cleanup filters volumes on it.
func projectMaskPrefix(projectDir string) string {
	return maskVolumePrefix + shortHash(projectDir) + "-"
}

func maskVolumeName(projectDir, cleanSub string) string {
	return projectMaskPrefix(projectDir) + shortHash(cleanSub)
}

// validateMaskSub accepts only a path that stays inside the project: relative,
// no "..", cleaned, and not the .agentbox or .git surface (which have their own
// mounts/protections - masking them would clash or break git protection).
func validateMaskSub(sub string) (clean string, err error) {
	if filepath.IsAbs(sub) {
		return "", fmt.Errorf("masked path %q must be relative", sub)
	}
	clean = filepath.Clean(filepath.FromSlash(sub))
	if clean == "." || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("masked path %q must stay inside the project", sub)
	}
	first := clean
	if i := strings.IndexRune(clean, filepath.Separator); i >= 0 {
		first = clean[:i]
	}
	if first == ".agentbox" || first == ".git" {
		return "", fmt.Errorf("masked path %q clashes with the %s mount", sub, first)
	}
	return clean, nil
}

// maskedEntries reads and validates the project's masked-dirs file. An absent
// file yields (nil, nil). An invalid entry fails closed rather than being
// skipped, the same spirit as the git-protection overlay.
func maskedEntries(projectDir string) ([]maskEntry, error) {
	subs, err := maskdirs.ParseFile(filepath.Join(projectDir, ".agentbox", "masked-dirs"))
	if err != nil {
		return nil, fmt.Errorf("read masked-dirs: %w", err)
	}
	if len(subs) == 0 {
		return nil, nil
	}

	entries := make([]maskEntry, 0, len(subs))
	for _, sub := range subs {
		clean, err := validateMaskSub(sub)
		if err != nil {
			return nil, err
		}
		entries = append(entries, maskEntry{
			sub:    clean,
			target: filepath.Join(projectDir, clean),
			volume: maskVolumeName(projectDir, clean),
		})
	}
	return entries, nil
}

// renderMaskCompose builds a compose fragment that mounts each entry's named
// volume over its target, sets AGENTBOX_MASK_PATHS (newline-joined targets, so
// paths with spaces or colons survive), and declares the volumes so compose
// creates them on first run and reuses them after.
func renderMaskCompose(entries []maskEntry) string {
	targets := make([]string, len(entries))
	for i, e := range entries {
		targets[i] = e.target
	}

	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  agentbox:\n")
	b.WriteString("    volumes:\n")
	for _, e := range entries {
		b.WriteString("      - type: volume\n")
		fmt.Fprintf(&b, "        source: %q\n", e.volume)
		fmt.Fprintf(&b, "        target: %q\n", e.target)
	}
	// list form (not the "KEY: value" map form): it matches the core compose
	// file, so compose never has to reconcile a list and a map for the same
	// environment key when merging the fragments.
	b.WriteString("    environment:\n")
	fmt.Fprintf(&b, "      - %q\n", "AGENTBOX_MASK_PATHS="+strings.Join(targets, "\n"))

	b.WriteString("volumes:\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "  %q:\n", e.volume)
		fmt.Fprintf(&b, "    name: %q\n", e.volume)
	}
	return b.String()
}

// writeMaskFragment writes the masking overlay as a temporary compose file to
// append to the run. It returns ("", noop) when nothing is masked, so callers
// can append unconditionally.
func writeMaskFragment(entries []maskEntry) (path string, cleanup func(), err error) {
	noop := func() {}

	if len(entries) == 0 {
		return "", noop, nil
	}

	name, err := writeComposeFragment("agentbox-mask", renderMaskCompose(entries))
	if err != nil {
		return "", noop, err
	}
	return name, func() { os.Remove(name) }, nil
}

// orphanMaskVolumes returns the names present but not in keep - the volumes
// whose masked-dirs line was removed since they were created.
func orphanMaskVolumes(present, keep []string) []string {
	var orphans []string
	for _, name := range present {
		if !slices.Contains(keep, name) {
			orphans = append(orphans, name)
		}
	}
	return orphans
}

// pruneOrphanMaskVolumes removes this project's mask volumes that are no longer
// desired. A missing or in-use volume is not fatal - cleanup must never block a
// run, so errors are surfaced by the caller but do not abort.
func pruneOrphanMaskVolumes(projectDir string, keep []string) error {
	ctx := context.Background()
	prefix := projectMaskPrefix(projectDir)
	lsCmd := exec.CommandContext(ctx, "docker", "volume", "ls",
		"--filter", "name="+prefix, "--format", "{{.Name}}") // #nosec G204 -- prefix is a sha-derived, fixed-charset string
	var out, stderr bytes.Buffer
	lsCmd.Stdout = &out
	lsCmd.Stderr = &stderr
	if err := lsCmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("list mask volumes: %s", msg)
		}
		return fmt.Errorf("list mask volumes: %w", err)
	}

	var present []string
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			present = append(present, line)
		}
	}

	// keep going past a failed removal: an orphan held by a still-running
	// container must not stop the others from being pruned this run.
	var errs []error
	for _, name := range orphanMaskVolumes(present, keep) {
		rmCmd := exec.CommandContext(ctx, "docker", "volume", "rm", name) // #nosec G204 -- name comes from docker volume ls filtered by our prefix
		if err := rmCmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("remove orphan mask volume %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
