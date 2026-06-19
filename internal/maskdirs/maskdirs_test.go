package maskdirs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectMaskDirs__root_and_one_level_deep(t *testing.T) {
	// arrange
	root := t.TempDir()
	mkdirs(t, root, ".venv", "node_modules", "frontend/node_modules", "backend/.venv")

	// act
	got := DetectMaskDirs(root)

	// assert
	want := []string{".venv", "backend/.venv", "frontend/node_modules", "node_modules"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectMaskDirs__ignores_vendor(t *testing.T) {
	// arrange
	root := t.TempDir()
	mkdirs(t, root, "vendor", "node_modules")

	// act
	got := DetectMaskDirs(root)

	// assert
	want := []string{"node_modules"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectMaskDirs__skips_agentbox_and_git_and_does_not_descend_node_modules(t *testing.T) {
	// arrange
	root := t.TempDir()
	mkdirs(t, root,
		".agentbox/.venv",
		".git/node_modules",
		"node_modules/sub/node_modules",
	)

	// act
	got := DetectMaskDirs(root)

	// assert
	want := []string{"node_modules"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDefaultFileContent__with_detected(t *testing.T) {
	// arrange
	detected := []string{".venv", "frontend/node_modules"}

	// act
	content := string(DefaultFileContent(detected))

	// assert
	if !strings.Contains(content, "\n.venv\n") {
		t.Errorf("detected .venv not written as active line:\n%s", content)
	}
	if !strings.Contains(content, "\nfrontend/node_modules\n") {
		t.Errorf("detected nested dir not written as active line:\n%s", content)
	}
	if !strings.Contains(content, "# node_modules\n") {
		t.Errorf("undetected candidate not written as comment:\n%s", content)
	}
	if strings.Contains(content, "# .venv\n") {
		t.Errorf("detected dir should not also be a comment:\n%s", content)
	}
}

func TestDefaultFileContent__without_detected(t *testing.T) {
	// act
	content := string(DefaultFileContent(nil))

	// assert
	for _, name := range candidateNames {
		if !strings.Contains(content, "# "+name+"\n") {
			t.Errorf("candidate %q not written as comment:\n%s", name, content)
		}
	}
}

func TestDefaultFileContent__vendor_is_commented_suggestion(t *testing.T) {
	// act
	content := string(DefaultFileContent(nil))

	// assert
	if !strings.Contains(content, "# vendor\n") {
		t.Errorf("vendor suggestion not written as comment:\n%s", content)
	}
	if strings.Contains(content, "\nvendor\n") {
		t.Errorf("vendor must never be an active line:\n%s", content)
	}
}

func TestEnsureFile__creates_when_absent(t *testing.T) {
	// arrange
	path := filepath.Join(t.TempDir(), "masked-dirs")

	// act
	if err := EnsureFile(path, []string{".venv"}); err != nil {
		t.Fatal(err)
	}

	// assert
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := DefaultFileContent([]string{".venv"}); !reflect.DeepEqual(got, want) {
		t.Errorf("content mismatch")
	}
}

func TestEnsureFile__preserves_existing(t *testing.T) {
	// arrange
	path := filepath.Join(t.TempDir(), "masked-dirs")
	existing := []byte("custom-dir\n")
	if err := os.WriteFile(path, existing, 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	if err := EnsureFile(path, []string{".venv"}); err != nil {
		t.Fatal(err)
	}

	// assert
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, existing) {
		t.Errorf("existing file was overwritten: %q", got)
	}
}

func TestParseFile__strips_comments_and_blanks(t *testing.T) {
	// arrange
	path := filepath.Join(t.TempDir(), "masked-dirs")
	content := "# header\n\n.venv\n  node_modules  \n# comment\nfrontend/node_modules\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	got, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// assert
	want := []string{".venv", "node_modules", "frontend/node_modules"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFile__missing_file_returns_nil(t *testing.T) {
	// act
	got, err := ParseFile(filepath.Join(t.TempDir(), "does-not-exist"))

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
