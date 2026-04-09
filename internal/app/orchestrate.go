package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	"github.com/lauritsk/hatchctl/internal/runtime"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

const (
	phaseResolve   = "Resolve"
	phaseImage     = "Image"
	phaseContainer = "Container"
	phaseBridge    = "Bridge"
	phaseLifecycle = "Lifecycle"
	phaseState     = "State"
	phaseExec      = "Exec"
	phaseConfig    = "Config"
)

func (s *Service) upWithExecutor(ctx context.Context, req UpRequest, workspacePlan workspaceplan.WorkspacePlan) (UpResult, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	dotfiles, err := req.Defaults.Dotfiles.runtime().Normalized()
	if err != nil {
		return UpResult{}, err
	}
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return UpResult{}, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:                  workspacePlan,
		Resolved:              resolved,
		Debug:                 req.Global.Debug,
		Events:                req.IO.Events,
		LoadState:             true,
		AllowMissingContainer: true,
		InspectContainer:      true,
	})
	if err != nil {
		return UpResult{}, err
	}
	resolved = session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return UpResult{}, err
	}
	if err := os.MkdirAll(resolved.StateDir, 0o700); err != nil {
		return UpResult{}, err
	}
	state := session.State()
	tracker := runtime.NewStateTracker(resolved.StateDir, state)
	runner.EmitPhaseProgress(req.IO.Events, phaseImage, "Reconciling container image")
	image, imagePlan, err := runner.ReconcileImage(ctx, workspacePlan, resolved, req.IO.Events)
	if err != nil {
		return UpResult{}, err
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseImage, "Applying runtime metadata")
	if err := runner.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = runtime.InjectSSHAgent(resolved.Merged); err != nil {
			return UpResult{}, err
		}
	}
	helperArch, err := runner.InspectImageArchitecture(ctx, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeSession *bridge.Session
	runner.EmitPhaseProgress(req.IO.Events, phaseBridge, "Configuring bridge support")
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
	runner.EmitPhaseProgress(req.IO.Events, phaseContainer, "Reconciling managed container")
	containerID, containerKey, created, err := runner.ReconcileContainer(ctx, session.Observed(), resolved, image, imagePlan, workspacePlan.Capabilities.Bridge.Enabled, workspacePlan.Capabilities.SSHAgent.Enabled, req.Recreate, req.IO.Events)
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
	inspect, err := runner.InspectContainer(ctx, containerID)
	if err != nil {
		return UpResult{}, err
	}
	session.SetContainerInspect(&inspect)
	runner.EmitPhaseProgress(req.IO.Events, phaseContainer, "Reconciling container user")
	if err := runner.EnsureUpdatedUIDContainer(ctx, resolved, image, containerID, req.IO.Events); err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	if bridgeSession != nil {
		tracker.BeginBridge("start", containerKey)
		if err := tracker.Persist(); err != nil {
			return UpResult{}, err
		}
		runner.EmitPhaseProgress(req.IO.Events, phaseBridge, "Starting bridge session")
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
	lifecycleKey, err := runner.DesiredLifecycleKey(resolved, containerKey, dotfiles)
	if err != nil {
		return UpResult{}, err
	}
	observed := session.Observed()
	lifecyclePlan := reconcile.PlanUpLifecycle(observed, reconcile.DesiredLifecycle{Key: lifecycleKey, Dotfiles: reconcile.DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}, Created: created})
	tracker.BeginLifecycle(string(lifecyclePlan.TransitionKind), lifecycleKey)
	if lifecyclePlan.RunCreate && runtime.DotfilesNeedsInstall(state, dotfiles) {
		tracker.BeginDotfiles("install", lifecycleKey)
	}
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseLifecycle, "Running lifecycle commands")
	if err := runner.RunLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, req.IO.Events, lifecyclePlan); err != nil {
		return UpResult{}, err
	}
	if lifecyclePlan.RunCreate && runtime.DotfilesNeedsInstall(state, dotfiles) {
		tracker.CompleteDotfiles(dotfiles)
	}
	tracker.CompleteLifecycle(lifecycleKey, dotfiles)
	if bridgeReport == nil {
		tracker.SetBridge(false, "")
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseState, "Writing workspace state")
	if err := tracker.Persist(); err != nil {
		return UpResult{}, err
	}
	return UpResult{ContainerID: containerID, Image: image, RemoteWorkspaceFolder: resolved.RemoteWorkspace, StateDir: resolved.StateDir, Bridge: bridgeReport}, nil
}

func (s *Service) buildWithExecutor(ctx context.Context, req BuildRequest, workspacePlan workspaceplan.WorkspacePlan) (BuildResult, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return BuildResult{}, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:     workspacePlan,
		Resolved: resolved,
		Debug:    req.Global.Debug,
		Events:   req.IO.Events,
	})
	if err != nil {
		return BuildResult{}, err
	}
	resolved = session.Resolved()
	if err := policy.EnsureWorkspaceTrust(resolved, workspacePlan.Trust.WorkspaceAllowed); err != nil {
		return BuildResult{}, err
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseImage, "Reconciling container image")
	image, _, err := runner.ReconcileImage(ctx, workspacePlan, resolved, req.IO.Events)
	if err != nil {
		return BuildResult{}, err
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseImage, "Applying runtime metadata")
	if err := runner.EnrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (s *Service) execWithExecutor(ctx context.Context, req ExecRequest, workspacePlan workspaceplan.WorkspacePlan) (int, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return 0, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:             workspacePlan,
		Resolved:         resolved,
		Debug:            req.Global.Debug,
		Events:           req.IO.Events,
		Enrich:           true,
		FindContainer:    true,
		InspectContainer: true,
	})
	if err != nil {
		return 0, err
	}
	resolved = session.Resolved()
	if owner := session.Observed().Control.Coordination.ActiveOwner; owner != nil {
		return 0, &storefs.WorkspaceBusyError{StateDir: resolved.StateDir, Owner: owner}
	}
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = runtime.InjectSSHAgent(resolved.Merged); err != nil {
			return 0, err
		}
		session.SetResolved(resolved)
		if err := runtime.EnsureContainerHasSSHAgent(session.ContainerInspect(), runtime.SSHAgentContainerSocketPath()); err != nil {
			return 0, err
		}
	}
	if err := session.RevalidateReadTarget(ctx); err != nil {
		return 0, err
	}
	interactive := runtime.ShouldAllocateTTY(req.IO.Stdin, req.IO.Stdout)
	execReq, err := runner.DockerExecRequest(ctx, session.Observed(), req.IO.Stdin != nil, interactive, req.RemoteEnv, req.Args, dockercli.Streams{Stdin: req.IO.Stdin, Stdout: req.IO.Stdout, Stderr: req.IO.Stderr})
	if err != nil {
		return 0, err
	}
	if interactive {
		runner.ClearProgress(req.IO.Events)
	} else {
		runner.EmitPhaseProgress(req.IO.Events, phaseExec, fmt.Sprintf("Executing command in %s", session.ContainerID()))
	}
	err = runner.ExecuteContainerCommand(ctx, execReq)
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

func (s *Service) readConfigWithExecutor(ctx context.Context, req ReadConfigRequest, workspacePlan workspaceplan.WorkspacePlan) (ReadConfigResult, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	dotfiles, err := req.Defaults.Dotfiles.runtime().Normalized()
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseConfig, "Inspecting resolved configuration")
	if err != nil {
		return ReadConfigResult{}, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:                  workspacePlan,
		Resolved:              resolved,
		Debug:                 req.Global.Debug,
		Events:                req.IO.Events,
		Enrich:                true,
		LoadState:             true,
		FindContainer:         true,
		AllowMissingContainer: true,
		InspectContainer:      true,
	})
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved = session.Resolved()
	image := session.Image()
	state := session.State()
	if workspacePlan.Capabilities.SSHAgent.Enabled {
		if resolved.Merged, err = runtime.InjectSSHAgent(resolved.Merged); err != nil {
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
	imageUser, err := runner.InspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := session.ManagedContainer()
	if err != nil {
		return ReadConfigResult{}, err
	}
	return ReadConfigResult{WorkspaceFolder: resolved.WorkspaceFolder, ConfigPath: resolved.ConfigPath, WorkspaceMount: resolved.WorkspaceMount, SourceKind: resolved.SourceKind, HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(), HasCreateCommand: len(resolved.Merged.OnCreateCommands) > 0 || len(resolved.Merged.UpdateContentCommands) > 0 || len(resolved.Merged.PostCreateCommands) > 0, HasStartCommand: len(resolved.Merged.PostStartCommands) > 0, HasAttachCommand: len(resolved.Merged.PostAttachCommands) > 0, Image: image, ImageUser: imageUser, ContainerName: resolved.ContainerName, StateDir: resolved.StateDir, CacheDir: resolved.CacheDir, RemoteUser: resolvedUser, ContainerUser: resolved.Merged.ContainerUser, RemoteEnv: runtime.RedactSensitiveMap(resolved.Merged.RemoteEnv), ContainerEnv: runtime.RedactSensitiveMap(resolved.Merged.ContainerEnv), Mounts: resolved.Merged.Mounts, ForwardPorts: []string(resolved.Merged.ForwardPorts), Bridge: bridgeReport, Dotfiles: runtime.DotfilesStatusFromState(state, dotfiles), MetadataCount: len(resolved.Merged.Metadata), ManagedContainer: managedContainer}, nil
}

func (s *Service) runLifecycleWithExecutor(ctx context.Context, req RunLifecycleRequest, workspacePlan workspaceplan.WorkspacePlan) (RunLifecycleResult, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	dotfiles, err := req.Defaults.Dotfiles.runtime().Normalized()
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseResolve, "Resolving development container")
	if err != nil {
		return RunLifecycleResult{}, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:          workspacePlan,
		Resolved:      resolved,
		Debug:         req.Global.Debug,
		Events:        req.IO.Events,
		Enrich:        true,
		FindContainer: true,
		LoadState:     true,
	})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved = session.Resolved()
	state := session.State()
	observed := session.Observed()
	phase := strings.ToLower(req.Phase)
	if phase == "" {
		phase = "all"
	}
	lifecycleKey, err := runner.DesiredLifecycleKey(resolved, state.ContainerKey, dotfiles)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	lifecyclePlan := reconcile.PlanLifecycleCommand(observed, reconcile.DesiredLifecycle{Key: lifecycleKey, Requested: phase, Dotfiles: reconcile.DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath}})
	tracker := runtime.NewStateTracker(resolved.StateDir, state)
	if lifecyclePlan.RunCreate {
		tracker.BeginLifecycle(string(lifecyclePlan.TransitionKind), lifecyclePlan.Key)
		if runtime.DotfilesNeedsInstall(state, dotfiles) {
			tracker.BeginDotfiles("install", lifecycleKey)
		}
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	runner.EmitPhaseProgress(req.IO.Events, phaseLifecycle, "Running lifecycle commands")
	if err := runner.RunLifecyclePlan(ctx, observed, state, dotfiles, workspacePlan.Trust.HostLifecycleAllowed, req.IO.Events, lifecyclePlan); err != nil {
		return RunLifecycleResult{}, err
	}
	if lifecyclePlan.RunCreate {
		if runtime.DotfilesNeedsInstall(state, dotfiles) {
			tracker.CompleteDotfiles(dotfiles)
		}
		tracker.CompleteLifecycle(lifecycleKey, dotfiles)
		if err := tracker.Persist(); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	return RunLifecycleResult{ContainerID: session.ContainerID(), Phase: phase}, nil
}

func (s *Service) bridgeDoctorWithExecutor(ctx context.Context, req BridgeDoctorRequest, workspacePlan workspaceplan.WorkspacePlan) (bridge.Report, error) {
	runner := s.executor.CloneWithIO(req.IO.Stdin, req.IO.Stdout, req.IO.Stderr)
	resolved, err := s.materializeResolved(ctx, runner, workspacePlan, req.Global.Debug, req.IO.Events, phaseBridge, "Inspecting bridge state")
	if err != nil {
		return bridge.Report{}, err
	}
	session, err := runner.PrepareObservedSession(ctx, runtime.ObservedSessionOptions{
		Plan:     workspacePlan,
		Resolved: resolved,
		Debug:    req.Global.Debug,
		Events:   req.IO.Events,
	})
	if err != nil {
		return bridge.Report{}, err
	}
	return bridgecap.Doctor(session.Resolved().StateDir)
}

func (s *Service) materializeResolved(ctx context.Context, runner *runtime.Runner, workspacePlan workspaceplan.WorkspacePlan, debug bool, events ui.Sink, phase string, label string) (devcontainer.ResolvedConfig, error) {
	runner.EmitPhaseProgress(events, phase, label)
	resolved, err := workspaceplan.NewResolver().Materialize(ctx, workspacePlan, runner.VerificationCheck())
	if err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if err := runner.VerifyResolvedFeatures(resolved, events); err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if debug {
		runner.EmitPlan(events, resolved)
	}
	return resolved, nil
}
