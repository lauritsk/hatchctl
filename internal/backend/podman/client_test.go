package podman

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewConfiguresPodmanDefaults(t *testing.T) {
	binDir := t.TempDir()
	writePodmanScript(t, binDir, "podman", "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
	t.Setenv("PATH", binDir)

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

func TestComposeOptionsUsesPodmanComposeWhenAvailable(t *testing.T) {
	binDir := t.TempDir()
	writePodmanScript(t, binDir, "podman", "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 1; fi\nexit 0\n")
	writePodmanScript(t, binDir, "podman-compose", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	binary, command := composeOptions("podman")
	if binary != "podman-compose" {
		t.Fatalf("unexpected compose binary %q", binary)
	}
	if command != nil {
		t.Fatalf("expected external compose command, got %#v", command)
	}
}

func TestComposeOptionsPrefersNativeCompose(t *testing.T) {
	binDir := t.TempDir()
	writePodmanScript(t, binDir, "podman", "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
	writePodmanScript(t, binDir, "podman-compose", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	binary, command := composeOptions("podman")
	if binary != "" {
		t.Fatalf("expected native podman compose, got %q", binary)
	}
	if !reflect.DeepEqual(command, []string{"compose"}) {
		t.Fatalf("unexpected compose command %#v", command)
	}
}

func writePodmanScript(t *testing.T, dir string, name string, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
