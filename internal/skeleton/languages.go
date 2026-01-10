package skeleton

import (
	"os"
	"path/filepath"
)

// Language represents a supported programming language with its detection rules.
type Language struct {
	Name         string
	TemplateName string // name in template filename (e.g., "go", "python")
}

// SupportedLanguages returns all supported languages in display order.
func SupportedLanguages() []Language {
	return []Language{
		{Name: "Go", TemplateName: "go"},
		{Name: "Python", TemplateName: "python"},
	}
}

// DetectionResult contains information about language detection.
type DetectionResult struct {
	Language Language
	Detected bool
	Reason   string // explanation why language was detected
}

// DetectLanguages checks the user's environment for installed languages.
func DetectLanguages(homeDir string) []DetectionResult {
	languages := SupportedLanguages()
	results := make([]DetectionResult, len(languages))

	for i, lang := range languages {
		results[i] = DetectionResult{Language: lang}
		results[i].Detected, results[i].Reason = detectLanguage(lang.TemplateName, homeDir)
	}

	return results
}

func detectLanguage(templateName, homeDir string) (detected bool, reason string) {
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
