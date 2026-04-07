package devcontainer

import (
	"os"
	"path/filepath"
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

func ReadFeatureLockFile(configPath string) (FeatureLockFile, bool, error) {
	path := FeatureLockFilePath(configPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(bytesTrimSpace(data)) == 0 {
		return FeatureLockFile{}, true, nil
	}
	data, err = standardizeJSONC(data)
	if err != nil {
		return nil, true, err
	}
	lock := FeatureLockFile{}
	if err := jsonUnmarshal(data, &lock); err != nil {
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
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := jsonMarshalIndent(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func WriteFeatureStateFile(stateDir string, features []ResolvedFeature) error {
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
	data, err := jsonMarshalIndent(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func bytesTrimSpace(data []byte) []byte {
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t') {
		start++
	}
	end := len(data)
	for end > start && (data[end-1] == ' ' || data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == '\t') {
		end--
	}
	return data[start:end]
}
