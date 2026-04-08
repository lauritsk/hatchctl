package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/command"
)

type Client struct {
	Binary string
	runner command.Runner
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
	return &Client{Binary: binary, runner: command.Local{}}
}

func (c *Client) Run(ctx context.Context, opts RunOptions) error {
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if shouldUseCombinedOutput(opts.Args) {
			data, err := c.runner.CombinedOutput(ctx, command.Command{Binary: c.Binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin})
			if data != "" && opts.Stdout != nil {
				_, _ = io.WriteString(opts.Stdout, data)
				if !strings.HasSuffix(data, "\n") {
					_, _ = io.WriteString(opts.Stdout, "\n")
				}
			}
			if err == nil {
				return nil
			}
			if !retryableExit255(err) || attempt == maxAttempts || ctx.Err() != nil {
				return &Error{Args: append([]string(nil), opts.Args...), Stderr: data, Err: err}
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		stderr := opts.Stderr
		var stderrBuffer bytes.Buffer
		if stderr == nil {
			stderr = &stderrBuffer
		}
		err := c.runner.Run(ctx, command.Command{Binary: c.Binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: stderr})
		if err == nil {
			return nil
		}
		if !retryableExit255(err) || attempt == maxAttempts || ctx.Err() != nil {
			return &Error{Args: append([]string(nil), opts.Args...), Stderr: stderrBuffer.String(), Err: err}
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return nil
}

func retryableExit255(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == 255
}

func shouldUseCombinedOutput(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "build" {
		return true
	}
	if args[0] != "compose" {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "build" || arg == "up" {
			return true
		}
	}
	return false
}

func (c *Client) Output(ctx context.Context, args ...string) (string, error) {
	return c.OutputOptions(ctx, RunOptions{Args: args})
}

func (c *Client) OutputOptions(ctx context.Context, opts RunOptions) (string, error) {
	stdout, stderr, err := c.runner.Output(ctx, command.Command{Binary: c.Binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin})
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
	data, err := c.runner.CombinedOutput(ctx, command.Command{Binary: c.Binary, Args: args})
	if err != nil {
		return "", &Error{Args: append([]string(nil), args...), Stderr: data, Err: err}
	}
	return data, nil
}
