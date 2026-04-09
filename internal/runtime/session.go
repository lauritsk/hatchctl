package runtime

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

type workspaceSessionOptions struct {
	Plan                  workspaceplan.WorkspacePlan
	ProgressPhase         string
	ProgressLabel         string
	Debug                 bool
	Events                ui.Sink
	Enrich                bool
	LoadState             bool
	FindContainer         bool
	AllowMissingContainer bool
	InspectContainer      bool
}

type workspaceSession struct {
	runner   *Runner
	prepared preparedWorkspace
}

type workspaceStateTracker struct {
	stateDir string
	state    devcontainer.State
}

func (r *Runner) prepareSession(ctx context.Context, opts workspaceSessionOptions) (*workspaceSession, error) {
	r.emitPhaseProgress(opts.Events, opts.ProgressPhase, opts.ProgressLabel)
	resolved, err := r.planner.Materialize(ctx, opts.Plan, r.imageVerifier.Check)
	if err != nil {
		return nil, err
	}
	if err := r.verifyResolvedFeatures(resolved, opts.Events); err != nil {
		return nil, err
	}
	if opts.Debug {
		r.emitPlan(opts.Events, resolved)
	}
	prepared := preparedWorkspace{resolved: resolved, image: preparedImage(resolved)}
	if opts.Enrich {
		r.emitPhaseProgress(opts.Events, phaseConfig, "Applying runtime metadata")
		if err := r.enrichMergedConfig(ctx, &prepared.resolved, prepared.image); err != nil {
			return nil, err
		}
	}
	observed, err := reconcile.NewObserver(r.backend).Observe(ctx, reconcile.ObserveRequest{
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
	return &workspaceSession{runner: r, prepared: prepared}, nil
}

func (s *workspaceSession) Resolved() devcontainer.ResolvedConfig {
	return s.prepared.resolved
}

func (s *workspaceSession) SetResolved(resolved devcontainer.ResolvedConfig) {
	s.prepared.resolved = resolved
}

func (s *workspaceSession) Image() string {
	return s.prepared.image
}

func (s *workspaceSession) State() devcontainer.State {
	return s.prepared.state
}

func (s *workspaceSession) SetState(state devcontainer.State) {
	s.prepared.state = state
	s.prepared.containerID = state.ContainerID
	if s.prepared.containerInspect != nil && s.prepared.containerInspect.ID != state.ContainerID {
		s.prepared.containerInspect = nil
	}
}

func (s *workspaceSession) ContainerID() string {
	return s.prepared.containerID
}

func (s *workspaceSession) SetContainerID(containerID string) {
	s.prepared.containerID = containerID
	s.prepared.state.ContainerID = containerID
	if s.prepared.containerInspect != nil && s.prepared.containerInspect.ID != containerID {
		s.prepared.containerInspect = nil
	}
}

func (s *workspaceSession) ContainerInspect() *docker.ContainerInspect {
	return s.prepared.containerInspect
}

func (s *workspaceSession) SetContainerInspect(inspect *docker.ContainerInspect) {
	s.prepared.containerInspect = inspect
}

func (s *workspaceSession) EffectiveRemoteUser(ctx context.Context) (string, error) {
	return s.runner.effectiveRemoteUser(ctx, s.prepared)
}

func (s *workspaceSession) ManagedContainer() (*ManagedContainer, error) {
	return s.runner.readManagedContainerState(s.prepared)
}

func (s *workspaceSession) RevalidateReadTarget(ctx context.Context) error {
	if s.prepared.observed.ReadTarget.PrimaryContainer == "" {
		return nil
	}
	return reconcile.NewObserver(s.runner.backend).RevalidateReadToken(ctx, s.prepared.observed)
}

func newWorkspaceStateTracker(stateDir string, state devcontainer.State) *workspaceStateTracker {
	return &workspaceStateTracker{stateDir: stateDir, state: state}
}

func (t *workspaceStateTracker) State() devcontainer.State {
	return t.state
}

func (t *workspaceStateTracker) Persist() error {
	return devcontainer.WriteState(t.stateDir, t.state)
}

func (t *workspaceStateTracker) BeginContainer(containerID string, containerKey string) {
	t.state.ContainerID = containerID
	t.state.ContainerKey = containerKey
	t.state.LifecycleReady = false
	t.state.LifecycleKey = ""
	t.state.Transition = nil
	t.state.BridgeEnabled = false
	t.state.BridgeSessionID = ""
	t.setDotfiles(DotfilesOptions{}, false)
}

func (t *workspaceStateTracker) SetContainer(containerID string, containerKey string) {
	t.state.ContainerID = containerID
	t.state.ContainerKey = containerKey
}

func (t *workspaceStateTracker) SetBridge(enabled bool, sessionID string) {
	t.state.BridgeEnabled = enabled
	t.state.BridgeSessionID = sessionID
}

func (t *workspaceStateTracker) MarkLifecycleReady(dotfiles DotfilesOptions) {
	t.state.Transition = nil
	t.state.LifecycleReady = true
	t.setDotfiles(dotfiles, dotfiles.Enabled())
}

func (t *workspaceStateTracker) BeginLifecycle(kind string, key string) {
	t.state.Transition = &devcontainer.StateTransition{Kind: kind, Key: key}
	t.state.LifecycleReady = false
	t.state.LifecycleKey = ""
}

func (t *workspaceStateTracker) CompleteLifecycle(key string, dotfiles DotfilesOptions) {
	t.state.Transition = nil
	t.state.LifecycleReady = true
	t.state.LifecycleKey = key
	t.setDotfiles(dotfiles, dotfiles.Enabled())
}

func (t *workspaceStateTracker) setDotfiles(dotfiles DotfilesOptions, ready bool) {
	t.state.DotfilesReady = ready
	t.state.DotfilesRepo = dotfiles.Repository
	t.state.DotfilesInstall = dotfiles.InstallCommand
	t.state.DotfilesTarget = dotfiles.TargetPath
	if dotfiles.Repository == "" {
		t.state.DotfilesReady = false
		t.state.DotfilesInstall = ""
		t.state.DotfilesTarget = ""
	}
}
