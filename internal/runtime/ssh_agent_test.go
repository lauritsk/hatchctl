package runtime

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestInjectSSHAgentAddsMountAndEnv(t *testing.T) {
	socketDir, err := os.MkdirTemp("", "hc-ssh-")
	if err != nil {
		t.Fatalf("create temp socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "agent.sock")
	listener := newUnixSocketListener(t, socketPath)
	defer listener.Close()
	t.Setenv("SSH_AUTH_SOCK", socketPath)

	merged, err := injectSSHAgent(devcontainer.MergedConfig{})
	if err != nil {
		t.Fatalf("inject ssh agent: %v", err)
	}
	if merged.RemoteEnv["SSH_AUTH_SOCK"] != sshAgentContainerSocketPath {
		t.Fatalf("unexpected remote env %#v", merged.RemoteEnv)
	}
	if merged.ContainerEnv["SSH_AUTH_SOCK"] != sshAgentContainerSocketPath {
		t.Fatalf("unexpected container env %#v", merged.ContainerEnv)
	}
	resolvedSocketPath, err := filepath.EvalSymlinks(socketPath)
	if err != nil {
		t.Fatalf("resolve socket path: %v", err)
	}
	wantMount := "type=bind,source=" + resolvedSocketPath + ",target=" + sshAgentContainerSocketPath
	if len(merged.Mounts) != 1 || merged.Mounts[0] != wantMount {
		t.Fatalf("unexpected mounts %#v", merged.Mounts)
	}
}

func TestInjectSSHAgentRejectsMissingSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing.sock"))

	_, err := injectSSHAgent(devcontainer.MergedConfig{})
	if err == nil || !strings.Contains(err.Error(), "ssh-agent passthrough requires SSH_AUTH_SOCK") {
		t.Fatalf("expected ssh-agent error, got %v", err)
	}
}

func TestInjectSSHAgentResolvesSymlinkedSocketPath(t *testing.T) {
	socketDir, err := os.MkdirTemp("/tmp", "hc-ssh-")
	if err != nil {
		t.Fatalf("create temp socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "agent.sock")
	listener := newUnixSocketListener(t, socketPath)
	defer listener.Close()

	linkDir, err := os.MkdirTemp("/tmp", "hc-ssh-link-")
	if err != nil {
		t.Fatalf("create temp symlink dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(linkDir) })
	linkPath := filepath.Join(linkDir, "agent-link.sock")
	if err := os.Symlink(socketPath, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	t.Setenv("SSH_AUTH_SOCK", linkPath)

	merged, err := injectSSHAgent(devcontainer.MergedConfig{})
	if err != nil {
		t.Fatalf("inject ssh agent through symlink: %v", err)
	}
	resolvedSocketPath, err := filepath.EvalSymlinks(socketPath)
	if err != nil {
		t.Fatalf("resolve socket path: %v", err)
	}
	wantMount := "type=bind,source=" + resolvedSocketPath + ",target=" + sshAgentContainerSocketPath
	if len(merged.Mounts) != 1 || merged.Mounts[0] != wantMount {
		t.Fatalf("unexpected mounts %#v", merged.Mounts)
	}
}

func TestEnsureContainerHasSSHAgentAcceptsLabelOrMount(t *testing.T) {
	t.Parallel()

	inspect := &docker.ContainerInspect{Config: docker.InspectConfig{Labels: map[string]string{devcontainer.SSHAgentLabel: "true"}}}
	if err := ensureContainerHasSSHAgent(inspect, sshAgentContainerSocketPath); err != nil {
		t.Fatalf("expected label to satisfy ssh-agent check, got %v", err)
	}

	inspect = &docker.ContainerInspect{Mounts: []docker.ContainerMount{{Destination: sshAgentContainerSocketPath}}}
	if err := ensureContainerHasSSHAgent(inspect, sshAgentContainerSocketPath); err != nil {
		t.Fatalf("expected mount to satisfy ssh-agent check, got %v", err)
	}
}

func TestEnsureContainerHasSSHAgentRejectsMissingMount(t *testing.T) {
	t.Parallel()

	err := ensureContainerHasSSHAgent(&docker.ContainerInspect{}, sshAgentContainerSocketPath)
	if err == nil || !strings.Contains(err.Error(), "rerun 'hatchctl up --ssh --recreate'") {
		t.Fatalf("expected recreate guidance, got %v", err)
	}
}

func newUnixSocketListener(t *testing.T, path string) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return listener
}
