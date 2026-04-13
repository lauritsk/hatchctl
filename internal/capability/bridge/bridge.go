package bridgecap

import (
	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func Prepare(stateDir string, helperArch string, backendID string, bridgeHosts []string) (*bridge.Session, error) {
	return bridge.Prepare(stateDir, true, helperArch, backendID, bridgeHosts)
}

func Preview(stateDir string, enabled bool) (*bridge.Session, error) {
	return bridge.Preview(stateDir, enabled)
}

func Inject(session *bridge.Session, merged spec.MergedConfig) spec.MergedConfig {
	return bridge.Inject(session, merged)
}

func Start(session *bridge.Session, containerID string) (*bridge.Session, error) {
	return bridge.Start(session, containerID)
}

func Doctor(stateDir string) (bridge.Report, error) {
	return bridge.Doctor(stateDir)
}

func EnabledFromInspect(inspect *backend.ContainerInspect, state storefs.WorkspaceState) bool {
	if inspect != nil && inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true" {
		return true
	}
	return state.BridgeEnabled
}
