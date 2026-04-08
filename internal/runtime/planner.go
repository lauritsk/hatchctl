package runtime

import (
	"context"
	"errors"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type workspacePlanner struct {
	runner   *Runner
	resolver workspaceResolver
}

func (p *workspacePlanner) prepareResolved(ctx context.Context, opts prepareResolveOptions) (devcontainer.ResolvedConfig, error) {
	p.runner.emitProgress(opts.Events, opts.ProgressLabel)
	resolveOpts := devcontainer.ResolveOptions{LockfilePolicy: opts.LockfilePolicy, FeatureHTTPTimeout: opts.FeatureTimeout, ReadPlanCache: true}
	resolveOpts.VerifyImage = p.runner.imageVerifier.Verify
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
		resolved, err = p.resolver.ResolveReadOnly(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	} else {
		resolved, err = p.resolver.Resolve(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	}
	if err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if opts.Debug {
		p.runner.emitPlan(opts.Events, resolved)
	}
	return resolved, nil
}

func (p *workspacePlanner) prepareEnrichedResolved(ctx context.Context, opts prepareResolveOptions) (devcontainer.ResolvedConfig, string, error) {
	resolved, err := p.prepareResolved(ctx, opts)
	if err != nil {
		return devcontainer.ResolvedConfig{}, "", err
	}
	image := preparedImage(resolved)
	p.runner.emitProgress(opts.Events, "Applying runtime metadata")
	if err := p.runner.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return devcontainer.ResolvedConfig{}, "", err
	}
	return resolved, image, nil
}

func (p *workspacePlanner) prepareWorkspace(ctx context.Context, opts prepareWorkspaceOptions) (preparedWorkspace, error) {
	resolved, err := p.prepareResolved(ctx, opts.resolve)
	if err != nil {
		return preparedWorkspace{}, err
	}
	prepared := preparedWorkspace{resolved: resolved, image: preparedImage(resolved)}
	if opts.enrich {
		p.runner.emitProgress(opts.resolve.Events, "Applying runtime metadata")
		if err := p.runner.enrichMergedConfig(ctx, &prepared.resolved, prepared.image); err != nil {
			return preparedWorkspace{}, err
		}
	}
	if opts.loadState {
		state, err := p.runner.stateStore.Read(prepared.resolved.StateDir)
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
		p.runner.emitProgress(opts.resolve.Events, "Finding managed container")
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
		inspect, err := p.runner.docker.InspectContainer(ctx, prepared.containerID)
		if err != nil {
			return preparedWorkspace{}, err
		}
		prepared.containerInspect = &inspect
	}
	return prepared, nil
}
