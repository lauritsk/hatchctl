package runtime

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type hostCommandRunner func(context.Context, string, []string, commandIO) error

type commandIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func defaultHostCommandRunner(ctx context.Context, cwd string, args []string, streams commandIO) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr
	cmd.Stdin = streams.Stdin
	return cmd.Run()
}

func runHostLifecycle(ctx context.Context, cwd string, command devcontainer.LifecycleCommand, streams commandIO, runner hostCommandRunner) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		return runner(ctx, cwd, args, streams)
	}, command)
}

func runCommand(ctx context.Context, runner func(context.Context, []string) error, command devcontainer.LifecycleCommand) error {
	switch command.Kind {
	case "string":
		return runner(ctx, []string{"/bin/sh", "-lc", command.Value})
	case "array":
		if len(command.Args) == 0 {
			return nil
		}
		return runner(ctx, command.Args)
	case "object":
		for _, step := range command.SortedSteps() {
			if err := runCommand(ctx, runner, step.Command); err != nil {
				return fmt.Errorf("lifecycle step %s: %w", step.Name, err)
			}
		}
		return nil
	default:
		return nil
	}
}
