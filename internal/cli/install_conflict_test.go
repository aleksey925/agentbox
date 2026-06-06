package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIsHomebrewManaged(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"homebrew arm", "/opt/homebrew/Cellar/agentbox/1.2.0/bin/agentbox", true},
		{"homebrew intel", "/usr/local/Cellar/agentbox/1.2.0/bin/agentbox", true},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/agentbox/1.2.0/bin/agentbox", true},
		{"local bin", "/home/user/.local/bin/agentbox", false},
		{"usr local bin", "/usr/local/bin/agentbox", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			got := isHomebrewManaged(tt.path)

			// assert
			if got != tt.want {
				t.Errorf("isHomebrewManaged(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDuplicateBinaries(t *testing.T) {
	dir1 := newBinaryDir(t)
	dir2 := newBinaryDir(t)
	dir3 := newBinaryDir(t)
	current := filepath.Join(dir1, "agentbox")

	t.Run("other binaries on PATH", func(t *testing.T) {
		// arrange
		t.Setenv("PATH", dir1+string(os.PathListSeparator)+dir2+string(os.PathListSeparator)+dir3)

		// act
		got := duplicateBinaries(current)

		// assert
		want := []string{filepath.Join(dir2, "agentbox"), filepath.Join(dir3, "agentbox")}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("duplicateBinaries() = %v, want %v", got, want)
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		// arrange
		t.Setenv("PATH", dir1)

		// act
		got := duplicateBinaries(current)

		// assert
		if len(got) != 0 {
			t.Errorf("duplicateBinaries() = %v, want empty", got)
		}
	})
}

func newBinaryDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agentbox"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
