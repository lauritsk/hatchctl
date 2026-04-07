package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestWriteFeatureBuildContextUsesOwnerOnlyGeneratedFiles(t *testing.T) {
	buildDir := t.TempDir()
	featureDir := filepath.Join(t.TempDir(), "feature-a")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatalf("mkdir feature dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write install script: %v", err)
	}

	features := []devcontainer.ResolvedFeature{{
		Path:    featureDir,
		Options: map[string]string{"SECRET_TOKEN": "top-secret"},
	}}
	if err := writeFeatureBuildContext(buildDir, features, "root", "vscode", nil); err != nil {
		t.Fatalf("write feature build context: %v", err)
	}

	for _, path := range []string{
		filepath.Join(buildDir, "devcontainer-features.builtin.env"),
		filepath.Join(buildDir, "Dockerfile"),
		filepath.Join(buildDir, "feature-00", "devcontainer-features.env"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected owner-only permissions for %s, got %#o", path, got)
		}
	}
}
