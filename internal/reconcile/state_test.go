package reconcile

import (
	"testing"

	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestStateTrackerPersistsLifecycleBridgeAndDotfilesState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	tracker := NewStateTracker(stateDir, storefs.WorkspaceState{})
	plan := LifecyclePlan{RunCreate: true, RunStart: true, RunAttach: true, Key: "lifecycle-key", TransitionKind: LifecyclePhaseAll}
	dotfiles := DotfilesConfig{Repository: "github.com/example/dotfiles", InstallCommand: "install.sh", TargetPath: "$HOME/.dotfiles"}
	if err := tracker.persistUpdate(func() {
		tracker.ApplyContainer("container-123", "container-key", true)
	}); err != nil {
		t.Fatalf("persist container state: %v", err)
	}
	if err := tracker.persistUpdate(func() {
		tracker.BeginBridge("start", "container-key")
	}); err != nil {
		t.Fatalf("persist bridge transition: %v", err)
	}
	if err := tracker.persistUpdate(func() {
		tracker.BeginPlannedLifecycle(plan, true)
	}); err != nil {
		t.Fatalf("persist lifecycle transition: %v", err)
	}
	if err := tracker.persistUpdate(func() {
		tracker.CompletePlannedLifecycle(plan, dotfiles, true)
	}); err != nil {
		t.Fatalf("persist lifecycle completion: %v", err)
	}
	if err := tracker.persistUpdate(func() {
		tracker.EnableBridge("bridge-session")
	}); err != nil {
		t.Fatalf("persist bridge session: %v", err)
	}
	if err := tracker.persistUpdate(func() {
		tracker.SetTrustedRefs([]string{"ghcr.io/example/feature@sha256:abc"})
	}); err != nil {
		t.Fatalf("persist trusted refs: %v", err)
	}

	loaded, err := LoadStateTracker(stateDir)
	if err != nil {
		t.Fatalf("load tracker: %v", err)
	}
	state := loaded.State()
	if loaded.stateDir != stateDir {
		t.Fatalf("unexpected loaded tracker state dir %q", loaded.stateDir)
	}
	if state.ContainerID != "container-123" || state.ContainerKey != "container-key" {
		t.Fatalf("unexpected container state %#v", state)
	}
	if !state.LifecycleReady || state.LifecycleKey != "lifecycle-key" || state.LifecycleTransition != nil {
		t.Fatalf("unexpected lifecycle state %#v", state)
	}
	if !state.BridgeEnabled || state.BridgeSessionID != "bridge-session" || state.BridgeTransition != nil {
		t.Fatalf("unexpected bridge state %#v", state)
	}
	if !state.DotfilesReady || state.DotfilesRepo != "github.com/example/dotfiles" || state.DotfilesInstall != "install.sh" || state.DotfilesTarget != "$HOME/.dotfiles" || state.DotfilesTransition != nil {
		t.Fatalf("unexpected dotfiles state %#v", state)
	}
	if len(state.TrustedRefs) != 1 || state.TrustedRefs[0] != "ghcr.io/example/feature@sha256:abc" {
		t.Fatalf("unexpected trusted refs %#v", state.TrustedRefs)
	}
}

func TestStateTrackerBeginPlannedLifecycleSkipsDotfilesWithoutCreate(t *testing.T) {
	t.Parallel()

	tracker := NewStateTracker(t.TempDir(), storefs.WorkspaceState{})
	tracker.BeginPlannedLifecycle(LifecyclePlan{RunStart: true, Key: "lifecycle-key"}, true)
	state := tracker.State()
	if state.LifecycleTransition == nil || state.LifecycleTransition.Key != "lifecycle-key" {
		t.Fatalf("unexpected lifecycle transition %#v", state)
	}
	if state.DotfilesTransition != nil {
		t.Fatalf("expected dotfiles transition to remain unset, got %#v", state)
	}
}

func TestStateTrackerApplyContainerAndBridgeHelpers(t *testing.T) {
	t.Parallel()

	tracker := NewStateTracker(t.TempDir(), storefs.WorkspaceState{})
	tracker.ApplyContainer("container-123", "container-key", true)
	state := tracker.State()
	if state.ContainerID != "container-123" || state.ContainerKey != "container-key" {
		t.Fatalf("unexpected created container state %#v", state)
	}
	if state.LifecycleReady || state.BridgeEnabled {
		t.Fatalf("expected create path to reset lifecycle and bridge state, got %#v", state)
	}

	tracker.ApplyContainer("container-456", "next-key", false)
	tracker.EnableBridge("bridge-session")
	state = tracker.State()
	if state.ContainerID != "container-456" || state.ContainerKey != "next-key" {
		t.Fatalf("unexpected reused container state %#v", state)
	}
	if !state.BridgeEnabled || state.BridgeSessionID != "bridge-session" {
		t.Fatalf("unexpected bridge state %#v", state)
	}

	tracker.DisableBridge()
	state = tracker.State()
	if state.BridgeEnabled || state.BridgeSessionID != "" {
		t.Fatalf("expected bridge to be cleared, got %#v", state)
	}
}
