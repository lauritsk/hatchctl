package runtime

import (
	"context"
	"io"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	"github.com/lauritsk/hatchctl/internal/security"
)

type ObservedSessionOptions = observedSessionOptions
type Session = workspaceSession
type StateTracker = workspaceStateTracker

func (r *Runner) CloneWithIO(stdin io.Reader, stdout io.Writer, stderr io.Writer) *Runner {
	return r.withCommandIO(commandIO{Stdin: stdin, Stdout: stdout, Stderr: stderr})
}

func (r *Runner) PrepareObservedSession(ctx context.Context, opts ObservedSessionOptions) (*Session, error) {
	return r.prepareObservedSession(ctx, opts)
}

func NewStateTracker(stateDir string, state devcontainer.State) *StateTracker {
	return newWorkspaceStateTracker(stateDir, state)
}

func (r *Runner) ReconcileImage(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, reconcile.ImagePlan, error) {
	return r.reconcileImage(ctx, workspacePlan, resolved, events)
}

func (r *Runner) VerificationCheck() func(context.Context, string) security.VerificationResult {
	return r.imageVerifier.Check
}

func (r *Runner) VerifyResolvedFeatures(resolved devcontainer.ResolvedConfig, events ui.Sink) error {
	return r.verifyResolvedFeatures(resolved, events)
}

func (r *Runner) EnrichMergedConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig, image string) error {
	return r.enrichMergedConfig(ctx, resolved, image)
}

func InjectSSHAgent(merged devcontainer.MergedConfig) (devcontainer.MergedConfig, error) {
	return injectSSHAgent(merged)
}

func EnsureContainerHasSSHAgent(inspect *docker.ContainerInspect, target string) error {
	return ensureContainerHasSSHAgent(inspect, target)
}

func SSHAgentContainerSocketPath() string {
	return sshAgentContainerSocketPath
}

func (r *Runner) InspectImageArchitecture(ctx context.Context, image string) (string, error) {
	return r.inspectImageArchitecture(ctx, image)
}

func (r *Runner) ReconcileContainer(ctx context.Context, observed reconcile.ObservedState, resolved devcontainer.ResolvedConfig, image string, imagePlan reconcile.ImagePlan, bridgeEnabled bool, sshAgent bool, forceNew bool, events ui.Sink) (string, string, bool, error) {
	return r.reconcileContainer(ctx, observed, resolved, image, imagePlan, bridgeEnabled, sshAgent, forceNew, events)
}

func (r *Runner) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return r.backend.InspectContainer(ctx, containerID)
}

func (r *Runner) EnsureUpdatedUIDContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerID string, events ui.Sink) error {
	return r.ensureUpdatedUIDContainer(ctx, resolved, image, containerID, events)
}

func (r *Runner) DesiredLifecycleKey(resolved devcontainer.ResolvedConfig, containerKey string, dotfiles DotfilesOptions) (string, error) {
	return r.desiredLifecycleKey(resolved, containerKey, dotfiles)
}

func (r *Runner) RunLifecyclePlan(ctx context.Context, observed reconcile.ObservedState, state devcontainer.State, dotfiles DotfilesOptions, allowHostLifecycle bool, events ui.Sink, plan reconcile.LifecyclePlan) error {
	return r.runLifecyclePlan(ctx, observed, state, dotfiles, allowHostLifecycle, events, plan)
}

func (r *Runner) DockerExecRequest(ctx context.Context, observed reconcile.ObservedState, stdin bool, tty bool, extraEnv map[string]string, command []string, streams dockercli.Streams) (dockercli.ExecRequest, error) {
	return r.dockerExecRequest(ctx, observed, stdin, tty, extraEnv, command, streams)
}

func (r *Runner) ExecuteContainerCommand(ctx context.Context, req dockercli.ExecRequest) error {
	return r.backend.Exec(ctx, req)
}

func (r *Runner) InspectImageUser(ctx context.Context, image string) (string, error) {
	return r.inspectImageUser(ctx, image)
}

func (r *Runner) EmitPhaseProgress(events ui.Sink, phase string, message string) {
	r.emitPhaseProgress(events, phase, message)
}

func (r *Runner) EmitPlan(events ui.Sink, resolved devcontainer.ResolvedConfig) {
	r.emitPlan(events, resolved)
}

func (r *Runner) ClearProgress(events ui.Sink) {
	r.clearProgress(events)
}

func ShouldAllocateTTY(stdin io.Reader, stdout io.Writer) bool {
	return shouldAllocateTTY(stdin, stdout)
}

func RedactSensitiveMap(values map[string]string) map[string]string {
	return redactSensitiveMap(values)
}

func DotfilesStatusFromState(state devcontainer.State, opts DotfilesOptions) *DotfilesStatus {
	return dotfilesStatus(state, opts)
}

func DotfilesNeedsInstall(state devcontainer.State, opts DotfilesOptions) bool {
	status := dotfilesStatus(state, opts)
	return status != nil && status.NeedsInstall
}
