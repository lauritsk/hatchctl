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
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.commandIO(), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands); err != nil {
			return err
		}
	}
	if dotfiles.Enabled() && (created || !dotfilesStateMatches(state, dotfiles)) {
		if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
			return err
		}
	}
	if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
		return err
	}
	return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string, state devcontainer.State, dotfiles DotfilesOptions, runDotfiles bool, events ui.Sink) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.commandIO(), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, r.commandIO(), r.hostCommandRunner); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := r.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		return nil
	case "start":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands)
	case "attach":
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
	default:
		return fmt.Errorf("unknown lifecycle phase %q", phase)
	}
}

func (r *Runner) runContainerLifecycleList(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, commands []devcontainer.LifecycleCommand) error {
	for _, command := range commands {
		if err := r.runContainerLifecycle(ctx, containerID, resolved, command); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runContainerLifecycle(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, command devcontainer.LifecycleCommand) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs := []string{"exec", "-i"}
		user := resolved.Merged.RemoteUser
		if user == "" {
			user = resolved.Merged.ContainerUser
		}
		if user != "" {
			dockerArgs = append(dockerArgs, "-u", user)
		}
		for _, key := range devcontainer.SortedMapKeys(resolved.Merged.RemoteEnv) {
			value := resolved.Merged.RemoteEnv[key]
			dockerArgs = append(dockerArgs, "-e", key+"="+value)
		}
		dockerArgs = append(dockerArgs, containerID)
		dockerArgs = append(dockerArgs, args...)
		return r.docker.Run(ctx, docker.RunOptions{Args: dockerArgs, Stdout: r.stdout, Stderr: r.stderr})
	}, command)
}
