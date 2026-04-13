package sshagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

const (
	ContainerSocketPath          = "/tmp/hatchctl-ssh-agent.sock"
	DarwinHostServicesSocketPath = "/run/host-services/ssh-auth.sock"
)

var ErrUnavailable = errors.New("ssh-agent passthrough requires SSH_AUTH_SOCK to point to a readable host socket")

func Inject(goos string, hostSocket string, merged spec.MergedConfig) (spec.MergedConfig, error) {
	source, err := MountSource(goos, hostSocket)
	if err != nil {
		return merged, err
	}
	containerEnv := cloneMap(merged.ContainerEnv)
	remoteEnv := cloneMap(merged.RemoteEnv)
	mount := fmt.Sprintf("type=bind,source=%s,target=%s", source, ContainerSocketPath)
	containerEnv["SSH_AUTH_SOCK"] = ContainerSocketPath
	remoteEnv["SSH_AUTH_SOCK"] = ContainerSocketPath
	merged.ContainerEnv = containerEnv
	merged.RemoteEnv = remoteEnv
	merged.Mounts = appendMount(merged.Mounts, mount)
	return merged, nil
}

func MountSource(goos string, hostSocket string) (string, error) {
	if goos == "darwin" {
		return DarwinHostServicesSocketPath, nil
	}
	if hostSocket == "" {
		return "", ErrUnavailable
	}
	resolvedSocket, err := filepath.EvalSymlinks(hostSocket)
	if err == nil && resolvedSocket != "" {
		hostSocket = resolvedSocket
	}
	info, err := os.Stat(hostSocket)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return "", fmt.Errorf("%w: %q is not a socket", ErrUnavailable, hostSocket)
	}
	return hostSocket, nil
}

func EnsureAttached(inspect *backend.ContainerInspect) error {
	if inspect == nil {
		return fmt.Errorf("managed container does not have ssh-agent passthrough; rerun 'hatchctl up --ssh --recreate'")
	}
	if inspect.Config.Labels[devcontainer.SSHAgentLabel] == "true" || HasTargetMount(*inspect, ContainerSocketPath) {
		return nil
	}
	return fmt.Errorf("managed container does not have ssh-agent passthrough; rerun 'hatchctl up --ssh --recreate'")
}

func HasTargetMount(inspect backend.ContainerInspect, target string) bool {
	for _, mount := range inspect.Mounts {
		if mount.Destination == target {
			return true
		}
	}
	return false
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func appendMount(mounts []string, mount string) []string {
	for _, existing := range mounts {
		if existing == mount {
			return mounts
		}
	}
	return append(mounts, mount)
}
