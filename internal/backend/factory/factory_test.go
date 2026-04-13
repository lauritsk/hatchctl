package factory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
)

func TestNormalizeNameDefaultsToDocker(t *testing.T) {
	t.Parallel()

	if got := NormalizeName(""); got != "docker" {
		t.Fatalf("expected empty backend to default to docker, got %q", got)
	}
	if got := NormalizeName(" docker "); got != "docker" {
		t.Fatalf("expected whitespace to be trimmed, got %q", got)
	}
	if got := NormalizeName(" AUTO "); got != "auto" {
		t.Fatalf("expected auto backend to normalize, got %q", got)
	}
}

func TestNewReturnsUnsupportedBackendError(t *testing.T) {
	t.Parallel()

	_, err := New("nerdctl")
	var unsupported backend.UnsupportedBackendError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected unsupported backend error, got %v", err)
	}
	if unsupported.Name != "nerdctl" {
		t.Fatalf("unexpected backend name %q", unsupported.Name)
	}
}

func TestNewSupportsPodman(t *testing.T) {
	t.Parallel()

	client, err := New("podman")
	if err != nil {
		t.Fatalf("new podman client: %v", err)
	}
	if client.ID() != "podman" {
		t.Fatalf("unexpected client id %q", client.ID())
	}
	if client.BridgeHost() != "host.containers.internal" {
		t.Fatalf("unexpected podman bridge host %q", client.BridgeHost())
	}
}

func TestDetectNamePrefersDockerThenPodman(t *testing.T) {
	binDir := t.TempDir()
	writeBackendBinary(t, binDir, "docker")
	writeBackendBinary(t, binDir, "podman")
	t.Setenv("PATH", binDir)

	name, err := DetectName()
	if err != nil {
		t.Fatalf("detect backend: %v", err)
	}
	if name != "docker" {
		t.Fatalf("expected docker preference, got %q", name)
	}
}

func TestDetectNameReturnsPodmanWhenDockerMissing(t *testing.T) {
	binDir := t.TempDir()
	writeBackendBinary(t, binDir, "podman")
	t.Setenv("PATH", binDir)

	name, err := DetectName()
	if err != nil {
		t.Fatalf("detect backend: %v", err)
	}
	if name != "podman" {
		t.Fatalf("expected podman fallback, got %q", name)
	}
}

func TestNewAutoReturnsDetectedBackend(t *testing.T) {
	binDir := t.TempDir()
	writeBackendBinary(t, binDir, "podman")
	t.Setenv("PATH", binDir)

	client, err := New("auto")
	if err != nil {
		t.Fatalf("new auto backend: %v", err)
	}
	if client.ID() != "podman" {
		t.Fatalf("expected detected podman backend, got %q", client.ID())
	}
}

func TestDetectNameReturnsErrorWhenNoBackendExists(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := DetectName()
	if err == nil || !strings.Contains(err.Error(), "no supported backend") {
		t.Fatalf("expected auto detect error, got %v", err)
	}
}

func writeBackendBinary(t *testing.T, dir string, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write backend binary %s: %v", name, err)
	}
}
