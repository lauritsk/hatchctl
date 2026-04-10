package reconcile

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestStateTrackerPersistsLifecycleBridgeAndDotfilesState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	tracker := NewStateTracker(stateDir, devcontainer.State{})
	plan := LifecyclePlan{RunCreate: true, RunStart: true, RunAttach: true, Key: "lifecycle-key", TransitionKind: LifecyclePhaseAll}
	dotfiles := DotfilesConfig{Repository: "github.com/example/dotfiles", InstallCommand: "install.sh", TargetPath: "$HOME/.dotfiles"}
	tracker.BeginContainer("container-123", "container-key")
	tracker.BeginBridge("start", "container-key")
	tracker.BeginPlannedLifecycle(plan, true)
	tracker.CompletePlannedLifecycle(plan, dotfiles, true)
	tracker.SetBridge(true, "bridge-session")
	tracker.SetTrustedRefs([]string{"ghcr.io/example/feature@sha256:abc"})
	if err := tracker.Persist(); err != nil {
		t.Fatalf("persist state: %v", err)
	}

	state, err := devcontainer.ReadState(stateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
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

	tracker := NewStateTracker(t.TempDir(), devcontainer.State{})
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

	tracker := NewStateTracker(t.TempDir(), devcontainer.State{})
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
