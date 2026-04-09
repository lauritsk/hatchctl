package reconcile

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/lauritsk/hatchctl/internal/bridge"
	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type CommandStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Events ui.Sink
}

type UpOptions struct {
	Recreate bool
	Debug    bool
	IO       CommandStreams
}

type BuildOptions struct {
	Debug bool
	IO    CommandStreams
}

type ExecOptions struct {
	Args      []string
	RemoteEnv map[string]string
	Debug     bool
	IO        CommandStreams
}

type ReadConfigOptions struct {
	Debug bool
	IO    CommandStreams
}

type RunLifecycleOptions struct {
	Phase string
	Debug bool
	IO    CommandStreams
}

type BridgeDoctorOptions struct {
	Debug bool
	IO    CommandStreams
}

func (e *Executor) Up(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts UpOptions) (UpResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	dotfiles, err := normalizeDotfilesPreference(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return UpResult{}, err
	}
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return UpResult{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, LoadState: true, AllowMissingContainer: true, InspectContainer: true})
	if err != nil {
		return UpResult{}, err
	}
	resolved = session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return UpResult{}, err
	}
	if err := storefs.EnsureWorkspaceStateDir(resolved.StateDir); err != nil {
		return UpResult{}, err
	}
	state := session.State()
	tracker := NewStateTracker(resolved.StateDir, state)
	executor.emitPhaseProgress(opts.IO.Events, phaseImage, "Reconciling container image")
	image, imagePlan, err := executor.ReconcileImage(ctx, workspacePlan, resolved, opts.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseImage, "Applying runtime metadata")
	if err := executor.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return UpResult{}, err
		}
	}
	helperArch, err := executor.InspectImageArchitecture(ctx, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeSession *bridge.Session
	executor.emitPhaseProgress(opts.IO.Events, phaseBridge, "Configuring bridge support")
	if workspacePlan.Capabilities.Bridge.Enabled {
		bridgeSession, err = bridgecap.Prepare(resolved.StateDir, helperArch)
		if err == nil {
			resolved.Merged = bridgecap.Inject(bridgeSession, resolved.Merged)
		}
	}
	if err != nil {
		return UpResult{}, err
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseContainer, "Reconciling managed container")
	containerID, containerKey, created, err := executor.ReconcileContainer(ctx, session.Observed(), resolved, image, imagePlan, workspacePlan.Capabilities.Bridge.Enabled, workspacePlan.Capabilities.SSHAgent.Enabled, opts.Recreate, opts.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	tracker.ApplyContainer(containerID, containerKey, created)
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	session.SetContainerID(containerID)
	inspect, err := executor.engine.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: containerID})
	if err != nil {
		return UpResult{}, err
	}
	session.SetContainerInspect(&inspect)
	executor.emitPhaseProgress(opts.IO.Events, phaseContainer, "Reconciling container user")
	if err := executor.EnsureUpdatedUIDContainer(ctx, resolved, image, containerID, opts.IO.Events); err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	if bridgeSession != nil {
		tracker.BeginBridge("start", containerKey)
		if err := tracker.Persist(); err != nil {
			return UpResult{}, err
		}
		executor.emitPhaseProgress(opts.IO.Events, phaseBridge, "Starting bridge session")
		startedBridge, err := bridgecap.Start(bridgeSession, containerID)
		if err != nil {
			return UpResult{}, err
		}
		bridgeReport = bridge.ReportFromSession(startedBridge)
		tracker.EnableBridge(bridgeReport.ID)
		if err := tracker.Persist(); err != nil {
			return UpResult{}, err
		}
	}
	lifecycleKey, err := executor.DesiredLifecycleKey(resolved, containerKey, DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath})
	if err != nil {
		return UpResult{}, err
	}
	observed := session.Observed()
	dotfilesConfig := DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}
	lifecyclePlan := PlanUpLifecycle(observed, DesiredLifecycle{Key: lifecycleKey, Dotfiles: dotfilesConfig, Created: created})
	tracker.BeginPlannedLifecycle(lifecyclePlan, DotfilesNeedsInstall(state, dotfiles))
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseLifecycle, "Running lifecycle commands")
	if err := executor.RunLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, opts.IO.Events, lifecyclePlan); err != nil {
		return UpResult{}, err
	}
	tracker.CompletePlannedLifecycle(lifecyclePlan, dotfilesConfig, DotfilesNeedsInstall(state, dotfiles))
	if bridgeReport == nil {
		tracker.DisableBridge()
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseState, "Writing workspace state")
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	return UpResult{ContainerID: containerID, Image: image, RemoteWorkspaceFolder: resolved.RemoteWorkspace, StateDir: resolved.StateDir, Bridge: bridgeReport}, nil
}

func (e *Executor) Build(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts BuildOptions) (BuildResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return BuildResult{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events})
	if err != nil {
		return BuildResult{}, err
	}
	resolved = session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return BuildResult{}, err
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseImage, "Reconciling container image")
	image, _, err := executor.ReconcileImage(ctx, workspacePlan, resolved, opts.IO.Events)
	if err != nil {
		return BuildResult{}, err
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseImage, "Applying runtime metadata")
	if err := executor.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (e *Executor) Exec(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts ExecOptions) (int, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return 0, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, Enrich: true, FindContainer: true, InspectContainer: true})
	if err != nil {
		return 0, err
	}
	resolved = session.Resolved()
	if owner := session.Observed().Control.Coordination.ActiveOwner; owner != nil {
		return 0, &storefs.WorkspaceBusyError{StateDir: resolved.StateDir, Owner: owner}
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return 0, err
		}
		session.SetResolved(resolved)
		if err := EnsureContainerHasSSHAgent(session.ContainerInspect()); err != nil {
			return 0, err
		}
	}
	interactive := ShouldAllocateTTY(opts.IO.Stdin, opts.IO.Stdout)
	req, err := executor.DockerExecRequest(ctx, session.Observed(), opts.IO.Stdin != nil, interactive, opts.RemoteEnv, opts.Args, dockercli.Streams{Stdin: opts.IO.Stdin, Stdout: opts.IO.Stdout, Stderr: opts.IO.Stderr})
	if err != nil {
		return 0, err
	}
	if err := session.RevalidateReadTarget(ctx); err != nil {
		return 0, err
	}
	if interactive {
		executor.clearProgress(opts.IO.Events)
	} else {
		executor.emitPhaseProgress(opts.IO.Events, phaseExec, fmt.Sprintf("Executing command in %s", session.ContainerID()))
	}
	err = executor.engine.Exec(ctx, req)
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

func (e *Executor) ReadConfig(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts ReadConfigOptions) (ReadConfigResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	dotfiles, err := normalizeDotfilesPreference(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseConfig, "Inspecting resolved configuration")
	if err != nil {
		return ReadConfigResult{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, LoadState: true, FindContainer: true, AllowMissingContainer: true, InspectContainer: true})
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved = session.Resolved()
	image := session.Image()
	state := session.State()
	if err := executor.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		if !docker.IsNotFound(err) {
			return ReadConfigResult{}, err
		}
		resolved.Merged = devcontainer.MergeMetadata(resolved.Config, featureMetadata(resolved.Features))
	}
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
	imageUser, err := executor.InspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := session.ManagedContainer()
	if err != nil {
		return ReadConfigResult{}, err
	}
	return ReadConfigResult{WorkspaceFolder: resolved.WorkspaceFolder, ConfigPath: resolved.ConfigPath, WorkspaceMount: resolved.WorkspaceMount, SourceKind: resolved.SourceKind, HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(), HasCreateCommand: len(resolved.Merged.OnCreateCommands) > 0 || len(resolved.Merged.UpdateContentCommands) > 0 || len(resolved.Merged.PostCreateCommands) > 0, HasStartCommand: len(resolved.Merged.PostStartCommands) > 0, HasAttachCommand: len(resolved.Merged.PostAttachCommands) > 0, Image: image, ImageUser: imageUser, ContainerName: resolved.ContainerName, StateDir: resolved.StateDir, CacheDir: resolved.CacheDir, RemoteUser: resolvedUser, ContainerUser: resolved.Merged.ContainerUser, RemoteEnv: RedactSensitiveMap(resolved.Merged.RemoteEnv), ContainerEnv: RedactSensitiveMap(resolved.Merged.ContainerEnv), Mounts: resolved.Merged.Mounts, ForwardPorts: []string(resolved.Merged.ForwardPorts), Bridge: bridgeReport, Dotfiles: DotfilesStatusFromState(state, dotfiles), MetadataCount: len(resolved.Merged.Metadata), ManagedContainer: managedContainer}, nil
}

func (e *Executor) RunLifecycle(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	dotfiles, err := normalizeDotfilesPreference(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return RunLifecycleResult{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, Enrich: true, FindContainer: true, LoadState: true})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved = session.Resolved()
	state := session.State()
	observed := session.Observed()
	phase := opts.Phase
	lifecycleKey, err := executor.DesiredLifecycleKey(resolved, state.ContainerKey, DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	dotfilesConfig := DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}
	lifecyclePlan, err := PlanLifecycleCommand(observed, DesiredLifecycle{Key: lifecycleKey, Requested: phase, Dotfiles: dotfilesConfig})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	tracker := NewStateTracker(resolved.StateDir, state)
	if lifecyclePlan.RunCreate {
		tracker.BeginPlannedLifecycle(lifecyclePlan, DotfilesNeedsInstall(state, dotfiles))
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	executor.emitPhaseProgress(opts.IO.Events, phaseLifecycle, "Running lifecycle commands")
	if err := executor.RunLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, opts.IO.Events, lifecyclePlan); err != nil {
		return RunLifecycleResult{}, err
	}
	if lifecyclePlan.RunCreate {
		tracker.CompletePlannedLifecycle(lifecyclePlan, dotfilesConfig, DotfilesNeedsInstall(state, dotfiles))
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	return RunLifecycleResult{ContainerID: session.ContainerID(), Phase: phase}, nil
}

func (e *Executor) BridgeDoctor(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts BridgeDoctorOptions) (bridge.Report, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	resolved, err := executor.Materialize(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseBridge, "Inspecting bridge state")
	if err != nil {
		return bridge.Report{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events})
	if err != nil {
		return bridge.Report{}, err
	}
	return bridgecap.Doctor(session.Resolved().StateDir)
}
