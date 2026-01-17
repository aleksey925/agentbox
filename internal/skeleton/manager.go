package skeleton

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aleksey925/agentbox/internal/config"
)

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
// skeleton.local/ is never touched (user data).
func (m *Manager) CreateSkeleton(presets []string) error {
	// delete existing skeleton/ (fully managed by agentbox)
	if err := os.RemoveAll(m.paths.SkeletonDir); err != nil {
		return fmt.Errorf("remove old skeleton: %w", err)
	}

	// create skeleton directories
	if err := os.MkdirAll(m.paths.SkeletonComposeDir, 0o755); err != nil {
		return fmt.Errorf("create skeleton compose dir: %w", err)
	}

	// copy core template
	coreTemplate, err := GetCoreTemplate()
	if err != nil {
		return fmt.Errorf("get core template: %w", err)
	}
	corePath := filepath.Join(m.paths.SkeletonComposeDir, coreTemplate.Filename)
	if writeErr := os.WriteFile(corePath, coreTemplate.Content, 0o644); writeErr != nil {
		return fmt.Errorf("write core template: %w", writeErr)
	}

	// copy preset templates
	for _, preset := range presets {
		presetTemplate, presetErr := GetPresetTemplate(preset)
		if presetErr != nil {
			return fmt.Errorf("get preset template %s: %w", preset, presetErr)
		}
		presetPath := filepath.Join(m.paths.SkeletonComposeDir, presetTemplate.Filename)
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

	// copy local.yml template to skeleton/compose/
	localContent, err := GetEmbeddedLocalYml()
	if err != nil {
		return fmt.Errorf("get local.yml template: %w", err)
	}
	localPath := filepath.Join(m.paths.SkeletonComposeDir, "local.yml")
	if writeErr := os.WriteFile(localPath, localContent, 0o644); writeErr != nil {
		return fmt.Errorf("write local.yml template: %w", writeErr)
	}

	// create skeleton.local/compose/ if not exists (user data, never deleted)
	if err := os.MkdirAll(m.paths.SkeletonLocalComposeDir, 0o755); err != nil {
		return fmt.Errorf("create skeleton.local compose dir: %w", err)
	}

	return nil
}

// GetEnabledPresets returns presets currently enabled in skeleton.
// It parses existing files in skeleton/compose/ directory.
func (m *Manager) GetEnabledPresets() ([]string, error) {
	entries, err := os.ReadDir(m.paths.SkeletonComposeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skeleton compose dir: %w", err)
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

// CopyToProject copies skeleton files to project's .agentbox/ directory.
// Files from skeleton.local/ have priority over skeleton/.
// local.yml is only created if it doesn't exist in project.
func (m *Manager) CopyToProject(projectDir string) ([]string, error) {
	agentboxDir := filepath.Join(projectDir, ".agentbox")

	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .agentbox dir: %w", err)
	}

	copiedFiles := make([]string, 0, 10)

	// 1. Copy compose files from skeleton/ (except local.yml)
	skeletonFiles, err := m.copyComposeFiles(m.paths.SkeletonComposeDir, agentboxDir, "local.yml")
	if err != nil {
		return nil, err
	}
	copiedFiles = append(copiedFiles, skeletonFiles...)

	// 2. Copy compose files from skeleton.local/ (overwrites skeleton/ on conflict, except local.yml)
	localFiles, err := m.copyComposeFiles(m.paths.SkeletonLocalComposeDir, agentboxDir, "local.yml")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, f := range localFiles {
		if !slices.Contains(copiedFiles, f) {
			copiedFiles = append(copiedFiles, f)
		}
	}

	// 3. Copy Dockerfile from skeleton/
	if dockerName, err := m.copyDockerfile(agentboxDir); err != nil {
		return nil, err
	} else if dockerName != "" {
		copiedFiles = append(copiedFiles, dockerName)
	}

	// 4. Copy local.yml only if it doesn't exist in project
	if localCreated, err := m.copyLocalYml(agentboxDir); err != nil {
		return nil, err
	} else if localCreated {
		copiedFiles = append(copiedFiles, "local.yml")
	}

	return copiedFiles, nil
}

// copyComposeFiles copies compose files from srcDir to dstDir, skipping excludeFile.
func (m *Manager) copyComposeFiles(srcDir, dstDir, excludeFile string) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcDir, err)
	}

	copied := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || e.Name() == excludeFile {
			continue
		}
		srcPath := filepath.Join(srcDir, e.Name())
		dstPath := filepath.Join(dstDir, e.Name())

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", e.Name(), err)
		}
		copied = append(copied, e.Name())
	}
	return copied, nil
}

// copyDockerfile copies the Dockerfile from skeleton/ to dstDir.
func (m *Manager) copyDockerfile(dstDir string) (string, error) {
	entries, err := os.ReadDir(m.paths.SkeletonDir)
	if err != nil {
		return "", fmt.Errorf("read skeleton dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".agentbox") {
			continue
		}
		srcPath := filepath.Join(m.paths.SkeletonDir, e.Name())
		dstPath := filepath.Join(dstDir, "Dockerfile.agentbox")

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return "", fmt.Errorf("read Dockerfile: %w", err)
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return "", fmt.Errorf("write Dockerfile: %w", err)
		}
		return "Dockerfile.agentbox", nil
	}
	return "", nil
}

// copyLocalYml copies local.yml from skeleton to dstDir if it doesn't exist.
// Priority: skeleton.local/compose/local.yml > skeleton/compose/local.yml
func (m *Manager) copyLocalYml(dstDir string) (bool, error) {
	localPath := filepath.Join(dstDir, "local.yml")
	if _, err := os.Stat(localPath); err == nil {
		return false, nil // file exists
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("check local.yml: %w", err)
	}

	// try skeleton.local/ first
	content, err := os.ReadFile(filepath.Join(m.paths.SkeletonLocalComposeDir, "local.yml"))
	if err != nil {
		// fall back to skeleton/
		content, err = os.ReadFile(filepath.Join(m.paths.SkeletonComposeDir, "local.yml"))
		if err != nil {
			return false, fmt.Errorf("read local.yml from skeleton: %w", err)
		}
	}

	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		return false, fmt.Errorf("write local.yml: %w", err)
	}
	return true, nil
}

// UpdateInfo contains information about available template updates.
type UpdateInfo struct {
	Name           string
	CurrentVersion int
	LatestVersion  int
}

// CheckUpdates compares skeleton versions with embedded template versions.
func (m *Manager) CheckUpdates() ([]UpdateInfo, error) {
	// get embedded templates
	embeddedTemplates, err := GetEmbeddedComposeTemplates()
	if err != nil {
		return nil, fmt.Errorf("get embedded templates: %w", err)
	}

	// build map of latest versions
	latestVersions := make(map[string]int)
	for _, t := range embeddedTemplates {
		latestVersions[t.Name] = t.Version
	}

	// also check Dockerfile
	dockerfile, err := GetEmbeddedDockerfile()
	if err == nil {
		latestVersions["Dockerfile"] = dockerfile.Version
	}

	// read skeleton files
	var updates []UpdateInfo

	// check compose files
	composeEntries, err := os.ReadDir(m.paths.SkeletonComposeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skeleton compose dir: %w", err)
	}

	for _, e := range composeEntries {
		if e.IsDir() {
			continue
		}
		name, currentVersion := ParseTemplateName(e.Name())
		// skip files without corresponding embedded template
		latestVersion, ok := latestVersions[name]
		if !ok || currentVersion == -1 {
			continue
		}
		if currentVersion < latestVersion {
			updates = append(updates, UpdateInfo{
				Name:           name,
				CurrentVersion: currentVersion,
				LatestVersion:  latestVersion,
			})
		}
	}

	// check Dockerfile
	dockerfiles, err := os.ReadDir(m.paths.SkeletonDir)
	if err == nil {
		for _, e := range dockerfiles {
			if !strings.HasSuffix(e.Name(), ".agentbox") {
				continue
			}
			name, currentVersion := ParseTemplateName(e.Name())
			latestVersion, ok := latestVersions["Dockerfile"]
			if !ok {
				continue
			}
			if currentVersion < latestVersion {
				updates = append(updates, UpdateInfo{
					Name:           name,
					CurrentVersion: currentVersion,
					LatestVersion:  latestVersion,
				})
			}
		}
	}

	return updates, nil
}

// ProjectAgentboxDir is the name of the agentbox directory in projects.
const ProjectAgentboxDir = ".agentbox"

// GitExcludeEntries returns entries to add to .git/info/exclude.
func GitExcludeEntries() []string {
	return []string{ProjectAgentboxDir + "/"}
}
