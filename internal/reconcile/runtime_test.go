package reconcile

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestPlanImageReusesManagedImageWhenReuseKeyMatches(t *testing.T) {
	t.Parallel()

	plan := PlanImage(DesiredImage{TargetImage: "hatchctl-demo", BuildMode: ImageBuildModeDocker, ReuseKey: "key-1"}, &docker.ImageInspect{Config: docker.InspectConfig{Labels: map[string]string{ImageKeyLabel: "key-1"}}})
	if plan.Action != ImageActionReuseTarget {
		t.Fatalf("expected managed image reuse, got %#v", plan)
	}
}

func TestPlanImageBuildsManagedImageWhenReuseKeyDiffers(t *testing.T) {
	t.Parallel()

	plan := PlanImage(DesiredImage{TargetImage: "hatchctl-demo", BuildMode: ImageBuildModeFeatures, ReuseKey: "key-2"}, &docker.ImageInspect{Config: docker.InspectConfig{Labels: map[string]string{ImageKeyLabel: "old"}}})
	if plan.Action != ImageActionBuildTarget || plan.BuildMode != ImageBuildModeFeatures {
		t.Fatalf("expected managed image rebuild, got %#v", plan)
	}
}

func TestPlanContainerUsesContainerKeyInsteadOfAdHocFlags(t *testing.T) {
	t.Parallel()

	observed := ObservedState{
		Target: RuntimeTarget{PrimaryContainer: "container-123"},
		Container: &docker.ContainerInspect{
			ID:    "container-123",
			State: docker.ContainerState{Running: true},
			Config: docker.InspectConfig{Labels: map[string]string{
				ContainerKeyLabel: "desired-key",
			}},
		},
	}
	plan, err := PlanContainer(observed, DesiredContainer{ReuseKey: "desired-key"})
	if err != nil {
		t.Fatalf("plan container: %v", err)
	}
	if plan.Action != ContainerActionReuse || !plan.Reused {
		t.Fatalf("expected container reuse, got %#v", plan)
	}

	plan, err = PlanContainer(observed, DesiredContainer{ReuseKey: "new-key"})
	if err != nil {
		t.Fatalf("plan container: %v", err)
	}
	if plan.Action != ContainerActionReplace || !plan.NeedsCleanup {
		t.Fatalf("expected container replacement, got %#v", plan)
	}
}

func TestPlanUpLifecycleUsesPersistedKeyAndTransitionState(t *testing.T) {
	t.Parallel()

	observed := ObservedState{Control: ControlState{WorkspaceState: devcontainer.State{LifecycleReady: true, LifecycleKey: "lifecycle-key"}}}
	plan := PlanUpLifecycle(observed, DesiredLifecycle{Key: "lifecycle-key"})
	if plan.RunCreate {
		t.Fatalf("expected create lifecycle to be skipped, got %#v", plan)
	}
	if !plan.RunStart || !plan.RunAttach {
		t.Fatalf("expected start/attach lifecycle to remain enabled, got %#v", plan)
	}

	observed.Control.WorkspaceState.LifecycleTransition = &devcontainer.StateTransition{Kind: "all", Key: "old"}
	plan = PlanUpLifecycle(observed, DesiredLifecycle{Key: "lifecycle-key"})
	if !plan.RunCreate || !plan.NeedsRecovery {
		t.Fatalf("expected pending lifecycle transition to force recovery, got %#v", plan)
	}
}
