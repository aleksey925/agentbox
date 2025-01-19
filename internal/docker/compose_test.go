package docker

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/aleksey925/agentbox/internal/skeleton"
)

func TestParseContainersOutput(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago\n" +
		"789xyz000111\tother-agentbox-1\t5 minutes ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	expected := []Container{
		{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"},
		{ID: "789xyz000111", Name: "other-agentbox-1", Started: "5 minutes ago"},
	}

	if len(containers) != len(expected) {
		t.Fatalf("len(containers) = %d, want %d", len(containers), len(expected))
	}

	for i, c := range containers {
		if c != expected[i] {
			t.Errorf("containers[%d] = %+v, want %+v", i, c, expected[i])
		}
	}
}

func TestParseContainersOutput__empty(t *testing.T) {
	// act
	containers := parseContainersOutput("")

	// assert
	if len(containers) != 0 {
		t.Errorf("len(containers) = %d, want 0", len(containers))
	}
}

func TestParseContainersOutput__incomplete_line(t *testing.T) {
	// arrange
	output := "abc123\tmy-project-agentbox-1"

	// act
	containers := parseContainersOutput(output)

	// assert
	if len(containers) != 0 {
		t.Errorf("incomplete lines should be skipped, got %d containers", len(containers))
	}
}

func TestParseContainersOutput__mixed_valid_invalid(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago\n" +
		"incomplete\tline\n" +
		"789xyz000111\tother-agentbox-1\t5 minutes ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	expected := []Container{
		{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"},
		{ID: "789xyz000111", Name: "other-agentbox-1", Started: "5 minutes ago"},
	}

	if len(containers) != len(expected) {
		t.Fatalf("len(containers) = %d, want %d", len(containers), len(expected))
	}

	for i, c := range containers {
		if c != expected[i] {
			t.Errorf("containers[%d] = %+v, want %+v", i, c, expected[i])
		}
	}
}

func TestParseContainersOutput__single_container(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	if len(containers) != 1 {
		t.Fatalf("len(containers) = %d, want 1", len(containers))
	}

	expected := Container{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"}
	if containers[0] != expected {
		t.Errorf("containers[0] = %+v, want %+v", containers[0], expected)
	}
}

func TestDiscoverComposeFiles__core_only(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentboxDir, "core.v1.yml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	files, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{filepath.Join(agentboxDir, "core.v1.yml")}
	if len(files) != len(expected) {
		t.Fatalf("len(files) = %d, want %d", len(files), len(expected))
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %s, want %s", i, f, expected[i])
		}
	}
}

func TestDiscoverComposeFiles__core_with_presets(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"core.v1.yml", "go.v1.yml", "python.v1.yml", "local.yml"} {
		if err := os.WriteFile(filepath.Join(agentboxDir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// act
	files, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// expected order: core first, then alphabetically, local.yml last
	expected := []string{
		filepath.Join(agentboxDir, "core.v1.yml"),
		filepath.Join(agentboxDir, "go.v1.yml"),
		filepath.Join(agentboxDir, "python.v1.yml"),
		filepath.Join(agentboxDir, "local.yml"),
	}
	if len(files) != len(expected) {
		t.Fatalf("len(files) = %d, want %d", len(files), len(expected))
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %s, want %s", i, f, expected[i])
		}
	}
}

func TestDiscoverComposeFiles__preset_removed__still_works(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// only core and local, no presets — simulates user deleting preset files
	for _, name := range []string{"core.v1.yml", "local.yml"} {
		if err := os.WriteFile(filepath.Join(agentboxDir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// act
	files, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{
		filepath.Join(agentboxDir, "core.v1.yml"),
		filepath.Join(agentboxDir, "local.yml"),
	}
	if len(files) != len(expected) {
		t.Fatalf("len(files) = %d, want %d", len(files), len(expected))
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %s, want %s", i, f, expected[i])
		}
	}
}

func TestDiscoverComposeFiles__empty_directory__returns_error(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err == nil {
		t.Fatal("expected error for empty .agentbox directory")
	}
	expectedMsg := "no compose files found in .agentbox/. Run 'agentbox init' to fix"
	if err.Error() != expectedMsg {
		t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestDiscoverComposeFiles__no_agentbox_dir__returns_error(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// act
	_, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err == nil {
		t.Fatal("expected error when .agentbox directory doesn't exist")
	}
}

func TestDiscoverComposeFiles__ignores_non_yml_files(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	agentboxDir := filepath.Join(tmpDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// yml files
	if err := os.WriteFile(filepath.Join(agentboxDir, "core.v1.yml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-yml files should be ignored
	if err := os.WriteFile(filepath.Join(agentboxDir, "Dockerfile.agentbox"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentboxDir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	files, err := DiscoverComposeFiles(tmpDir)

	// assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{filepath.Join(agentboxDir, "core.v1.yml")}
	if len(files) != len(expected) {
		t.Fatalf("len(files) = %d, want %d; got %v", len(files), len(expected), files)
	}
}

func TestBuildRunArgs(t *testing.T) {
	// arrange
	projectDir := "/home/user/myproject"
	composeFiles := []string{
		"/home/user/myproject/.agentbox/core.v1.yml",
		"/home/user/myproject/.agentbox/go.v1.yml",
	}

	// act
	args := buildRunArgs(projectDir, composeFiles)

	// assert
	expected := []string{
		"compose", "--project-directory", "/home/user/myproject",
		"-f", "/home/user/myproject/.agentbox/core.v1.yml",
		"-f", "/home/user/myproject/.agentbox/go.v1.yml",
		"run", "--rm", "agentbox",
	}
	if len(args) != len(expected) {
		t.Fatalf("len(args) = %d, want %d\nargs: %v", len(args), len(expected), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestBuildBuildArgs(t *testing.T) {
	// arrange
	projectDir := "/home/user/myproject"
	composeFiles := []string{"/home/user/myproject/.agentbox/core.v1.yml"}

	// act
	args := buildBuildArgs(projectDir, composeFiles, false)

	// assert
	expected := []string{
		"compose", "--project-directory", "/home/user/myproject",
		"-f", "/home/user/myproject/.agentbox/core.v1.yml",
		"build",
	}
	if len(args) != len(expected) {
		t.Fatalf("len(args) = %d, want %d\nargs: %v", len(args), len(expected), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestBuildBuildArgs__with_no_cache(t *testing.T) {
	// arrange
	projectDir := "/home/user/myproject"
	composeFiles := []string{"/home/user/myproject/.agentbox/core.v1.yml"}

	// act
	args := buildBuildArgs(projectDir, composeFiles, true)

	// assert
	expected := []string{
		"compose", "--project-directory", "/home/user/myproject",
		"-f", "/home/user/myproject/.agentbox/core.v1.yml",
		"build", "--no-cache",
	}
	if len(args) != len(expected) {
		t.Fatalf("len(args) = %d, want %d\nargs: %v", len(args), len(expected), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestSharedVolumes__match_core_template(t *testing.T) {
	// arrange
	coreTemplate, err := skeleton.GetCoreTemplate()
	if err != nil {
		t.Fatalf("GetCoreTemplate error: %v", err)
	}
	content := string(coreTemplate.Content)

	// extract volume names with "external: true" from YAML
	// pattern matches: name: volume-name followed by external: true
	re := regexp.MustCompile(`name:\s*(\S+)\s+external:\s*true`)
	matches := re.FindAllStringSubmatch(content, -1)

	var externalVolumes []string
	for _, m := range matches {
		externalVolumes = append(externalVolumes, m[1])
	}

	if len(externalVolumes) == 0 {
		t.Fatal("no external volumes found in core template")
	}

	// act & assert
	sharedSet := make(map[string]bool)
	for _, v := range SharedVolumes {
		sharedSet[v] = true
	}

	for _, vol := range externalVolumes {
		if !sharedSet[vol] {
			t.Errorf("external volume %q in core.v1.yml is missing from SharedVolumes", vol)
		}
	}

	// also check reverse: all SharedVolumes should be in template
	for _, vol := range SharedVolumes {
		found := false
		for _, extVol := range externalVolumes {
			if extVol == vol {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SharedVolumes contains %q but it's not in core.v1.yml as external", vol)
		}
	}
}
