package runtime

import (
	"context"
	"fmt"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func (r *Runner) runLifecycleForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, state devcontainer.State, dotfiles DotfilesOptions, events ui.Sink) error {
	if created || !state.LifecycleReady {
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, "Running initializeCommand", r.commandIO()), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, "Running onCreateCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, "Running updateContentCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, "Running postCreateCommand"); err != nil {
			return err
		}
	}
	if dotfiles.Enabled() && (created || !dotfilesStateMatches(state, dotfiles)) {
		if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
			return err
		}
	}
	if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, "Running postStartCommand"); err != nil {
		return err
	}
	return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, "Running postAttachCommand")
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string, state devcontainer.State, dotfiles DotfilesOptions, runDotfiles bool, events ui.Sink) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, "Running initializeCommand", r.commandIO()), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, "Running onCreateCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, "Running updateContentCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, "Running postCreateCommand"); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, "Running postStartCommand"); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, "Running postAttachCommand")
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.progressCommandIO(events, "Running initializeCommand", r.commandIO()), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, "Running onCreateCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, "Running updateContentCommand"); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, "Running postCreateCommand"); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		return nil
	case "start":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, "Running postStartCommand")
	case "attach":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, "Running postAttachCommand")
	default:
		return fmt.Errorf("unknown lifecycle phase %q", phase)
	}
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
		return r.docker.Run(ctx, r.progressDockerRunOptions(events, label, docker.RunOptions{Args: dockerArgs, Stdout: r.stdout, Stderr: r.stderr}))
	}, command)
}
