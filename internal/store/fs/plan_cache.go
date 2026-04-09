package fs

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

type resolvedPlanCache[T any] struct {
	Version  int    `json:"version"`
	Key      string `json:"key"`
	Resolved T      `json:"resolved"`
}

func ReadResolvedPlanCache[T any](cacheDir string, key string, version int, validate func(T) error) (T, bool, error) {
	var zero T
	data, err := fileutil.ReadFile(filepath.Join(cacheDir, "resolved-plan.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var cache resolvedPlanCache[T]
	if err := json.Unmarshal(data, &cache); err != nil {
		return zero, false, nil
	}
	if cache.Version != version || cache.Key != key {
		return zero, false, nil
	}
	if validate != nil {
		if err := validate(cache.Resolved); err != nil {
			return zero, false, nil
		}
	}
	return cache.Resolved, true, nil
}

func WriteResolvedPlanCache[T any](cacheDir string, key string, version int, resolved T) error {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resolvedPlanCache[T]{Version: version, Key: key, Resolved: resolved}, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(filepath.Join(cacheDir, "resolved-plan.json"), data, 0o600)
}
