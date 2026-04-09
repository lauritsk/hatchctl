package fs

import (
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

func ComposeOverridePath(stateDir string) string {
	return filepath.Join(stateDir, "docker-compose.override.yml")
}

func WriteComposeOverride(stateDir string, contents []byte) (string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", err
	}
	path := ComposeOverridePath(stateDir)
	if err := fileutil.WriteFile(path, contents, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
