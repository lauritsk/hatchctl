package runtime

import (
	"context"
	"io"

	"github.com/lauritsk/hatchctl/internal/command"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type runtimeBackend interface {
	Run(context.Context, runtimeCommand) error
	Output(context.Context, runtimeCommand) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
}

type runtimeCommandKind string

const (
	runtimeCommandDocker runtimeCommandKind = "docker"
	runtimeCommandHost   runtimeCommandKind = "host"
)

type runtimeCommand struct {
	Kind   runtimeCommandKind
	Label  string
	Binary string
	Dir    string
	Env    []string
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Events ui.Sink
}

type localRuntimeBackend struct {
	runner      *Runner
	docker      *docker.Client
	hostCommand command.Runner
}

func newLocalRuntimeBackend(runner *Runner, dockerClient *docker.Client) runtimeBackend {
	return &localRuntimeBackend{runner: runner, docker: dockerClient, hostCommand: command.Local{}}
}

func (b *localRuntimeBackend) Run(ctx context.Context, cmd runtimeCommand) error {
	switch cmd.Kind {
	case runtimeCommandDocker:
		if len(cmd.Args) == 0 {
			return nil
		}
		return b.docker.Run(ctx, b.runner.progressDockerRunOptions(cmd.Events, cmd.Label, docker.RunOptions{Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr}))
	case runtimeCommandHost:
		if cmd.Binary == "" {
			return nil
		}
		return b.hostCommand.Run(ctx, command.Command{Binary: cmd.Binary, Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr})
	default:
		return nil
	}
}

func (b *localRuntimeBackend) InspectImage(ctx context.Context, image string) (docker.ImageInspect, error) {
	return b.docker.InspectImage(ctx, image)
}

func (b *localRuntimeBackend) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return b.docker.InspectContainer(ctx, containerID)
}

func (b *localRuntimeBackend) Output(ctx context.Context, cmd runtimeCommand) (string, error) {
	switch cmd.Kind {
	case runtimeCommandDocker:
		if len(cmd.Args) == 0 {
			return "", nil
		}
		return b.docker.OutputOptions(ctx, docker.RunOptions{Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin})
	case runtimeCommandHost:
		if cmd.Binary == "" {
			return "", nil
		}
		stdout, _, err := b.hostCommand.Output(ctx, command.Command{Binary: cmd.Binary, Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin})
		return stdout, err
	default:
		return "", nil
	}
}

var _ runtimeBackend = (*localRuntimeBackend)(nil)
