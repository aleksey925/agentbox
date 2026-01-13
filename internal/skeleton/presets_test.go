package skeleton

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSupportedPresets(t *testing.T) {
	// act
	presets := SupportedPresets()

	// assert
	if len(presets) == 0 {
		t.Error("expected at least one preset")
	}

	expectedNames := []string{"Go", "Python"}
	for i, preset := range presets {
		if preset.Name != expectedNames[i] {
			t.Errorf("preset[%d].Name = %s, want %s", i, preset.Name, expectedNames[i])
		}
	}
}

func TestDetectPresets(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()

	// clear environment variables that would affect detection
	oldGopath := os.Getenv("GOPATH")
	_ = os.Unsetenv("GOPATH")
	defer func() {
		if oldGopath != "" {
			_ = os.Setenv("GOPATH", oldGopath)
		}
	}()

	// act
	results := DetectPresets(tmpDir)

	// assert
	if len(results) != len(SupportedPresets()) {
		t.Errorf("expected %d results, got %d", len(SupportedPresets()), len(results))
	}

	// all should be not detected in empty dir (with env vars cleared)
	for _, r := range results {
		if r.Detected {
			t.Errorf("preset %s should not be detected in empty dir", r.Preset.Name)
		}
	}
}

func TestDetectPresets__detects_go_by_directory(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	goDir := filepath.Join(tmpDir, "go")
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// clear GOPATH so directory detection is tested
	oldGopath := os.Getenv("GOPATH")
	_ = os.Unsetenv("GOPATH")
	defer func() {
		if oldGopath != "" {
			_ = os.Setenv("GOPATH", oldGopath)
		}
	}()

	// act
	results := DetectPresets(tmpDir)

	// assert
	var goResult DetectionResult
	for _, r := range results {
		if r.Preset.TemplateName == "go" {
			goResult = r
			break
		}
	}

	if !goResult.Detected {
		t.Error("Go should be detected when ~/go exists")
	}
	if goResult.Reason != "~/go exists" {
		t.Errorf("reason = %s, want '~/go exists'", goResult.Reason)
	}
}

func TestDetectPresets__detects_go_by_gopath(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	_ = os.Setenv("GOPATH", "/some/path")
	defer func() { _ = os.Unsetenv("GOPATH") }()

	// act
	results := DetectPresets(tmpDir)

	// assert
	var goResult DetectionResult
	for _, r := range results {
		if r.Preset.TemplateName == "go" {
			goResult = r
			break
		}
	}

	if !goResult.Detected {
		t.Error("Go should be detected when $GOPATH is set")
	}
	if goResult.Reason != "$GOPATH is set" {
		t.Errorf("reason = %s, want '$GOPATH is set'", goResult.Reason)
	}
}

func TestDetectPresets__detects_python(t *testing.T) {
	// arrange
	tmpDir := t.TempDir()
	uvDir := filepath.Join(tmpDir, ".cache", "uv")
	if err := os.MkdirAll(uvDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	results := DetectPresets(tmpDir)

	// assert
	var pythonResult DetectionResult
	for _, r := range results {
		if r.Preset.TemplateName == "python" {
			pythonResult = r
			break
		}
	}

	if !pythonResult.Detected {
		t.Error("Python should be detected when ~/.cache/uv exists")
	}
	if pythonResult.Reason != "~/.cache/uv exists" {
		t.Errorf("reason = %s, want '~/.cache/uv exists'", pythonResult.Reason)
	}
}
