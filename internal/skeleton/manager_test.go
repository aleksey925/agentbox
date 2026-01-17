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
	skeletonDir := filepath.Join(agentboxDir, "skeleton")
	skeletonLocalDir := filepath.Join(agentboxDir, "skeleton.local")

	return &config.Paths{
		HomeDir:                 tmpDir,
		AgentboxDir:             agentboxDir,
		BinDir:                  filepath.Join(agentboxDir, "bin"),
		SkeletonDir:             skeletonDir,
		SkeletonComposeDir:      filepath.Join(skeletonDir, "compose"),
		SkeletonLocalDir:        skeletonLocalDir,
		SkeletonLocalComposeDir: filepath.Join(skeletonLocalDir, "compose"),
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

	// check local.yml exists
	localFile := filepath.Join(paths.SkeletonComposeDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
	}

	// check skeleton.local/compose/ created
	if _, err := os.Stat(paths.SkeletonLocalComposeDir); os.IsNotExist(err) {
		t.Error("skeleton.local/compose/ not created")
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

	// check local.yml exists
	localFile := filepath.Join(paths.SkeletonComposeDir, "local.yml")
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("local.yml not created")
	}

	// check only core + local.yml (no preset files)
	entries, _ := os.ReadDir(paths.SkeletonComposeDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 files (core + local.yml), got %d", len(entries))
	}

	// check skeleton.local/compose/ created
	if _, err := os.Stat(paths.SkeletonLocalComposeDir); os.IsNotExist(err) {
		t.Error("skeleton.local/compose/ not created")
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
	if !slices.Equal(entries, expected) {
		t.Errorf("entries = %v, want %v", entries, expected)
	}
}

func TestCopyToProject__copies_skeleton_local(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	// create user preset in skeleton.local/compose/
	userPreset := []byte("# ssh-agent preset\nservices:\n  agentbox: {}")
	userPresetPath := filepath.Join(paths.SkeletonLocalComposeDir, "ssh-agent.yml")
	if err := os.WriteFile(userPresetPath, userPreset, 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	copiedFiles, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	// check ssh-agent.yml copied
	sshAgentFile := filepath.Join(projectDir, ".agentbox", "ssh-agent.yml")
	if _, statErr := os.Stat(sshAgentFile); os.IsNotExist(statErr) {
		t.Error("ssh-agent.yml not copied from skeleton.local/")
	}

	// check content matches
	content, readErr := os.ReadFile(sshAgentFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(content, userPreset) {
		t.Errorf("ssh-agent.yml content mismatch, got: %s", content)
	}

	// check file is in copied list
	if !slices.Contains(copiedFiles, "ssh-agent.yml") {
		t.Error("ssh-agent.yml not in copied files list")
	}
}

func TestCopyToProject__skeleton_local_priority(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	// create user override in skeleton.local/compose/ with same name as system preset
	userOverride := []byte("# user override for go\nservices:\n  agentbox:\n    volumes:\n      - ~/my-go:/go")
	userOverridePath := filepath.Join(paths.SkeletonLocalComposeDir, "go.v1.yml")
	if err := os.WriteFile(userOverridePath, userOverride, 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	_, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	// check go.v1.yml has user content (skeleton.local/ priority)
	goFile := filepath.Join(projectDir, ".agentbox", "go.v1.yml")
	content, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, userOverride) {
		t.Errorf("go.v1.yml should be from skeleton.local/, got: %s", content)
	}
}

func TestCopyToProject__local_yml_from_skeleton_local(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)
	if err := manager.CreateSkeleton([]string{}); err != nil {
		t.Fatal(err)
	}

	// create custom local.yml in skeleton.local/compose/
	customLocalYml := []byte("# custom local.yml from skeleton.local\nservices:\n  agentbox: {}")
	customLocalPath := filepath.Join(paths.SkeletonLocalComposeDir, "local.yml")
	if err := os.WriteFile(customLocalPath, customLocalYml, 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()

	// act
	copiedFiles, err := manager.CopyToProject(projectDir)

	// assert
	if err != nil {
		t.Fatalf("CopyToProject error: %v", err)
	}

	// check local.yml has content from skeleton.local/ (priority over skeleton/)
	localFile := filepath.Join(projectDir, ".agentbox", "local.yml")
	content, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, customLocalYml) {
		t.Errorf("local.yml should be from skeleton.local/, got: %s", content)
	}

	if !slices.Contains(copiedFiles, "local.yml") {
		t.Error("local.yml not in copied files list")
	}
}

func TestCreateSkeleton__preserves_skeleton_local(t *testing.T) {
	// arrange
	paths := createTestPaths(t)
	manager := NewManager(paths)

	// first create with initial presets
	if err := manager.CreateSkeleton([]string{"go"}); err != nil {
		t.Fatal(err)
	}

	// create user preset in skeleton.local/compose/
	userPreset := []byte("# my custom preset")
	userPresetPath := filepath.Join(paths.SkeletonLocalComposeDir, "custom.yml")
	if err := os.WriteFile(userPresetPath, userPreset, 0o644); err != nil {
		t.Fatal(err)
	}

	// act: recreate skeleton (simulates update)
	err := manager.CreateSkeleton([]string{"python"})

	// assert
	if err != nil {
		t.Fatalf("CreateSkeleton error: %v", err)
	}

	// check skeleton.local/compose/custom.yml preserved
	content, err := os.ReadFile(userPresetPath)
	if err != nil {
		t.Fatalf("user preset should be preserved: %v", err)
	}
	if !bytes.Equal(content, userPreset) {
		t.Errorf("user preset content changed, got: %s", content)
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

	// create .DS_Store in skeleton.local/compose/
	dsStorePath := filepath.Join(paths.SkeletonLocalComposeDir, ".DS_Store")
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
