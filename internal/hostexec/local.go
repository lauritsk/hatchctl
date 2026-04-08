package hostexec

import (
	"context"
	"io"
	"os"
	"syscall"

	"github.com/lauritsk/hatchctl/internal/command"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type Command struct {
	Binary string
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type StartOptions struct {
	Command
	SysProcAttr *syscall.SysProcAttr
}

type Executor interface {
	RunHost(context.Context, Command) error
	OutputHost(context.Context, Command) (string, string, error)
	StartHost(StartOptions) (*os.Process, error)
	RunDocker(context.Context, docker.RunOptions) error
	DockerOutput(context.Context, docker.RunOptions) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
}

type Local struct {
	host   command.Runner
	docker *docker.Client
}

func NewLocal(dockerBinary string) *Local {
	return &Local{host: command.Local{}, docker: docker.NewClient(dockerBinary)}
}

func NewLocalWithDocker(client *docker.Client) *Local {
	return &Local{host: command.Local{}, docker: client}
}

func (e *Local) RunHost(ctx context.Context, cmd Command) error {
	return e.host.Run(ctx, command.Command{Binary: cmd.Binary, Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr})
}

func (e *Local) OutputHost(ctx context.Context, cmd Command) (string, string, error) {
	return e.host.Output(ctx, command.Command{Binary: cmd.Binary, Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr})
}

func (e *Local) StartHost(opts StartOptions) (*os.Process, error) {
	return e.host.Start(command.StartOptions{
		Command: command.Command{
			Binary: opts.Binary,
			Args:   opts.Args,
			Dir:    opts.Dir,
			Env:    opts.Env,
			Stdin:  opts.Stdin,
			Stdout: opts.Stdout,
			Stderr: opts.Stderr,
		},
		SysProcAttr: opts.SysProcAttr,
	})
}

func (e *Local) RunDocker(ctx context.Context, opts docker.RunOptions) error {
	return e.docker.Run(ctx, opts)
}

func (e *Local) DockerOutput(ctx context.Context, opts docker.RunOptions) (string, error) {
	return e.docker.OutputOptions(ctx, opts)
}

func (e *Local) InspectImage(ctx context.Context, image string) (docker.ImageInspect, error) {
	return e.docker.InspectImage(ctx, image)
}

func (e *Local) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	return e.docker.InspectContainer(ctx, containerID)
}

var _ Executor = (*Local)(nil)
