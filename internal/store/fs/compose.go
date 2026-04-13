package fs

import (
	"path/filepath"
)

func ComposeOverridePath(stateDir string) string {
	return filepath.Join(stateDir, "docker-compose.override.yml")
}

func WriteComposeOverride(stateDir string, contents []byte) (string, error) {
	path := ComposeOverridePath(stateDir)
	if err := writeFile(path, contents, 0o700, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
