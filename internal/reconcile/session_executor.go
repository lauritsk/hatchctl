package reconcile

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
)

type ObservedSessionOptions struct {
	Plan                  workspaceplan.WorkspacePlan
	Resolved              devcontainer.ResolvedConfig
	Debug                 bool
	Events                ui.Sink
	Enrich                bool
	LoadState             bool
	FindContainer         bool
	AllowMissingContainer bool
	InspectContainer      bool
}

type Session struct {
	executor *Executor
	prepared preparedWorkspace
}

func (e *Executor) PrepareObservedSession(ctx context.Context, opts ObservedSessionOptions) (*Session, error) {
	resolved := opts.Resolved
	if opts.Debug {
		e.emitResolvedPlan(opts.Events, resolved)
	}
	prepared := preparedWorkspace{resolved: resolved, image: preparedImage(resolved)}
	if opts.Enrich {
		e.emitPhaseProgress(opts.Events, phaseConfig, "Applying runtime metadata")
		if err := e.EnrichMergedConfig(ctx, &prepared.resolved, prepared.image); err != nil {
			return nil, err
		}
	}
	observed, err := NewObserver(e.observerBackend()).Observe(ctx, ObserveRequest{
		Plan:               opts.Plan,
		Resolved:           prepared.resolved,
		ImageRef:           prepared.image,
		LoadControlState:   opts.LoadState || opts.FindContainer || opts.InspectContainer,
		ObserveTarget:      opts.LoadState || opts.FindContainer || opts.InspectContainer,
		InspectTarget:      opts.InspectContainer,
		AllowMissingTarget: opts.AllowMissingContainer,
	})
	if err != nil {
		return nil, err
	}
	prepared.resolved = observed.Resolved
	prepared.state = observed.Control.WorkspaceState
	prepared.containerID = observed.Target.PrimaryContainer
	prepared.containerInspect = observed.Container
	prepared.observed = observed
	return &Session{executor: e, prepared: prepared}, nil
}

func (s *Session) Resolved() devcontainer.ResolvedConfig {
	return s.prepared.resolved
}

func (s *Session) SetResolved(resolved devcontainer.ResolvedConfig) {
	s.prepared.resolved = resolved
	s.prepared.observed.Resolved = resolved
}

func (s *Session) Image() string {
	return s.prepared.image
}

func (s *Session) State() devcontainer.State {
	return s.prepared.state
}

func (s *Session) SetState(state devcontainer.State) {
	s.prepared.state = state
	s.prepared.observed.Control.WorkspaceState = state
	s.prepared.containerID = state.ContainerID
	s.prepared.observed.Target.PrimaryContainer = state.ContainerID
	if s.prepared.containerInspect != nil && s.prepared.containerInspect.ID != state.ContainerID {
		s.prepared.containerInspect = nil
		s.prepared.observed.Container = nil
	}
}

func (s *Session) ContainerID() string {
	return s.prepared.containerID
}

func (s *Session) SetContainerID(containerID string) {
	s.prepared.containerID = containerID
	s.prepared.state.ContainerID = containerID
	s.prepared.observed.Target.PrimaryContainer = containerID
	s.prepared.observed.Control.WorkspaceState.ContainerID = containerID
	if s.prepared.containerInspect != nil && s.prepared.containerInspect.ID != containerID {
		s.prepared.containerInspect = nil
		s.prepared.observed.Container = nil
	}
}

func (s *Session) ContainerInspect() *docker.ContainerInspect {
	return s.prepared.containerInspect
}

func (s *Session) SetContainerInspect(inspect *docker.ContainerInspect) {
	s.prepared.containerInspect = inspect
	s.prepared.observed.Container = inspect
}

func (s *Session) Observed() ObservedState {
	return s.prepared.observed
}

func (s *Session) EffectiveRemoteUser(ctx context.Context) (string, error) {
	return s.executor.effectiveRemoteUser(ctx, s.prepared)
}

func (s *Session) ManagedContainer() (*ManagedContainer, error) {
	return s.executor.readManagedContainerState(s.prepared)
}

func (s *Session) RevalidateReadTarget(ctx context.Context) error {
	if s.prepared.observed.ReadTarget.PrimaryContainer == "" {
		return nil
	}
	return NewObserver(s.executor.observerBackend()).RevalidateReadToken(ctx, s.prepared.observed)
}
