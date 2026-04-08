package runtime

import (
	"context"
	"fmt"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type runtimeEngineAdapter struct {
	runner *Runner
}

func (e *runtimeEngineAdapter) Run(ctx context.Context, label string, opts docker.RunOptions, events ui.Sink) error {
	return e.runner.docker.Run(ctx, e.runner.progressDockerRunOptions(events, label, opts))
}

func (e *runtimeEngineAdapter) BuildImage(ctx context.Context, label string, dir string, args []string, events ui.Sink) error {
	return e.Run(ctx, label, docker.RunOptions{Args: args, Dir: dir, Stdout: e.runner.stdout, Stderr: e.runner.stderr}, events)
}

func (e *runtimeEngineAdapter) StartContainer(ctx context.Context, containerID string, events ui.Sink) error {
	return e.Run(ctx, fmt.Sprintf("Starting existing container %s", containerID), docker.RunOptions{Args: []string{"start", containerID}, Stdout: e.runner.stdout, Stderr: e.runner.stderr}, events)
}

func (e *runtimeEngineAdapter) RemoveContainer(ctx context.Context, containerID string, events ui.Sink) error {
	return e.Run(ctx, fmt.Sprintf("Removing managed container %s", containerID), docker.RunOptions{Args: []string{"rm", "-f", containerID}, Stdout: e.runner.stdout, Stderr: e.runner.stderr}, events)
}

func (e *runtimeEngineAdapter) ContainerStatus(ctx context.Context, containerID string) (string, error) {
	return e.runner.docker.Output(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
}

func (e *runtimeEngineAdapter) RunContainer(ctx context.Context, args []string) (string, error) {
	return e.runner.docker.Output(ctx, args...)
}

func (e *runtimeEngineAdapter) ComposeUp(ctx context.Context, resolved devcontainer.ResolvedConfig, overridePath string, events ui.Sink) error {
	return e.Run(ctx, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), docker.RunOptions{Args: append(e.runner.composeArgs(resolved, overridePath), "up", "--no-build", "-d", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: e.runner.stdout, Stderr: e.runner.stderr}, events)
}

func (e *runtimeEngineAdapter) ComposeBuild(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) error {
	return e.BuildImage(ctx, fmt.Sprintf("Building compose service %s", resolved.ComposeService), resolved.ConfigDir, append(e.runner.composeBaseArgs(resolved), "build", resolved.ComposeService), events)
}
