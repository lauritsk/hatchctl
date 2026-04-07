package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Client struct {
	Binary string
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
	return &Client{Binary: binary}
}

func (c *Client) Run(ctx context.Context, opts RunOptions) error {
	cmd := exec.CommandContext(ctx, c.Binary, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return &Error{Args: append([]string(nil), opts.Args...), Err: err}
	}
	return nil
}

func (c *Client) Output(ctx context.Context, args ...string) (string, error) {
	return c.OutputOptions(ctx, RunOptions{Args: args})
}

func (c *Client) OutputOptions(ctx context.Context, opts RunOptions) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := c.Run(ctx, RunOptions{Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		var dockerErr *Error
		if errors.As(err, &dockerErr) {
			dockerErr.Stderr = stderr.String()
			return "", dockerErr
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *Client) CombinedOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", &Error{Args: append([]string(nil), args...), Stderr: string(data), Err: err}
	}
	return strings.TrimSpace(string(data)), nil
}
