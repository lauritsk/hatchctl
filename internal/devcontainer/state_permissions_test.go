package devcontainer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteStateUsesOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(t.TempDir(), "state")
	if err := WriteState(stateDir, State{ContainerID: "container-123"}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	assertMode(t, stateDir, 0o700)
	assertMode(t, filepath.Join(stateDir, "state.json"), 0o600)
}

func TestWriteResolvedPlanCacheUsesOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := writeResolvedPlanCache(cacheDir, "cache-key", ResolvedConfig{ConfigPath: "/tmp/devcontainer.json"}); err != nil {
		t.Fatalf("write plan cache: %v", err)
	}
	assertMode(t, cacheDir, 0o700)
	assertMode(t, filepath.Join(cacheDir, "resolved-plan.json"), 0o600)
}

func TestWriteFeatureStateFileUsesOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(t.TempDir(), "state")
	if err := WriteFeatureStateFile(stateDir, []ResolvedFeature{{Metadata: MetadataEntry{ID: "feature-a"}, Source: "./feature-a"}}); err != nil {
		t.Fatalf("write feature state: %v", err)
	}
	assertMode(t, stateDir, 0o700)
	assertMode(t, filepath.Join(stateDir, "features-lock.json"), 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("unexpected mode for %s: got %o want %o", path, got, want)
	}
}
