package devcontainer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

type FeatureLockFile map[string]FeatureLockEntry

type FeatureLockfilePolicy string

const (
	FeatureLockfilePolicyAuto   FeatureLockfilePolicy = "auto"
	FeatureLockfilePolicyFrozen FeatureLockfilePolicy = "frozen"
	FeatureLockfilePolicyUpdate FeatureLockfilePolicy = "update"
)

func ParseFeatureLockfilePolicy(value string) (FeatureLockfilePolicy, error) {
	policy := FeatureLockfilePolicy(value)
	switch policy {
	case "", FeatureLockfilePolicyAuto:
		return FeatureLockfilePolicyAuto, nil
	case FeatureLockfilePolicyFrozen, FeatureLockfilePolicyUpdate:
		return policy, nil
	default:
		return "", fmt.Errorf("invalid lockfile policy %q; expected auto, frozen, or update", value)
	}
}

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
	data, err = standardizeJSONC(path, data)
	if err != nil {
		return nil, true, err
	}
	lock := FeatureLockFile{}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, true, err
	}
	return lock, true, nil
}

func WriteFeatureLockFile(configPath string, features []ResolvedFeature) error {
	path := FeatureLockFilePath(configPath)
	lock := FeatureLockFile{}
	for _, feature := range features {
		entry := FeatureLockEntry{
			Version:   feature.Version,
			Resolved:  feature.Resolved,
			Integrity: feature.Integrity,
		}
		if entry.Version == "" && entry.Resolved == "" && entry.Integrity == "" {
			continue
		}
		lock[feature.Source] = entry
	}
	if len(lock) == 0 {
		if err := fileutil.RemoveFile(path); err != nil {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o644)
}

func WriteFeatureStateFile(stateDir string, features []ResolvedFeature) error {
	path := filepath.Join(stateDir, "features-lock.json")
	if len(features) == 0 {
		if err := fileutil.RemoveFile(path); err != nil {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	state := FeatureStateFile{Features: make([]FeatureStateEntry, 0, len(features))}
	for _, feature := range features {
		entry := FeatureStateEntry{
			ID:        feature.Metadata.ID,
			Source:    feature.Source,
			Kind:      feature.SourceKind,
			Path:      feature.Path,
			Resolved:  feature.Resolved,
			Integrity: feature.Integrity,
			Options:   cloneMap(feature.Options),
		}
		if len(entry.Options) == 0 {
			entry.Options = nil
		}
		state.Features = append(state.Features, entry)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o600)
}
