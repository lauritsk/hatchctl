package reconcile

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/command"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/security"
	"golang.org/x/term"
)

type engine interface {
	InspectImage(context.Context, dockercli.InspectImageRequest) (docker.ImageInspect, error)
	InspectContainer(context.Context, dockercli.InspectContainerRequest) (docker.ContainerInspect, error)
	BuildImage(context.Context, dockercli.BuildImageRequest) error
	RunDetachedContainer(context.Context, dockercli.RunDetachedContainerRequest) (string, error)
	StartContainer(context.Context, dockercli.StartContainerRequest) error
	RemoveContainer(context.Context, dockercli.RemoveContainerRequest) error
	ListContainers(context.Context, dockercli.ListContainersRequest) (string, error)
	ComposeConfig(context.Context, dockercli.ComposeConfigRequest) (string, error)
	ComposeBuild(context.Context, dockercli.ComposeBuildRequest) error
	ComposeUp(context.Context, dockercli.ComposeUpRequest) error
	Exec(context.Context, dockercli.ExecRequest) error
	ExecOutput(context.Context, dockercli.ExecRequest) (string, error)
}

type observerEngine struct {
	engine engine
}

type Executor struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	engine        engine
	hostCommand   command.Runner
	imageVerifier *policy.ImageVerificationPolicy
	planner       *workspaceplan.Resolver
}

type DotfilesStatus struct {
	Configured     bool   `json:"configured"`
	Applied        bool   `json:"applied"`
	NeedsInstall   bool   `json:"needsInstall"`
	Repository     string `json:"repository,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
}

type UpResult struct {
	ContainerID           string         `json:"containerId"`
	Image                 string         `json:"image"`
	RemoteWorkspaceFolder string         `json:"remoteWorkspaceFolder"`
	StateDir              string         `json:"stateDir"`
	Bridge                *bridge.Report `json:"bridge,omitempty"`
}

type BuildResult struct {
	Image string `json:"image"`
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

type RunLifecycleResult struct {
	ContainerID string `json:"containerId"`
	Phase       string `json:"phase"`
}

type ExitError struct {
	Code int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
}

type commandIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type preparedWorkspace struct {
	resolved         devcontainer.ResolvedConfig
	image            string
	state            devcontainer.State
	containerID      string
	containerInspect *docker.ContainerInspect
	observed         ObservedState
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

var isTerminal = term.IsTerminal

func NewExecutor(client *docker.Client) *Executor {
	return NewExecutorWithIO(client, os.Stdin, os.Stdout, os.Stderr)
}

func NewExecutorWithIO(client *docker.Client, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Executor {
	executor := &Executor{
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		hostCommand:   command.Local{},
		imageVerifier: policy.NewImageVerificationPolicy(stdin, stderr),
		planner:       workspaceplan.NewResolver(),
	}
	if client != nil {
		executor.engine = dockercli.New(client)
	}
	return executor
}

func (e *Executor) cloneWithIO(stdin io.Reader, stdout io.Writer, stderr io.Writer) *Executor {
	clone := *e
	if stdin == nil {
		stdin = e.stdin
	}
	if stdout == nil {
		stdout = e.stdout
	}
	if stderr == nil {
		stderr = e.stderr
	}
	clone.stdin = stdin
	clone.stdout = stdout
	clone.stderr = stderr
	if e.imageVerifier != nil {
		clone.imageVerifier = e.imageVerifier.CloneWithIO(stdin, stderr)
	} else {
		clone.imageVerifier = policy.NewImageVerificationPolicy(stdin, stderr)
	}
	if e.planner != nil {
		clone.planner = e.planner.Clone()
	} else {
		clone.planner = workspaceplan.NewResolver()
	}
	if clone.hostCommand == nil {
		clone.hostCommand = command.Local{}
	}
	return &clone
}

func (e *Executor) VerificationCheck() func(context.Context, string) security.VerificationResult {
	return e.imageVerifier.Check
}

func (e *Executor) VerifyResolvedFeatures(resolved devcontainer.ResolvedConfig, events ui.Sink) error {
	for _, feature := range resolved.Features {
		allowUnverified := feature.SourceKind == "oci" && (policy.AllowInsecureFeatureVerification() || policy.IsLoopbackOCIReference(feature.Resolved))
		if err := e.imageVerifier.ApplyFeature(feature.Source, feature.Verification, allowUnverified, events); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) Materialize(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, debug bool, events ui.Sink, phase string, label string) (devcontainer.ResolvedConfig, error) {
	e.emitPhaseProgress(events, phase, label)
	resolved, err := e.planner.Materialize(ctx, workspacePlan, e.VerificationCheck())
	if err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if err := e.VerifyResolvedFeatures(resolved, events); err != nil {
		return devcontainer.ResolvedConfig{}, err
	}
	if debug {
		e.emitResolvedPlan(events, resolved)
	}
	return resolved, nil
}

func preparedImage(resolved devcontainer.ResolvedConfig) string {
	image := resolved.Config.Image
	if image == "" && resolved.SourceKind != "compose" {
		image = resolved.ImageName
	}
	return image
}

func (e *Executor) emitPhaseProgress(events ui.Sink, phase string, message string) {
	if events == nil || message == "" {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventProgress, Phase: phase, Message: message})
}

func (e *Executor) clearProgress(events ui.Sink) {
	if events == nil {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventClear})
}

func (e *Executor) emitResolvedPlan(events ui.Sink, resolved devcontainer.ResolvedConfig) {
	if events == nil {
		return
	}
	events.Emit(ui.Event{Kind: ui.EventDebug, Message: fmt.Sprintf("plan source=%s config=%s workspace=%s state=%s target-image=%s", resolved.SourceKind, resolved.ConfigPath, resolved.WorkspaceFolder, resolved.StateDir, resolved.ImageName)})
}

func (e *Executor) verifyImageReference(ctx context.Context, ref string, events ui.Sink) error {
	return e.imageVerifier.ApplyImage(e.imageVerifier.Check(ctx, ref), events)
}

func (e *Executor) commandIO() commandIO {
	return commandIO{Stdin: e.stdin, Stdout: e.stdout, Stderr: e.stderr}
}

func ShouldAllocateTTY(stdin io.Reader, stdout io.Writer) bool {
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
	return isTerminal(int(fd))
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

func (e *Executor) observerBackend() backend {
	return observerEngine{engine: e.engine}
}

func (b observerEngine) InspectImage(ctx context.Context, ref string) (docker.ImageInspect, error) {
	return b.engine.InspectImage(ctx, dockercli.InspectImageRequest{Reference: ref})
}

func (b observerEngine) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return b.engine.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: containerID})
}

func (b observerEngine) ListContainers(ctx context.Context, req dockercli.ListContainersRequest) (string, error) {
	return b.engine.ListContainers(ctx, req)
}
