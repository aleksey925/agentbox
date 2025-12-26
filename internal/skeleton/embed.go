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

func Extract(destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

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

func IsExtracted(destDir string) bool {
	for _, name := range skeletonFiles {
		path := filepath.Join(destDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func Files() []string {
	return skeletonFiles
}
