package runtime

import (
	"context"
	"errors"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type workspacePlanner struct {
	runner          *Runner
	resolve         func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	resolveReadOnly func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	readState       func(string) (devcontainer.State, error)
}

func newWorkspacePlanner(runner *Runner) *workspacePlanner {
	return (&workspacePlanner{runner: runner}).withDefaults()
}

func (p *workspacePlanner) cloneForRunner(runner *Runner) *workspacePlanner {
	if p == nil {
		return newWorkspacePlanner(runner)
	}
	clone := *p
	clone.runner = runner
	return clone.withDefaults()
}

func (p *workspacePlanner) withDefaults() *workspacePlanner {
	if p.resolve == nil {
		p.resolve = devcontainer.ResolveWithOptions
	}
	if p.resolveReadOnly == nil {
		p.resolveReadOnly = devcontainer.ResolveReadOnlyWithOptions
	}
	if p.readState == nil {
		p.readState = devcontainer.ReadState
	}
	return p
}

func (p *workspacePlanner) prepareResolved(ctx context.Context, opts prepareResolveOptions) (devcontainer.ResolvedConfig, error) {
	p = p.withDefaults()
	p.runner.emitPhaseProgress(opts.Events, opts.ProgressPhase, opts.ProgressLabel)
	resolveOpts := devcontainer.ResolveOptions{LockfilePolicy: opts.LockfilePolicy, FeatureHTTPTimeout: opts.FeatureTimeout, ReadPlanCache: true, StateBaseDir: opts.StateDir, CacheBaseDir: opts.CacheDir}
	resolveOpts.VerifyImage = p.runner.imageVerifier.Check
	if !opts.ReadOnly {
		resolveOpts.AllowNetwork = true
		resolveOpts.WritePlanCache = true
		resolveOpts.WriteFeatureLock = true
		resolveOpts.WriteFeatureState = true
	}
	var (
		resolved devcontainer.ResolvedConfig
		err      error
	)
	if opts.ReadOnly {
		resolved, err = p.resolveReadOnly(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	} else {
		resolved, err = p.resolve(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	}
	if err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if err := p.runner.verifyResolvedFeatures(resolved, opts.Events); err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if opts.Debug {
		p.runner.emitPlan(opts.Events, resolved)
	}
	return resolved, nil
}

func (p *workspacePlanner) prepareWorkspace(ctx context.Context, opts prepareWorkspaceOptions) (preparedWorkspace, error) {
	p = p.withDefaults()
	resolved, err := p.prepareResolved(ctx, opts.resolve)
	if err != nil {
		return preparedWorkspace{}, err
	}
	prepared := preparedWorkspace{resolved: resolved, image: preparedImage(resolved)}
	if opts.enrich {
		p.runner.emitPhaseProgress(opts.resolve.Events, phaseConfig, "Applying runtime metadata")
		if err := p.runner.enrichMergedConfig(ctx, &prepared.resolved, prepared.image); err != nil {
			return preparedWorkspace{}, err
		}
	}
	if opts.loadState {
		state, err := p.readState(prepared.resolved.StateDir)
		if err != nil {
			return preparedWorkspace{}, err
		}
		state, err = p.runner.reconcileState(ctx, prepared.resolved, state)
		if err != nil {
			return preparedWorkspace{}, err
		}
		prepared.state = state
		prepared.containerID = state.ContainerID
	}
	if opts.findContainer && prepared.containerID == "" {
		p.runner.emitPhaseProgress(opts.resolve.Events, phaseContainer, "Finding managed container")
		containerID, err := p.runner.findContainer(ctx, prepared.resolved)
		if err != nil {
			if opts.allowMissingContainer && errors.Is(err, errManagedContainerNotFound) {
				return prepared, nil
			}
			return preparedWorkspace{}, err
		}
		prepared.containerID = containerID
	}
	if opts.inspectContainer && prepared.containerID != "" {
		inspect, err := p.runner.backend.InspectContainer(ctx, prepared.containerID)
		if err != nil {
			return preparedWorkspace{}, err
		}
		prepared.containerInspect = &inspect
	}
	return prepared, nil
}
