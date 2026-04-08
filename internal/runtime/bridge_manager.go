package runtime

import (
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type runtimeBridgeManager struct{}

func (runtimeBridgeManager) Apply(resolved *devcontainer.ResolvedConfig, enabled bool, helperArch string) (*bridge.Report, error) {
	report, merged, err := bridge.Apply(resolved.StateDir, enabled, helperArch, resolved.Merged)
	if err != nil {
		return nil, err
	}
	resolved.Merged = merged
	return (*bridge.Report)(report), nil
}

func (runtimeBridgeManager) Preview(resolved *devcontainer.ResolvedConfig, enabled bool) (*bridge.Report, error) {
	report, merged, err := bridge.Preview(resolved.StateDir, enabled, resolved.Merged)
	if err != nil {
		return nil, err
	}
	resolved.Merged = merged
	return (*bridge.Report)(report), nil
}

func (runtimeBridgeManager) Start(stateDir string, enabled bool, helperArch string, containerID string) (*bridge.Session, error) {
	return bridge.Start(stateDir, enabled, helperArch, containerID)
}

func (runtimeBridgeManager) Doctor(stateDir string) (bridge.Report, error) {
	return bridge.Doctor(stateDir)
}
