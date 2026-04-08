package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

type commandIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

var errHostLifecycleNotAllowed = errors.New("host lifecycle commands require explicit trust")

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

func (r *Runner) runLifecycleForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, state devcontainer.State, dotfiles DotfilesOptions, allowHostLifecycle bool, events ui.Sink) error {
	if created || !state.LifecycleReady {
		if err := ensureHostLifecycleAllowed(resolved.Config.InitializeCommand, allowHostLifecycle); err != nil {
			return err
		}
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), r.commandIO()), r.backend); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
	}
	if dotfiles.Enabled() && (created || !dotfilesStateMatches(state, dotfiles)) {
		if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
			return err
		}
	}
	if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand")); err != nil {
		return err
	}
	return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string, state devcontainer.State, dotfiles DotfilesOptions, runDotfiles bool, allowHostLifecycle bool, events ui.Sink) error {
	switch phase {
	case "all":
		if err := ensureHostLifecycleAllowed(resolved.Config.InitializeCommand, allowHostLifecycle); err != nil {
			return err
		}
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), r.commandIO()), r.backend); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand")); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
	case "create":
		if err := ensureHostLifecycleAllowed(resolved.Config.InitializeCommand, allowHostLifecycle); err != nil {
			return err
		}
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), r.commandIO()), r.backend); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		return nil
	case "start":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand"))
	case "attach":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
	default:
		return fmt.Errorf("invalid lifecycle phase %q; expected all, create, start, or attach", phase)
	}
}

func ensureHostLifecycleAllowed(command devcontainer.LifecycleCommand, allow bool) error {
	if command.Empty() || allow {
		return nil
	}
	return fmt.Errorf("%w; rerun with --allow-host-lifecycle or set HATCHCTL_ALLOW_HOST_LIFECYCLE=1", errHostLifecycleNotAllowed)
}

func (r *Runner) runContainerLifecycleList(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, commands []devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	for _, command := range commands {
		if err := r.runContainerLifecycle(ctx, containerID, resolved, command, events, label); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runContainerLifecycle(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, command devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs, err := r.dockerExecArgs(ctx, containerID, resolved, true, false, nil, args)
		if err != nil {
			return err
		}
		return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: label, Args: dockerArgs, Stdout: r.stdout, Stderr: r.stderr, Events: events})
	}, command)
}
