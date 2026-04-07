package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func runHostLifecycle(ctx context.Context, cwd string, command devcontainer.LifecycleCommand) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
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
