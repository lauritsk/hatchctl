package runtime

import (
	"context"
	"errors"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
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
	if opts.LoadState {
		state, err := devcontainer.ReadState(prepared.resolved.StateDir)
		if err != nil {
			return nil, err
		}
		state, err = r.reconcileState(ctx, prepared.resolved, state)
		if err != nil {
			return nil, err
		}
		prepared.state = state
		prepared.containerID = state.ContainerID
	}
	if opts.FindContainer && prepared.containerID == "" {
		r.emitPhaseProgress(opts.Events, phaseContainer, "Finding managed container")
		containerID, err := r.findContainer(ctx, prepared.resolved)
		if err != nil {
			if opts.AllowMissingContainer && errors.Is(err, errManagedContainerNotFound) {
				return &workspaceSession{runner: r, prepared: prepared}, nil
			}
			return nil, err
		}
		prepared.containerID = containerID
	}
	if opts.InspectContainer && prepared.containerID != "" {
		inspect, err := r.backend.InspectContainer(ctx, prepared.containerID)
		if err != nil {
			return nil, err
		}
		prepared.containerInspect = &inspect
	}
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

func newWorkspaceStateTracker(stateDir string, state devcontainer.State) *workspaceStateTracker {
	return &workspaceStateTracker{stateDir: stateDir, state: state}
}

func (t *workspaceStateTracker) State() devcontainer.State {
	return t.state
}

func (t *workspaceStateTracker) Persist() error {
	return devcontainer.WriteState(t.stateDir, t.state)
}

func (t *workspaceStateTracker) BeginContainer(containerID string) {
	t.state.ContainerID = containerID
	t.state.LifecycleReady = false
	t.state.BridgeEnabled = false
	t.state.BridgeSessionID = ""
	t.setDotfiles(DotfilesOptions{}, false)
}

func (t *workspaceStateTracker) SetBridge(enabled bool, sessionID string) {
	t.state.BridgeEnabled = enabled
	t.state.BridgeSessionID = sessionID
}

func (t *workspaceStateTracker) MarkLifecycleReady(dotfiles DotfilesOptions) {
	t.state.LifecycleReady = true
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
