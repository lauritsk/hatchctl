package runtime

import (
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type runtimeBridgeManager struct{}

func (runtimeBridgeManager) Apply(resolved *devcontainer.ResolvedConfig, enabled bool, helperArch string) (*bridge.Session, error) {
	if !enabled {
		return nil, nil
	}
	session, err := (bridge.Planner{}).Prepare(resolved.StateDir, enabled, helperArch)
	if err != nil {
		return nil, err
	}
	resolved.Merged = (bridge.Planner{}).Inject(session, resolved.Merged)
	return session, nil
}

func (runtimeBridgeManager) Preview(resolved *devcontainer.ResolvedConfig, enabled bool) (*bridge.Session, error) {
	session, err := (bridge.Planner{}).Preview(resolved.StateDir, enabled)
	if err != nil {
		return nil, err
	}
	resolved.Merged = (bridge.Planner{}).Inject(session, resolved.Merged)
	return session, nil
}

func (runtimeBridgeManager) Start(session *bridge.Session, containerID string) (*bridge.Session, error) {
	return (bridge.Runtime{}).Start(session, containerID)
}

func (runtimeBridgeManager) Doctor(stateDir string) (bridge.Report, error) {
	return bridge.Doctor(stateDir)
}
