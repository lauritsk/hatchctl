package fs

import (
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

func FeatureBuildDir(stateDir string) string {
	return filepath.Join(stateDir, "features-build")
}

func ResetFeatureBuildDir(stateDir string) (string, error) {
	buildDir := FeatureBuildDir(stateDir)
	if err := os.RemoveAll(buildDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(buildDir, 0o700); err != nil {
		return "", err
	}
	return buildDir, nil
}

func WriteFeatureBuildFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, mode)
}

func CopyFeatureSource(src string, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return os.CopyFS(dst, os.DirFS(src))
}
