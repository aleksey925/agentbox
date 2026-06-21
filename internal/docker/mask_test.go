package docker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateMaskSub__accepts_relative_and_nested(t *testing.T) {
	// arrange
	cases := map[string]string{
		"node_modules":          "node_modules",
		"frontend/node_modules": filepath.Join("frontend", "node_modules"),
		"./.venv":               ".venv",
	}

	for in, want := range cases {
		// act
		got, err := validateMaskSub(in)

		// assert
		if err != nil {
			t.Errorf("validateMaskSub(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("validateMaskSub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateMaskSub__rejects(t *testing.T) {
	// arrange
	cases := []string{
		"/abs/path",
		"..",
		"../escape",
		"foo/../../escape",
		".agentbox",
		".agentbox/sub",
		".git",
		".git/hooks",
		".",
	}

	for _, in := range cases {
		// act
		_, err := validateMaskSub(in)

		// assert
		if err == nil {
			t.Errorf("validateMaskSub(%q) = nil error, want rejection", in)
		}
	}
}

func TestMaskVolumeName__deterministic_and_project_scoped(t *testing.T) {
	// arrange
	projA, projB := "/home/user/a", "/home/user/b"

	// act
	a1 := maskVolumeName(projA, "node_modules")
	a2 := maskVolumeName(projA, "node_modules")
	b := maskVolumeName(projB, "node_modules")

	// assert
	if a1 != a2 {
		t.Errorf("not deterministic: %q != %q", a1, a2)
	}
	if a1 == b {
		t.Errorf("different projects share a volume name: %q", a1)
	}
	if !strings.HasPrefix(a1, projectMaskPrefix(projA)) {
		t.Errorf("%q missing project prefix %q", a1, projectMaskPrefix(projA))
	}
}

func TestMaskedEntries__absent_file(t *testing.T) {
	// arrange
	projectDir := t.TempDir()

	// act
	entries, err := maskedEntries(projectDir)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("got %v, want nil", entries)
	}
}

func TestMaskedEntries__builds_target_and_volume(t *testing.T) {
	// arrange
	projectDir := t.TempDir()
	writeMaskFile(t, projectDir, "node_modules\nfrontend/node_modules\n")

	// act
	entries, err := maskedEntries(projectDir)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	want := []maskEntry{
		{
			sub:    "node_modules",
			target: filepath.Join(projectDir, "node_modules"),
			volume: maskVolumeName(projectDir, "node_modules"),
		},
		{
			sub:    filepath.Join("frontend", "node_modules"),
			target: filepath.Join(projectDir, "frontend", "node_modules"),
			volume: maskVolumeName(projectDir, filepath.Join("frontend", "node_modules")),
		},
	}
	if !slices.Equal(entries, want) {
		t.Errorf("got %v, want %v", entries, want)
	}
}

func TestMaskedEntries__rejects_invalid(t *testing.T) {
	// arrange
	projectDir := t.TempDir()
	writeMaskFile(t, projectDir, "node_modules\n.git/hooks\n")

	// act
	_, err := maskedEntries(projectDir)

	// assert
	if err == nil {
		t.Fatal("expected error for invalid entry")
	}
}

func TestRenderMaskCompose__path_with_space_round_trips(t *testing.T) {
	// arrange
	entries := []maskEntry{
		{sub: "node_modules", target: "/home/My Project/node_modules", volume: "agentbox-mask-aaa-bbb"},
	}

	// act
	out := renderMaskCompose(entries)

	// assert
	wantTarget := `target: "/home/My Project/node_modules"`
	if !strings.Contains(out, wantTarget) {
		t.Errorf("rendered fragment missing quoted target:\n%s", out)
	}
	wantEnv := `- "AGENTBOX_MASK_PATHS=/home/My Project/node_modules"`
	if !strings.Contains(out, wantEnv) {
		t.Errorf("rendered fragment missing env path:\n%s", out)
	}
	if strings.Contains(out, "AGENTBOX_MASK_PATHS:") {
		t.Errorf("environment must be list form, not map form:\n%s", out)
	}
	wantVol := `"agentbox-mask-aaa-bbb":`
	if !strings.Contains(out, wantVol) {
		t.Errorf("rendered fragment missing volume declaration:\n%s", out)
	}
	if strings.Contains(out, "external") {
		t.Errorf("volume must not be external:\n%s", out)
	}
}

func TestRenderMaskCompose__multiple_paths_newline_joined(t *testing.T) {
	// arrange
	entries := []maskEntry{
		{sub: "a", target: "/p/a", volume: "v1"},
		{sub: "b", target: "/p/b", volume: "v2"},
	}

	// act
	out := renderMaskCompose(entries)

	// assert
	if !strings.Contains(out, `- "AGENTBOX_MASK_PATHS=/p/a\n/p/b"`) {
		t.Errorf("paths not newline-joined:\n%s", out)
	}
}

func TestOrphanMaskVolumes__set_difference(t *testing.T) {
	// arrange
	present := []string{"v1", "v2", "v3"}
	keep := []string{"v2"}

	// act
	got := orphanMaskVolumes(present, keep)

	// assert
	if !slices.Equal(got, []string{"v1", "v3"}) {
		t.Errorf("got %v, want [v1 v3]", got)
	}
}

func writeMaskFile(t *testing.T, projectDir, content string) {
	t.Helper()
	agentboxDir := filepath.Join(projectDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentboxDir, "masked-dirs"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
