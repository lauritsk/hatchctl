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
	docker            *docker.Client
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
	hostCommandRunner hostCommandRunner
}

func NewRunner(client *docker.Client) *Runner {
	return NewRunnerWithIO(client, os.Stdin, os.Stdout, os.Stderr)
}

func NewRunnerWithIO(client *docker.Client, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Runner {
	return &Runner{
		docker:            client,
		stdin:             stdin,
		stdout:            stdout,
		stderr:            stderr,
		hostCommandRunner: defaultHostCommandRunner,
	}
}

type UpOptions struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Dotfiles       DotfilesOptions
	Recreate       bool
	BridgeEnabled  bool
	Verbose        bool
	Debug          bool
	Events         ui.Sink
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
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Verbose        bool
	Debug          bool
	Events         ui.Sink
}

type BuildResult struct {
	Image string `json:"image"`
}

type ExecOptions struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
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
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Dotfiles       DotfilesOptions
	Verbose        bool
	Debug          bool
	Events         ui.Sink
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

type RunLifecycleOptions struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Dotfiles       DotfilesOptions
	Verbose        bool
	Debug          bool
	Events         ui.Sink
	Phase          string
}

type RunLifecycleResult struct {
	ContainerID string `json:"containerId"`
	Phase       string `json:"phase"`
}

type BridgeDoctorOptions struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	Verbose        bool
	Debug          bool
	Events         ui.Sink
}

type ExitError struct {
	Code int
}

type prepareResolveOptions struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	LockfilePolicy devcontainer.FeatureLockfilePolicy
	ReadOnly       bool
	ProgressLabel  string
	Debug          bool
	Events         ui.Sink
}

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (r *Runner) Up(ctx context.Context, opts UpOptions) (UpResult, error) {
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return UpResult{}, err
	}
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, loadState: true})
	if err != nil {
		return UpResult{}, err
	}
	resolved := prepared.resolved
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return UpResult{}, err
	}
	state := prepared.state

	if opts.Recreate && state.ContainerID != "" {
		r.emitProgress(opts.Events, "Recreating managed container")
		_ = r.removeContainer(ctx, state.ContainerID, opts.Events)
		state = devcontainer.State{}
	}

	r.emitProgress(opts.Events, "Ensuring container image")
	image, err := r.ensureImage(ctx, resolved, opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	r.emitProgress(opts.Events, "Applying runtime metadata")
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	helperArch, err := r.inspectImageArchitecture(ctx, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	r.emitProgress(opts.Events, "Configuring bridge support")
	bridgeReport, err = r.applyBridgeConfig(&resolved, opts.BridgeEnabled, helperArch)
	if err != nil {
		return UpResult{}, err
	}
	overridePath := ""
	if resolved.SourceKind == "compose" {
		r.emitProgress(opts.Events, "Preparing Compose override")
		overridePath, err = writeComposeOverride(resolved, image)
		if err != nil {
			return UpResult{}, err
		}
		defer os.Remove(overridePath)
	}

	r.emitProgress(opts.Events, "Ensuring managed container")
	containerID, created, err := r.ensureContainer(ctx, resolved, image, opts.BridgeEnabled, overridePath, opts.Events)
	if err != nil {
		return UpResult{}, err
	}
	r.emitProgress(opts.Events, "Reconciling container user")
	if err := r.ensureUpdatedUIDContainer(ctx, resolved, image, containerID, opts.Events); err != nil {
		return UpResult{}, err
	}
	if bridgeReport != nil {
		r.emitProgress(opts.Events, "Starting bridge session")
		startedBridge, err := bridge.Start(resolved.StateDir, opts.BridgeEnabled, helperArch, containerID)
		if err != nil {
			return UpResult{}, err
		}
		bridgeReport = (*bridge.Report)(startedBridge)
	}

	r.emitProgress(opts.Events, "Running lifecycle commands")
	if err := r.runLifecycleForUp(ctx, resolved, containerID, created, state, dotfiles, opts.Events); err != nil {
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
	r.emitProgress(opts.Events, "Writing workspace state")
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
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}})
	if err != nil {
		return BuildResult{}, err
	}
	resolved := prepared.resolved
	r.emitProgress(opts.Events, "Ensuring container image")
	image, err := r.ensureImage(ctx, resolved, opts.Events)
	if err != nil {
		return BuildResult{}, err
	}
	r.emitProgress(opts.Events, "Applying runtime metadata")
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (r *Runner) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ProgressLabel:  "Resolving development container",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}, enrich: true, findContainer: true, inspectContainer: true})
	if err != nil {
		return 0, err
	}
	resolved := prepared.resolved
	user, err := r.effectiveRemoteUser(ctx, prepared)
	if err != nil {
		return 0, err
	}

	args := []string{"exec"}
	if opts.Stdin != nil {
		args = append(args, "-i")
	}
	interactive := shouldAllocateTTY(opts.Stdin, opts.Stdout)
	if interactive {
		args = append(args, "-t")
	}
	if user != "" {
		args = append(args, "-u", user)
	}
	for _, key := range devcontainer.SortedMapKeys(resolved.Merged.RemoteEnv) {
		value := resolved.Merged.RemoteEnv[key]
		args = append(args, "-e", key+"="+value)
	}
	for key, value := range opts.RemoteEnv {
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, prepared.containerID)
	args = append(args, opts.Args...)
	if interactive {
		r.clearProgress(opts.Events)
	} else {
		r.emitProgress(opts.Events, fmt.Sprintf("Executing command in %s", prepared.containerID))
	}

	err = r.docker.Run(ctx, docker.RunOptions{
		Args:   args,
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
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
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return ReadConfigResult{}, err
	}
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
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
	var bridgeReport *bridge.Report
	bridgeReport, err = r.previewBridgeConfig(&resolved, state.BridgeEnabled)
	if err != nil {
		return ReadConfigResult{}, err
	}
	if state.BridgeEnabled {
		report, err := bridge.Doctor(resolved.StateDir)
		if err != nil {
			return ReadConfigResult{}, err
		}
		bridgeReport = (*bridge.Report)(&report)
	}
	prepared.resolved = resolved
	resolvedUser, err := r.effectiveRemoteUser(ctx, prepared)
	if err != nil {
		return ReadConfigResult{}, err
	}
	imageUser, err := r.inspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := r.readManagedContainerState(prepared)
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
		RemoteUser:           resolvedUser,
		ContainerUser:        resolved.Merged.ContainerUser,
		RemoteEnv:            resolved.Merged.RemoteEnv,
		ContainerEnv:         resolved.Merged.ContainerEnv,
		Mounts:               resolved.Merged.Mounts,
		ForwardPorts:         []string(resolved.Merged.ForwardPorts),
		Bridge:               bridgeReport,
		Dotfiles:             dotfilesStatus(state, dotfiles),
		MetadataCount:        len(resolved.Merged.Metadata),
		ManagedContainer:     managedContainer,
	}, nil
}

func (r *Runner) RunLifecycle(ctx context.Context, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	dotfiles, err := opts.Dotfiles.Normalized()
	if err != nil {
		return RunLifecycleResult{}, err
	}
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
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
	r.emitProgress(opts.Events, "Running lifecycle commands")
	runDotfiles := phase == "all" || phase == "create"
	if err := r.runLifecyclePhase(ctx, resolved, prepared.containerID, phase, state, dotfiles, runDotfiles, opts.Events); err != nil {
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
	prepared, err := r.prepareWorkspace(ctx, prepareWorkspaceOptions{resolve: prepareResolveOptions{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		ProgressLabel:  "Inspecting bridge state",
		Debug:          opts.Debug,
		Events:         opts.Events,
	}})
	if err != nil {
		return bridge.Report{}, err
	}
	return bridge.Doctor(prepared.resolved.StateDir)
}

func (r *Runner) prepareResolved(ctx context.Context, opts prepareResolveOptions) (devcontainer.ResolvedConfig, error) {
	r.emitProgress(opts.Events, opts.ProgressLabel)
	resolveOpts := devcontainer.ResolveOptions{LockfilePolicy: opts.LockfilePolicy, FeatureHTTPTimeout: opts.FeatureTimeout}
	if !opts.ReadOnly {
		resolveOpts.AllowNetwork = true
		resolveOpts.WriteFeatureLock = true
		resolveOpts.WriteFeatureState = true
	}
	var (
		resolved devcontainer.ResolvedConfig
		err      error
	)
	if opts.ReadOnly {
		resolved, err = devcontainer.ResolveReadOnlyWithOptions(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	} else {
		resolved, err = devcontainer.ResolveWithOptions(ctx, opts.Workspace, opts.ConfigPath, resolveOpts)
	}
	if err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if opts.Debug {
		r.emitPlan(opts.Events, resolved)
	}
	return resolved, nil
}

func (r *Runner) prepareEnrichedResolved(ctx context.Context, opts prepareResolveOptions) (devcontainer.ResolvedConfig, string, error) {
	resolved, err := r.prepareResolved(ctx, opts)
	if err != nil {
		return devcontainer.ResolvedConfig{}, "", err
	}
	image := preparedImage(resolved)
	r.emitProgress(opts.Events, "Applying runtime metadata")
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return devcontainer.ResolvedConfig{}, "", err
	}
	return resolved, image, nil
}

func (r *Runner) prepareWorkspace(ctx context.Context, opts prepareWorkspaceOptions) (preparedWorkspace, error) {
	resolved, err := r.prepareResolved(ctx, opts.resolve)
	if err != nil {
		return preparedWorkspace{}, err
	}
	prepared := preparedWorkspace{resolved: resolved, image: preparedImage(resolved)}
	if opts.enrich {
		r.emitProgress(opts.resolve.Events, "Applying runtime metadata")
		if err := r.enrichMergedConfig(ctx, &prepared.resolved, prepared.image); err != nil {
			return preparedWorkspace{}, err
		}
	}
	if opts.loadState {
		state, err := devcontainer.ReadState(prepared.resolved.StateDir)
		if err != nil {
			return preparedWorkspace{}, err
		}
		state, err = r.reconcileState(ctx, prepared.resolved, state)
		if err != nil {
			return preparedWorkspace{}, err
		}
		prepared.state = state
		prepared.containerID = state.ContainerID
	}
	if opts.findContainer && prepared.containerID == "" {
		r.emitProgress(opts.resolve.Events, "Finding managed container")
		containerID, err := r.findContainer(ctx, prepared.resolved)
		if err != nil {
			if opts.allowMissingContainer && errors.Is(err, errManagedContainerNotFound) {
				return prepared, nil
			}
			return preparedWorkspace{}, err
		}
		prepared.containerID = containerID
	}
	if opts.inspectContainer && prepared.containerID != "" {
		inspect, err := r.docker.InspectContainer(ctx, prepared.containerID)
		if err != nil {
			return preparedWorkspace{}, err
		}
		prepared.containerInspect = &inspect
	}
	return prepared, nil
}

func preparedImage(resolved devcontainer.ResolvedConfig) string {
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	return image
}

func (r *Runner) emitProgress(events ui.Sink, message string) {
	if events == nil || message == "" {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventProgress, Message: message})
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

const updateUIDCommand = `REMOTE_USER="$1"; NEW_UID="$2"; NEW_GID="$3"; \
eval $(sed -n "s/${REMOTE_USER}:[^:]*:\([^:]*\):\([^:]*\):[^:]*:\([^:]*\).*/OLD_UID=\1;OLD_GID=\2;HOME_FOLDER=\3/p" /etc/passwd); \
eval $(sed -n "s/\([^:]*\):[^:]*:${NEW_UID}:.*/EXISTING_USER=\1/p" /etc/passwd); \
eval $(sed -n "s/\([^:]*\):[^:]*:${NEW_GID}:.*/EXISTING_GROUP=\1/p" /etc/group); \
if [ -z "$OLD_UID" ]; then \
	echo "Remote user not found in /etc/passwd ($REMOTE_USER)."; \
elif [ "$OLD_UID" = "$NEW_UID" -a "$OLD_GID" = "$NEW_GID" ]; then \
	echo "UIDs and GIDs are the same ($NEW_UID:$NEW_GID)."; \
elif [ "$OLD_UID" != "$NEW_UID" -a -n "$EXISTING_USER" ]; then \
	echo "User with UID exists ($EXISTING_USER=$NEW_UID)."; \
else \
	if [ "$OLD_GID" != "$NEW_GID" -a -n "$EXISTING_GROUP" ]; then \
		echo "Group with GID exists ($EXISTING_GROUP=$NEW_GID)."; \
		NEW_GID="$OLD_GID"; \
	fi; \
	echo "Updating UID:GID from $OLD_UID:$OLD_GID to $NEW_UID:$NEW_GID."; \
	sed -i -e "s/\(${REMOTE_USER}:[^:]*:\)[^:]*:[^:]*/\1${NEW_UID}:${NEW_GID}/" /etc/passwd; \
	if [ "$OLD_GID" != "$NEW_GID" ]; then \
		sed -i -e "s/\([^:]*:[^:]*:\)${OLD_GID}:/\1${NEW_GID}:/" /etc/group; \
	fi; \
	chown -R $NEW_UID:$NEW_GID $HOME_FOLDER; \
fi`

func shouldAllocateTTY(stdin io.Reader, stdout io.Writer) bool {
	inFile, ok := stdin.(*os.File)
	if !ok || !isCharacterDevice(inFile) {
		return false
	}
	outFile, ok := stdout.(*os.File)
	if !ok || !isCharacterDevice(outFile) {
		return false
	}
	return true
}

func isCharacterDevice(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
