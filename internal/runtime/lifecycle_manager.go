package runtime

import (
	"context"
	"fmt"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

type runtimeLifecycleManager struct {
	runner *Runner
}

func lifecycleProgressLabel(name string) string {
	return fmt.Sprintf("Running %s lifecycle hook", name)
}

func (m *runtimeLifecycleManager) RunForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, state devcontainer.State, dotfiles DotfilesOptions, events ui.Sink) error {
	if created || !state.LifecycleReady {
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, m.runner.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), m.runner.commandIO()), m.runner.backend); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
	}
	if dotfiles.Enabled() && (created || !dotfilesStateMatches(state, dotfiles)) {
		if err := m.runner.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
			return err
		}
	}
	if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand")); err != nil {
		return err
	}
	return m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
}

func (m *runtimeLifecycleManager) RunPhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string, state devcontainer.State, dotfiles DotfilesOptions, runDotfiles bool, events ui.Sink) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, m.runner.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), m.runner.commandIO()), m.runner.backend); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := m.runner.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand")); err != nil {
			return err
		}
		return m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, m.runner.progressCommandIO(events, lifecycleProgressLabel("initializeCommand"), m.runner.commandIO()), m.runner.backend); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
			return err
		}
		if err := m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
			return err
		}
		if runDotfiles && dotfiles.Enabled() && !dotfilesStateMatches(state, dotfiles) {
			if err := m.runner.installDotfiles(ctx, containerID, resolved, dotfiles, events); err != nil {
				return err
			}
		}
		return nil
	case "start":
		return m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand"))
	case "attach":
		return m.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
	default:
		return fmt.Errorf("invalid lifecycle phase %q; expected all, create, start, or attach", phase)
	}
}

func (m *runtimeLifecycleManager) runContainerLifecycleList(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, commands []devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	for _, command := range commands {
		if err := m.runContainerLifecycle(ctx, containerID, resolved, command, events, label); err != nil {
			return err
		}
	}
	return nil
}

func (m *runtimeLifecycleManager) runContainerLifecycle(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, command devcontainer.LifecycleCommand, events ui.Sink, label string) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		dockerArgs, err := m.runner.dockerExecArgs(ctx, containerID, resolved, true, false, nil, args)
		if err != nil {
			return err
		}
		return m.runner.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: label, Args: dockerArgs, Stdout: m.runner.stdout, Stderr: m.runner.stderr, Events: events})
	}, command)
}
