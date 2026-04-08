package runtime

import (
	"errors"
	"fmt"
	"os"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

const sshAgentContainerSocketPath = "/tmp/hatchctl-ssh-agent.sock"

var errSSHAgentUnavailable = errors.New("ssh-agent passthrough requires SSH_AUTH_SOCK to point to a readable host socket")

func injectSSHAgent(merged devcontainer.MergedConfig) (devcontainer.MergedConfig, error) {
	hostSocket := os.Getenv("SSH_AUTH_SOCK")
	if hostSocket == "" {
		return merged, errSSHAgentUnavailable
	}
	info, err := os.Stat(hostSocket)
	if err != nil {
		return merged, fmt.Errorf("%w: %s", errSSHAgentUnavailable, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return merged, fmt.Errorf("%w: %q is not a socket", errSSHAgentUnavailable, hostSocket)
	}
	containerEnv := cloneStringMap(merged.ContainerEnv)
	remoteEnv := cloneStringMap(merged.RemoteEnv)
	mount := fmt.Sprintf("type=bind,source=%s,target=%s", hostSocket, sshAgentContainerSocketPath)
	containerEnv["SSH_AUTH_SOCK"] = sshAgentContainerSocketPath
	remoteEnv["SSH_AUTH_SOCK"] = sshAgentContainerSocketPath
	merged.ContainerEnv = containerEnv
	merged.RemoteEnv = remoteEnv
	merged.Mounts = appendMount(merged.Mounts, mount)
	return merged, nil
}

func ensureContainerHasSSHAgent(inspect *docker.ContainerInspect, target string) error {
	if inspect == nil {
		return fmt.Errorf("managed container does not have ssh-agent passthrough; rerun 'hatchctl up --ssh --recreate'")
	}
	if inspect.Config.Labels[devcontainer.SSHAgentLabel] == "true" || containerHasMountTarget(*inspect, target) {
		return nil
	}
	return fmt.Errorf("managed container does not have ssh-agent passthrough; rerun 'hatchctl up --ssh --recreate'")
}

func containerHasMountTarget(inspect docker.ContainerInspect, target string) bool {
	for _, mount := range inspect.Mounts {
		if mount.Destination == target {
			return true
		}
	}
	return false
}

func cloneStringMap(values map[string]string) map[string]string {
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
