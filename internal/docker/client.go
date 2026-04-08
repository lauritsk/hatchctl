package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/lauritsk/hatchctl/internal/process"
)

type Client struct {
	Binary string
	runner process.Runner
}

type RunOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
	Env    []string
	Args   []string
}

type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	message := strings.TrimSpace(e.Stderr)
	if message == "" {
		message = e.Err.Error()
	}
	return fmt.Sprintf("docker %s: %s", strings.Join(e.Args, " "), message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) ExitCode() (int, bool) {
	var exitErr *exec.ExitError
	if !errors.As(e.Err, &exitErr) {
		return 0, false
	}
	return exitErr.ExitCode(), true
}

func IsNotFound(err error) bool {
	var dockerErr *Error
	if !errors.As(err, &dockerErr) {
		return false
	}
	message := strings.ToLower(dockerErr.Stderr)
	return strings.Contains(message, "no such") || strings.Contains(message, "not found")
}

func NewClient(binary string) *Client {
	return &Client{Binary: binary, runner: process.Runner{}}
}

func (c *Client) Run(ctx context.Context, opts RunOptions) error {
	if err := c.runner.Run(ctx, c.Binary, opts.Args, process.RunOptions{Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: opts.Stderr}); err != nil {
		return &Error{Args: append([]string(nil), opts.Args...), Err: err}
	}
	return nil
}

func (c *Client) Output(ctx context.Context, args ...string) (string, error) {
	return c.OutputOptions(ctx, RunOptions{Args: args})
}

func (c *Client) OutputOptions(ctx context.Context, opts RunOptions) (string, error) {
	stdout, stderr, err := c.runner.Output(ctx, c.Binary, opts.Args, process.RunOptions{Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin})
	if err != nil {
		var dockerErr *Error
		if errors.As(err, &dockerErr) {
			dockerErr.Stderr = stderr
			return "", dockerErr
		}
		return "", &Error{Args: append([]string(nil), opts.Args...), Stderr: stderr, Err: err}
	}
	return stdout, nil
}

func (c *Client) CombinedOutput(ctx context.Context, args ...string) (string, error) {
	data, err := c.runner.CombinedOutput(ctx, c.Binary, args, process.RunOptions{})
	if err != nil {
		return "", &Error{Args: append([]string(nil), args...), Stderr: data, Err: err}
	}
	return data, nil
}
