package skeleton

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksey925/agentbox/internal/config"
)

// Manager handles skeleton creation, backup, and copying operations.
type Manager struct {
	paths *config.Paths
}

// NewManager creates a new skeleton manager.
func NewManager(paths *config.Paths) *Manager {
	return &Manager{paths: paths}
}

// CreateSkeleton creates a new skeleton with the specified presets.
// If skeleton already exists, it must be backed up first using BackupSkeleton.
func (m *Manager) CreateSkeleton(presets []string) error {
	// ensure skeleton directories exist
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

	return nil
}

// BackupSkeleton moves existing skeleton to skeleton.backup directory.
// If backup already exists, it is removed first (only one backup is kept).
func (m *Manager) BackupSkeleton() error {
	// check if skeleton exists
	if _, err := os.Stat(m.paths.SkeletonDir); os.IsNotExist(err) {
		return nil // nothing to backup
	}

	// remove existing backup if any
	if err := os.RemoveAll(m.paths.SkeletonBackupDir); err != nil {
		return fmt.Errorf("remove old backup: %w", err)
	}

	// move skeleton to backup
	if err := os.Rename(m.paths.SkeletonDir, m.paths.SkeletonBackupDir); err != nil {
		return fmt.Errorf("backup skeleton: %w", err)
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
// local.yml is only created if it doesn't exist.
func (m *Manager) CopyToProject(projectDir string) ([]string, error) {
	agentboxDir := filepath.Join(projectDir, ".agentbox")

	// ensure .agentbox directory exists
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .agentbox dir: %w", err)
	}

	// copy compose files from skeleton
	composeEntries, err := os.ReadDir(m.paths.SkeletonComposeDir)
	if err != nil {
		return nil, fmt.Errorf("read skeleton compose dir: %w", err)
	}

	copiedFiles := make([]string, 0, len(composeEntries)+2) // compose files + Dockerfile + local.yml

	for _, e := range composeEntries {
		if e.IsDir() {
			continue
		}
		srcPath := filepath.Join(m.paths.SkeletonComposeDir, e.Name())
		dstPath := filepath.Join(agentboxDir, e.Name())

		fileContent, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), readErr)
		}
		if writeErr := os.WriteFile(dstPath, fileContent, 0o644); writeErr != nil {
			return nil, fmt.Errorf("write %s: %w", e.Name(), writeErr)
		}
		copiedFiles = append(copiedFiles, e.Name())
	}

	// copy Dockerfile from skeleton
	dockerfiles, err := os.ReadDir(m.paths.SkeletonDir)
	if err != nil {
		return nil, fmt.Errorf("read skeleton dir: %w", err)
	}

	for _, e := range dockerfiles {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".agentbox") {
			continue
		}
		srcPath := filepath.Join(m.paths.SkeletonDir, e.Name())
		// in project, save without version: Dockerfile.agentbox
		dstPath := filepath.Join(agentboxDir, "Dockerfile.agentbox")

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("read Dockerfile: %w", err)
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return nil, fmt.Errorf("write Dockerfile: %w", err)
		}
		copiedFiles = append(copiedFiles, "Dockerfile.agentbox")
	}

	// create local.yml only if it doesn't exist
	localPath := filepath.Join(agentboxDir, "local.yml")
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		localContent, err := GetEmbeddedLocalYml()
		if err != nil {
			return nil, fmt.Errorf("get local.yml template: %w", err)
		}
		if err := os.WriteFile(localPath, localContent, 0o644); err != nil {
			return nil, fmt.Errorf("write local.yml: %w", err)
		}
		copiedFiles = append(copiedFiles, "local.yml")
	}

	return copiedFiles, nil
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
