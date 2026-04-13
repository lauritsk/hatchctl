package reconcile

import (
	"slices"

	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type StateTracker struct {
	stateDir string
	state    storefs.WorkspaceState
}

func LoadStateTracker(stateDir string) (*StateTracker, error) {
	if stateDir == "" {
		return NewStateTracker("", storefs.WorkspaceState{}), nil
	}
	state, err := storefs.ReadWorkspaceState(stateDir)
	if err != nil {
		return nil, err
	}
	return NewStateTracker(stateDir, state), nil
}

func NewStateTracker(stateDir string, state storefs.WorkspaceState) *StateTracker {
	return &StateTracker{stateDir: stateDir, state: state}
}

func (t *StateTracker) State() storefs.WorkspaceState {
	return t.state
}

func (t *StateTracker) Persist() error {
	return storefs.WriteWorkspaceState(t.stateDir, t.state)
}

func (t *StateTracker) BeginContainer(containerID string, containerKey string) {
	t.state.ContainerID = containerID
	t.state.ContainerKey = containerKey
	t.state.LifecycleReady = false
	t.state.LifecycleKey = ""
	t.state.LifecycleTransition = nil
	t.state.BridgeEnabled = false
	t.state.BridgeSessionID = ""
	t.state.BridgeTransition = nil
	t.setDotfiles(DotfilesConfig{})
}

func (t *StateTracker) ApplyContainer(containerID string, containerKey string, created bool) {
	if created {
		t.BeginContainer(containerID, containerKey)
		return
	}
	t.SetContainer(containerID, containerKey)
}

func (t *StateTracker) ApplyContainerAndPersist(containerID string, containerKey string, created bool) error {
	t.ApplyContainer(containerID, containerKey, created)
	return t.Persist()
}

func (t *StateTracker) SetContainer(containerID string, containerKey string) {
	t.state.ContainerID = containerID
	t.state.ContainerKey = containerKey
}

func (t *StateTracker) SetTrustedRefs(refs []string) {
	t.state.TrustedRefs = slices.Clone(refs)
}

func (t *StateTracker) SetTrustedRefsAndPersist(refs []string) error {
	t.SetTrustedRefs(refs)
	return t.Persist()
}

func (t *StateTracker) SetBridge(enabled bool, sessionID string) {
	t.state.BridgeEnabled = enabled
	t.state.BridgeSessionID = sessionID
	t.state.BridgeTransition = nil
}

func (t *StateTracker) EnableBridge(sessionID string) {
	t.SetBridge(true, sessionID)
}

func (t *StateTracker) EnableBridgeAndPersist(sessionID string) error {
	t.EnableBridge(sessionID)
	return t.Persist()
}

func (t *StateTracker) DisableBridge() {
	t.SetBridge(false, "")
}

func (t *StateTracker) BeginLifecycle(kind LifecyclePhase, key string) {
	t.state.LifecycleTransition = &storefs.StateTransition{Kind: string(kind), Key: key}
	t.state.LifecycleReady = false
	t.state.LifecycleKey = ""
}

func (t *StateTracker) BeginPlannedLifecycle(plan LifecyclePlan, installDotfiles bool) {
	t.BeginLifecycle(plan.TransitionKind, plan.Key)
	if plan.RunCreate && installDotfiles {
		t.BeginDotfiles("install", plan.Key)
	}
}

func (t *StateTracker) BeginPlannedLifecycleAndPersist(plan LifecyclePlan, installDotfiles bool) error {
	t.BeginPlannedLifecycle(plan, installDotfiles)
	return t.Persist()
}

func (t *StateTracker) CompleteLifecycle(key string, dotfiles DotfilesConfig) {
	t.state.LifecycleTransition = nil
	t.state.LifecycleReady = true
	t.state.LifecycleKey = key
	t.setDotfiles(dotfiles)
}

func (t *StateTracker) CompletePlannedLifecycle(plan LifecyclePlan, dotfiles DotfilesConfig, installDotfiles bool) {
	if plan.RunCreate && installDotfiles {
		t.CompleteDotfiles(dotfiles)
	}
	t.CompleteLifecycle(plan.Key, dotfiles)
}

func (t *StateTracker) CompletePlannedLifecycleAndPersist(plan LifecyclePlan, dotfiles DotfilesConfig, installDotfiles bool) error {
	t.CompletePlannedLifecycle(plan, dotfiles, installDotfiles)
	return t.Persist()
}

func (t *StateTracker) BeginBridge(kind string, key string) {
	t.state.BridgeTransition = &storefs.StateTransition{Kind: kind, Key: key}
	t.state.BridgeEnabled = false
	t.state.BridgeSessionID = ""
}

func (t *StateTracker) BeginBridgeAndPersist(kind string, key string) error {
	t.BeginBridge(kind, key)
	return t.Persist()
}

func (t *StateTracker) BeginDotfiles(kind string, key string) {
	t.state.DotfilesTransition = &storefs.StateTransition{Kind: kind, Key: key}
	t.state.DotfilesReady = false
}

func (t *StateTracker) CompleteDotfiles(dotfiles DotfilesConfig) {
	t.state.DotfilesTransition = nil
	t.setDotfiles(dotfiles)
}

func (t *StateTracker) setDotfiles(dotfiles DotfilesConfig) {
	t.state.DotfilesRepo = dotfiles.Repository
	t.state.DotfilesTransition = nil
	t.state.DotfilesReady = dotfiles.Enabled()
	if !t.state.DotfilesReady {
		t.state.DotfilesInstall = ""
		t.state.DotfilesTarget = ""
		return
	}
	t.state.DotfilesInstall = dotfiles.InstallCommand
	t.state.DotfilesTarget = dotfiles.TargetPath
}
