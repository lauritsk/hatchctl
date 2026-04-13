package fs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
	"github.com/tailscale/hujson"
)

type FeatureLockFile map[string]FeatureLockEntry

type FeatureLockEntry struct {
	Version   string `json:"version,omitempty"`
	Resolved  string `json:"resolved,omitempty"`
	Integrity string `json:"integrity,omitempty"`
}

type FeatureStateFile struct {
	Features []FeatureStateEntry `json:"features"`
}

type FeatureStateEntry struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Kind      string            `json:"kind,omitempty"`
	Path      string            `json:"path,omitempty"`
	Resolved  string            `json:"resolved,omitempty"`
	Integrity string            `json:"integrity,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
}

func FeatureLockFilePath(configPath string) string {
	dir := filepath.Dir(configPath)
	if filepath.Base(configPath) == ".devcontainer.json" {
		return filepath.Join(dir, ".devcontainer-lock.json")
	}
	return filepath.Join(dir, "devcontainer-lock.json")
}

func FeatureCacheDir(cacheDir string) string {
	return filepath.Join(cacheDir, "features-cache")
}

func ReadFeatureLockFile(configPath string) (FeatureLockFile, bool, error) {
	path := FeatureLockFilePath(configPath)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return FeatureLockFile{}, true, nil
	}
	data, err = hujson.Standardize(data)
	if err != nil {
		return nil, true, err
	}
	lock := FeatureLockFile{}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, true, err
	}
	return lock, true, nil
}

func WriteFeatureLockFile(configPath string, lock FeatureLockFile) error {
	path := FeatureLockFilePath(configPath)
	return writeOptionalJSON(path, len(lock) == 0, lock, 0o755, 0o644)
}

func WriteFeatureStateFile(stateDir string, state FeatureStateFile) error {
	path := filepath.Join(stateDir, "features-lock.json")
	return writeOptionalJSON(path, len(state.Features) == 0, state, 0o700, 0o600)
}
