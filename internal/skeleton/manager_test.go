package skeleton

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aleksey925/agentbox/internal/config"
)

func createTestPaths(t *testing.T) *config.Paths {
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	skeletonDir := filepath.Join(agentboxDir, "skeleton")

	return &config.Paths{
		HomeDir:            tmpDir,
		AgentboxDir:        agentboxDir,
		BinDir:             filepath.Join(agentboxDir, "bin"),
		SkeletonDir:        skeletonDir,
		SkeletonBackupDir:  filepath.Join(agentboxDir, "skeleton.backup"),
		SkeletonComposeDir: filepath.Join(skeletonDir, "compose"),
	}
}

func TestCreateSkeleton(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// act
	err := manager.CreateSkeleton([]string{"go", "python"})

	// assert
	if err != nil {
		t.Fatalf("CreateSkeleton error: %v", err)
	}

	// check core.v1.yml exists
	coreFile := filepath.Join(paths.SkeletonComposeDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not created")
	}

	// check go.v1.yml exists
	goFile := filepath.Join(paths.SkeletonComposeDir, "go.v1.yml")
	if _, err := os.Stat(goFile); os.IsNotExist(err) {
		t.Error("go.v1.yml not created")
	}

	// check python.v1.yml exists
	pythonFile := filepath.Join(paths.SkeletonComposeDir, "python.v1.yml")
	if _, err := os.Stat(pythonFile); os.IsNotExist(err) {
		t.Error("python.v1.yml not created")
	}

	// check Dockerfile.v1.agentbox exists
	dockerFile := filepath.Join(paths.SkeletonDir, "Dockerfile.v1.agentbox")
	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Error("Dockerfile.v1.agentbox not created")
	}
}

func TestCreateSkeleton__no_presets(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// act
	err := manager.CreateSkeleton(nil)

	// assert
	if err != nil {
		t.Fatalf("CreateSkeleton error: %v", err)
	}

	// check core.v1.yml exists (always created)
	coreFile := filepath.Join(paths.SkeletonComposeDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not created")
	}

	// check no preset files
	entries, _ := os.ReadDir(paths.SkeletonComposeDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file (core only), got %d", len(entries))
	}
}

func TestBackupSkeleton(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// create skeleton first
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	// act
	err := manager.BackupSkeleton()

	// assert
	if err != nil {
		t.Fatalf("BackupSkeleton error: %v", err)
	}

	// skeleton dir should not exist
	if _, err := os.Stat(paths.SkeletonDir); !os.IsNotExist(err) {
		t.Error("skeleton dir should be moved")
	}

	// backup dir should exist
	if _, err := os.Stat(paths.SkeletonBackupDir); os.IsNotExist(err) {
		t.Error("backup dir not created")
	}

	// backup should contain compose files
	backupComposeDir := filepath.Join(paths.SkeletonBackupDir, "compose")
	coreFile := filepath.Join(backupComposeDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not in backup")
	}
}

func TestBackupSkeleton__no_skeleton(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// act (no skeleton exists)
	err := manager.BackupSkeleton()

	// assert
	if err != nil {
		t.Fatalf("BackupSkeleton should not error when no skeleton: %v", err)
	}
}

func TestBackupSkeleton__replaces_old_backup(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// create first skeleton and backup
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.BackupSkeleton(); err != nil {
		t.Fatal(err)
	}

	// create marker file in backup to verify it gets replaced
	markerFile := filepath.Join(paths.SkeletonBackupDir, "marker.txt")
	if err := os.WriteFile(markerFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// create new skeleton
	if err := manager.CreateSkeleton([]string{"python"}); err != nil {
		t.Fatal(err)
	}

	// act
	err := manager.BackupSkeleton()

	// assert
	if err != nil {
		t.Fatalf("BackupSkeleton error: %v", err)
	}

	// marker file should be gone (old backup replaced)
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Error("old backup should be replaced")
	}
}

func TestGetEnabledPresets(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go", "python"}); err != nil {
		t.Fatal(err)
	}

	// act
	presets, err := manager.GetEnabledPresets()

	// assert
	if err != nil {
		t.Fatalf("GetEnabledPresets error: %v", err)
	}

	if len(presets) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(presets))
	}

	// check both presets present (order may vary)
	presetSet := make(map[string]bool)
	for _, p := range presets {
		presetSet[p] = true
	}
	if !presetSet["go"] {
		t.Error("go not in enabled presets")
	}
	if !presetSet["python"] {
		t.Error("python not in enabled presets")
	}
}

func TestGetEnabledPresets__no_skeleton(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// act
	presets, err := manager.GetEnabledPresets()

	// assert
	if err != nil {
		t.Fatalf("GetEnabledPresets error: %v", err)
	}

	if presets != nil {
		t.Errorf("expected nil, got %v", presets)
	}
}

func TestCopyToProject(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	copiedFiles, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	if len(copiedFiles) == 0 {
		t.Error("no files copied")
	}

	// check .agentbox directory created
	agentboxDir := filepath.Join(projectDir, ".agentbox")
	if _, err := os.Stat(agentboxDir); os.IsNotExist(err) {
		t.Error(".agentbox dir not created")
	}

	// check core.v1.yml copied
	coreFile := filepath.Join(agentboxDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not copied")
	}

	// check Dockerfile.agentbox copied (without version)
	dockerFile := filepath.Join(agentboxDir, "Dockerfile.agentbox")
	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Error("Dockerfile.agentbox not copied")
	}

	// check local.yml created
	localFile := filepath.Join(agentboxDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
	}
}

func TestCopyToProject__preserves_local_yml(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	agentboxDir := filepath.Join(projectDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// create existing local.yml with custom content
	localFile := filepath.Join(agentboxDir, "local.yml")
	customContent := []byte("# my custom config")
	if err := os.WriteFile(localFile, customContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	// check local.yml NOT overwritten
	content, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, customContent) {
		t.Errorf("local.yml was overwritten, got: %s", content)
	}
}

func TestCheckUpdates__no_updates(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	// act
	updates, err := manager.CheckUpdates()

	// assert
	if err != nil {
		t.Fatalf("CheckUpdates error: %v", err)
	}

	if len(updates) != 0 {
		t.Errorf("expected no updates, got %d", len(updates))
	}
}

func TestCheckUpdates__no_skeleton(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// act
	updates, err := manager.CheckUpdates()

	// assert
	if err != nil {
		t.Fatalf("CheckUpdates error: %v", err)
	}

	if updates != nil {
		t.Errorf("expected nil, got %v", updates)
	}
}

func TestGitExcludeEntries(t *testing.T) {
	// act
	entries := GitExcludeEntries()

	// assert
	expected := []string{".agentbox/"}
	if len(entries) != len(expected) {
		t.Fatalf("len(entries) = %d, want %d", len(entries), len(expected))
	}

	for i, e := range entries {
		if e != expected[i] {
			t.Errorf("entries[%d] = %s, want %s", i, e, expected[i])
		}
	}
}
