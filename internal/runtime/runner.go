package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	"golang.org/x/term"
)

var isTerminal = term.IsTerminal

type Runner struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	backend       runtimeBackend
	imageVerifier *policy.ImageVerificationPolicy
	planner       *workspaceplan.Resolver
}

func NewRunner(client *docker.Client) *Runner {
	return NewRunnerWithIO(client, os.Stdin, os.Stdout, os.Stderr)
}

func NewRunnerWithIO(client *docker.Client, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Runner {
	runner := &Runner{
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		imageVerifier: policy.NewImageVerificationPolicy(stdin, stderr),
	}
	runner.backend = newLocalRuntimeBackend(runner, client)
	runner.planner = workspaceplan.NewResolver()
	return runner
}

type UpOptions struct {
	Plan               workspaceplan.WorkspacePlan
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
	Plan           workspaceplan.WorkspacePlan
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
	Plan           workspaceplan.WorkspacePlan
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
	Plan           workspaceplan.WorkspacePlan
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
	observed         reconcile.ObservedState
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
	Plan               workspaceplan.WorkspacePlan
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
		allowUnverified := feature.SourceKind == "oci" && (policy.AllowInsecureFeatureVerification() || policy.IsLoopbackOCIReference(feature.Resolved))
		if err := r.imageVerifier.ApplyFeature(feature.Source, feature.Verification, allowUnverified, events); err != nil {
			return err
		}
	}
	return nil
}

type BridgeDoctorOptions struct {
	Plan           workspaceplan.WorkspacePlan
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

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
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
	clone.imageVerifier = r.imageVerifier.CloneWithIO(clone.stdin, clone.stderr)
	if backend, ok := r.backend.(*localRuntimeBackend); ok {
		clone.backend = &localRuntimeBackend{runner: &clone, docker: backend.docker, hostCommand: backend.hostCommand}
	}
	clone.planner = r.planner.Clone()
	return &clone
}

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
