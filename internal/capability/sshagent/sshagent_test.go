package sshagent

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestInjectAddsSocketMountAndEnvironment(t *testing.T) {
	t.Parallel()

	merged, err := Inject("darwin", "", spec.MergedConfig{})
	if err != nil {
		t.Fatalf("inject ssh-agent: %v", err)
	}
	if merged.ContainerEnv["SSH_AUTH_SOCK"] != ContainerSocketPath || merged.RemoteEnv["SSH_AUTH_SOCK"] != ContainerSocketPath {
		t.Fatalf("expected ssh auth socket env, got %#v %#v", merged.ContainerEnv, merged.RemoteEnv)
	}
	if len(merged.Mounts) != 1 || merged.Mounts[0] != "type=bind,source="+DarwinHostServicesSocketPath+",target="+ContainerSocketPath {
		t.Fatalf("unexpected mounts %#v", merged.Mounts)
	}
}

func TestMountSourceRequiresReadableSocket(t *testing.T) {
	t.Parallel()

	file, err := os.CreateTemp("/tmp", "hatchctl-agent-*.sock")
	if err != nil {
		t.Fatalf("create temp socket path: %v", err)
	}
	socketPath := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	if err := os.Remove(socketPath); err != nil {
		t.Fatalf("remove temp file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on unix socket: %v", err)
	}
	defer listener.Close()

	resolved, err := MountSource("linux", socketPath)
	if err != nil {
		t.Fatalf("mount source: %v", err)
	}
	if resolved == "" {
		t.Fatal("expected resolved socket path")
	}
	if _, err := MountSource("linux", filepath.Join(t.TempDir(), "missing.sock")); err == nil {
		t.Fatal("expected missing socket to be rejected")
	}
}

func TestEnsureAttachedAcceptsLabelOrMount(t *testing.T) {
	t.Parallel()

	inspect := &backend.ContainerInspect{Config: backend.InspectConfig{Labels: map[string]string{devcontainer.SSHAgentLabel: "true"}}}
	if err := EnsureAttached(inspect); err != nil {
		t.Fatalf("expected labeled container to pass: %v", err)
	}
	inspect = &backend.ContainerInspect{Mounts: []backend.ContainerMount{{Destination: ContainerSocketPath}}}
	if err := EnsureAttached(inspect); err != nil {
		t.Fatalf("expected mounted container to pass: %v", err)
	}
	if err := EnsureAttached(&backend.ContainerInspect{}); err == nil {
		t.Fatal("expected missing ssh-agent passthrough to fail")
	}
}
