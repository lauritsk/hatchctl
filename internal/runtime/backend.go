package runtime

import (
	"context"
	"io"

	"github.com/lauritsk/hatchctl/internal/command"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
)

type runtimeBackend interface {
	Run(context.Context, runtimeCommand) error
	Output(context.Context, runtimeCommand) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
	BuildImage(context.Context, dockercli.BuildImageRequest) error
	RunDetachedContainer(context.Context, dockercli.RunDetachedContainerRequest) (string, error)
	StartContainer(context.Context, dockercli.StartContainerRequest) error
	RemoveContainer(context.Context, dockercli.RemoveContainerRequest) error
	ListContainers(context.Context, dockercli.ListContainersRequest) (string, error)
	ComposeConfig(context.Context, dockercli.ComposeConfigRequest) (string, error)
	ComposeBuild(context.Context, dockercli.ComposeBuildRequest) error
	ComposeUp(context.Context, dockercli.ComposeUpRequest) error
}

type runtimeCommandKind string

const (
	runtimeCommandDocker runtimeCommandKind = "docker"
	runtimeCommandHost   runtimeCommandKind = "host"
)

type runtimeCommand struct {
	Kind   runtimeCommandKind
	Phase  string
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
	docker      *dockercli.Client
	hostCommand command.Runner
}

func newLocalRuntimeBackend(runner *Runner, dockerClient *docker.Client) runtimeBackend {
	return &localRuntimeBackend{runner: runner, docker: dockercli.New(dockerClient), hostCommand: command.Local{}}
}

func (b *localRuntimeBackend) Run(ctx context.Context, cmd runtimeCommand) error {
	switch cmd.Kind {
	case runtimeCommandDocker:
		if len(cmd.Args) == 0 {
			return nil
		}
		opts := b.runner.progressDockerRunOptions(cmd.Events, cmd.Phase, cmd.Label, docker.RunOptions{Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr})
		return b.docker.Run(ctx, dockercli.CommandRequest{Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Streams: dockercli.Streams{Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: opts.Stderr}})
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
	return b.docker.InspectImage(ctx, dockercli.InspectImageRequest{Reference: image})
}

func (b *localRuntimeBackend) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return b.docker.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: containerID})
}

func (b *localRuntimeBackend) Output(ctx context.Context, cmd runtimeCommand) (string, error) {
	switch cmd.Kind {
	case runtimeCommandDocker:
		if len(cmd.Args) == 0 {
			return "", nil
		}
		return b.docker.Output(ctx, dockercli.CommandRequest{Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Streams: dockercli.Streams{Stdin: cmd.Stdin}})
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

func (b *localRuntimeBackend) BuildImage(ctx context.Context, req dockercli.BuildImageRequest) error {
	return b.docker.BuildImage(ctx, req)
}

func (b *localRuntimeBackend) RunDetachedContainer(ctx context.Context, req dockercli.RunDetachedContainerRequest) (string, error) {
	return b.docker.RunDetachedContainer(ctx, req)
}

func (b *localRuntimeBackend) StartContainer(ctx context.Context, req dockercli.StartContainerRequest) error {
	return b.docker.StartContainer(ctx, req)
}

func (b *localRuntimeBackend) RemoveContainer(ctx context.Context, req dockercli.RemoveContainerRequest) error {
	return b.docker.RemoveContainer(ctx, req)
}

func (b *localRuntimeBackend) ListContainers(ctx context.Context, req dockercli.ListContainersRequest) (string, error) {
	return b.docker.ListContainers(ctx, req)
}

func (b *localRuntimeBackend) ComposeConfig(ctx context.Context, req dockercli.ComposeConfigRequest) (string, error) {
	return b.docker.ComposeConfig(ctx, req)
}

func (b *localRuntimeBackend) ComposeBuild(ctx context.Context, req dockercli.ComposeBuildRequest) error {
	return b.docker.ComposeBuild(ctx, req)
}

func (b *localRuntimeBackend) ComposeUp(ctx context.Context, req dockercli.ComposeUpRequest) error {
	return b.docker.ComposeUp(ctx, req)
}

var _ runtimeBackend = (*localRuntimeBackend)(nil)
