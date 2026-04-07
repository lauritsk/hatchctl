package devcontainer

import (
	"os"
	"path/filepath"
	"sort"
)

type FeatureLockFile struct {
	Features []FeatureLockEntry `json:"features"`
}

type FeatureLockEntry struct {
	ID      string            `json:"id"`
	Source  string            `json:"source"`
	Path    string            `json:"path,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

func WriteFeatureLockFile(stateDir string, features []ResolvedFeature) error {
	path := filepath.Join(stateDir, "features-lock.json")
	if len(features) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	lock := FeatureLockFile{Features: make([]FeatureLockEntry, 0, len(features))}
	for _, feature := range features {
		entry := FeatureLockEntry{
			ID:      feature.Metadata.ID,
			Source:  feature.Source,
			Path:    feature.Path,
			Options: cloneMap(feature.Options),
		}
		if len(entry.Options) == 0 {
			entry.Options = nil
		} else {
			entry.Options = sortedFeatureOptions(entry.Options)
		}
		lock.Features = append(lock.Features, entry)
	}
	data, err := jsonMarshalIndent(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func sortedFeatureOptions(values map[string]string) map[string]string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]string, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}
