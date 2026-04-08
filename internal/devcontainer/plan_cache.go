package devcontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const resolvedPlanCacheVersion = 1

type resolvedPlanCache struct {
	Version  int            `json:"version"`
	Key      string         `json:"key"`
	Resolved ResolvedConfig `json:"resolved"`
}

func readResolvedPlanCache(stateDir string, key string) (ResolvedConfig, bool, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, "resolved-plan.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedConfig{}, false, nil
		}
		return ResolvedConfig{}, false, err
	}
	var cache resolvedPlanCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return ResolvedConfig{}, false, nil
	}
	if cache.Version != resolvedPlanCacheVersion || cache.Key != key {
		return ResolvedConfig{}, false, nil
	}
	return cache.Resolved, true, nil
}

func writeResolvedPlanCache(stateDir string, key string, resolved ResolvedConfig) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resolvedPlanCache{
		Version:  resolvedPlanCacheVersion,
		Key:      key,
		Resolved: resolved,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "resolved-plan.json"), data, 0o644)
}

func resolvedPlanCacheKey(configPath string, configDir string, config Config, composeFiles []string) (string, error) {
	h := sha256.New()
	writeHashString(h, filepath.Clean(configPath))
	if err := hashFile(h, configPath); err != nil {
		return "", err
	}
	lockPath := FeatureLockFilePath(configPath)
	if _, err := os.Stat(lockPath); err == nil {
		writeHashString(h, filepath.Clean(lockPath))
		if err := hashFile(h, lockPath); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	for _, path := range composeFiles {
		writeHashString(h, filepath.Clean(path))
		if err := hashFile(h, path); err != nil {
			return "", err
		}
	}
	localFeatures, err := resolveLocalFeaturePaths(configDir, config.Features)
	if err != nil {
		return "", err
	}
	for _, path := range localFeatures {
		writeHashString(h, filepath.Clean(path))
		if err := hashDir(h, path); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func resolveLocalFeaturePaths(configDir string, values map[string]any) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	paths := map[string]struct{}{}
	for source := range values {
		path, err := resolveLocalFeaturePath(configDir, source)
		if err == nil {
			paths[path] = struct{}{}
			continue
		}
		if !isMissingPathError(err) {
			return nil, err
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func hashDir(h hash.Hash, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		writeHashString(h, rel)
		if d.IsDir() {
			return nil
		}
		return hashFile(h, path)
	})
}

func hashFile(h hash.Hash, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(h, file); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	return nil
}

func writeHashString(h hash.Hash, value string) {
	_, _ = io.WriteString(h, value)
	_, _ = io.WriteString(h, "\n")
}
