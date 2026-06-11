package docker

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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

func TestBuildExecArgs(t *testing.T) {
	// act
	args := buildExecArgs("abc123", "/home/user/myproject")

	// assert
	expected := []string{"exec", "-it", "-w", "/home/user/myproject", "--", "abc123", "/bin/bash"}
	if !slices.Equal(args, expected) {
		t.Errorf("buildExecArgs() = %v, want %v", args, expected)
	}
}

func TestBuildExecArgs__no_working_dir(t *testing.T) {
	// act
	args := buildExecArgs("abc123", "")

	// assert
	expected := []string{"exec", "-it", "--", "abc123", "/bin/bash"}
	if !slices.Equal(args, expected) {
		t.Errorf("buildExecArgs() = %v, want %v", args, expected)
	}
}

func TestBuildExecArgs__dash_prefixed_id_lands_after_terminator(t *testing.T) {
	// a containerID starting with "-" must never precede "--", or docker would
	// read it as a flag (e.g. --privileged) instead of the target container.
	// act
	args := buildExecArgs("--privileged", "")

	// assert
	dashDash := slices.Index(args, "--")
	id := slices.Index(args, "--privileged")
	if dashDash == -1 || id <= dashDash {
		t.Errorf("containerID must follow the %q terminator: %v", "--", args)
	}
}

func TestContainerProjectPath__mirrors_host_path(t *testing.T) {
	// arrange
	hostDir := "/Users/alex/projects/myapp"

	// act
	got := containerProjectPath(hostDir)

	// assert
	if got != hostDir {
		t.Errorf("containerProjectPath(%q) = %q, want %q", hostDir, got, hostDir)
	}
}

func TestBuildRunEnv__exports_project_path(t *testing.T) {
	// arrange
	projectDir := "/Users/alex/projects/myapp"

	// act
	env := buildRunEnv(projectDir)

	// assert
	want := "AGENTBOX_PROJECT_PATH=" + projectDir
	if !slices.Contains(env, want) {
		t.Errorf("env does not contain %q", want)
	}
}

func TestBuildRunEnv__overrides_existing_project_path(t *testing.T) {
	// arrange
	t.Setenv("AGENTBOX_PROJECT_PATH", "/stale/value")
	projectDir := "/Users/alex/projects/myapp"

	// act
	env := buildRunEnv(projectDir)

	// assert
	if !slices.Contains(env, "AGENTBOX_PROJECT_PATH="+projectDir) {
		t.Errorf("env does not contain AGENTBOX_PROJECT_PATH=%s", projectDir)
	}
	if slices.Contains(env, "AGENTBOX_PROJECT_PATH=/stale/value") {
		t.Error("env still contains the stale AGENTBOX_PROJECT_PATH value")
	}
}

func TestBuildRunEnv__preserves_inherited_env(t *testing.T) {
	// arrange
	t.Setenv("AGENTBOX_TEST_MARKER", "preserved")
	projectDir := "/Users/alex/projects/myapp"

	// act
	env := buildRunEnv(projectDir)

	// assert
	if !slices.Contains(env, "AGENTBOX_TEST_MARKER=preserved") {
		t.Error("env does not preserve inherited variables")
	}
	projectPathEntries := 0
	for _, e := range env {
		if strings.HasPrefix(e, "AGENTBOX_PROJECT_PATH=") {
			projectPathEntries++
		}
	}
	if projectPathEntries != 1 {
		t.Errorf("env contains %d AGENTBOX_PROJECT_PATH entries, want 1", projectPathEntries)
	}
}

func TestSharedVolumes__match_templates(t *testing.T) {
	// arrange — gather external volumes declared across core + all preset templates
	contents := gatherTemplateContents(t)

	// pattern matches: name: volume-name followed by external: true
	re := regexp.MustCompile(`name:\s*(\S+)\s+external:\s*true`)
	var externalVolumes []string
	for _, content := range contents {
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			externalVolumes = append(externalVolumes, m[1])
		}
	}

	if len(externalVolumes) == 0 {
		t.Fatal("no external volumes found in templates")
	}

	// act & assert — SharedVolumes and declared external volumes must match exactly,
	// so every shared volume is created (EnsureSharedVolumes) and none is orphaned.
	sharedSet := make(map[string]bool)
	for _, v := range SharedVolumes {
		sharedSet[v] = true
	}

	for _, vol := range externalVolumes {
		if !sharedSet[vol] {
			t.Errorf("external volume %q declared in a template is missing from SharedVolumes", vol)
		}
	}

	for _, vol := range SharedVolumes {
		if !slices.Contains(externalVolumes, vol) {
			t.Errorf("SharedVolumes contains %q but no template declares it as external", vol)
		}
	}
}

func TestDockerfile_precreates_named_volume_mountpoints(t *testing.T) {
	// a fresh named volume mounted on a path absent from the image is root-owned,
	// so the non-root box user could not write it; every named-volume mount target
	// in a template must be pre-created in the Dockerfile.
	dockerfile, err := skeleton.GetEmbeddedDockerfile()
	if err != nil {
		t.Fatalf("GetEmbeddedDockerfile error: %v", err)
	}
	df := string(dockerfile.Content)

	// the mountpoint must be created AS box (after `USER box`), or the volume root
	// inherits root ownership and the non-root agent can't write it.
	userBox := strings.Index(df, "USER box")
	if userBox < 0 {
		t.Fatal("Dockerfile must drop to the non-root box user")
	}
	asBox := df[userBox:]

	// service volume entries that reference a named volume: "- <name>:<target>",
	// where <name> is a bare identifier (bind mounts start with ~, ., / or $).
	re := regexp.MustCompile(`(?m)^\s*-\s+([a-z][a-z0-9-]*):(/\S+)`)
	for _, content := range gatherTemplateContents(t) {
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if target := m[2]; !strings.Contains(asBox, target) {
				t.Errorf("named volume target %q is not pre-created as box in the Dockerfile (volume would be root-owned)", target)
			}
		}
	}
}

func gatherTemplateContents(t *testing.T) []string {
	t.Helper()

	core, err := skeleton.GetCoreTemplate()
	if err != nil {
		t.Fatalf("GetCoreTemplate error: %v", err)
	}
	contents := []string{string(core.Content)}
	for _, p := range skeleton.SupportedPresets() {
		tmpl, err := skeleton.GetPresetTemplate(p.TemplateName)
		if err != nil {
			t.Fatalf("GetPresetTemplate(%s) error: %v", p.TemplateName, err)
		}
		contents = append(contents, string(tmpl.Content))
	}
	return contents
}
