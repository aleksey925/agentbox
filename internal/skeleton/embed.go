package skeleton

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed files/*
var embeddedFS embed.FS

var skeletonFiles = []string{
	"Dockerfile.agentbox",
	"docker-compose.agentbox.yml",
}

// CopyTo copies embedded skeleton files directly to the destination directory.
func CopyTo(destDir string) error {
	for _, name := range skeletonFiles {
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

func Files() []string {
	return skeletonFiles
}
