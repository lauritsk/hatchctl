package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type Runner struct {
	docker *docker.Client
}

type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Image string        `json:"image"`
	Build *composeBuild `json:"build"`
}

type composeBuild struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile"`
	Target     string            `json:"target"`
	Args       map[string]string `json:"args"`
}

func (b *composeBuild) Enabled() bool {
	return b != nil && b.Context != ""
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return UpResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "source=%s image=%s workspace=%s\n", resolved.SourceKind, resolved.ImageName, resolved.WorkspaceFolder)
	}
	if resolved.SourceKind == "compose" && opts.BridgeEnabled {
		return UpResult{}, fmt.Errorf("compose bridge support is not implemented yet in hatchctl")
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
	if resolved.SourceKind != "compose" {
		image, err = r.ensureUpdatedUIDImage(ctx, resolved, image)
		if err != nil {
			return UpResult{}, err
		}
	}
	var bridgeReport *bridge.Report
	if resolved.SourceKind != "compose" {
		bridgeReport, err = r.applyBridgeConfig(&resolved, opts.BridgeEnabled)
		if err != nil {
			return UpResult{}, err
		}
	} else {
		if err := r.writeComposeOverride(ctx, resolved); err != nil {
			return UpResult{}, err
		}
	}

	containerID, created, err := r.ensureContainer(ctx, resolved, image, opts.BridgeEnabled)
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return BuildResult{}, err
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "source=%s image=%s workspace=%s\n", resolved.SourceKind, resolved.ImageName, resolved.WorkspaceFolder)
	}
	if resolved.SourceKind == "compose" {
		image, err := r.ensureComposeImage(ctx, resolved)
		if err != nil {
			return BuildResult{}, err
		}
		return BuildResult{Image: image}, nil
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
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
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	msg := err.Error()
	if strings.Contains(msg, "exit status ") {
		return parseExitStatus(msg), nil
	}
	return 0, err
}

func (r *Runner) ReadConfig(ctx context.Context, opts ReadConfigOptions) (ReadConfigResult, error) {
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
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
	if resolved.SourceKind == "compose" {
		if err := r.writeComposeOverride(ctx, resolved); err != nil {
			return ReadConfigResult{}, err
		}
	}
	var bridgeReport *bridge.Report
	if resolved.SourceKind != "compose" {
		bridgeReport, err = r.applyBridgeConfig(&resolved, state.BridgeEnabled)
		if err != nil {
			return ReadConfigResult{}, err
		}
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
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
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
	_ = ctx
	resolved, err := devcontainer.Resolve(opts.Workspace, opts.ConfigPath)
	if err != nil {
		return bridge.Report{}, err
	}
	return bridge.Doctor(resolved.StateDir)
}

func (r *Runner) enrichMergedConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig, image string) error {
	inspect, err := r.docker.InspectImage(ctx, image)
	if err != nil {
		if resolved.SourceKind == "compose" {
			resolved.Merged = devcontainer.MergeMetadata(resolved.Config, featureMetadata(resolved.Features))
			return nil
		}
		if isManagedImage(resolved, image) {
			resolved.Merged = devcontainer.MergeMetadata(resolved.Config, featureMetadata(resolved.Features))
			return nil
		}
		return err
	}
	metadata, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return err
	}
	if isManagedImage(resolved, image) {
		resolved.Merged = devcontainer.MergeMetadata(devcontainer.Config{}, metadata)
		resolved.Merged.Config = resolved.Config
		return nil
	}
	resolved.Merged = devcontainer.MergeMetadata(resolved.Config, metadata)
	return nil
}

func (r *Runner) ensureImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeImage(ctx, resolved)
	}
	if len(resolved.Features) > 0 {
		return r.ensureImageWithFeatures(ctx, resolved)
	}
	if resolved.Config.Image != "" {
		return resolved.Config.Image, nil
	}
	return resolved.ImageName, r.buildDockerfileImage(ctx, resolved, resolved.ImageName)
}

func (r *Runner) buildDockerfileImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageName string) error {
	dockerfile := resolved.ConfigDir
	contextDir := resolved.ConfigDir
	if rel := devcontainer.EffectiveDockerfile(resolved.Config); rel != "" {
		dockerfile = filepath.Join(resolved.ConfigDir, rel)
	}
	if rel := devcontainer.EffectiveContext(resolved.Config); rel != "" {
		contextDir = filepath.Join(resolved.ConfigDir, rel)
	}
	args := []string{"build", "-f", dockerfile, "-t", imageName}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return err
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
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
	return r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: os.Stdout, Stderr: os.Stderr})
}

func (r *Runner) ensureImageWithFeatures(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	baseImage := resolved.Config.Image
	if baseImage == "" {
		baseImage = resolved.ImageName + "-base"
		if err := r.buildDockerfileImage(ctx, resolved, baseImage); err != nil {
			return "", err
		}
	}
	return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage)
}

func (r *Runner) ensureFeaturesImageFromBase(ctx context.Context, resolved devcontainer.ResolvedConfig, baseImage string) (string, error) {
	imageUser, err := r.inspectImageUser(ctx, baseImage)
	if err != nil {
		return "", err
	}
	containerUser := firstNonEmpty(resolved.Merged.ContainerUser, imageUser, "root")
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, containerUser)
	buildDir := filepath.Join(resolved.StateDir, "features-build")
	if err := os.RemoveAll(buildDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return "", err
	}
	if err := writeFeatureBuildContext(buildDir, resolved.Features, containerUser, remoteUser, resolved.Merged.Metadata); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); err != nil {
		return "", fmt.Errorf("generated feature Dockerfile missing in %s: %w", buildDir, err)
	}
	args := []string{
		"build",
		"-f", filepath.Join(buildDir, "Dockerfile"),
		"-t", resolved.ImageName,
		"--build-arg", "BASE_IMAGE=" + baseImage,
		buildDir,
	}
	if err := r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		entries, _ := os.ReadDir(buildDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return "", fmt.Errorf("build features image from %s with files %v: %w", buildDir, names, err)
	}
	return resolved.ImageName, nil
}

func (r *Runner) ensureComposeImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	config, err := r.readComposeConfig(ctx, resolved)
	if err != nil {
		return "", err
	}
	service, ok := config.Services[resolved.ComposeService]
	if !ok {
		return "", fmt.Errorf("compose service %q not found", resolved.ComposeService)
	}
	baseImage := service.Image
	if service.Build.Enabled() {
		if err := r.docker.Run(ctx, docker.RunOptions{Args: append(r.composeBaseArgs(resolved), "build", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
			return "", err
		}
		if baseImage == "" {
			baseImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
	}
	if len(resolved.Features) > 0 {
		if baseImage == "" {
			return "", fmt.Errorf("compose service %q needs an image or build result for features", resolved.ComposeService)
		}
		return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage)
	}
	if baseImage != "" {
		return baseImage, nil
	}
	return resolved.ComposeProject + "-" + resolved.ComposeService, nil
}

func (r *Runner) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, bool, error) {
	containerID, err := r.findComposeContainer(ctx, resolved)
	if err == nil && containerID != "" {
		status, statusErr := r.docker.Output(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
		if statusErr == nil && status == "running" {
			return containerID, false, nil
		}
	}
	if err := r.docker.Run(ctx, docker.RunOptions{Args: append(r.composeArgs(resolved), "up", "--no-build", "-d", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		return "", false, err
	}
	containerID, err = r.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}

func (r *Runner) ensureUpdatedUIDImage(ctx context.Context, resolved devcontainer.ResolvedConfig, image string) (string, error) {
	if resolved.SourceKind == "compose" {
		return image, nil
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return image, nil
	}
	if resolved.Merged.UpdateRemoteUserUID != nil && !*resolved.Merged.UpdateRemoteUserUID {
		return image, nil
	}
	uid := os.Getuid()
	gid := os.Getgid()
	if uid <= 0 || gid <= 0 {
		return image, nil
	}
	inspect, err := r.docker.InspectImage(ctx, image)
	if err != nil {
		return image, err
	}
	imageUser := inspect.Config.User
	if imageUser == "" {
		imageUser = "root"
	}
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser, imageUser)
	if remoteUser == "" || remoteUser == "root" || isNumericUser(remoteUser) {
		return image, nil
	}
	derivedImage := resolved.ImageName + "-uid"
	dockerfilePath := filepath.Join(resolved.StateDir, "updateUID.Dockerfile")
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return image, err
	}
	if err := os.WriteFile(dockerfilePath, []byte(updateUIDDockerfile), 0o644); err != nil {
		return image, err
	}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return image, err
	}
	args := []string{
		"build",
		"-f", dockerfilePath,
		"-t", derivedImage,
		"--build-arg", "BASE_IMAGE=" + image,
		"--build-arg", "REMOTE_USER=" + remoteUser,
		"--build-arg", fmt.Sprintf("NEW_UID=%d", uid),
		"--build-arg", fmt.Sprintf("NEW_GID=%d", gid),
		"--build-arg", "IMAGE_USER=" + imageUser,
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
	args = append(args, resolved.StateDir)
	if err := r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		return image, err
	}
	return derivedImage, nil
}

func (r *Runner) ensureContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, bridgeEnabled bool) (string, bool, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeContainer(ctx, resolved)
	}
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
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return "", false, err
	}
	for key, value := range resolved.Labels {
		args = append(args, "--label", key+"="+value)
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
	if bridgeEnabled {
		args = append(args, "--label", devcontainer.BridgeEnabledLabel+"=true")
	}
	args = append(args, "--mount", resolved.WorkspaceMount, "--mount", stateMount)
	if resolved.Merged.Init {
		args = append(args, "--init")
	}
	if resolved.Merged.Privileged {
		args = append(args, "--privileged")
	}
	for _, cap := range resolved.Merged.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, sec := range resolved.Merged.SecurityOpt {
		args = append(args, "--security-opt", sec)
	}
	for _, key := range devcontainer.SortedMapKeys(resolved.Merged.ContainerEnv) {
		value := resolved.Merged.ContainerEnv[key]
		args = append(args, "-e", key+"="+value)
	}
	for _, mount := range resolved.Merged.Mounts {
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
	if resolved.SourceKind == "compose" {
		return r.findComposeContainer(ctx, resolved)
	}
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
	return "", errManagedContainerNotFound
}

var errManagedContainerNotFound = errors.New("managed container not found")

func (r *Runner) removeContainer(ctx context.Context, containerID string) error {
	return r.docker.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", containerID}, Stdout: os.Stdout, Stderr: os.Stderr})
}

func (r *Runner) reconcileState(ctx context.Context, resolved devcontainer.ResolvedConfig, state devcontainer.State) (devcontainer.State, error) {
	if state.ContainerID != "" {
		if _, err := r.docker.InspectContainer(ctx, state.ContainerID); err == nil {
			return state, nil
		} else if !strings.Contains(err.Error(), "No such object") && !strings.Contains(err.Error(), "not found") {
			return devcontainer.State{}, err
		}
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		if errors.Is(err, errManagedContainerNotFound) {
			return devcontainer.State{BridgeEnabled: state.BridgeEnabled, BridgeSessionID: state.BridgeSessionID}, nil
		}
		return devcontainer.State{}, err
	}
	state.ContainerID = containerID
	state.LifecycleReady = false
	return state, nil
}

func (r *Runner) applyBridgeConfig(resolved *devcontainer.ResolvedConfig, enabled bool) (*bridge.Report, error) {
	report, merged, err := bridge.Apply(resolved.StateDir, enabled, resolved.Merged)
	if err != nil {
		return nil, err
	}
	resolved.Merged = merged
	return (*bridge.Report)(report), nil
}

func (r *Runner) effectiveRemoteUser(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerID string) (string, error) {
	if user := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser); user != "" {
		return user, nil
	}
	if containerID != "" {
		inspect, err := r.docker.InspectContainer(ctx, containerID)
		if err != nil {
			return "", err
		}
		return inspect.Config.User, nil
	}
	return r.inspectImageUser(ctx, image)
}

func (r *Runner) inspectImageUser(ctx context.Context, image string) (string, error) {
	inspect, err := r.docker.InspectImage(ctx, image)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") || strings.Contains(err.Error(), "not found") {
			return "", nil
		}
		return "", err
	}
	return inspect.Config.User, nil
}

func (r *Runner) readManagedContainerState(ctx context.Context, resolved devcontainer.ResolvedConfig) (*ManagedContainer, error) {
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		if errors.Is(err, errManagedContainerNotFound) {
			return nil, nil
		}
		return nil, err
	}
	inspect, err := r.docker.InspectContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	metadata, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return nil, err
	}
	merged := devcontainer.MergeMetadata(resolved.Config, metadata)
	effectiveUser := firstNonEmpty(merged.RemoteUser, merged.ContainerUser, inspect.Config.User)
	return &ManagedContainer{
		ID:            inspect.ID,
		Name:          strings.TrimPrefix(inspect.Name, "/"),
		Image:         inspect.Image,
		Status:        inspect.State.Status,
		Running:       inspect.State.Running,
		RemoteUser:    effectiveUser,
		ContainerEnv:  envListToMap(inspect.Config.Env),
		Labels:        inspect.Config.Labels,
		ForwardPorts:  []string(merged.ForwardPorts),
		MetadataCount: len(metadata),
		BridgeEnabled: inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true",
	}, nil
}

func (r *Runner) runLifecycleForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, lifecycleReady bool) error {
	if created || !lifecycleReady {
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands); err != nil {
			return err
		}
	}
	if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
		return err
	}
	return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands)
	case "start":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands)
	case "attach":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
	default:
		return fmt.Errorf("unknown lifecycle phase %q", phase)
	}
}

func (r *Runner) runContainerLifecycleList(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, commands []devcontainer.LifecycleCommand) error {
	for _, command := range commands {
		if err := r.runContainerLifecycle(ctx, containerID, resolved, command); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runContainerLifecycle(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, command devcontainer.LifecycleCommand) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs := []string{"exec", "-i"}
		user := resolved.Merged.RemoteUser
		if user == "" {
			user = resolved.Merged.ContainerUser
		}
		if user != "" {
			dockerArgs = append(dockerArgs, "-u", user)
		}
		for _, key := range devcontainer.SortedMapKeys(resolved.Merged.RemoteEnv) {
			value := resolved.Merged.RemoteEnv[key]
			dockerArgs = append(dockerArgs, "-e", key+"="+value)
		}
		dockerArgs = append(dockerArgs, containerID)
		dockerArgs = append(dockerArgs, args...)
		return r.docker.Run(ctx, docker.RunOptions{Args: dockerArgs, Stdout: os.Stdout, Stderr: os.Stderr})
	}, command)
}

func envListToMap(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for _, entry := range values {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func featureMetadata(features []devcontainer.ResolvedFeature) []devcontainer.MetadataEntry {
	if len(features) == 0 {
		return nil
	}
	result := make([]devcontainer.MetadataEntry, 0, len(features))
	for _, feature := range features {
		result = append(result, feature.Metadata)
	}
	return result
}

func isManagedImage(resolved *devcontainer.ResolvedConfig, image string) bool {
	return image == resolved.ImageName || strings.HasPrefix(image, resolved.ImageName+"-")
}

func writeFeatureBuildContext(buildDir string, features []devcontainer.ResolvedFeature, containerUser string, remoteUser string, metadata []devcontainer.MetadataEntry) error {
	metadataLabel, err := devcontainer.MetadataLabelValue(metadata)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(buildDir, "devcontainer-features.builtin.env"), []byte(multilineEnv(containerUser, remoteUser)), 0o644); err != nil {
		return err
	}
	var dockerfile strings.Builder
	dockerfile.WriteString("ARG BASE_IMAGE\nFROM ${BASE_IMAGE}\nUSER root\n")
	dockerfile.WriteString("RUN mkdir -p /tmp/dev-container-features\n")
	dockerfile.WriteString("COPY devcontainer-features.builtin.env /tmp/dev-container-features/devcontainer-features.builtin.env\n")
	for i, feature := range features {
		rel := fmt.Sprintf("feature-%02d", i)
		dst := filepath.Join(buildDir, rel)
		if err := copyDir(feature.Path, dst); err != nil {
			return err
		}
		if len(feature.Options) > 0 {
			var lines []string
			for _, key := range sortedFeatureOptionKeys(feature.Options) {
				lines = append(lines, key+"="+feature.Options[key])
			}
			if err := os.WriteFile(filepath.Join(dst, "devcontainer-features.env"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				return err
			}
		}
		dockerfile.WriteString("COPY " + rel + " /tmp/hatchctl-features/" + rel + "\n")
		if len(feature.Metadata.ContainerEnv) > 0 {
			for _, key := range devcontainer.SortedMapKeys(feature.Metadata.ContainerEnv) {
				dockerfile.WriteString("ENV " + key + "=" + dockerfileQuotedValue(feature.Metadata.ContainerEnv[key]) + "\n")
			}
		}
		dockerfile.WriteString("RUN if [ -f /tmp/hatchctl-features/" + rel + "/install.sh ]; then cd /tmp/hatchctl-features/" + rel + " && chmod +x ./install.sh && set -a && . /tmp/dev-container-features/devcontainer-features.builtin.env && if [ -f ./devcontainer-features.env ]; then . ./devcontainer-features.env; fi && set +a && ./install.sh; fi\n")
	}
	if metadataLabel != "" {
		dockerfile.WriteString("LABEL " + devcontainer.ImageMetadataLabel + "=" + dockerfileQuotedValue(metadataLabel) + "\n")
	}
	return os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile.String()), 0o644)
}

func multilineEnv(containerUser string, remoteUser string) string {
	return "_CONTAINER_USER=" + containerUser + "\n_REMOTE_USER=" + remoteUser + "\n"
}

func sortedFeatureOptionKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dockerfileQuotedValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "")
	return "\"" + replacer.Replace(value) + "\""
}

func copyDir(src string, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) composeBaseArgs(resolved devcontainer.ResolvedConfig) []string {
	args := []string{"compose"}
	for _, file := range resolved.ComposeFiles {
		args = append(args, "-f", file)
	}
	if resolved.ComposeProject != "" {
		args = append(args, "-p", resolved.ComposeProject)
	}
	return args
}

func (r *Runner) composeArgs(resolved devcontainer.ResolvedConfig) []string {
	args := r.composeBaseArgs(resolved)
	if override := devcontainer.ComposeOverrideFile(resolved.StateDir); fileExists(override) {
		args = append(args, "-f", override)
	}
	return args
}

func (r *Runner) readComposeConfig(ctx context.Context, resolved devcontainer.ResolvedConfig) (composeConfig, error) {
	args := append(r.composeBaseArgs(resolved), "config", "--format", "json")
	output, err := r.docker.OutputOptions(ctx, docker.RunOptions{Args: args, Dir: resolved.ConfigDir})
	if err != nil {
		return composeConfig{}, err
	}
	var config composeConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return composeConfig{}, err
	}
	if config.Name != "" {
		resolved.ComposeProject = config.Name
	}
	return config, nil
}

func (r *Runner) findComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	project := resolved.ComposeProject
	if project == "" {
		config, err := r.readComposeConfig(ctx, resolved)
		if err != nil {
			return "", err
		}
		project = firstNonEmpty(config.Name, resolved.ComposeProject)
	}
	args := []string{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + project, "--filter", "label=com.docker.compose.service=" + resolved.ComposeService}
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
	return "", errManagedContainerNotFound
}

func (r *Runner) writeComposeOverride(ctx context.Context, resolved devcontainer.ResolvedConfig) error {
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return err
	}
	image, err := r.ensureComposeImage(ctx, resolved)
	if err != nil {
		return err
	}
	content := renderComposeOverride(resolved, image)
	return os.WriteFile(devcontainer.ComposeOverrideFile(resolved.StateDir), []byte(content), 0o644)
}

func renderComposeOverride(resolved devcontainer.ResolvedConfig, image string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  ")
	b.WriteString(resolved.ComposeService)
	b.WriteString(":\n")
	if len(resolved.Features) > 0 {
		b.WriteString("    pull_policy: never\n")
	}
	b.WriteString("    labels:\n")
	labels := map[string]string{}
	for key, value := range resolved.Labels {
		labels[key] = value
	}
	if metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata); err == nil && metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	for _, key := range sortedStringKeys(labels) {
		b.WriteString("      - ")
		b.WriteString(yamlQuoted(key + "=" + labels[key]))
		b.WriteString("\n")
	}
	if len(resolved.Merged.ContainerEnv) > 0 {
		b.WriteString("    environment:\n")
		for _, key := range devcontainer.SortedMapKeys(resolved.Merged.ContainerEnv) {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(key + "=" + resolved.Merged.ContainerEnv[key]))
			b.WriteString("\n")
		}
	}
	b.WriteString("    volumes:\n")
	allMounts := append([]string{resolved.WorkspaceMount}, resolved.Merged.Mounts...)
	namedVolumes := map[string]struct{}{}
	for _, mount := range allMounts {
		if value, ok := composeMountValue(mount); ok {
			b.WriteString("      - ")
			b.WriteString(value)
			b.WriteString("\n")
		}
		if source, ok := composeNamedVolume(mount); ok {
			namedVolumes[source] = struct{}{}
		}
	}
	if resolved.Merged.Init {
		b.WriteString("    init: true\n")
	}
	if resolved.Merged.Privileged {
		b.WriteString("    privileged: true\n")
	}
	if user := resolved.Merged.ContainerUser; user != "" {
		b.WriteString("    user: ")
		b.WriteString(yamlQuoted(user))
		b.WriteString("\n")
	}
	if overrideCommandEnabled(resolved.Config.OverrideCommand) {
		b.WriteString("    command: [\"/bin/sh\", \"-lc\", \"trap 'exit 0' TERM INT; while sleep 1000; do :; done\"]\n")
	}
	if len(resolved.Merged.CapAdd) > 0 {
		b.WriteString("    cap_add:\n")
		for _, value := range resolved.Merged.CapAdd {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(value))
			b.WriteString("\n")
		}
	}
	if len(resolved.Merged.SecurityOpt) > 0 {
		b.WriteString("    security_opt:\n")
		for _, value := range resolved.Merged.SecurityOpt {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(value))
			b.WriteString("\n")
		}
	}
	if image != "" {
		b.WriteString("    image: ")
		b.WriteString(yamlQuoted(image))
		b.WriteString("\n")
	}
	if len(namedVolumes) > 0 {
		b.WriteString("volumes:\n")
		for _, name := range sortedVolumeNames(namedVolumes) {
			b.WriteString("  ")
			b.WriteString(name)
			b.WriteString(":\n")
		}
	}
	return b.String()
}

func overrideCommandEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func composeMountValue(raw string) (string, bool) {
	parts := map[string]string{}
	for _, segment := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if !ok {
			continue
		}
		parts[key] = value
	}
	target := parts["target"]
	if target == "" {
		return "", false
	}
	switch parts["type"] {
	case "bind", "volume":
		source := parts["source"]
		if source == "" {
			return "", false
		}
		return yamlQuoted(source + ":" + target), true
	default:
		return "", false
	}
}

func composeNamedVolume(raw string) (string, bool) {
	parts := map[string]string{}
	for _, segment := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if !ok {
			continue
		}
		parts[key] = value
	}
	if parts["type"] != "volume" || parts["source"] == "" {
		return "", false
	}
	return parts["source"], true
}

func yamlQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedVolumeNames(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isNumericUser(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
