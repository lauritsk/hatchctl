package reconcile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/capability"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

const (
	ImageKeyLabel     = "hatchctl.reconcile.image-key"
	ContainerKeyLabel = "hatchctl.reconcile.container-key"
)

type ImageAction string

const (
	ImageActionUseTarget   ImageAction = "use-target"
	ImageActionReuseTarget ImageAction = "reuse-target"
	ImageActionBuildTarget ImageAction = "build-target"
)

type ImageBuildMode string

const (
	ImageBuildModeNone     ImageBuildMode = "none"
	ImageBuildModeBuild    ImageBuildMode = "build"
	ImageBuildModeFeatures ImageBuildMode = "features"
	ImageBuildModeProject  ImageBuildMode = "project"
)

type DesiredImage struct {
	TargetImage string
	BuildMode   ImageBuildMode
	ReuseKey    string
	Verify      bool
}

type ImagePlan struct {
	Action      ImageAction
	TargetImage string
	BuildMode   ImageBuildMode
	ReuseKey    string
	Verify      bool
}

type ContainerAction string

const (
	ContainerActionReuse   ContainerAction = "reuse"
	ContainerActionStart   ContainerAction = "start"
	ContainerActionCreate  ContainerAction = "create"
	ContainerActionReplace ContainerAction = "replace"
)

type DesiredContainer struct {
	ReuseKey string
	ForceNew bool
}

type ContainerPlan struct {
	Action       ContainerAction
	ContainerID  string
	Observed     *backend.ContainerInspect
	DesiredKey   string
	Reused       bool
	NeedsCleanup bool
}

type LifecyclePhase string

const (
	LifecyclePhaseNone   LifecyclePhase = ""
	LifecyclePhaseCreate LifecyclePhase = "create"
	LifecyclePhaseAll    LifecyclePhase = "all"
)

type DotfilesConfig = capability.Dotfiles

type DesiredLifecycle struct {
	Key       string
	Requested string
	Dotfiles  DotfilesConfig
	Created   bool
}

type LifecyclePlan struct {
	RunCreate      bool
	RunStart       bool
	RunAttach      bool
	Key            string
	TransitionKind LifecyclePhase
	NeedsRecovery  bool
}

var ErrContainerObservationMissing = errors.New("container observation is required for reconcile")

var ErrInvalidLifecyclePhase = errors.New("invalid lifecycle phase")

func PlanImage(desired DesiredImage, observed *backend.ImageInspect) ImagePlan {
	plan := ImagePlan{
		Action:      ImageActionUseTarget,
		TargetImage: desired.TargetImage,
		BuildMode:   desired.BuildMode,
		ReuseKey:    desired.ReuseKey,
		Verify:      desired.Verify,
	}
	if desired.BuildMode == ImageBuildModeNone {
		return plan
	}
	if desired.ReuseKey != "" && observed != nil && observed.Config.Labels[ImageKeyLabel] == desired.ReuseKey {
		plan.Action = ImageActionReuseTarget
		return plan
	}
	plan.Action = ImageActionBuildTarget
	plan.Verify = false
	return plan
}

func PlanContainer(observed ObservedState, desired DesiredContainer) (ContainerPlan, error) {
	plan := ContainerPlan{DesiredKey: desired.ReuseKey}
	if desired.ForceNew {
		if observed.Target.PrimaryContainer == "" {
			plan.Action = ContainerActionCreate
			return plan, nil
		}
		if observed.Container == nil {
			return ContainerPlan{}, ErrContainerObservationMissing
		}
		plan.Action = ContainerActionReplace
		plan.ContainerID = observed.Target.PrimaryContainer
		plan.Observed = observed.Container
		plan.NeedsCleanup = true
		return plan, nil
	}
	if observed.Target.PrimaryContainer == "" {
		plan.Action = ContainerActionCreate
		return plan, nil
	}
	if observed.Container == nil {
		return ContainerPlan{}, ErrContainerObservationMissing
	}
	plan.ContainerID = observed.Container.ID
	plan.Observed = observed.Container
	if observed.Container.Config.Labels[ContainerKeyLabel] != desired.ReuseKey {
		plan.Action = ContainerActionReplace
		plan.NeedsCleanup = true
		return plan, nil
	}
	if !observed.Container.State.Running {
		plan.Action = ContainerActionStart
		return plan, nil
	}
	plan.Action = ContainerActionReuse
	plan.Reused = true
	return plan, nil
}

func PlanUpLifecycle(observed ObservedState, desired DesiredLifecycle) LifecyclePlan {
	state := observed.Control.WorkspaceState
	createNeeded := desired.Created || !state.LifecycleReady || state.LifecycleKey != desired.Key || state.LifecycleTransition != nil
	return LifecyclePlan{
		RunCreate:      createNeeded,
		RunStart:       true,
		RunAttach:      true,
		Key:            desired.Key,
		TransitionKind: LifecyclePhaseAll,
		NeedsRecovery:  state.LifecycleTransition != nil,
	}
}

func NormalizeLifecyclePhase(requested string) (string, error) {
	phase := strings.ToLower(strings.TrimSpace(requested))
	if phase == "" {
		return "all", nil
	}
	switch phase {
	case "all", "create", "start", "attach":
		return phase, nil
	default:
		return "", fmt.Errorf("%w %q; expected all, create, start, or attach", ErrInvalidLifecyclePhase, requested)
	}
}

func PlanLifecycleCommand(observed ObservedState, desired DesiredLifecycle) (LifecyclePlan, error) {
	phase, err := NormalizeLifecyclePhase(desired.Requested)
	if err != nil {
		return LifecyclePlan{}, err
	}
	plan := LifecyclePlan{Key: desired.Key}
	switch phase {
	case "create":
		plan.RunCreate = true
		plan.TransitionKind = LifecyclePhaseCreate
	case "start":
		plan.RunStart = true
	case "attach":
		plan.RunAttach = true
	case "all":
		plan.RunCreate = true
		plan.RunStart = true
		plan.RunAttach = true
		plan.TransitionKind = LifecyclePhaseAll
	}
	plan.NeedsRecovery = observed.Control.WorkspaceState.LifecycleTransition != nil
	return plan, nil
}

func LifecycleStateApplied(state storefs.WorkspaceState, desiredKey string) bool {
	return state.LifecycleReady && state.LifecycleKey == desiredKey && state.LifecycleTransition == nil
}
