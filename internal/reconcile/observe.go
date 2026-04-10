package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

var (
	ErrObservedTargetNotFound = errors.New("observed runtime target not found")
	ErrObservedStateStale     = errors.New("observed runtime target changed before action")
)

type TargetKind string

const (
	TargetKindManagedContainer TargetKind = "managed-container"
	TargetKindComposeService   TargetKind = "compose-service"
)

type RuntimeTarget struct {
	Kind             TargetKind
	Image            string
	ContainerName    string
	ComposeProject   string
	ComposeService   string
	PrimaryContainer string
	Containers       []docker.ContainerInspect
}

type ControlState struct {
	WorkspaceState  devcontainer.State
	Coordination    storefs.CoordinationRecord
	PlanCachePath   string
	PlanCacheExists bool
}

type ReadToken struct {
	TargetKind             TargetKind
	ContainerName          string
	ComposeProject         string
	ComposeService         string
	PrimaryContainer       string
	CoordinationGeneration int64
}

type ObservedState struct {
	Plan       workspaceplan.WorkspacePlan
	Resolved   devcontainer.ResolvedConfig
	Target     RuntimeTarget
	Control    ControlState
	Capability CapabilityState
	Container  *docker.ContainerInspect
	Image      *docker.ImageInspect
	ImageRef   string
	ReadTarget ReadToken
}

type CapabilityState struct {
	SSHAgentAttached bool
	BridgeEnabled    bool
	DotfilesApplied  bool
	UIDRemapDesired  bool
}

type ObserveRequest struct {
	Plan               workspaceplan.WorkspacePlan
	Resolved           devcontainer.ResolvedConfig
	ImageRef           string
	LoadControlState   bool
	ObserveTarget      bool
	InspectTarget      bool
	ObserveImage       bool
	AllowMissingTarget bool
}

type backend interface {
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
	ListContainers(context.Context, dockercli.ListContainersRequest) (string, error)
}

type Observer struct {
	backend          backend
	readState        func(string) (storefs.WorkspaceState, error)
	readCoordination func(string) (storefs.CoordinationRecord, error)
}

func NewObserver(backend backend) *Observer {
	return (&Observer{backend: backend}).withDefaults()
}

func (o *Observer) Observe(ctx context.Context, req ObserveRequest) (ObservedState, error) {
	o = o.withDefaults()
	observed := ObservedState{
		Plan:     req.Plan,
		Resolved: req.Resolved,
		ImageRef: req.ImageRef,
		Target: RuntimeTarget{
			Kind:           targetKind(req.Resolved),
			Image:          req.ImageRef,
			ContainerName:  req.Resolved.ContainerName,
			ComposeProject: req.Resolved.ComposeProject,
			ComposeService: req.Resolved.ComposeService,
		},
		Control: ControlState{
			PlanCachePath: filepath.Join(req.Resolved.CacheDir, "resolved-plan.json"),
		},
	}
	if req.LoadControlState {
		state, err := o.readState(req.Resolved.StateDir)
		if err != nil {
			return ObservedState{}, err
		}
		observed.Control.WorkspaceState = state
		coordination, err := o.readCoordination(req.Resolved.StateDir)
		if err != nil && !os.IsNotExist(err) {
			return ObservedState{}, err
		}
		observed.Control.Coordination = coordination
		if _, err := os.Stat(observed.Control.PlanCachePath); err == nil {
			observed.Control.PlanCacheExists = true
		} else if !os.IsNotExist(err) {
			return ObservedState{}, err
		}
	}
	if req.ObserveTarget {
		target, state, container, err := o.observeTarget(ctx, req.Resolved, observed.Control.WorkspaceState, req.InspectTarget, req.AllowMissingTarget)
		if err != nil {
			return ObservedState{}, err
		}
		observed.Target = target
		observed.Control.WorkspaceState = state
		observed.Container = container
		observed.Capability = observeCapabilities(container, state, req.Resolved)
	}
	if req.ObserveImage && req.ImageRef != "" {
		inspect, err := o.backend.InspectImage(ctx, req.ImageRef)
		if err != nil {
			if !docker.IsNotFound(err) {
				return ObservedState{}, err
			}
		} else {
			observed.Image = &inspect
		}
	}
	observed.ReadTarget = observed.readToken()
	return observed, nil
}

func (o *Observer) RevalidateReadToken(ctx context.Context, observed ObservedState) error {
	token := observed.ReadTarget
	if token.PrimaryContainer == "" {
		return nil
	}
	coordination, err := o.readCoordination(observed.Resolved.StateDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if coordination.ActiveOwner != nil {
		return fmt.Errorf("%w: workspace mutation is active", ErrObservedStateStale)
	}
	if coordination.Generation != token.CoordinationGeneration {
		return fmt.Errorf("%w: workspace coordination changed", ErrObservedStateStale)
	}
	inspect, err := o.backend.InspectContainer(ctx, token.PrimaryContainer)
	if err != nil {
		if docker.IsNotFound(err) {
			return fmt.Errorf("%w: container %s disappeared", ErrObservedStateStale, token.PrimaryContainer)
		}
		return err
	}
	if inspect.ID != token.PrimaryContainer {
		return fmt.Errorf("%w: target container changed", ErrObservedStateStale)
	}
	if !readTokenMatchesInspect(token, inspect) {
		return fmt.Errorf("%w: target identity changed", ErrObservedStateStale)
	}
	return nil
}

func readTokenMatchesInspect(token ReadToken, inspect docker.ContainerInspect) bool {
	switch token.TargetKind {
	case TargetKindComposeService:
		if token.ComposeProject != "" && inspect.Config.Labels["com.docker.compose.project"] != token.ComposeProject {
			return false
		}
		if token.ComposeService != "" && inspect.Config.Labels["com.docker.compose.service"] != token.ComposeService {
			return false
		}
	}
	return true
}

func (o *Observer) observeTarget(ctx context.Context, resolved devcontainer.ResolvedConfig, state devcontainer.State, inspectTarget bool, allowMissing bool) (RuntimeTarget, devcontainer.State, *docker.ContainerInspect, error) {
	previousContainerID := state.ContainerID
	var candidates []docker.ContainerInspect
	var container *docker.ContainerInspect
	if resolved.SourceKind == "compose" {
		inspects, primary, err := o.observeComposeProject(ctx, resolved)
		if err != nil {
			if allowMissing && errors.Is(err, ErrObservedTargetNotFound) {
				return RuntimeTarget{Kind: TargetKindComposeService, Image: resolved.ImageName, ContainerName: resolved.ContainerName, ComposeProject: resolved.ComposeProject, ComposeService: resolved.ComposeService}, clearedState(state), nil, nil
			}
			return RuntimeTarget{}, devcontainer.State{}, nil, err
		}
		candidates = inspects
		if primary != nil {
			container = primary
		}
	} else {
		inspect, err := o.observeManagedContainer(ctx, resolved, state)
		if err != nil {
			if allowMissing && errors.Is(err, ErrObservedTargetNotFound) {
				return RuntimeTarget{Kind: TargetKindManagedContainer, Image: resolved.ImageName, ContainerName: resolved.ContainerName}, clearedState(state), nil, nil
			}
			return RuntimeTarget{}, devcontainer.State{}, nil, err
		}
		if inspect != nil {
			candidates = []docker.ContainerInspect{*inspect}
			container = inspect
		}
	}
	target := RuntimeTarget{
		Kind:           targetKind(resolved),
		Image:          resolved.ImageName,
		ContainerName:  resolved.ContainerName,
		ComposeProject: resolved.ComposeProject,
		ComposeService: resolved.ComposeService,
		Containers:     candidates,
	}
	if container != nil {
		target.PrimaryContainer = container.ID
		if previousContainerID != "" && previousContainerID != container.ID {
			state = clearedState(state)
		}
		state.ContainerID = container.ID
	}
	if container == nil {
		state = clearedState(state)
	}
	if !inspectTarget {
		container = nil
	}
	return target, state, container, nil
}

func (o *Observer) observeManagedContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, state devcontainer.State) (*docker.ContainerInspect, error) {
	if state.ContainerID != "" {
		inspect, err := o.backend.InspectContainer(ctx, state.ContainerID)
		if err == nil {
			return &inspect, nil
		}
		if !docker.IsNotFound(err) {
			return nil, err
		}
	}
	filters := make([]string, 0, len(resolved.Labels))
	for key, value := range resolved.Labels {
		filters = append(filters, "label="+key+"="+value)
	}
	inspects, err := o.inspectListedContainers(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: filters})
	if err != nil {
		return nil, err
	}
	if len(inspects) == 0 {
		return nil, ErrObservedTargetNotFound
	}
	best := bestContainer(inspects)
	return &best, nil
}

func (o *Observer) observeComposeProject(ctx context.Context, resolved devcontainer.ResolvedConfig) ([]docker.ContainerInspect, *docker.ContainerInspect, error) {
	if resolved.ComposeProject == "" {
		return nil, nil, ErrObservedTargetNotFound
	}
	inspects, err := o.inspectListedContainers(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: []string{"label=com.docker.compose.project=" + resolved.ComposeProject}})
	if err != nil {
		return nil, nil, err
	}
	if len(inspects) == 0 {
		return nil, nil, ErrObservedTargetNotFound
	}
	primaryCandidates := make([]docker.ContainerInspect, 0)
	for _, inspect := range inspects {
		if inspect.Config.Labels["com.docker.compose.service"] == resolved.ComposeService {
			primaryCandidates = append(primaryCandidates, inspect)
		}
	}
	if len(primaryCandidates) == 0 {
		return inspects, nil, ErrObservedTargetNotFound
	}
	best := bestContainer(primaryCandidates)
	return inspects, &best, nil
}

func (o *Observer) inspectListedContainers(ctx context.Context, req dockercli.ListContainersRequest) ([]docker.ContainerInspect, error) {
	output, err := o.backend.ListContainers(ctx, req)
	if err != nil {
		return nil, err
	}
	return inspectContainerList(ctx, output, inspectContainerWithObserverBackend(o.backend))
}

func clearedState(state devcontainer.State) devcontainer.State {
	return devcontainer.State{Version: state.Version, BridgeEnabled: state.BridgeEnabled, BridgeSessionID: state.BridgeSessionID, BridgeTransition: state.BridgeTransition}
}

func targetKind(resolved devcontainer.ResolvedConfig) TargetKind {
	if resolved.SourceKind == "compose" {
		return TargetKindComposeService
	}
	return TargetKindManagedContainer
}

func (o ObservedState) readToken() ReadToken {
	return ReadToken{
		TargetKind:             o.Target.Kind,
		ContainerName:          o.Target.ContainerName,
		ComposeProject:         o.Target.ComposeProject,
		ComposeService:         o.Target.ComposeService,
		PrimaryContainer:       o.Target.PrimaryContainer,
		CoordinationGeneration: o.Control.Coordination.Generation,
	}
}

func observeCapabilities(container *docker.ContainerInspect, state devcontainer.State, resolved devcontainer.ResolvedConfig) CapabilityState {
	capabilityState := CapabilityState{
		BridgeEnabled:   bridgecap.EnabledFromInspect(container, state),
		DotfilesApplied: state.DotfilesReady && state.DotfilesTransition == nil,
		UIDRemapDesired: resolved.Merged.UpdateRemoteUserUID == nil || *resolved.Merged.UpdateRemoteUserUID,
	}
	if container != nil {
		capabilityState.SSHAgentAttached = capssh.HasTargetMount(*container, capssh.ContainerSocketPath) || container.Config.Labels[devcontainer.SSHAgentLabel] == "true"
	}
	return capabilityState
}

func (o *Observer) withDefaults() *Observer {
	if o.readState == nil {
		o.readState = storefs.ReadWorkspaceState
	}
	if o.readCoordination == nil {
		o.readCoordination = storefs.ReadCoordination
	}
	return o
}
