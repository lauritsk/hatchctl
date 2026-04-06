package docker

import (
	"bytes"
	"context"
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
	return cmd.Run()
}

func (c *Client) Output(ctx context.Context, args ...string) (string, error) {
	return c.OutputOptions(ctx, RunOptions{Args: args})
}

func (c *Client) OutputOptions(ctx context.Context, opts RunOptions) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := c.Run(ctx, RunOptions{Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("docker %s: %s", strings.Join(opts.Args, " "), message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *Client) CombinedOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(string(data)), nil
}
