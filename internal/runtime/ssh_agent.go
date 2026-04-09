package runtime

import (
	"os"
	"runtime"

	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

const (
	sshAgentContainerSocketPath          = capssh.ContainerSocketPath
	sshAgentDarwinHostServicesSocketPath = capssh.DarwinHostServicesSocketPath
)

var errSSHAgentUnavailable = capssh.ErrUnavailable

func injectSSHAgent(merged devcontainer.MergedConfig) (devcontainer.MergedConfig, error) {
	return capssh.Inject(runtime.GOOS, os.Getenv("SSH_AUTH_SOCK"), merged)
}

func sshAgentMountSource(goos string, hostSocket string) (string, error) {
	return capssh.MountSource(goos, hostSocket)
}

func ensureContainerHasSSHAgent(inspect *docker.ContainerInspect, target string) error {
	_ = target
	return capssh.EnsureAttached(inspect)
}

func containerHasMountTarget(inspect docker.ContainerInspect, target string) bool {
	return capssh.HasTargetMount(inspect, target)
}
