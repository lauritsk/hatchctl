package factory

import (
	"errors"
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
