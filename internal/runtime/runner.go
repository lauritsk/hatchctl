package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type Runner struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	backend       runtimeBackend
	imageVerifier imageVerificationPolicy
	planner       *workspacePlanner
}

func NewRunner(client *docker.Client) *Runner {
	return NewRunnerWithIO(client, os.Stdin, os.Stdout, os.Stderr)
}

func NewRunnerWithIO(client *docker.Client, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Runner {
	runner := &Runner{
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		imageVerifier: newImageVerificationPolicy(stdin, stderr),
	}
	runner.backend = newLocalRuntimeBackend(runner, client)
	runner.planner = &workspacePlanner{runner: runner}
	return runner
}

type UpOptions struct {
	Workspace          string
	ConfigPath         string
	StateDir           string
	CacheDir           string
	FeatureTimeout     time.Duration
	LockfilePolicy     devcontainer.FeatureLockfilePolicy
	Dotfiles           DotfilesOptions
	AllowHostLifecycle bool
	TrustWorkspace     bool
	SSHAgent           bool
	Recreate           bool
	BridgeEnabled      bool
	Verbose            bool
	Debug              bool
	Events             ui.Sink
	Stdout             io.Writer
	Stderr             io.Writer
}

type UpResult struct {
	ContainerID           string         `json:"containerId"`
	Image                 string         `json:"image"`
	RemoteWorkspaceFolder string         `json:"remoteWorkspaceFolder"`
	StateDir              string         `json:"stateDir"`
	Bridge                *bridge.Report `json:"bridge,omitempty"`
}

type BuildOptions struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	TrustWorkspace bool
	Verbose        bool
	Debug          bool
	Events         ui.Sink
	Stdout         io.Writer
	Stderr         io.Writer
}

type BuildResult struct {
	Image string `json:"image"`
}

type ExecOptions struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	SSHAgent       bool
	Verbose        bool
	Debug          bool
	Events         ui.Sink
	Args           []string
	RemoteEnv      map[string]string
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
}

type ReadConfigOptions struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	SSHAgent       bool
	Dotfiles       DotfilesOptions
	Verbose        bool
	Debug          bool
	Events         ui.Sink
	Stdout         io.Writer
	Stderr         io.Writer
}

type ReadConfigResult struct {
	WorkspaceFolder      string            `json:"workspaceFolder"`
	ConfigPath           string            `json:"configPath"`
	WorkspaceMount       string            `json:"workspaceMount"`
	SourceKind           string            `json:"sourceKind"`
	HasInitializeCommand bool              `json:"hasInitializeCommand"`
	HasCreateCommand     bool              `json:"hasCreateCommand"`
	HasStartCommand      bool              `json:"hasStartCommand"`
	HasAttachCommand     bool              `json:"hasAttachCommand"`
	Image                string            `json:"image"`
	ImageUser            string            `json:"imageUser,omitempty"`
	ContainerName        string            `json:"containerName"`
	StateDir             string            `json:"stateDir"`
	CacheDir             string            `json:"cacheDir"`
	RemoteUser           string            `json:"remoteUser,omitempty"`
	ContainerUser        string            `json:"containerUser,omitempty"`
	RemoteEnv            map[string]string `json:"remoteEnv,omitempty"`
	ContainerEnv         map[string]string `json:"containerEnv,omitempty"`
	Mounts               []string          `json:"mounts,omitempty"`
	ForwardPorts         []string          `json:"forwardPorts,omitempty"`
	Bridge               *bridge.Report    `json:"bridge,omitempty"`
	Dotfiles             *DotfilesStatus   `json:"dotfiles,omitempty"`
	MetadataCount        int               `json:"metadataCount"`
	ManagedContainer     *ManagedContainer `json:"managedContainer,omitempty"`
}

type preparedWorkspace struct {
	resolved         devcontainer.ResolvedConfig
	image            string
	state            devcontainer.State
	containerID      string
	containerInspect *docker.ContainerInspect
}

type prepareWorkspaceOptions struct {
	resolve               prepareResolveOptions
	enrich                bool
	loadState             bool
	findContainer         bool
	allowMissingContainer bool
	inspectContainer      bool
}

type ManagedContainer struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	Image         string            `json:"image,omitempty"`
	Status        string            `json:"status,omitempty"`
	Running       bool              `json:"running"`
	RemoteUser    string            `json:"remoteUser,omitempty"`
	ContainerEnv  map[string]string `json:"containerEnv,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	ForwardPorts  []string          `json:"forwardPorts,omitempty"`
	MetadataCount int               `json:"metadataCount"`
	BridgeEnabled bool              `json:"bridgeEnabled,omitempty"`
}

const (
	phaseResolve   = "Resolve"
	phaseImage     = "Image"
	phaseContainer = "Container"
	phaseBridge    = "Bridge"
	phaseLifecycle = "Lifecycle"
	phaseState     = "State"
	phaseExec      = "Exec"
	phaseConfig    = "Config"
	phaseDotfiles  = "Dotfiles"
)

type RunLifecycleOptions struct {
	Workspace          string
	ConfigPath         string
	StateDir           string
	CacheDir           string
	FeatureTimeout     time.Duration
	LockfilePolicy     devcontainer.FeatureLockfilePolicy
	Dotfiles           DotfilesOptions
	AllowHostLifecycle bool
	Verbose            bool
	Debug              bool
	Events             ui.Sink
	Phase              string
	Stdout             io.Writer
	Stderr             io.Writer
}

type RunLifecycleResult struct {
	ContainerID string `json:"containerId"`
	Phase       string `json:"phase"`
}

func (r *Runner) verifyImageReference(ctx context.Context, ref string, events ui.Sink) error {
	return r.imageVerifier.ApplyImage(r.imageVerifier.Check(ctx, ref), events)
}

func (r *Runner) verifyResolvedFeatures(resolved devcontainer.ResolvedConfig, events ui.Sink) error {
	for _, feature := range resolved.Features {
		allowUnverified := feature.SourceKind == "oci" && (allowInsecureFeatureVerification() || isLoopbackOCIReference(feature.Resolved))
		if err := r.imageVerifier.ApplyFeature(feature.Source, feature.Verification, allowUnverified, events); err != nil {
			return err
		}
	}
	return nil
}

type BridgeDoctorOptions struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Verbose        bool
	Debug          bool
	Events         ui.Sink
	Stdout         io.Writer
	Stderr         io.Writer
}

type ExitError struct {
	Code int
}

type prepareResolveOptions struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	ReadOnly       bool
	ProgressPhase  string
	ProgressLabel  string
	Debug          bool
	Events         ui.Sink
}

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (r *Runner) Up(ctx context.Context, opts UpOptions) (UpResult, error) {
	runner := r.withCommandIO(commandIO{Stdin: r.stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return UpResult{}, err
	}
	prepared, err := runner.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ProgressPhase:  phaseResolve,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, loadState: true})
	if err != nil {
		return UpResult{}, err
	}
	resolved := prepared.resolved
	if err := ensureWorkspaceTrust(resolved, opts.TrustWorkspace); err != nil {
		return UpResult{}, err
	}
	if err := ensureDir(resolved.StateDir); err != nil {
		return UpResult{}, err
	}
	state := prepared.state

	if opts.Recreate && state.ContainerID != "" {
		runner.emitPhaseProgress(opts.Events, phaseContainer, "Recreating managed container")
		_ = runner.removeContainer(ctx, state.ContainerID, opts.Events)
		state = devcontainer.State{}
	}

	runner.emitPhaseProgress(opts.Events, phaseImage, "Ensuring container image")
	image, err := runner.ensureImage(ctx, resolved, opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	runner.emitPhaseProgress(opts.Events, phaseImage, "Applying runtime metadata")
	if err := runner.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	if opts.SSHAgent {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return UpResult{}, err
		}
	}
	helperArch, err := runner.inspectImageArchitecture(ctx, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeSession *bridge.Session
	runner.emitPhaseProgress(opts.Events, phaseBridge, "Configuring bridge support")
	if opts.BridgeEnabled {
		bridgeSession, err = bridge.Prepare(resolved.StateDir, true, helperArch)
		if err == nil {
			resolved.Merged = bridge.Inject(bridgeSession, resolved.Merged)
		}
	} else {
		bridgeSession = nil
	}
	if err != nil {
		return UpResult{}, err
	}
	overridePath := ""
	if resolved.SourceKind == "compose" {
		runner.emitPhaseProgress(opts.Events, phaseContainer, "Preparing Compose override")
		overridePath, err = writeComposeOverride(resolved, image)
		if err != nil {
			return UpResult{}, err
		}
		defer os.Remove(overridePath)
	}

	runner.emitPhaseProgress(opts.Events, phaseContainer, "Ensuring managed container")
	containerID, created, err := runner.ensureContainer(ctx, resolved, image, opts.BridgeEnabled, opts.SSHAgent, overridePath, opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	runner.emitPhaseProgress(opts.Events, phaseContainer, "Reconciling container user")
	if err := runner.ensureUpdatedUIDContainer(ctx, resolved, image, containerID, opts.Events); err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	if bridgeSession != nil {
		runner.emitPhaseProgress(opts.Events, phaseBridge, "Starting bridge session")
		startedBridge, err := bridge.Start(bridgeSession, containerID)
		if err != nil {
			return UpResult{}, err
		}
		bridgeReport = bridge.ReportFromSession(startedBridge)
	}

	runner.emitPhaseProgress(opts.Events, phaseLifecycle, "Running lifecycle commands")
	if err := runner.runLifecycleForUp(ctx, resolved, containerID, created, state, dotfiles, opts.AllowHostLifecycle, opts.Events); err != nil {
		return UpResult{}, err
	}

	state.ContainerID = containerID
	state.LifecycleReady = true
	state.BridgeEnabled = opts.BridgeEnabled
	state.DotfilesReady = dotfiles.Enabled()
	state.DotfilesRepo = dotfiles.Repository
	state.DotfilesInstall = dotfiles.InstallCommand
	state.DotfilesTarget = dotfiles.TargetPath
	if bridgeReport != nil {
		state.BridgeSessionID = bridgeReport.ID
	}
	runner.emitPhaseProgress(opts.Events, phaseState, "Writing workspace state")
	if err := devcontainer.WriteState(resolved.StateDir, state); err != nil {
		return UpResult{}, err
	}

	return UpResult{
		ContainerID:           containerID,
		Image:                 image,
		RemoteWorkspaceFolder: resolved.RemoteWorkspace,
		StateDir:              resolved.StateDir,
		Bridge:                bridgeReport,
	}, nil
}

func (r *Runner) Build(ctx context.Context, opts BuildOptions) (BuildResult, error) {
	runner := r.withCommandIO(commandIO{Stdin: r.stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
	prepared, err := runner.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ProgressPhase:  phaseResolve,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}})
	if err != nil {
		return BuildResult{}, err
	}
	resolved := prepared.resolved
	if err := ensureWorkspaceTrust(resolved, opts.TrustWorkspace); err != nil {
		return BuildResult{}, err
	}
	runner.emitPhaseProgress(opts.Events, phaseImage, "Ensuring container image")
	image, err := runner.ensureImage(ctx, resolved, opts.Events)
	if err != nil {
		return BuildResult{}, err
	}
	runner.emitPhaseProgress(opts.Events, phaseImage, "Applying runtime metadata")
	if err := runner.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (r *Runner) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	prepared, err := r.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		ProgressPhase:  phaseResolve,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, enrich: true, findContainer: true, inspectContainer: true})
	if err != nil {
		return 0, err
	}
	resolved := prepared.resolved
	if opts.SSHAgent {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return 0, err
		}
		if err := ensureContainerHasSSHAgent(prepared.containerInspect, sshAgentContainerSocketPath); err != nil {
			return 0, err
		}
	}
	interactive := shouldAllocateTTY(opts.Stdin, opts.Stdout)
	args, err := r.dockerExecArgs(ctx, prepared.containerID, resolved, opts.Stdin != nil, interactive, opts.RemoteEnv, opts.Args)
	if err != nil {
		return 0, err
	}
	if interactive {
		r.clearProgress(opts.Events)
	} else {
		r.emitPhaseProgress(opts.Events, phaseExec, fmt.Sprintf("Executing command in %s", prepared.containerID))
	}

	err = r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: "Executing command", Args: args, Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
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

func (r *Runner) ReadConfig(ctx context.Context, opts ReadConfigOptions) (ReadConfigResult, error) {
	runner := r.withCommandIO(commandIO{Stdin: r.stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return ReadConfigResult{}, err
	}
	prepared, err := runner.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		ProgressPhase:  phaseConfig,
		ProgressLabel:  "Inspecting resolved configuration",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, enrich: true, loadState: true, findContainer: true, allowMissingContainer: true, inspectContainer: true})
	if err != nil {
		return ReadConfigResult{}, err
	}
	resolved := prepared.resolved
	image := prepared.image
	state := prepared.state
	if opts.SSHAgent {
		if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
			return ReadConfigResult{}, err
		}
	}
	var bridgeSession *bridge.Session
	if state.BridgeEnabled {
		bridgeSession, err = bridge.Preview(resolved.StateDir, true)
		if err == nil {
			resolved.Merged = bridge.Inject(bridgeSession, resolved.Merged)
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
		report, err := bridge.Doctor(resolved.StateDir)
		if err != nil {
			return ReadConfigResult{}, err
		}
		bridgeReport = &report
	}
	prepared.resolved = resolved
	resolvedUser, err := runner.effectiveRemoteUser(ctx, prepared)
	if err != nil {
		return ReadConfigResult{}, err
	}
	imageUser, err := runner.inspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := runner.readManagedContainerState(prepared)
	if err != nil {
		return ReadConfigResult{}, err
	}
	return ReadConfigResult{
		WorkspaceFolder:      resolved.WorkspaceFolder,
		ConfigPath:           resolved.ConfigPath,
		WorkspaceMount:       resolved.WorkspaceMount,
		SourceKind:           resolved.SourceKind,
		HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(),
		HasCreateCommand:     len(resolved.Merged.OnCreateCommands) > 0 || len(resolved.Merged.UpdateContentCommands) > 0 || len(resolved.Merged.PostCreateCommands) > 0,
		HasStartCommand:      len(resolved.Merged.PostStartCommands) > 0,
		HasAttachCommand:     len(resolved.Merged.PostAttachCommands) > 0,
		Image:                image,
		ImageUser:            imageUser,
		ContainerName:        resolved.ContainerName,
		StateDir:             resolved.StateDir,
		CacheDir:             resolved.CacheDir,
		RemoteUser:           resolvedUser,
		ContainerUser:        resolved.Merged.ContainerUser,
		RemoteEnv:            redactSensitiveMap(resolved.Merged.RemoteEnv),
		ContainerEnv:         redactSensitiveMap(resolved.Merged.ContainerEnv),
		Mounts:               resolved.Merged.Mounts,
		ForwardPorts:         []string(resolved.Merged.ForwardPorts),
		Bridge:               bridgeReport,
		Dotfiles:             dotfilesStatus(state, dotfiles),
		MetadataCount:        len(resolved.Merged.Metadata),
		ManagedContainer:     managedContainer,
	}, nil
}

func (r *Runner) RunLifecycle(ctx context.Context, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	runner := r.withCommandIO(commandIO{Stdin: r.stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return RunLifecycleResult{}, err
	}
	prepared, err := runner.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		ProgressPhase:  phaseResolve,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, enrich: true, findContainer: true, loadState: true})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	resolved := prepared.resolved
	state := prepared.state
	phase := strings.ToLower(opts.Phase)
	if phase == "" {
		phase = "all"
	}
	runner.emitPhaseProgress(opts.Events, phaseLifecycle, "Running lifecycle commands")
	runDotfiles := phase == "all" || phase == "create"
	if err := runner.runLifecyclePhase(ctx, resolved, prepared.containerID, phase, state, dotfiles, runDotfiles, opts.AllowHostLifecycle, opts.Events); err != nil {
		return RunLifecycleResult{}, err
	}
	if runDotfiles {
		state.DotfilesReady = dotfiles.Enabled()
		state.DotfilesRepo = dotfiles.Repository
		state.DotfilesInstall = dotfiles.InstallCommand
		state.DotfilesTarget = dotfiles.TargetPath
		if err := devcontainer.WriteState(resolved.StateDir, state); err != nil {
			return RunLifecycleResult{}, err
		}
	}
	return RunLifecycleResult{ContainerID: prepared.containerID, Phase: phase}, nil
}

func (r *Runner) BridgeDoctor(ctx context.Context, opts BridgeDoctorOptions) (bridge.Report, error) {
	runner := r.withCommandIO(commandIO{Stdin: r.stdin, Stdout: opts.Stdout, Stderr: opts.Stderr})
	prepared, err := runner.planner.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateDir:       opts.StateDir,
		CacheDir:       opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		ProgressPhase:  phaseBridge,
		ProgressLabel:  "Inspecting bridge state",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}})
	if err != nil {
		return bridge.Report{}, err
	}
	return bridge.Doctor(prepared.resolved.StateDir)
}

func preparedImage(resolved devcontainer.ResolvedConfig) string {
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	return image
}

func (r *Runner) emitProgress(events ui.Sink, message string) {
	r.emitPhaseProgress(events, "", message)
}

func (r *Runner) emitPhaseProgress(events ui.Sink, phase string, message string) {
	if events == nil || message == "" {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventProgress, Phase: phase, Message: message})
}

func (r *Runner) clearProgress(events ui.Sink) {
	if events == nil {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventClear})
}

func (r *Runner) emitPlan(events ui.Sink, resolved devcontainer.ResolvedConfig) {
	if events == nil {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventDebug, Message: fmt.Sprintf("plan source=%s config=%s workspace=%s state=%s target-image=%s", resolved.SourceKind, resolved.ConfigPath, resolved.WorkspaceFolder, resolved.StateDir, resolved.ImageName)})
}

func (r *Runner) commandIO() commandIO {
	return commandIO{Stdin: r.stdin, Stdout: r.stdout, Stderr: r.stderr}
}

func (r *Runner) withCommandIO(streams commandIO) *Runner {
	clone := *r
	if streams.Stdin == nil {
		streams.Stdin = r.stdin
	}
	if streams.Stdout == nil {
		streams.Stdout = r.stdout
	}
	if streams.Stderr == nil {
		streams.Stderr = r.stderr
	}
	clone.stdin = streams.Stdin
	clone.stdout = streams.Stdout
	clone.stderr = streams.Stderr
	if backend, ok := r.backend.(*localRuntimeBackend); ok {
		clone.backend = &localRuntimeBackend{runner: &clone, docker: backend.docker, hostCommand: backend.hostCommand}
	}
	clone.planner = &workspacePlanner{runner: &clone}
	return &clone
}

const updateUIDScript = `set -eu
REMOTE_USER=$1
NEW_UID=$2
NEW_GID=$3

OLD_UID=
OLD_GID=
HOME_FOLDER=
while IFS=: read -r name _ uid gid _ home _; do
  if [ "$name" = "$REMOTE_USER" ]; then
    OLD_UID=$uid
    OLD_GID=$gid
    HOME_FOLDER=$home
    break
  fi
done < /etc/passwd

EXISTING_USER=
while IFS=: read -r name _ uid _; do
  if [ "$uid" = "$NEW_UID" ]; then
    EXISTING_USER=$name
    break
  fi
done < /etc/passwd

EXISTING_GROUP=
while IFS=: read -r name _ gid _; do
  if [ "$gid" = "$NEW_GID" ]; then
    EXISTING_GROUP=$name
    break
  fi
done < /etc/group

if [ -z "$OLD_UID" ]; then
  echo "Remote user not found in /etc/passwd ($REMOTE_USER)."
elif [ "$OLD_UID" = "$NEW_UID" ] && [ "$OLD_GID" = "$NEW_GID" ]; then
  echo "UIDs and GIDs are the same ($NEW_UID:$NEW_GID)."
elif [ "$OLD_UID" != "$NEW_UID" ] && [ -n "$EXISTING_USER" ]; then
  echo "User with UID exists ($EXISTING_USER=$NEW_UID)."
else
  if [ "$OLD_GID" != "$NEW_GID" ] && [ -n "$EXISTING_GROUP" ]; then
    echo "Group with GID exists ($EXISTING_GROUP=$NEW_GID)."
    NEW_GID=$OLD_GID
  fi
  echo "Updating UID:GID from $OLD_UID:$OLD_GID to $NEW_UID:$NEW_GID."
  sed -i -e "s/\(${REMOTE_USER}:[^:]*:\)[^:]*:[^:]*/\1${NEW_UID}:${NEW_GID}/" /etc/passwd
  if [ "$OLD_GID" != "$NEW_GID" ]; then
    sed -i -e "s/\([^:]*:[^:]*:\)${OLD_GID}:/\1${NEW_GID}:/" /etc/group
  fi
  chown -R "$NEW_UID:$NEW_GID" "$HOME_FOLDER"
fi`

func shouldAllocateTTY(stdin io.Reader, stdout io.Writer) bool {
	if !isTerminalStream(stdin) {
		return false
	}
	if !isTerminalStream(stdout) {
		return false
	}
	return true
}

func isTerminalStream(stream any) bool {
	fd, ok := streamFileDescriptor(stream)
	if !ok {
		return false
	}
	return isCharacterDevice(fd)
}

func streamFileDescriptor(stream any) (uintptr, bool) {
	type fdStream interface {
		Fd() uintptr
	}
	f, ok := stream.(fdStream)
	if !ok {
		return 0, false
	}
	return f.Fd(), true
}

func isCharacterDevice(fd uintptr) bool {
	file := os.NewFile(fd, "")
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
