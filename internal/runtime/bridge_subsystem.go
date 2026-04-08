package runtime

import (
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type bridgePlanner interface {
	Prepare(stateDir string, enabled bool, helperArch string) (*bridge.Session, error)
	Load(stateDir string) (*bridge.Session, error)
}

type bridgeConfigInjector interface {
	Inject(session *bridge.Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig
}

type bridgeStatePersister interface {
	Persist(state devcontainer.State, enabled bool, report *bridge.Report) devcontainer.State
}

type bridgeRuntime interface {
	Start(stateDir string, enabled bool, helperArch string, containerID string) (*bridge.Session, error)
	Doctor(stateDir string) (bridge.Report, error)
}

type bridgeSubsystem interface {
	Prepare(*devcontainer.ResolvedConfig, bool, string) (*bridge.Report, error)
	Preview(*devcontainer.ResolvedConfig, bool) (*bridge.Report, error)
	Activate(stateDir string, containerID string, helperArch string, prepared *bridge.Report) (*bridge.Report, error)
	Persist(devcontainer.State, bool, *bridge.Report) devcontainer.State
	Doctor(stateDir string) (bridge.Report, error)
}

type runtimeBridgeSubsystem struct {
	planner  bridgePlanner
	injector bridgeConfigInjector
	state    bridgeStatePersister
	runtime  bridgeRuntime
}

func newRuntimeBridgeSubsystem() bridgeSubsystem {
	return runtimeBridgeSubsystem{
		planner:  bridgePlannerAdapter{},
		injector: bridgeConfigInjectorAdapter{},
		state:    bridgeStatePersisterAdapter{},
		runtime:  bridgeRuntimeAdapter{},
	}
}

func (s runtimeBridgeSubsystem) Prepare(resolved *devcontainer.ResolvedConfig, enabled bool, helperArch string) (*bridge.Report, error) {
	if !enabled {
		return nil, nil
	}
	session, err := s.planner.Prepare(resolved.StateDir, enabled, helperArch)
	if err != nil {
		return nil, err
	}
	resolved.Merged = s.injector.Inject(session, resolved.Merged)
	return (*bridge.Report)(session), nil
}

func (s runtimeBridgeSubsystem) Preview(resolved *devcontainer.ResolvedConfig, enabled bool) (*bridge.Report, error) {
	if !enabled {
		return nil, nil
	}
	session, err := s.planner.Load(resolved.StateDir)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	resolved.Merged = s.injector.Inject(session, resolved.Merged)
	return (*bridge.Report)(session), nil
}

func (s runtimeBridgeSubsystem) Activate(stateDir string, containerID string, helperArch string, prepared *bridge.Report) (*bridge.Report, error) {
	if prepared == nil {
		return nil, nil
	}
	session, err := s.runtime.Start(stateDir, true, helperArch, containerID)
	if err != nil {
		return nil, err
	}
	return (*bridge.Report)(session), nil
}

func (s runtimeBridgeSubsystem) Persist(state devcontainer.State, enabled bool, report *bridge.Report) devcontainer.State {
	return s.state.Persist(state, enabled, report)
}

func (s runtimeBridgeSubsystem) Doctor(stateDir string) (bridge.Report, error) {
	return s.runtime.Doctor(stateDir)
}

type bridgePlannerAdapter struct{}

func (bridgePlannerAdapter) Prepare(stateDir string, enabled bool, helperArch string) (*bridge.Session, error) {
	return bridge.Prepare(stateDir, enabled, helperArch)
}

func (bridgePlannerAdapter) Load(stateDir string) (*bridge.Session, error) {
	return bridge.Load(stateDir)
}

type bridgeConfigInjectorAdapter struct{}

func (bridgeConfigInjectorAdapter) Inject(session *bridge.Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig {
	return bridge.Inject(session, merged)
}

type bridgeStatePersisterAdapter struct{}

func (bridgeStatePersisterAdapter) Persist(state devcontainer.State, enabled bool, report *bridge.Report) devcontainer.State {
	state.BridgeEnabled = enabled
	state.BridgeSessionID = ""
	if report != nil {
		state.BridgeSessionID = report.ID
	}
	return state
}

type bridgeRuntimeAdapter struct{}

func (bridgeRuntimeAdapter) Start(stateDir string, enabled bool, helperArch string, containerID string) (*bridge.Session, error) {
	return bridge.Start(stateDir, enabled, helperArch, containerID)
}

func (bridgeRuntimeAdapter) Doctor(stateDir string) (bridge.Report, error) {
	return bridge.Doctor(stateDir)
}

var (
	_ bridgePlanner        = bridgePlannerAdapter{}
	_ bridgeConfigInjector = bridgeConfigInjectorAdapter{}
	_ bridgeStatePersister = bridgeStatePersisterAdapter{}
	_ bridgeRuntime        = bridgeRuntimeAdapter{}
	_ bridgeSubsystem      = runtimeBridgeSubsystem{}
)
