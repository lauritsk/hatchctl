package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type Runner struct {
	docker *docker.Client
}

func NewRunner(client *docker.Client) *Runner {
	return &Runner{docker: client}
}

type UpOptions struct {
	Workspace     string
	ConfigPath    string
	Recreate      bool
	BridgeEnabled bool
	Verbose       bool
}

type UpResult struct {
	ContainerID           string         `json:"containerId"`
	Image                 string         `json:"image"`
	RemoteWorkspaceFolder string         `json:"remoteWorkspaceFolder"`
	StateDir              string         `json:"stateDir"`
	Bridge                *bridge.Report `json:"bridge,omitempty"`
}

type BuildOptions struct {
	Workspace  string
	ConfigPath string
	Verbose    bool
}

type BuildResult struct {
	Image string `json:"image"`
}

type ExecOptions struct {
	Workspace  string
	ConfigPath string
	Args       []string
	RemoteEnv  map[string]string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

type ReadConfigOptions struct {
	Workspace  string
	ConfigPath string
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
	MetadataCount        int               `json:"metadataCount"`
	ManagedContainer     *ManagedContainer `json:"managedContainer,omitempty"`
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
	Workspace  string
	ConfigPath string
	Phase      string
}

type RunLifecycleResult struct {
	ContainerID string `json:"containerId"`
	Phase       string `json:"phase"`
}

type BridgeDoctorOptions struct {
	Workspace  string
	ConfigPath string
}

type ExitError struct {
	Code int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (r *Runner) Up(ctx context.Context, opts UpOptions) (UpResult, error) {
	resolved, err := devcontainer.Resolve(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return UpResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "plan source=%s config=%s workspace=%s state=%s target-image=%s\n", resolved.SourceKind, resolved.ConfigPath, resolved.WorkspaceFolder, resolved.StateDir, resolved.ImageName)
	}
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return UpResult{}, err
	}

	state, err := devcontainer.ReadState(resolved.StateDir)
	if err != nil {
		return UpResult{}, err
	}
	state, err = r.reconcileState(ctx, resolved, state)
	if err != nil {
		return UpResult{}, err
	}

	if opts.Recreate && state.ContainerID != "" {
		_ = r.removeContainer(ctx, state.ContainerID)
		state = devcontainer.State{}
	}

	image, err := r.ensureImage(ctx, resolved)
	if err != nil {
		return UpResult{}, err
	}
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return UpResult{}, err
	}
	image, err = r.ensureUpdatedUIDImage(ctx, resolved, image)
	if err != nil {
		return UpResult{}, err
	}
	var bridgeReport *bridge.Report
	bridgeReport, err = r.applyBridgeConfig(&resolved, opts.BridgeEnabled)
	if err != nil {
		return UpResult{}, err
	}
	overridePath := ""
	if resolved.SourceKind == "compose" {
		overridePath, err = writeComposeOverride(resolved, image)
		if err != nil {
			return UpResult{}, err
		}
		defer os.Remove(overridePath)
	}

	containerID, created, err := r.ensureContainer(ctx, resolved, image, opts.BridgeEnabled, overridePath)
	if err != nil {
		return UpResult{}, err
	}
	if bridgeReport != nil {
		startedBridge, err := bridge.Start(resolved.StateDir, opts.BridgeEnabled, containerID)
		if err != nil {
			return UpResult{}, err
		}
		bridgeReport = (*bridge.Report)(startedBridge)
	}

	if err := r.runLifecycleForUp(ctx, resolved, containerID, created, state.LifecycleReady); err != nil {
		return UpResult{}, err
	}

	state.ContainerID = containerID
	state.LifecycleReady = true
	state.BridgeEnabled = opts.BridgeEnabled
	if bridgeReport != nil {
		state.BridgeSessionID = bridgeReport.ID
	}
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
	resolved, err := devcontainer.Resolve(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return BuildResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "plan source=%s config=%s workspace=%s state=%s target-image=%s\n", resolved.SourceKind, resolved.ConfigPath, resolved.WorkspaceFolder, resolved.StateDir, resolved.ImageName)
	}
	image, err := r.ensureImage(ctx, resolved)
	if err != nil {
		return BuildResult{}, err
	}
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return BuildResult{}, err
	}
	image, err = r.ensureUpdatedUIDImage(ctx, resolved, image)
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (r *Runner) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	resolved, err := devcontainer.Resolve(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return 0, err
	}
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return 0, err
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		return 0, err
	}
	user, err := r.effectiveRemoteUser(ctx, resolved, image, containerID)
	if err != nil {
		return 0, err
	}

	args := []string{"exec"}
	if opts.Stdin != nil {
		args = append(args, "-i")
	}
	if shouldAllocateTTY(opts.Stdin, opts.Stdout) {
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
	args = append(args, containerID)
	args = append(args, opts.Args...)

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
	resolved, err := devcontainer.ResolveReadOnly(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return ReadConfigResult{}, err
	}
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return ReadConfigResult{}, err
	}
	state, err := devcontainer.ReadState(resolved.StateDir)
	if err != nil {
		return ReadConfigResult{}, err
	}
	state, err = r.reconcileState(ctx, resolved, state)
	if err != nil {
		return ReadConfigResult{}, err
	}
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
	resolvedUser, err := r.effectiveRemoteUser(ctx, resolved, image, "")
	if err != nil {
		return ReadConfigResult{}, err
	}
	imageUser, err := r.inspectImageUser(ctx, image)
	if err != nil {
		return ReadConfigResult{}, err
	}
	managedContainer, err := r.readManagedContainerState(ctx, resolved)
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
		MetadataCount:        len(resolved.Merged.Metadata),
		ManagedContainer:     managedContainer,
	}, nil
}

func (r *Runner) RunLifecycle(ctx context.Context, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	resolved, err := devcontainer.Resolve(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	if err := r.enrichMergedConfig(ctx, &resolved, image); err != nil {
		return RunLifecycleResult{}, err
	}
	phase := strings.ToLower(opts.Phase)
	if phase == "" {
		phase = "all"
	}
	if err := r.runLifecyclePhase(ctx, resolved, containerID, phase); err != nil {
		return RunLifecycleResult{}, err
	}
	return RunLifecycleResult{ContainerID: containerID, Phase: phase}, nil
}

func (r *Runner) BridgeDoctor(ctx context.Context, opts BridgeDoctorOptions) (bridge.Report, error) {
	resolved, err := devcontainer.ResolveReadOnly(ctx, opts.Workspace, opts.ConfigPath)
	if err != nil {
		return bridge.Report{}, err
	}
	return bridge.Doctor(resolved.StateDir)
}

const updateUIDDockerfile = `ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

ARG REMOTE_USER
ARG NEW_UID
ARG NEW_GID

RUN eval $(sed -n "s/${REMOTE_USER}:[^:]*:\([^:]*\):\([^:]*\):[^:]*:\([^:]*\).*/OLD_UID=\1;OLD_GID=\2;HOME_FOLDER=\3/p" /etc/passwd); \
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
	fi

ARG IMAGE_USER
USER $IMAGE_USER
`

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
