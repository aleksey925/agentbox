package skeleton

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/aleksey925/agentbox/internal/config"
)

// isSystemFile returns true for OS-generated files that should be ignored during copying.
func isSystemFile(name string) bool {
	// macOS Finder metadata
	if name == ".DS_Store" {
		return true
	}
	// macOS AppleDouble resource fork files (._filename)
	if strings.HasPrefix(name, "._") {
		return true
	}
	return false
}

// Manager handles skeleton creation and copying operations.
type Manager struct {
	paths *config.Paths
}

// NewManager creates a new skeleton manager.
func NewManager(paths *config.Paths) *Manager {
	return &Manager{paths: paths}
}

// CreateSkeleton creates a new skeleton with the specified presets.
// Existing skeleton/ is deleted completely and recreated.
// All files are written directly to skeleton/ (flat structure).
func (m *Manager) CreateSkeleton(presets []string) error {
	// delete existing skeleton/ (fully managed by agentbox)
	if err := os.RemoveAll(m.paths.SkeletonDir); err != nil {
		return fmt.Errorf("remove old skeleton: %w", err)
	}

	// create skeleton directory (flat structure, no subdirectories)
	if err := os.MkdirAll(m.paths.SkeletonDir, 0o755); err != nil {
		return fmt.Errorf("create skeleton dir: %w", err)
	}

	// copy core template
	coreTemplate, err := GetCoreTemplate()
	if err != nil {
		return fmt.Errorf("get core template: %w", err)
	}
	corePath := filepath.Join(m.paths.SkeletonDir, coreTemplate.Filename)
	if writeErr := os.WriteFile(corePath, coreTemplate.Content, 0o644); writeErr != nil {
		return fmt.Errorf("write core template: %w", writeErr)
	}

	// copy preset templates
	for _, preset := range presets {
		presetTemplate, presetErr := GetPresetTemplate(preset)
		if presetErr != nil {
			return fmt.Errorf("get preset template %s: %w", preset, presetErr)
		}
		presetPath := filepath.Join(m.paths.SkeletonDir, presetTemplate.Filename)
		if writeErr := os.WriteFile(presetPath, presetTemplate.Content, 0o644); writeErr != nil {
			return fmt.Errorf("write preset template %s: %w", preset, writeErr)
		}
	}

	// copy Dockerfile template
	dockerfile, err := GetEmbeddedDockerfile()
	if err != nil {
		return fmt.Errorf("get Dockerfile template: %w", err)
	}
	dockerfilePath := filepath.Join(m.paths.SkeletonDir, dockerfile.Filename)
	if writeErr := os.WriteFile(dockerfilePath, dockerfile.Content, 0o644); writeErr != nil {
		return fmt.Errorf("write Dockerfile template: %w", writeErr)
	}

	// copy local.yml template
	localContent, err := GetEmbeddedLocalYml()
	if err != nil {
		return fmt.Errorf("get local.yml template: %w", err)
	}
	localPath := filepath.Join(m.paths.SkeletonDir, "local.yml")
	if writeErr := os.WriteFile(localPath, localContent, 0o644); writeErr != nil {
		return fmt.Errorf("write local.yml template: %w", writeErr)
	}

	return nil
}

// GetEnabledPresets returns presets currently enabled in skeleton.
// It parses existing files in skeleton/ directory (flat structure).
func (m *Manager) GetEnabledPresets() ([]string, error) {
	entries, err := os.ReadDir(m.paths.SkeletonDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skeleton dir: %w", err)
	}

	var presets []string
	supportedPresets := make(map[string]bool)
	for _, preset := range SupportedPresets() {
		supportedPresets[preset.TemplateName] = true
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, _ := ParseTemplateName(e.Name())
		// skip core and non-preset files
		if name != "core" && supportedPresets[name] {
			presets = append(presets, name)
		}
	}

	return presets, nil
}

// cleanProjectDir removes all files from project's .agentbox/ except those
// preserved across reinit (local.yml, .gitignore).
func cleanProjectDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read project dir: %w", err)
	}

	for _, e := range entries {
		if e.Name() == "local.yml" || e.Name() == ".gitignore" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", e.Name(), err)
		}
	}
	return nil
}

// ensureRealDir rejects a path that exists but is a symlink or a non-directory.
// A cloned repo can commit `.agentbox -> ..` (or any other target), and
// cleanProjectDir would then delete the target's contents while the copy lands
// outside the project.
func ensureRealDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%s exists but is not a real directory, refusing to use it", path)
	}
	return nil
}

// CopyToProject copies skeleton files to project's .agentbox/ directory.
// All files except local.yml are removed first, then skeleton is copied.
// local.yml is only created if it doesn't exist in project.
func (m *Manager) CopyToProject(projectDir string) ([]string, error) {
	agentboxDir := filepath.Join(projectDir, ".agentbox")

	if err := ensureRealDir(agentboxDir); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .agentbox dir: %w", err)
	}

	// clean existing files except local.yml
	if err := cleanProjectDir(agentboxDir); err != nil {
		return nil, err
	}

	// check if local.yml exists before copying
	localYmlExists := false
	if _, err := os.Stat(filepath.Join(agentboxDir, "local.yml")); err == nil {
		localYmlExists = true
	}

	// copy all files from skeleton/ (flat structure)
	entries, err := os.ReadDir(m.paths.SkeletonDir)
	if err != nil {
		return nil, fmt.Errorf("read skeleton dir: %w", err)
	}

	copiedFiles := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || isSystemFile(e.Name()) {
			continue
		}

		// skip symlinks: a skeleton entry pointing elsewhere (e.g. core.v1.yml ->
		// ~/.ssh/id_rsa) would otherwise copy the target's contents into the project.
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}

		// skip local.yml if it already exists in project
		if e.Name() == "local.yml" && localYmlExists {
			continue
		}

		srcPath := filepath.Join(m.paths.SkeletonDir, e.Name())
		dstPath := filepath.Join(agentboxDir, e.Name())

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		// O_NOFOLLOW: refuse to write through a symlink planted at the destination,
		// so a copy can never land outside .agentbox/.
		out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", e.Name(), err)
		}
		if _, err := out.Write(content); err != nil {
			out.Close()
			return nil, fmt.Errorf("write %s: %w", e.Name(), err)
		}
		if err := out.Close(); err != nil {
			return nil, fmt.Errorf("close %s: %w", e.Name(), err)
		}
		copiedFiles = append(copiedFiles, e.Name())
	}

	// .agentbox/ is local-only state — keep it out of git regardless of the
	// repo's root .gitignore. preserve any user customizations.
	gitignorePath := filepath.Join(agentboxDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte("*\n"), 0o644); err != nil {
			return nil, fmt.Errorf("write .gitignore: %w", err)
		}
	}

	return copiedFiles, nil
}

// HasRealFiles checks if directory contains any non-system files.
func HasRealFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !isSystemFile(e.Name()) {
			return true
		}
	}
	return false
}
