package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func (r *Runner) Up(ctx context.Context, opts UpOptions) (UpResult, error) {
	return upCommand{runner: r, opts: opts}.run(ctx)
}

func (r *Runner) Build(ctx context.Context, opts BuildOptions) (BuildResult, error) {
	return buildCommand{runner: r, opts: opts}.run(ctx)
}

func (r *Runner) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	return execCommandAction{runner: r, opts: opts}.run(ctx)
}

func (r *Runner) ReadConfig(ctx context.Context, opts ReadConfigOptions) (ReadConfigResult, error) {
	return readConfigCommand{runner: r, opts: opts}.run(ctx)
}

func (r *Runner) RunLifecycle(ctx context.Context, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	return runLifecycleCommand{runner: r, opts: opts}.run(ctx)
}

func (r *Runner) BridgeDoctor(ctx context.Context, opts BridgeDoctorOptions) (bridge.Report, error) {
	return bridgeDoctorCommand{runner: r, opts: opts}.run(ctx)
}

type upCommand struct {
	runner *Runner
	opts   UpOptions
}

func (c upCommand) run(ctx context.Context) (UpResult, error) {
	runner := c.runner.withCommandIO(commandIO{Stdin: c.runner.stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	workspacePlan, err := planForUp(c.opts)
	if err != nil {
		return UpResult{}, err
	}
	dotfiles, err := dotfilesOptionsFromPlan(workspacePlan).Normalized()
	if err != nil {
		return UpResult{}, err
	}
	session, err := runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:                  workspacePlan,
		ProgressPhase:         phaseResolve,
		ProgressLabel:         "Resolving development container",
		Debug:                 c.opts.Debug,
		Events:                c.opts.Events,
		LoadState:             true,
		AllowMissingContainer: true,
		InspectContainer:      true,
	})
	if err != nil {
		return UpResult{}, err
	}
	resolved := session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return UpResult{}, err
	}
	if err := ensureDir(resolved.StateDir); err != nil {
		return UpResult{}, err
	}
	state := session.State()
	tracker := newWorkspaceStateTracker(resolved.StateDir, state)
	runner.emitPhaseProgress(c.opts.Events, phaseImage, "Reconciling container image")
	image, imagePlan, err := runner.reconcileImage(ctx, workspacePlan, resolved, c.opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	runner.emitPhaseProgress(c.opts.Events, phaseImage, "Applying runtime metadata")
	if err := runner.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return UpResult{}, err
		}
	}
	helperArch, err := runner.inspectImageArchitecture(ctx, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeSession *bridge.Session
	runner.emitPhaseProgress(c.opts.Events, phaseBridge, "Configuring bridge support")
	if workspacePlan.Capabilities.Bridge.Enabled {
		bridgeSession, err = bridgecap.Prepare(resolved.StateDir, helperArch)
		if err == nil {
			resolved.Merged = bridgecap.Inject(bridgeSession, resolved.Merged)
		}
	} else {
		bridgeSession = nil
	}
	if err != nil {
		return UpResult{}, err
	}
	runner.emitPhaseProgress(c.opts.Events, phaseContainer, "Reconciling managed container")
	containerID, containerKey, created, err := runner.reconcileContainer(ctx, session.Observed(), resolved, image, imagePlan, workspacePlan.Capabilities.Bridge.Enabled, workspacePlan.Capabilities.SSHAgent.Enabled, c.opts.Recreate, c.opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	if created {
		tracker.BeginContainer(containerID, containerKey)
	} else {
		tracker.SetContainer(containerID, containerKey)
	}
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	session.SetContainerID(containerID)
	inspect, err := runner.backend.InspectContainer(ctx, containerID)
	if err != nil {
		return UpResult{}, err
	}
	session.SetContainerInspect(&inspect)
	runner.emitPhaseProgress(c.opts.Events, phaseContainer, "Reconciling container user")
	if err := runner.ensureUpdatedUIDContainer(ctx, resolved, image, containerID, c.opts.Events); err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	if bridgeSession != nil {
		runner.emitPhaseProgress(c.opts.Events, phaseBridge, "Starting bridge session")
		startedBridge, err := bridgecap.Start(bridgeSession, containerID)
		if err != nil {
			return UpResult{}, err
		}
		bridgeReport = bridge.ReportFromSession(startedBridge)
		tracker.SetBridge(true, bridgeReport.ID)
		if err := tracker.Persist(); err != nil {
			return UpResult{}, err
		}
	}

	lifecycleKey, err := runner.desiredLifecycleKey(resolved, containerKey, dotfiles)
	if err != nil {
		return UpResult{}, err
	}
	observed := session.Observed()
	lifecyclePlan := reconcile.PlanUpLifecycle(observed, reconcile.DesiredLifecycle{Key: lifecycleKey, Dotfiles: reconcile.DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}, Created: created})
	tracker.BeginLifecycle(string(lifecyclePlan.TransitionKind), lifecycleKey)
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	runner.emitPhaseProgress(c.opts.Events, phaseLifecycle, "Running lifecycle commands")
	if err := runner.runLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, c.opts.Events, lifecyclePlan); err != nil {
		return UpResult{}, err
	}

	tracker.CompleteLifecycle(lifecycleKey, dotfiles)
	if bridgeReport == nil {
		tracker.SetBridge(false, "")
	}
	runner.emitPhaseProgress(c.opts.Events, phaseState, "Writing workspace state")
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}

	return UpResult{ContainerID: containerID, Image: image, RemoteWorkspaceFolder: resolved.RemoteWorkspace, StateDir: resolved.StateDir, Bridge: bridgeReport}, nil
}

type buildCommand struct {
	runner *Runner
	opts   BuildOptions
}

func (c buildCommand) run(ctx context.Context) (BuildResult, error) {
	runner := c.runner.withCommandIO(commandIO{Stdin: c.runner.stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	workspacePlan, err := planForBuild(c.opts)
	if err != nil {
		return BuildResult{}, err
	}
	session, err := runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:          workspacePlan,
		ProgressPhase: phaseResolve,
		ProgressLabel: "Resolving development container",
		Debug:         c.opts.Debug,
		Events:        c.opts.Events,
	})
	if err != nil {
		return BuildResult{}, err
	}
	resolved := session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return BuildResult{}, err
	}
	runner.emitPhaseProgress(c.opts.Events, phaseImage, "Reconciling container image")
	image, _, err := runner.reconcileImage(ctx, workspacePlan, resolved, c.opts.Events)
	if err != nil {
		return BuildResult{}, err
	}
	runner.emitPhaseProgress(c.opts.Events, phaseImage, "Applying runtime metadata")
	if err := runner.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

type execCommandAction struct {
	runner *Runner
	opts   ExecOptions
}

func (c execCommandAction) run(ctx context.Context) (int, error) {
	workspacePlan, err := planForExec(c.opts)
	if err != nil {
		return 0, err
	}
	session, err := c.runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:             workspacePlan,
		ProgressPhase:    phaseResolve,
		ProgressLabel:    "Resolving development container",
		Debug:            c.opts.Debug,
		Events:           c.opts.Events,
		Enrich:           true,
		FindContainer:    true,
		InspectContainer: true,
	})
	if err != nil {
		return 0, err
	}
	resolved := session.Resolved()
	if owner := session.Observed().Control.Coordination.ActiveOwner; owner != nil {
		return 0, &storefs.WorkspaceBusyError{StateDir: resolved.StateDir, Owner: owner}
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return 0, err
		}
		session.SetResolved(resolved)
		if err := ensureContainerHasSSHAgent(session.ContainerInspect(), sshAgentContainerSocketPath); err != nil {
			return 0, err
		}
	}
	if err := session.RevalidateReadTarget(ctx); err != nil {
		return 0, err
	}
	interactive := shouldAllocateTTY(c.opts.Stdin, c.opts.Stdout)
	req, err := c.runner.dockerExecRequest(ctx, session.Observed(), c.opts.Stdin != nil, interactive, c.opts.RemoteEnv, c.opts.Args, dockercli.Streams{Stdin: c.opts.Stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	if err != nil {
		return 0, err
	}
	if interactive {
		c.runner.clearProgress(c.opts.Events)
	} else {
		c.runner.emitPhaseProgress(c.opts.Events, phaseExec, fmt.Sprintf("Executing command in %s", session.ContainerID()))
	}
	err = c.runner.backend.Exec(ctx, req)
	if err == nil {
		return 0, nil
	}
	var dockerErr *docker.Error
	if errors.As(err, &dockerErr) {
		if code, ok := dockerErr.ExitCode(); ok {
			return code, nil
		}
	}
	return 0, err
}

type readConfigCommand struct {
	runner *Runner
	opts   ReadConfigOptions
}

func (c readConfigCommand) run(ctx context.Context) (ReadConfigResult, error) {
	runner := c.runner.withCommandIO(commandIO{Stdin: c.runner.stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	workspacePlan, err := planForReadConfig(c.opts)
	if err != nil {
		return ReadConfigResult{}, err
	}
	dotfiles, err := dotfilesOptionsFromPlan(workspacePlan).Normalized()
	if err != nil {
		return ReadConfigResult{}, err
	}
	session, err := runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:                  workspacePlan,
		ProgressPhase:         phaseConfig,
		ProgressLabel:         "Inspecting resolved configuration",
		Debug:                 c.opts.Debug,
		Events:                c.opts.Events,
		Enrich:                true,
		LoadState:             true,
		FindContainer:         true,
		AllowMissingContainer: true,
		InspectContainer:      true,
	})
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved := session.Resolved()
	image := session.Image()
	state := session.State()
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return ReadConfigResult{}, err
		}
	}
	var bridgeSession *bridge.Session
	if state.BridgeEnabled {
		bridgeSession, err = bridgecap.Preview(resolved.StateDir, true)
		if err == nil {
			resolved.Merged = bridgecap.Inject(bridgeSession, resolved.Merged)
		}
	}
	if err != nil {
		return ReadConfigResult{}, err
	}
	var bridgeReport *bridge.Report
	if bridgeSession != nil {
		bridgeReport = bridge.ReportFromSession(bridgeSession)
	}
	if state.BridgeEnabled {
		report, err := bridgecap.Doctor(resolved.StateDir)
		if err != nil {
			return ReadConfigResult{}, err
		}
		bridgeReport = &report
	}
	session.SetResolved(resolved)
	resolvedUser, err := session.EffectiveRemoteUser(ctx)
	if err != nil {
		return ReadConfigResult{}, err
	}
	imageUser, err := runner.inspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := session.ManagedContainer()
	if err != nil {
		return ReadConfigResult{}, err
	}
	return ReadConfigResult{WorkspaceFolder: resolved.WorkspaceFolder, ConfigPath: resolved.ConfigPath, WorkspaceMount: resolved.WorkspaceMount, SourceKind: resolved.SourceKind, HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(), HasCreateCommand: len(resolved.Merged.OnCreateCommands) > 0 || len(resolved.Merged.UpdateContentCommands) > 0 || len(resolved.Merged.PostCreateCommands) > 0, HasStartCommand: len(resolved.Merged.PostStartCommands) > 0, HasAttachCommand: len(resolved.Merged.PostAttachCommands) > 0, Image: image, ImageUser: imageUser, ContainerName: resolved.ContainerName, StateDir: resolved.StateDir, CacheDir: resolved.CacheDir, RemoteUser: resolvedUser, ContainerUser: resolved.Merged.ContainerUser, RemoteEnv: redactSensitiveMap(resolved.Merged.RemoteEnv), ContainerEnv: redactSensitiveMap(resolved.Merged.ContainerEnv), Mounts: resolved.Merged.Mounts, ForwardPorts: []string(resolved.Merged.ForwardPorts), Bridge: bridgeReport, Dotfiles: dotfilesStatus(state, dotfiles), MetadataCount: len(resolved.Merged.Metadata), ManagedContainer: managedContainer}, nil
}

type runLifecycleCommand struct {
	runner *Runner
	opts   RunLifecycleOptions
}

func (c runLifecycleCommand) run(ctx context.Context) (RunLifecycleResult, error) {
	runner := c.runner.withCommandIO(commandIO{Stdin: c.runner.stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	workspacePlan, err := planForLifecycle(c.opts)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	dotfiles, err := dotfilesOptionsFromPlan(workspacePlan).Normalized()
	if err != nil {
		return RunLifecycleResult{}, err
	}
	session, err := runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:          workspacePlan,
		ProgressPhase: phaseResolve,
		ProgressLabel: "Resolving development container",
		Debug:         c.opts.Debug,
		Events:        c.opts.Events,
		Enrich:        true,
		FindContainer: true,
		LoadState:     true,
	})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved := session.Resolved()
	state := session.State()
	observed := session.Observed()
	phase := strings.ToLower(c.opts.Phase)
	if phase == "" {
		phase = "all"
	}
	lifecycleKey, err := runner.desiredLifecycleKey(resolved, state.ContainerKey, dotfiles)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	lifecyclePlan := reconcile.PlanLifecycleCommand(observed, reconcile.DesiredLifecycle{Key: lifecycleKey, Requested: phase, Dotfiles: reconcile.DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}})
	tracker := newWorkspaceStateTracker(resolved.StateDir, state)
	if lifecyclePlan.RunCreate {
		tracker.BeginLifecycle(string(lifecyclePlan.TransitionKind), lifecyclePlan.Key)
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	runner.emitPhaseProgress(c.opts.Events, phaseLifecycle, "Running lifecycle commands")
	if err := runner.runLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, c.opts.Events, lifecyclePlan); err != nil {
		return RunLifecycleResult{}, err
	}
	if lifecyclePlan.RunCreate {
		tracker.CompleteLifecycle(lifecycleKey, dotfiles)
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	return RunLifecycleResult{ContainerID: session.ContainerID(), Phase: phase}, nil
}

type bridgeDoctorCommand struct {
	runner *Runner
	opts   BridgeDoctorOptions
}

func (c bridgeDoctorCommand) run(ctx context.Context) (bridge.Report, error) {
	runner := c.runner.withCommandIO(commandIO{Stdin: c.runner.stdin, Stdout: c.opts.Stdout, Stderr: c.opts.Stderr})
	workspacePlan, err := planForBridgeDoctor(c.opts)
	if err != nil {
		return bridge.Report{}, err
	}
	session, err := runner.prepareSession(ctx, workspaceSessionOptions{
		Plan:          workspacePlan,
		ProgressPhase: phaseBridge,
		ProgressLabel: "Inspecting bridge state",
		Debug:         c.opts.Debug,
		Events:        c.opts.Events,
	})
	if err != nil {
		return bridge.Report{}, err
	}
	return bridgecap.Doctor(session.Resolved().StateDir)
}
