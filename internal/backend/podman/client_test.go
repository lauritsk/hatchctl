package podman

import "testing"

func TestNewConfiguresPodmanDefaults(t *testing.T) {
	t.Parallel()

	client := New("")
	if client.ID() != "podman" {
		t.Fatalf("unexpected client id %q", client.ID())
	}
	if client.BridgeHost() != "host.containers.internal" {
		t.Fatalf("unexpected bridge host %q", client.BridgeHost())
	}
	if client.BuildDefinitionFileName() != "Containerfile" {
		t.Fatalf("unexpected build definition file %q", client.BuildDefinitionFileName())
	}
}
