package skeleton

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed files/*
var embeddedFS embed.FS

// overwriteFiles are always overwritten by 'agentbox init'
var overwriteFiles = []string{
	"Dockerfile.agentbox",
	"docker-compose.agentbox.yml",
}

// userFiles are created only if they don't exist (never overwritten)
var userFiles = []string{
	"docker-compose.agentbox.local.yml",
}

// CopyTo copies embedded skeleton files directly to the destination directory.
// These files are always overwritten.
func CopyTo(destDir string) error {
	for _, name := range overwriteFiles {
		data, err := embeddedFS.ReadFile("files/" + name)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destDir, name)
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

// CopyUserFilesIfMissing copies user-specific files only if they don't exist.
// These files are never overwritten to preserve user customizations.
func CopyUserFilesIfMissing(destDir string) ([]string, error) {
	var created []string

	for _, name := range userFiles {
		destPath := filepath.Join(destDir, name)

		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		data, err := embeddedFS.ReadFile("files/" + name)
		if err != nil {
			return created, err
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return created, err
		}
		created = append(created, name)
	}

	return created, nil
}

// OverwriteFiles returns files that are always overwritten by init.
func OverwriteFiles() []string {
	return overwriteFiles
}

// Files returns all skeleton files (for git exclude).
func Files() []string {
	all := make([]string, 0, len(overwriteFiles)+len(userFiles))
	all = append(all, overwriteFiles...)
	all = append(all, userFiles...)
	return all
}
