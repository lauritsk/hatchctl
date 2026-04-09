package devcontainer

import (
	"fmt"

	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type FeatureLockFile = storefs.FeatureLockFile

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

type FeatureLockEntry = storefs.FeatureLockEntry

type FeatureStateFile = storefs.FeatureStateFile

type FeatureStateEntry = storefs.FeatureStateEntry

func FeatureLockFilePath(configPath string) string {
	return storefs.FeatureLockFilePath(configPath)
}

func ReadFeatureLockFile(configPath string) (FeatureLockFile, bool, error) {
	return storefs.ReadFeatureLockFile(configPath)
}

func WriteFeatureLockFile(configPath string, features []ResolvedFeature) error {
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
	return storefs.WriteFeatureLockFile(configPath, lock)
}

func WriteFeatureStateFile(stateDir string, features []ResolvedFeature) error {
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
	return storefs.WriteFeatureStateFile(stateDir, state)
}
