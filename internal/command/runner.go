package command

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
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

type Runner interface {
	Run(context.Context, Command) error
	Output(context.Context, Command) (string, string, error)
	CombinedOutput(context.Context, Command) (string, error)
	Start(StartOptions) (*os.Process, error)
}

type Local struct{}

func (Local) Run(ctx context.Context, cmd Command) error {
	process := exec.CommandContext(ctx, cmd.Binary, cmd.Args...)
	process.Dir = cmd.Dir
	process.Env = cmd.Env
	process.Stdin = cmd.Stdin
	process.Stdout = cmd.Stdout
	process.Stderr = cmd.Stderr
	return process.Run()
}

func (r Local) Output(ctx context.Context, cmd Command) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := r.Run(ctx, Command{Binary: cmd.Binary, Args: cmd.Args, Dir: cmd.Dir, Env: cmd.Env, Stdin: cmd.Stdin, Stdout: &stdout, Stderr: &stderr})
	return strings.TrimSpace(stdout.String()), stderr.String(), err
}

func (Local) CombinedOutput(ctx context.Context, cmd Command) (string, error) {
	process := exec.CommandContext(ctx, cmd.Binary, cmd.Args...)
	process.Dir = cmd.Dir
	process.Env = cmd.Env
	process.Stdin = cmd.Stdin
	data, err := process.CombinedOutput()
	return strings.TrimSpace(string(data)), err
}

func (Local) Start(opts StartOptions) (*os.Process, error) {
	process := exec.Command(opts.Binary, opts.Args...)
	process.Dir = opts.Dir
	process.Env = opts.Env
	process.Stdout = opts.Stdout
	process.Stderr = opts.Stderr
	process.SysProcAttr = opts.SysProcAttr
	if err := process.Start(); err != nil {
		return nil, err
	}
	return process.Process, nil
}

func AppendEnv(base []string, extra ...string) []string {
	result := append([]string(nil), base...)
	result = append(result, extra...)
	return result
}
