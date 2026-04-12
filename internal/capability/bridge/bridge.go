package bridgecap

import (
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func Prepare(stateDir string, helperArch string) (*bridge.Session, error) {
	return bridge.Prepare(stateDir, true, helperArch)
}

func Preview(stateDir string, enabled bool) (*bridge.Session, error) {
	return bridge.Preview(stateDir, enabled)
}

func Inject(session *bridge.Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig {
	return bridge.Inject(session, merged)
}

func Start(session *bridge.Session, containerID string) (*bridge.Session, error) {
	return bridge.Start(session, containerID)
}

func Doctor(stateDir string) (bridge.Report, error) {
	return bridge.Doctor(stateDir)
}

func EnabledFromInspect(inspect *docker.ContainerInspect, state storefs.WorkspaceState) bool {
	if inspect != nil && inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true" {
		return true
	}
	return state.BridgeEnabled
}
