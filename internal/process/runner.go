package process

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type RunOptions struct {
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type StartOptions struct {
	Binary      string
	Args        []string
	Dir         string
	Env         []string
	Stdout      io.Writer
	Stderr      io.Writer
	SysProcAttr *syscall.SysProcAttr
}

type Runner struct{}

func (Runner) Run(ctx context.Context, binary string, args []string, opts RunOptions) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

func (r Runner) Output(ctx context.Context, binary string, args []string, opts RunOptions) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := r.Run(ctx, binary, args, RunOptions{Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: &stdout, Stderr: &stderr})
	return strings.TrimSpace(stdout.String()), stderr.String(), err
}

func (Runner) CombinedOutput(ctx context.Context, binary string, args []string, opts RunOptions) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdin = opts.Stdin
	data, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(data)), err
}

func (Runner) Start(opts StartOptions) (*os.Process, error) {
	cmd := exec.Command(opts.Binary, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = opts.SysProcAttr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
