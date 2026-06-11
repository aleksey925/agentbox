package skeleton

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aleksey925/agentbox/internal/config"
)

func createTestPaths(t *testing.T) *config.Paths {
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")

	return &config.Paths{
		HomeDir:     tmpDir,
		AgentboxDir: agentboxDir,
		BinDir:      filepath.Join(agentboxDir, "bin"),
		SkeletonDir: filepath.Join(agentboxDir, "skeleton"),
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

	// check core.v1.yml exists (flat structure)
	coreFile := filepath.Join(paths.SkeletonDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not created")
	}

	// check go.v1.yml exists
	goFile := filepath.Join(paths.SkeletonDir, "go.v1.yml")
	if _, err := os.Stat(goFile); os.IsNotExist(err) {
		t.Error("go.v1.yml not created")
	}

	// check python.v1.yml exists
	pythonFile := filepath.Join(paths.SkeletonDir, "python.v1.yml")
	if _, err := os.Stat(pythonFile); os.IsNotExist(err) {
		t.Error("python.v1.yml not created")
	}

	// check Dockerfile.v1.agentbox exists
	dockerFile := filepath.Join(paths.SkeletonDir, "Dockerfile.v1.agentbox")
	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Error("Dockerfile.v1.agentbox not created")
	}

	// check local.yml exists
	localFile := filepath.Join(paths.SkeletonDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
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
	coreFile := filepath.Join(paths.SkeletonDir, "core.v1.yml")
	if _, err := os.Stat(coreFile); os.IsNotExist(err) {
		t.Error("core.v1.yml not created")
	}

	// check local.yml exists
	localFile := filepath.Join(paths.SkeletonDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
	}

	// check Dockerfile exists
	dockerFile := filepath.Join(paths.SkeletonDir, "Dockerfile.v1.agentbox")
	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Error("Dockerfile.v1.agentbox not created")
	}

	// check only core + local.yml + Dockerfile (no preset files)
	entries, _ := os.ReadDir(paths.SkeletonDir)
	if len(entries) != 3 {
		t.Errorf("expected 3 files (core + local.yml + Dockerfile), got %d", len(entries))
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

	slices.Sort(presets)
	expected := []string{"go", "python"}
	if !slices.Equal(presets, expected) {
		t.Errorf("presets = %v, want %v", presets, expected)
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

	// check Dockerfile.v1.agentbox copied (with version)
	dockerFile := filepath.Join(agentboxDir, "Dockerfile.v1.agentbox")
	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Error("Dockerfile.v1.agentbox not copied")
	}

	// check local.yml created
	localFile := filepath.Join(agentboxDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
	}

	// check .gitignore created with `*` so .agentbox/ stays out of git
	gitignore, readErr := os.ReadFile(filepath.Join(agentboxDir, ".gitignore"))
	if readErr != nil {
		t.Fatalf("read .gitignore: %v", readErr)
	}
	if string(gitignore) != "*\n" {
		t.Errorf(".gitignore content = %q, want %q", gitignore, "*\n")
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

func TestCopyToProject__preserves_gitignore(t *testing.T) {
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

	// create existing .gitignore with custom content
	gitignoreFile := filepath.Join(agentboxDir, ".gitignore")
	customContent := []byte("*\n!keep.txt\n")
	if err := os.WriteFile(gitignoreFile, customContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	content, err := os.ReadFile(gitignoreFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, customContent) {
		t.Errorf(".gitignore was overwritten, got: %s", content)
	}
}

func TestIsSystemFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{"DS_Store is system file", ".DS_Store", true},
		{"AppleDouble file", "._something", true},
		{"normal yml file", "core.v1.yml", false},
		{"dotfile preset allowed", ".custom-preset.yml", false},
		{"local.yml allowed", "local.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			result := isSystemFile(tt.filename)

			// assert
			if result != tt.expected {
				t.Errorf("isSystemFile(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestCopyToProject__skips_system_files(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{}); err != nil {
		t.Fatal(err)
	}

	// create .DS_Store in skeleton/
	dsStorePath := filepath.Join(paths.SkeletonDir, ".DS_Store")
	if err := os.WriteFile(dsStorePath, []byte("fake ds_store"), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	copiedFiles, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	// .DS_Store should not be in copied files
	if slices.Contains(copiedFiles, ".DS_Store") {
		t.Error(".DS_Store should not be in copied files list")
	}

	// .DS_Store should not exist in project
	projectDSStore := filepath.Join(projectDir, ".agentbox", ".DS_Store")
	if _, err := os.Stat(projectDSStore); !os.IsNotExist(err) {
		t.Error(".DS_Store should not be copied to project")
	}
}

func TestCopyToProject__skips_symlinks(t *testing.T) {
	// arrange - a symlink planted in the skeleton must not have its target copied
	// into the project (e.g. core.v1.yml -> ~/.ssh/id_rsa).
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton(nil); err != nil {
		t.Fatal(err)
	}

	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(paths.SkeletonDir, "leak.yml")); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	copiedFiles, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}
	if slices.Contains(copiedFiles, "leak.yml") {
		t.Error("symlink must not be copied")
	}
	if _, statErr := os.Lstat(filepath.Join(projectDir, ".agentbox", "leak.yml")); !os.IsNotExist(statErr) {
		t.Errorf("symlinked entry must not appear in project (lstat err = %v)", statErr)
	}
}

func TestCopyToProject__rejects_symlinked_agentbox_dir(t *testing.T) {
	// arrange - a cloned repo can commit `.agentbox` as a symlink (e.g. -> ..);
	// following it would let cleanProjectDir delete the target's contents and
	// land the copy outside the project.
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton(nil); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	sentinel := filepath.Join(target, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	if err := os.Symlink(target, filepath.Join(projectDir, ".agentbox")); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := manager.CopyToProject(projectDir)

	// assert
	if err == nil {
		t.Fatal("CopyToProject must refuse a symlinked .agentbox")
	}
	if _, statErr := os.Stat(sentinel); statErr != nil {
		t.Errorf("symlink target content must stay untouched: %v", statErr)
	}
}

func TestHasRealFiles(t *testing.T) {
	// arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	result := HasRealFiles(dir)

	// assert
	if !result {
		t.Error("expected true for directory with real files")
	}
}

func TestHasRealFiles__empty_dir(t *testing.T) {
	// arrange
	dir := t.TempDir()

	// act
	result := HasRealFiles(dir)

	// assert
	if result {
		t.Error("expected false for empty directory")
	}
}

func TestHasRealFiles__only_system_files(t *testing.T) {
	// arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "._hidden"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	result := HasRealFiles(dir)

	// assert
	if result {
		t.Error("expected false for directory with only system files")
	}
}

func TestHasRealFiles__nonexistent_dir(t *testing.T) {
	// act
	result := HasRealFiles("/nonexistent/path")

	// assert
	if result {
		t.Error("expected false for nonexistent directory")
	}
}

func TestCleanProjectDir(t *testing.T) {
	// arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "core.v1.yml"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.v1.yml"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	err := cleanProjectDir(dir)

	// assert
	if err != nil {
		t.Fatalf("cleanProjectDir error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected empty directory, got %d files", len(entries))
	}
}

func TestCleanProjectDir__preserves_local_yml(t *testing.T) {
	// arrange
	dir := t.TempDir()
	localContent := []byte("# my config")
	if err := os.WriteFile(filepath.Join(dir, "local.yml"), localContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "core.v1.yml"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	err := cleanProjectDir(dir)

	// assert
	if err != nil {
		t.Fatalf("cleanProjectDir error: %v", err)
	}

	// local.yml should be preserved
	content, err := os.ReadFile(filepath.Join(dir, "local.yml"))
	if err != nil {
		t.Fatal("local.yml should be preserved")
	}
	if !bytes.Equal(content, localContent) {
		t.Error("local.yml content was modified")
	}

	// other files should be removed
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file (local.yml), got %d", len(entries))
	}
}

func TestCleanProjectDir__nonexistent_dir(t *testing.T) {
	// act
	err := cleanProjectDir("/nonexistent/path")

	// assert
	if err != nil {
		t.Errorf("expected no error for nonexistent directory, got: %v", err)
	}
}
