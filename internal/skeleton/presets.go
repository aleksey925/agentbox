package skeleton

import (
	"os"
	"path/filepath"
)

// Preset represents a supported environment preset with its detection rules.
type Preset struct {
	Name         string
	TemplateName string // name in template filename (e.g., "go", "python")
}

// SupportedPresets returns all supported presets in display order.
func SupportedPresets() []Preset {
	return []Preset{
		{Name: "Go", TemplateName: "go"},
		{Name: "Python", TemplateName: "python"},
	}
}

// DetectionResult contains information about preset detection.
type DetectionResult struct {
	Preset   Preset
	Detected bool
	Reason   string // explanation why preset was detected
}

// DetectPresets checks the user's environment for installed development tools.
func DetectPresets(homeDir string) []DetectionResult {
	presets := SupportedPresets()
	results := make([]DetectionResult, len(presets))

	for i, preset := range presets {
		results[i] = DetectionResult{Preset: preset}
		results[i].Detected, results[i].Reason = detectPreset(preset.TemplateName, homeDir)
	}

	return results
}

func detectPreset(templateName, homeDir string) (detected bool, reason string) {
	switch templateName {
	case "go":
		return detectGo(homeDir)
	case "python":
		return detectPython(homeDir)
	}
	return false, ""
}

func detectGo(homeDir string) (bool, string) {
	// check $GOPATH
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		return true, "$GOPATH is set"
	}
	// check ~/.go
	if dirExists(filepath.Join(homeDir, ".go")) {
		return true, "~/.go exists"
	}
	// check ~/go
	if dirExists(filepath.Join(homeDir, "go")) {
		return true, "~/go exists"
	}
	return false, ""
}

func detectPython(homeDir string) (bool, string) {
	// check ~/.cache/uv
	if dirExists(filepath.Join(homeDir, ".cache", "uv")) {
		return true, "~/.cache/uv exists"
	}
	// check ~/.cache/pypoetry
	if dirExists(filepath.Join(homeDir, ".cache", "pypoetry")) {
		return true, "~/.cache/pypoetry exists"
	}
	// check ~/.local/share/virtualenvs (pipenv)
	if dirExists(filepath.Join(homeDir, ".local", "share", "virtualenvs")) {
		return true, "~/.local/share/virtualenvs exists"
	}
	return false, ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
