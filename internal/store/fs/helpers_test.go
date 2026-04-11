package fs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFeatureLockFilePathMatchesConfigLocation(t *testing.T) {
	t.Parallel()

	rootConfig := filepath.Join("/workspace", ".devcontainer.json")
	if got := FeatureLockFilePath(rootConfig); got != filepath.Join("/workspace", ".devcontainer-lock.json") {
		t.Fatalf("unexpected root lockfile path %q", got)
	}
	configPath := filepath.Join("/workspace", ".devcontainer", "devcontainer.json")
	if got := FeatureLockFilePath(configPath); got != filepath.Join("/workspace", ".devcontainer", "devcontainer-lock.json") {
		t.Fatalf("unexpected nested lockfile path %q", got)
	}
}

func TestReadAndWriteFeatureLockFileRoundTripAndRemoveEmpty(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	lockPath := FeatureLockFilePath(configPath)
	lock := FeatureLockFile{
		"feature-a": {Version: "1", Resolved: "ghcr.io/example/feature-a:1", Integrity: "sha256:abc"},
	}

	if err := WriteFeatureLockFile(configPath, lock); err != nil {
		t.Fatalf("write feature lock file: %v", err)
	}
	assertMode(t, lockPath, 0o644)
	got, ok, err := ReadFeatureLockFile(configPath)
	if err != nil {
		t.Fatalf("read feature lock file: %v", err)
	}
	if !ok || !reflect.DeepEqual(got, lock) {
		t.Fatalf("unexpected feature lockfile: ok=%v got=%#v", ok, got)
	}

	if err := WriteFeatureLockFile(configPath, FeatureLockFile{}); err != nil {
		t.Fatalf("remove empty feature lock file: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected empty lockfile write to remove file, got %v", err)
	}
}

func TestReadFeatureLockFileSupportsJSONCAndEmptyFiles(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	lockPath := FeatureLockFilePath(configPath)
	jsonc := "{\n  // comment\n  \"feature\": {\"resolved\": \"ghcr.io/example/feature:1\", \"integrity\": \"sha256:def\"}\n}\n"
	if err := os.WriteFile(lockPath, []byte(jsonc), 0o644); err != nil {
		t.Fatalf("seed jsonc lockfile: %v", err)
	}
	lock, ok, err := ReadFeatureLockFile(configPath)
	if err != nil {
		t.Fatalf("read jsonc lockfile: %v", err)
	}
	if !ok || lock["feature"].Integrity != "sha256:def" {
		t.Fatalf("unexpected jsonc lockfile %#v", lock)
	}

	if err := os.WriteFile(lockPath, []byte(" \n\t"), 0o644); err != nil {
		t.Fatalf("seed empty lockfile: %v", err)
	}
	lock, ok, err = ReadFeatureLockFile(configPath)
	if err != nil {
		t.Fatalf("read empty lockfile: %v", err)
	}
	if !ok || len(lock) != 0 {
		t.Fatalf("expected empty lockfile to decode as empty map, got ok=%v lock=%#v", ok, lock)
	}
}

func TestWriteFeatureStateFileRoundTripsAndRemovesEmpty(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	path := filepath.Join(stateDir, "features-lock.json")
	state := FeatureStateFile{Features: []FeatureStateEntry{{ID: "go", Source: "ghcr.io/devcontainers/features/go:1", Kind: "oci", Path: "/tmp/go", Resolved: "ghcr.io/devcontainers/features/go@sha256:abc", Integrity: "sha256:abc", Options: map[string]string{"version": "1.24"}}}}

	if err := WriteFeatureStateFile(stateDir, state); err != nil {
		t.Fatalf("write feature state file: %v", err)
	}
	assertMode(t, path, 0o600)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read feature state file: %v", err)
	}
	if !strings.Contains(string(data), "ghcr.io/devcontainers/features/go@sha256:abc") {
		t.Fatalf("expected serialized feature state, got %q", string(data))
	}

	if err := WriteFeatureStateFile(stateDir, FeatureStateFile{}); err != nil {
		t.Fatalf("remove feature state file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected empty feature state to remove file, got %v", err)
	}
}

func TestComposeOverrideHelpersWriteRoundTrip(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	path := ComposeOverridePath(stateDir)
	if got := path; got != filepath.Join(stateDir, "docker-compose.override.yml") {
		t.Fatalf("unexpected compose override path %q", got)
	}

	written, err := WriteComposeOverride(stateDir, []byte("services:\n  app:\n    image: demo\n"))
	if err != nil {
		t.Fatalf("write compose override: %v", err)
	}
	if written != path {
		t.Fatalf("unexpected written path %q", written)
	}
	assertMode(t, path, 0o600)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read compose override: %v", err)
	}
	if !strings.Contains(string(data), "image: demo") {
		t.Fatalf("unexpected compose override contents %q", string(data))
	}
}

func TestResolvedPlanCacheReadWriteAndValidation(t *testing.T) {
	t.Parallel()

	type cached struct {
		Value string `json:"value"`
	}

	cacheDir := t.TempDir()
	if _, ok, err := ReadResolvedPlanCache[cached](cacheDir, "missing", 1, nil); err != nil || ok {
		t.Fatalf("expected missing cache, got ok=%v err=%v", ok, err)
	}
	if err := WriteResolvedPlanCache(cacheDir, "key-1", 1, cached{Value: "demo"}); err != nil {
		t.Fatalf("write plan cache: %v", err)
	}
	assertMode(t, filepath.Join(cacheDir, "resolved-plan.json"), 0o600)

	got, ok, err := ReadResolvedPlanCache[cached](cacheDir, "key-1", 1, func(value cached) error {
		if value.Value != "demo" {
			t.Fatalf("unexpected cached value %#v", value)
		}
		return nil
	})
	if err != nil || !ok || got.Value != "demo" {
		t.Fatalf("unexpected cached plan: ok=%v err=%v got=%#v", ok, err, got)
	}
	if _, ok, err := ReadResolvedPlanCache[cached](cacheDir, "wrong-key", 1, nil); err != nil || ok {
		t.Fatalf("expected key mismatch to miss cache, got ok=%v err=%v", ok, err)
	}
	if _, ok, err := ReadResolvedPlanCache[cached](cacheDir, "key-1", 2, nil); err != nil || ok {
		t.Fatalf("expected version mismatch to miss cache, got ok=%v err=%v", ok, err)
	}
	if _, _, err := ReadResolvedPlanCache[cached](cacheDir, "key-1", 1, func(cached) error { return os.ErrPermission }); !os.IsPermission(err) {
		t.Fatalf("expected validate error to bubble up, got %v", err)
	}
}

func TestWorkspacePathHelpersUseStableHashedKeys(t *testing.T) {
	t.Parallel()

	workspace := "/workspace/demo"
	configPath := "/workspace/demo/.devcontainer/devcontainer.json"
	key := hashKey(workspace + "\n" + configPath)
	if got := WorkspaceScopedDir("/state-root", workspace, configPath); got != filepath.Join("/state-root", "workspaces", key) {
		t.Fatalf("unexpected workspace scoped dir %q", got)
	}
	if got := ContainerName(workspace, configPath); got != "hatchctl-"+key {
		t.Fatalf("unexpected container name %q", got)
	}
	if got := ImageName(workspace, configPath); got != "hatchctl-"+key {
		t.Fatalf("unexpected image name %q", got)
	}
	if len(key) != 16 {
		t.Fatalf("expected short hash key, got %q", key)
	}
	if FeatureCacheDir("/cache-root") != filepath.Join("/cache-root", "features-cache") {
		t.Fatalf("unexpected feature cache dir %q", FeatureCacheDir("/cache-root"))
	}
	linuxRoots := OutputRootsForPlatform("linux", "/home/demo", "/config", "/cache", "/state")
	if linuxRoots.StateRoot != filepath.Join("/state", "hatchctl") || linuxRoots.CacheRoot != filepath.Join("/cache", "hatchctl") {
		t.Fatalf("unexpected linux roots %#v", linuxRoots)
	}
	darwinRoots := OutputRootsForPlatform("darwin", "/Users/demo", "/config", "/cache", "/ignored")
	if darwinRoots.StateRoot != filepath.Join("/config", "hatchctl") || darwinRoots.CacheRoot != filepath.Join("/cache", "hatchctl") {
		t.Fatalf("unexpected darwin roots %#v", darwinRoots)
	}
}
