package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestWriteFeatureBuildContextUsesOwnerOnlyGeneratedFiles(t *testing.T) {
	t.Parallel()

	buildDir := t.TempDir()
	featureDir := filepath.Join(t.TempDir(), "feature-a")
	baseImage := "mcr.microsoft.com/devcontainers/base:ubuntu"
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
	if err := writeFeatureBuildContext(buildDir, baseImage, features, "root", "vscode", nil, "image-key"); err != nil {
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

	dockerfile, err := os.ReadFile(filepath.Join(buildDir, "Dockerfile"))
	if err != nil {
		t.Fatalf("read generated Dockerfile: %v", err)
	}
	wantPrefix := "FROM " + baseImage + "\n"
	if len(dockerfile) < len(wantPrefix) || string(dockerfile[:len(wantPrefix)]) != wantPrefix {
		t.Fatalf("expected generated Dockerfile to start with %q, got %q", wantPrefix, string(dockerfile))
	}
	if strings.Contains(string(dockerfile), "ARG BASE_IMAGE") || strings.Contains(string(dockerfile), "FROM ${BASE_IMAGE}") {
		t.Fatalf("expected generated Dockerfile to inline the base image, got %q", string(dockerfile))
	}
}

func TestMergeManagedImageMetadataPreservesBaseImageEntries(t *testing.T) {
	t.Parallel()

	base := []devcontainer.MetadataEntry{{RemoteUser: "vscode"}}
	overlay := []devcontainer.MetadataEntry{{ID: "mise"}}

	merged := mergeManagedImageMetadata(base, overlay)
	if len(merged) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(merged))
	}
	if merged[0].RemoteUser != "vscode" {
		t.Fatalf("expected base metadata first, got %#v", merged[0])
	}
	if merged[1].ID != "mise" {
		t.Fatalf("expected overlay metadata last, got %#v", merged[1])
	}
	merged[0].RemoteUser = "root"
	if base[0].RemoteUser != "vscode" {
		t.Fatalf("expected base metadata to remain unchanged, got %#v", base[0])
	}
}

func TestDockerfileQuotedValueEscapesShellExpansion(t *testing.T) {
	t.Parallel()

	got := dockerfileQuotedValue(`printf %s "$HOME"`)
	if got != `"printf %s \"\$HOME\""` {
		t.Fatalf("unexpected quoted value %q", got)
	}
}
