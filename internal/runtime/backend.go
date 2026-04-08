package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/hostexec"
)

type runtimeBackend interface {
	RunDocker(context.Context, string, docker.RunOptions, ui.Sink) error
	DockerOutput(context.Context, docker.RunOptions) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
	RunHost(context.Context, string, []string, commandIO) error
	BuildImage(context.Context, string, string, []string, ui.Sink) error
	StartContainer(context.Context, string, ui.Sink) error
	RemoveContainer(context.Context, string, ui.Sink) error
	ContainerStatus(context.Context, string) (string, error)
	RunContainer(context.Context, []string) (string, error)
	ComposeUp(context.Context, devcontainer.ResolvedConfig, string, ui.Sink) error
	ComposeBuild(context.Context, devcontainer.ResolvedConfig, ui.Sink) error
	DockerExec(ctx context.Context, label string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, events ui.Sink) error
}

type localRuntimeBackend struct {
	runner   *Runner
	executor hostexec.Executor
}

func newLocalRuntimeBackend(runner *Runner, executor hostexec.Executor) runtimeBackend {
	return &localRuntimeBackend{runner: runner, executor: executor}
}

func (b *localRuntimeBackend) RunDocker(ctx context.Context, label string, opts docker.RunOptions, events ui.Sink) error {
	return b.executor.RunDocker(ctx, b.runner.progressDockerRunOptions(events, label, opts))
}

func (b *localRuntimeBackend) DockerOutput(ctx context.Context, opts docker.RunOptions) (string, error) {
	if len(opts.Args) == 0 {
		return "", nil
	}
	return b.executor.DockerOutput(ctx, opts)
}

func (b *localRuntimeBackend) InspectImage(ctx context.Context, image string) (docker.ImageInspect, error) {
	return b.executor.InspectImage(ctx, image)
}

func (b *localRuntimeBackend) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return b.executor.InspectContainer(ctx, containerID)
}

func (b *localRuntimeBackend) RunHost(ctx context.Context, cwd string, args []string, streams commandIO) error {
	if len(args) == 0 {
		return nil
	}
	return b.executor.RunHost(ctx, hostexec.Command{Binary: args[0], Args: args[1:], Dir: cwd, Stdin: streams.Stdin, Stdout: streams.Stdout, Stderr: streams.Stderr})
}

func (b *localRuntimeBackend) BuildImage(ctx context.Context, label string, dir string, args []string, events ui.Sink) error {
	return b.RunDocker(ctx, label, docker.RunOptions{Args: args, Dir: dir, Stdout: b.runner.stdout, Stderr: b.runner.stderr}, events)
}

func (b *localRuntimeBackend) StartContainer(ctx context.Context, containerID string, events ui.Sink) error {
	return b.RunDocker(ctx, fmt.Sprintf("Starting existing container %s", containerID), docker.RunOptions{Args: []string{"start", containerID}, Stdout: b.runner.stdout, Stderr: b.runner.stderr}, events)
}

func (b *localRuntimeBackend) RemoveContainer(ctx context.Context, containerID string, events ui.Sink) error {
	return b.RunDocker(ctx, fmt.Sprintf("Removing managed container %s", containerID), docker.RunOptions{Args: []string{"rm", "-f", containerID}, Stdout: b.runner.stdout, Stderr: b.runner.stderr}, events)
}

func (b *localRuntimeBackend) ContainerStatus(ctx context.Context, containerID string) (string, error) {
	return b.executor.DockerOutput(ctx, docker.RunOptions{Args: []string{"inspect", "--format", "{{.State.Status}}", containerID}})
}

func (b *localRuntimeBackend) RunContainer(ctx context.Context, args []string) (string, error) {
	return b.executor.DockerOutput(ctx, docker.RunOptions{Args: args})
}

func (b *localRuntimeBackend) ComposeUp(ctx context.Context, resolved devcontainer.ResolvedConfig, overridePath string, events ui.Sink) error {
	return b.RunDocker(ctx, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), docker.RunOptions{Args: append(composeArgs(resolved, overridePath), "up", "--no-build", "-d", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: b.runner.stdout, Stderr: b.runner.stderr}, events)
}

func (b *localRuntimeBackend) ComposeBuild(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) error {
	return b.BuildImage(ctx, fmt.Sprintf("Building compose service %s", resolved.ComposeService), resolved.ConfigDir, append(composeBaseArgs(resolved), "build", resolved.ComposeService), events)
}

func (b *localRuntimeBackend) DockerExec(ctx context.Context, label string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, events ui.Sink) error {
	return b.RunDocker(ctx, label, docker.RunOptions{Args: args, Stdin: stdin, Stdout: stdout, Stderr: stderr}, events)
}

var _ runtimeBackend = (*localRuntimeBackend)(nil)
