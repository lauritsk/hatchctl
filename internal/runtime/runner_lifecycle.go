package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func (r *Runner) runLifecycleForUp(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, created bool, lifecycleReady bool) error {
	if created || !lifecycleReady {
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
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
	if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
		return err
	}
	return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
}

func (r *Runner) runLifecyclePhase(ctx context.Context, resolved devcontainer.ResolvedConfig, containerID string, phase string) error {
	switch phase {
	case "all":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
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
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostStartCommands); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostAttachCommands)
	case "create":
		if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.OnCreateCommands); err != nil {
			return err
		}
		if err := r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.UpdateContentCommands); err != nil {
			return err
		}
		return r.runContainerLifecycleList(ctx, containerID, resolved, resolved.Merged.PostCreateCommands)
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
		return r.docker.Run(ctx, docker.RunOptions{Args: dockerArgs, Stdout: os.Stdout, Stderr: os.Stderr})
	}, command)
}
