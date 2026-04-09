package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureBuildArtifactsUseOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	buildDir, err := ResetFeatureBuildDir(stateDir)
	if err != nil {
		t.Fatalf("reset feature build dir: %v", err)
	}
	if buildDir != filepath.Join(stateDir, "features-build") {
		t.Fatalf("unexpected build dir %q", buildDir)
	}
	assertMode(t, buildDir, 0o700)

	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if err := WriteFeatureBuildFile(dockerfilePath, []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}
	assertMode(t, dockerfilePath, 0o600)

	helperPath := filepath.Join(buildDir, "devcontainer-features-install.sh")
	if err := WriteFeatureBuildFile(helperPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write helper script: %v", err)
	}
	assertMode(t, helperPath, 0o755)
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
