package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

type commandIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func runHostLifecycle(ctx context.Context, cwd string, command devcontainer.LifecycleCommand, streams commandIO, backend runtimeBackend) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return backend.Run(ctx, runtimeCommand{Kind: runtimeCommandHost, Binary: args[0], Args: args[1:], Dir: cwd, Stdin: streams.Stdin, Stdout: streams.Stdout, Stderr: streams.Stderr})
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

func lifecycleProgressLabel(name string) string {
	return fmt.Sprintf("Running %s lifecycle hook", name)
}

func (r *Runner) runLifecyclePlan(ctx context.Context, observed reconcile.ObservedState, state devcontainer.State, dotfiles DotfilesOptions, allowHostLifecycle bool, events ui.Sink, plan reconcile.LifecyclePlan) error {
	if plan.RunCreate {
		if err := r.runCreateLifecycle(ctx, observed, state, dotfiles, true, allowHostLifecycle, events); err != nil {
			return err
		}
	}
	if plan.RunStart {
		if err := r.runStartLifecycle(ctx, observed, events); err != nil {
			return err
		}
	}
	if plan.RunAttach {
		if err := r.runAttachLifecycle(ctx, observed, events); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runCreateLifecycle(ctx context.Context, observed reconcile.ObservedState, state devcontainer.State, dotfiles DotfilesOptions, runDotfiles bool, allowHostLifecycle bool, events ui.Sink) error {
	resolved := observed.Resolved
	if err := policy.EnsureHostLifecycleAllowed(resolved.Config.InitializeCommand, allowHostLifecycle); err != nil {
		return err
	}
	if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, phaseLifecycle, lifecycleProgressLabel("initializeCommand"), r.commandIO()), r.backend); err != nil {
		return err
	}
	if err := r.runContainerLifecycleList(ctx, observed, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
		return err
	}
	if err := r.runContainerLifecycleList(ctx, observed, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
		return err
	}
	if err := r.runContainerLifecycleList(ctx, observed, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
		return err
	}
	if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
		if err := r.installDotfiles(ctx, observed, dotfiles, events); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runStartLifecycle(ctx context.Context, observed reconcile.ObservedState, events ui.Sink) error {
	return r.runContainerLifecycleList(ctx, observed, observed.Resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand"))
}

func (r *Runner) runAttachLifecycle(ctx context.Context, observed reconcile.ObservedState, events ui.Sink) error {
	return r.runContainerLifecycleList(ctx, observed, observed.Resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
}

func (r *Runner) runContainerLifecycleList(ctx context.Context, observed reconcile.ObservedState, commands []devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	for _, command := range commands {
		if err := r.runContainerLifecycle(ctx, observed, command, events, label); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runContainerLifecycle(ctx context.Context, observed reconcile.ObservedState, command devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs, err := r.dockerExecArgs(ctx, observed, true, false, nil, args)
		if err != nil {
			return err
		}
		return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Phase: phaseLifecycle, Label: label, Args: dockerArgs, Stdout: r.stdout, Stderr: r.stderr, Events: events})
	}, command)
}
