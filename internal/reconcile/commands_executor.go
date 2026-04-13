package reconcile

import (
	"context"
	"fmt"
	"io"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/bridge"
	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	capdot "github.com/lauritsk/hatchctl/internal/capability/dotfiles"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/spec"
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

func (e *Executor) ensureBackendSupport(resolved devcontainer.ResolvedConfig, bridgeEnabled bool) error {
	caps := e.engine.Capabilities()
	if resolved.SourceKind == "compose" && !caps.ProjectServices {
		return backend.UnsupportedCapabilityError{Backend: e.engine.ID(), Capability: "compose-based devcontainers"}
	}
	if bridgeEnabled && !caps.Bridge {
		return backend.UnsupportedCapabilityError{Backend: e.engine.ID(), Capability: "bridge integration"}
	}
	return nil
}

func (e *Executor) Up(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts UpOptions) (UpResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	session, resolved, dotfiles, tracker, workspacePlan, err := executor.prepareUp(ctx, workspacePlan, opts)
	if err != nil {
		return UpResult{}, err
	}
	resolved, image, imagePlan, bridgeSession, err := executor.prepareUpRuntime(ctx, workspacePlan, resolved, opts.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	session.SetResolved(resolved)
	containerID, containerKey, created, err := executor.reconcileUpContainer(ctx, session, tracker, workspacePlan, resolved, image, imagePlan, opts.Recreate, opts.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	bridgeReport, err := executor.startUpBridge(ctx, tracker, bridgeSession, containerKey, containerID, opts.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	session.SetState(tracker.State())
	if err := executor.runUpLifecycle(ctx, session, tracker, workspacePlan, dotfiles, containerKey, created, opts.IO.Events); err != nil {
		return UpResult{}, err
	}
	if bridgeReport == nil {
		tracker.DisableBridge()
	}
	tracker.SetTrustedRefs(executor.imageVerifier.TrustedRefs())
	executor.emitPhaseProgress(opts.IO.Events, phaseState, "Writing workspace state")
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	return UpResult{ContainerID: containerID, Image: image, RemoteWorkspaceFolder: resolved.RemoteWorkspace, StateDir: resolved.StateDir, Bridge: bridgeReport}, nil
}

func (e *Executor) prepareUp(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts UpOptions) (*Session, devcontainer.ResolvedConfig, capdot.Config, *StateTracker, workspaceplan.WorkspacePlan, error) {
	dotfiles, err := capdot.Normalize(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	workspacePlan, resolved, err := e.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	session, err := e.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, LoadState: true, AllowMissingContainer: true, InspectContainer: true})
	if err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	resolved = session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	if err := e.ensureBackendSupport(resolved, workspacePlan.Capabilities.Bridge.Enabled); err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	if err := storefs.EnsureWorkspaceStateDir(resolved.StateDir); err != nil {
		return nil, devcontainer.ResolvedConfig{}, capdot.Config{}, nil, workspaceplan.WorkspacePlan{}, err
	}
	tracker := NewStateTracker(resolved.StateDir, session.State())
	return session, resolved, dotfiles, tracker, workspacePlan, nil
}

func (e *Executor) prepareUpRuntime(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, resolved devcontainer.ResolvedConfig, events ui.Sink) (devcontainer.ResolvedConfig, string, ImagePlan, *bridge.Session, error) {
	e.emitPhaseProgress(events, phaseImage, "Reconciling container image")
	image, imagePlan, err := e.ReconcileImage(ctx, workspacePlan, resolved, events)
	if err != nil {
		return devcontainer.ResolvedConfig{}, "", ImagePlan{}, nil, err
	}
	e.emitPhaseProgress(events, phaseImage, "Applying runtime metadata")
	if err := e.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		return devcontainer.ResolvedConfig{}, "", ImagePlan{}, nil, err
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return devcontainer.ResolvedConfig{}, "", ImagePlan{}, nil, err
		}
	}
	bridgeSession, err := e.prepareUpBridge(ctx, resolved.StateDir, image, workspacePlan.Capabilities.Bridge.Enabled, events)
	if err != nil {
		return devcontainer.ResolvedConfig{}, "", ImagePlan{}, nil, err
	}
	if bridgeSession != nil {
		resolved.Merged = bridgecap.Inject(bridgeSession, resolved.Merged)
	}
	return resolved, image, imagePlan, bridgeSession, nil
}

func (e *Executor) prepareUpBridge(ctx context.Context, stateDir string, image string, enabled bool, events ui.Sink) (*bridge.Session, error) {
	e.emitPhaseProgress(events, phaseBridge, "Configuring bridge support")
	if !enabled {
		return nil, nil
	}
	helperArch, err := e.InspectImageArchitecture(ctx, image)
	if err != nil {
		return nil, err
	}
	return bridgecap.Prepare(stateDir, helperArch, e.engine.ID(), e.engine.BridgeHost())
}

func (e *Executor) reconcileUpContainer(ctx context.Context, session *Session, tracker *StateTracker, workspacePlan workspaceplan.WorkspacePlan, resolved devcontainer.ResolvedConfig, image string, imagePlan ImagePlan, recreate bool, events ui.Sink) (string, string, bool, error) {
	e.emitPhaseProgress(events, phaseContainer, "Reconciling managed container")
	containerID, containerKey, created, err := e.ReconcileContainer(ctx, session.Observed(), resolved, image, imagePlan, workspacePlan.Capabilities.Bridge.Enabled, workspacePlan.Capabilities.SSHAgent.Enabled, recreate, events)
	if err != nil {
		return "", "", false, err
	}
	tracker.ApplyContainer(containerID, containerKey, created)
	if err := tracker.Persist(); err != nil {
		return "", "", false, err
	}
	session.SetState(tracker.State())
	session.SetContainerID(containerID)
	inspect, err := e.engine.InspectContainer(ctx, containerID)
	if err != nil {
		return "", "", false, err
	}
	session.SetContainerInspect(&inspect)
	e.emitPhaseProgress(events, phaseContainer, "Reconciling container user")
	if err := e.EnsureUpdatedUIDContainer(ctx, resolved, image, containerID, events); err != nil {
		return "", "", false, err
	}
	return containerID, containerKey, created, nil
}

func (e *Executor) startUpBridge(ctx context.Context, tracker *StateTracker, bridgeSession *bridge.Session, containerKey string, containerID string, events ui.Sink) (*bridge.Report, error) {
	if bridgeSession == nil {
		return nil, nil
	}
	tracker.BeginBridge("start", containerKey)
	if err := tracker.Persist(); err != nil {
		return nil, err
	}
	e.emitPhaseProgress(events, phaseBridge, "Starting bridge session")
	startedBridge, err := bridgecap.Start(bridgeSession, containerID)
	if err != nil {
		return nil, err
	}
	bridgeReport := bridge.ReportFromSession(startedBridge)
	tracker.EnableBridge(bridgeReport.ID)
	if err := tracker.Persist(); err != nil {
		return nil, err
	}
	return bridgeReport, nil
}

func (e *Executor) runUpLifecycle(ctx context.Context, session *Session, tracker *StateTracker, workspacePlan workspaceplan.WorkspacePlan, dotfiles capdot.Config, containerKey string, created bool, events ui.Sink) error {
	state := session.State()
	lifecycleKey, err := LifecycleKey(session.Resolved(), containerKey, dotfiles)
	if err != nil {
		return err
	}
	observed := session.Observed()
	lifecyclePlan := PlanUpLifecycle(observed, DesiredLifecycle{Key: lifecycleKey, Dotfiles: dotfiles, Created: created})
	tracker.BeginPlannedLifecycle(lifecyclePlan, DotfilesNeedsInstall(state, dotfiles))
	if err := tracker.Persist(); err != nil {
		return err
	}
	e.emitPhaseProgress(events, phaseLifecycle, "Running lifecycle commands")
	if err := e.RunLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, events, lifecyclePlan); err != nil {
		return err
	}
	tracker.CompletePlannedLifecycle(lifecyclePlan, dotfiles, DotfilesNeedsInstall(state, dotfiles))
	return nil
}

func (e *Executor) Build(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts BuildOptions) (BuildResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	workspacePlan, resolved, err := executor.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return BuildResult{}, err
	}
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return BuildResult{}, err
	}
	if err := executor.ensureBackendSupport(resolved, false); err != nil {
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
	if err := persistTrustedRefs(resolved.StateDir, executor.imageVerifier.TrustedRefs()); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (e *Executor) Exec(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts ExecOptions) (int, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	workspacePlan, resolved, err := executor.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return 0, err
	}
	if err := executor.ensureBackendSupport(resolved, false); err != nil {
		return 0, err
	}
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, FindContainer: true, InspectContainer: true})
	if err != nil {
		return 0, err
	}
	resolved = session.Resolved()
	if err := executor.enrichExecResolvedConfig(ctx, session, &resolved); err != nil {
		return 0, err
	}
	session.SetResolved(resolved)
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
	req, err := executor.ExecRequest(ctx, session.Observed(), opts.IO.Stdin != nil, interactive, opts.RemoteEnv, opts.Args, backend.Streams{Stdin: opts.IO.Stdin, Stdout: opts.IO.Stdout, Stderr: opts.IO.Stderr})
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
	if code, ok := backend.ExitCode(err); ok {
		return code, nil
	}
	return 0, err
}

func (e *Executor) enrichExecResolvedConfig(ctx context.Context, session *Session, resolved *devcontainer.ResolvedConfig) error {
	if session == nil || resolved == nil {
		return nil
	}
	if inspect := session.ContainerInspect(); inspect != nil {
		metadata, err := spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
		if err != nil {
			return err
		}
		if len(metadata) > 0 {
			metadata, err = e.mergeSourceImageMetadata(ctx, *resolved, inspect.Image, metadata)
			if err != nil {
				return err
			}
			resolved.Merged = spec.MergeMetadata(spec.Config{}, metadata)
			resolved.Merged.Config = resolved.Config
			return nil
		}
		if inspect.Image != "" {
			return e.EnrichMergedConfig(ctx, resolved, inspect.Image)
		}
	}
	return e.EnrichMergedConfig(ctx, resolved, session.Image())
}

func (e *Executor) ReadConfig(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts ReadConfigOptions) (ReadConfigResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	dotfiles, err := capdot.Normalize(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return ReadConfigResult{}, err
	}
	workspacePlan, resolved, err := executor.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseConfig, "Inspecting resolved configuration")
	if err != nil {
		return ReadConfigResult{}, err
	}
	if err := executor.ensureBackendSupport(resolved, false); err != nil {
		return ReadConfigResult{}, err
	}
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, LoadState: true, FindContainer: true, AllowMissingContainer: true, InspectContainer: true})
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved = session.Resolved()
	image := session.Image()
	state := session.State()
	if err := executor.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		if !backend.IsNotFound(err) {
			return ReadConfigResult{}, err
		}
		resolved.Merged = spec.MergeMetadata(resolved.Config, featureMetadata(resolved.Features))
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return ReadConfigResult{}, err
		}
	}
	var bridgeReport *bridge.Report
	if state.BridgeEnabled {
		report, err := bridgecap.Doctor(resolved.StateDir)
		if err != nil {
			return ReadConfigResult{}, err
		}
		resolved.Merged = bridgecap.Inject(reportSession(report), resolved.Merged)
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
	managedContainer, err := session.ManagedContainer(ctx)
	if err != nil {
		return ReadConfigResult{}, err
	}
	return ReadConfigResult{WorkspaceFolder: resolved.WorkspaceFolder, ConfigPath: resolved.ConfigPath, WorkspaceMount: resolved.WorkspaceMount, SourceKind: resolved.SourceKind, HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(), HasCreateCommand: len(resolved.Merged.OnCreateCommands) > 0 || len(resolved.Merged.UpdateContentCommands) > 0 || len(resolved.Merged.PostCreateCommands) > 0, HasStartCommand: len(resolved.Merged.PostStartCommands) > 0, HasAttachCommand: len(resolved.Merged.PostAttachCommands) > 0, Image: image, ImageUser: imageUser, ContainerName: resolved.ContainerName, StateDir: resolved.StateDir, CacheDir: resolved.CacheDir, RemoteUser: resolvedUser, ContainerUser: resolved.Merged.ContainerUser, RemoteEnv: RedactSensitiveMap(resolved.Merged.RemoteEnv), ContainerEnv: RedactSensitiveMap(resolved.Merged.ContainerEnv), Mounts: resolved.Merged.Mounts, ForwardPorts: []string(resolved.Merged.ForwardPorts), Bridge: bridgeReport, Dotfiles: DotfilesStatusFromState(state, dotfiles), MetadataCount: len(resolved.Merged.Metadata), ManagedContainer: managedContainer}, nil
}

func persistTrustedRefs(stateDir string, refs []string) error {
	if stateDir == "" {
		return nil
	}
	state, err := storefs.ReadWorkspaceState(stateDir)
	if err != nil {
		return err
	}
	state.TrustedRefs = refs
	return storefs.WriteWorkspaceState(stateDir, state)
}

func (e *Executor) RunLifecycle(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	dotfiles, err := capdot.Normalize(workspacePlan.Preferences.Dotfiles)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	workspacePlan, resolved, err := executor.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return RunLifecycleResult{}, err
	}
	if err := executor.ensureBackendSupport(resolved, false); err != nil {
		return RunLifecycleResult{}, err
	}
	session, err := executor.PrepareObservedSession(ctx, ObservedSessionOptions{Plan: workspacePlan, Resolved: resolved, Debug: opts.Debug, Events: opts.IO.Events, Enrich: true, FindContainer: true, LoadState: true})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved = session.Resolved()
	state := session.State()
	observed := session.Observed()
	phase := opts.Phase
	lifecycleKey, err := LifecycleKey(resolved, state.ContainerKey, dotfiles)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	lifecyclePlan, err := PlanLifecycleCommand(observed, DesiredLifecycle{Key: lifecycleKey, Requested: phase, Dotfiles: dotfiles})
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
		tracker.CompletePlannedLifecycle(lifecyclePlan, dotfiles, DotfilesNeedsInstall(state, dotfiles))
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	return RunLifecycleResult{ContainerID: session.ContainerID(), Phase: phase}, nil
}

func (e *Executor) BridgeDoctor(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, opts BridgeDoctorOptions) (bridge.Report, error) {
	executor := e.cloneWithIO(opts.IO.Stdin, opts.IO.Stdout, opts.IO.Stderr)
	_, resolved, err := executor.materializeWorkspace(ctx, workspacePlan, opts.Debug, opts.IO.Events, phaseBridge, "Inspecting bridge state")
	if err != nil {
		return bridge.Report{}, err
	}
	return bridgecap.Doctor(resolved.StateDir)
}

func (e *Executor) materializeWorkspace(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, debug bool, events ui.Sink, phase string, label string) (workspaceplan.WorkspacePlan, devcontainer.ResolvedConfig, error) {
	resolved, err := e.Materialize(ctx, workspacePlan, debug, events, phase, label)
	if err != nil {
		return workspaceplan.WorkspacePlan{}, devcontainer.ResolvedConfig{}, err
	}
	workspacePlan = workspacePlan.WithResolved(resolved)
	return workspacePlan, resolved, nil
}

func reportSession(report bridge.Report) *bridge.Session {
	if !report.Enabled {
		return nil
	}
	return &bridge.Session{
		ID:         report.ID,
		Backend:    report.Backend,
		Enabled:    report.Enabled,
		HelperArch: report.HelperArch,
		Host:       report.Host,
		Port:       report.Port,
		StatePath:  report.StatePath,
		ConfigPath: report.ConfigPath,
		PIDPath:    report.PIDPath,
		StatusPath: report.StatusPath,
		HelperPath: report.HelperPath,
		MountPath:  report.MountPath,
		BinPath:    report.BinPath,
		Status:     report.Status,
	}
}
