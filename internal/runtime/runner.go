package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	WorkspaceFolder      string `json:"workspaceFolder"`
	ConfigPath           string `json:"configPath"`
	WorkspaceMount       string `json:"workspaceMount"`
	SourceKind           string `json:"sourceKind"`
	HasInitializeCommand bool   `json:"hasInitializeCommand"`
	HasCreateCommand     bool   `json:"hasCreateCommand"`
	HasStartCommand      bool   `json:"hasStartCommand"`
	HasAttachCommand     bool   `json:"hasAttachCommand"`
	Image                string `json:"image"`
	ContainerName        string `json:"containerName"`
	StateDir             string `json:"stateDir"`
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return UpResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "source=%s image=%s workspace=%s\n", resolved.SourceKind, resolved.ImageName, resolved.WorkspaceFolder)
	}
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return UpResult{}, err
	}

	state, err := devcontainer.ReadState(resolved.StateDir)
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

	containerID, created, err := r.ensureContainer(ctx, resolved, image, opts.BridgeEnabled)
	if err != nil {
		return UpResult{}, err
	}

	bridgeReport, err := r.prepareBridge(resolved.StateDir, opts.BridgeEnabled)
	if err != nil {
		return UpResult{}, err
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return BuildResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "source=%s image=%s workspace=%s\n", resolved.SourceKind, resolved.ImageName, resolved.WorkspaceFolder)
	}
	image, err := r.ensureImage(ctx, resolved)
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Image: image}, nil
}

func (r *Runner) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return 0, err
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		return 0, err
	}

	args := []string{"exec", "-i"}
	user := devcontainer.RemoteExecUser(resolved.Config)
	if user != "" {
		args = append(args, "-u", user)
	}
	for key, value := range resolved.Config.RemoteEnv {
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
	msg := err.Error()
	if strings.Contains(msg, "exit status ") {
		return parseExitStatus(msg), nil
	}
	return 0, err
}

func (r *Runner) ReadConfig(ctx context.Context, opts ReadConfigOptions) (ReadConfigResult, error) {
	_ = ctx
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return ReadConfigResult{}, err
	}
	image := resolved.Config.Image
	if image == "" {
		image = resolved.ImageName
	}
	return ReadConfigResult{
		WorkspaceFolder:      resolved.WorkspaceFolder,
		ConfigPath:           resolved.ConfigPath,
		WorkspaceMount:       resolved.WorkspaceMount,
		SourceKind:           resolved.SourceKind,
		HasInitializeCommand: !resolved.Config.InitializeCommand.Empty(),
		HasCreateCommand:     !resolved.Config.OnCreateCommand.Empty() || !resolved.Config.UpdateContentCommand.Empty() || !resolved.Config.PostCreateCommand.Empty(),
		HasStartCommand:      !resolved.Config.PostStartCommand.Empty(),
		HasAttachCommand:     !resolved.Config.PostAttachCommand.Empty(),
		Image:                image,
		ContainerName:        resolved.ContainerName,
		StateDir:             resolved.StateDir,
	}, nil
}

func (r *Runner) RunLifecycle(ctx context.Context, opts RunLifecycleOptions) (RunLifecycleResult, error) {
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
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
	_ = ctx
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return bridge.Report{}, err
	}
	return bridge.Doctor(resolved.StateDir)
}

func (r *Runner) ensureImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.Config.Image != "" {
		return resolved.Config.Image, nil
	}

	dockerfile := resolved.ConfigDir
	contextDir := resolved.ConfigDir
	if rel := devcontainer.EffectiveDockerfile(resolved.Config); rel != "" {
		dockerfile = filepath.Join(resolved.ConfigDir, rel)
	}
	if rel := devcontainer.EffectiveContext(resolved.Config); rel != "" {
		contextDir = filepath.Join(resolved.ConfigDir, rel)
	}
	args := []string{"build", "-f", dockerfile, "-t", resolved.ImageName}
	if resolved.Config.Build != nil && resolved.Config.Build.Target != "" {
		args = append(args, "--target", resolved.Config.Build.Target)
	}
	if resolved.Config.Build != nil {
		for key, value := range resolved.Config.Build.Args {
			args = append(args, "--build-arg", key+"="+value)
		}
		args = append(args, resolved.Config.Build.Options...)
	}
	args = append(args, contextDir)
	return resolved.ImageName, r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: os.Stdout, Stderr: os.Stderr})
}

func (r *Runner) ensureContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, bridgeEnabled bool) (string, bool, error) {
	containerID, err := r.findContainer(ctx, resolved)
	if err == nil && containerID != "" {
		status, statusErr := r.docker.Output(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
		if statusErr == nil && status != "running" {
			if err := r.docker.Run(ctx, docker.RunOptions{Args: []string{"start", containerID}, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
				return "", false, err
			}
		}
		return containerID, false, nil
	}

	stateMount := fmt.Sprintf("type=bind,source=%s,target=%s", resolved.StateDir, "/var/run/hatchctl")
	args := []string{"run", "-d", "--name", resolved.ContainerName}
	for key, value := range resolved.Labels {
		args = append(args, "--label", key+"="+value)
	}
	if bridgeEnabled {
		args = append(args, "--label", devcontainer.BridgeEnabledLabel+"=true")
	}
	args = append(args, "--mount", resolved.WorkspaceMount, "--mount", stateMount)
	if resolved.Config.Init != nil && *resolved.Config.Init {
		args = append(args, "--init")
	}
	if resolved.Config.Privileged != nil && *resolved.Config.Privileged {
		args = append(args, "--privileged")
	}
	for _, cap := range resolved.Config.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, sec := range resolved.Config.SecurityOpt {
		args = append(args, "--security-opt", sec)
	}
	for key, value := range resolved.Config.ContainerEnv {
		args = append(args, "-e", key+"="+value)
	}
	for _, mount := range resolved.Config.Mounts {
		args = append(args, "--mount", mount)
	}
	args = append(args, resolved.Config.RunArgs...)
	args = append(args, image)
	args = append(args, devcontainer.ContainerCommand(resolved.Config)...)

	containerID, err = r.docker.Output(ctx, args...)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}

func (r *Runner) findContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	args := []string{"ps", "-aq"}
	for key, value := range resolved.Labels {
		args = append(args, "--filter", "label="+key+"="+value)
	}
	result, err := r.docker.Output(ctx, args...)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errors.New("managed container not found")
}

func (r *Runner) removeContainer(ctx context.Context, containerID string) error {
	return r.docker.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", containerID}, Stdout: os.Stdout, Stderr: os.Stderr})
}

func (r *Runner) prepareBridge(stateDir string, enabled bool) (*bridge.Report, error) {
	report, err := bridge.Prepare(stateDir, enabled)
	if err != nil {
		return nil, err
	}
	return (*bridge.Report)(report), nil
}

func (r *Runner) runLifecycleForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, lifecycleReady bool) error {
	if created || !lifecycleReady {
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.OnCreateCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.UpdateContentCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostCreateCommand); err != nil {
			return err
		}
	}
	if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostStartCommand); err != nil {
		return err
	}
	return r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostAttachCommand)
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.OnCreateCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.UpdateContentCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostCreateCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostStartCommand); err != nil {
			return err
		}
		return r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostAttachCommand)
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.OnCreateCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.UpdateContentCommand); err != nil {
			return err
		}
		return r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostCreateCommand)
	case "start":
		return r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostStartCommand)
	case "attach":
		return r.runContainerLifecycle(ctx, containerID, resolved, resolved.Config.PostAttachCommand)
	default:
		return fmt.Errorf("unknown lifecycle phase %q", phase)
	}
}

func (r *Runner) runContainerLifecycle(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, command devcontainer.LifecycleCommand) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs := []string{"exec", "-i"}
		user := devcontainer.RemoteExecUser(resolved.Config)
		if user != "" {
			dockerArgs = append(dockerArgs, "-u", user)
		}
		for key, value := range resolved.Config.RemoteEnv {
			dockerArgs = append(dockerArgs, "-e", key+"="+value)
		}
		dockerArgs = append(dockerArgs, containerID)
		dockerArgs = append(dockerArgs, args...)
		return r.docker.Run(ctx, docker.RunOptions{Args: dockerArgs, Stdout: os.Stdout, Stderr: os.Stderr})
	}, command)
}
