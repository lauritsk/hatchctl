package reconcile

import (
	"context"
	"fmt"
	"os"
	stdruntime "runtime"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	bridgecap "github.com/lauritsk/hatchctl/internal/capability/bridge"
	capdot "github.com/lauritsk/hatchctl/internal/capability/dotfiles"
	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/command"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func injectSSHAgent(merged spec.MergedConfig) (spec.MergedConfig, error) {
	return capssh.Inject(stdruntime.GOOS, os.Getenv("SSH_AUTH_SOCK"), merged)
}

func EnsureContainerHasSSHAgent(inspect *docker.ContainerInspect) error {
	return capssh.EnsureAttached(inspect)
}

func DotfilesStatusFromState(state storefs.WorkspaceState, cfg capdot.Config) *DotfilesStatus {
	status := capdot.StatusFor(state, cfg)
	if status == nil {
		return nil
	}
	return &DotfilesStatus{Configured: status.Configured, Applied: status.Applied, NeedsInstall: status.NeedsInstall, Repository: status.Repository, InstallCommand: status.InstallCommand, TargetPath: status.TargetPath}
}

func DotfilesNeedsInstall(state storefs.WorkspaceState, cfg capdot.Config) bool {
	status := DotfilesStatusFromState(state, cfg)
	return status != nil && status.NeedsInstall
}

func runHostLifecycle(ctx context.Context, cwd string, lifecycle spec.LifecycleCommand, streams commandIO, host command.Runner) error {
	if lifecycle.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return host.Run(ctx, command.Command{Binary: args[0], Args: args[1:], Dir: cwd, Stdin: streams.Stdin, Stdout: streams.Stdout, Stderr: streams.Stderr})
	}, lifecycle)
}

func runCommand(ctx context.Context, runner func(context.Context, []string) error, command spec.LifecycleCommand) error {
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

func (e *Executor) RunLifecyclePlan(ctx context.Context, observed ObservedState, state storefs.WorkspaceState, dotfiles capdot.Config, allowHostLifecycle bool, events ui.Sink, plan LifecyclePlan) error {
	if plan.RunCreate {
		if err := e.runCreateLifecycle(ctx, observed, state, dotfiles, true, allowHostLifecycle, events); err != nil {
			return err
		}
	}
	if plan.RunStart {
		if err := e.runStartLifecycle(ctx, observed, events); err != nil {
			return err
		}
	}
	if plan.RunAttach {
		if err := e.runAttachLifecycle(ctx, observed, events); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) runCreateLifecycle(ctx context.Context, observed ObservedState, state storefs.WorkspaceState, dotfiles capdot.Config, runDotfiles bool, allowHostLifecycle bool, events ui.Sink) error {
	resolved := observed.Resolved
	if err := policy.EnsureHostLifecycleAllowed(resolved.Config.InitializeCommand, allowHostLifecycle); err != nil {
		return err
	}
	if err := runHostLifecycle(ctx, resolved.WorkspaceFolder, resolved.Config.InitializeCommand, e.progressCommandIO(events, phaseLifecycle, lifecycleProgressLabel("initializeCommand"), e.commandIO()), e.hostCommand); err != nil {
		return err
	}
	if err := e.runContainerLifecycleList(ctx, observed, resolved.Merged.OnCreateCommands, events, lifecycleProgressLabel("onCreateCommand")); err != nil {
		return err
	}
	if err := e.runContainerLifecycleList(ctx, observed, resolved.Merged.UpdateContentCommands, events, lifecycleProgressLabel("updateContentCommand")); err != nil {
		return err
	}
	if err := e.runContainerLifecycleList(ctx, observed, resolved.Merged.PostCreateCommands, events, lifecycleProgressLabel("postCreateCommand")); err != nil {
		return err
	}
	if runDotfiles && dotfiles.Enabled() && !capdot.StateMatches(state, dotfiles) {
		if err := e.installDotfiles(ctx, observed, dotfiles, events); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) runStartLifecycle(ctx context.Context, observed ObservedState, events ui.Sink) error {
	return e.runContainerLifecycleList(ctx, observed, observed.Resolved.Merged.PostStartCommands, events, lifecycleProgressLabel("postStartCommand"))
}

func (e *Executor) runAttachLifecycle(ctx context.Context, observed ObservedState, events ui.Sink) error {
	return e.runContainerLifecycleList(ctx, observed, observed.Resolved.Merged.PostAttachCommands, events, lifecycleProgressLabel("postAttachCommand"))
}

func (e *Executor) runContainerLifecycleList(ctx context.Context, observed ObservedState, commands []spec.LifecycleCommand, events ui.Sink, label string) error {
	for _, command := range commands {
		if err := e.runContainerLifecycle(ctx, observed, command, events, label); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) runContainerLifecycle(ctx context.Context, observed ObservedState, command spec.LifecycleCommand, events ui.Sink, label string) error {
	if command.Empty() {
		return nil
	}
	return runCommand(ctx, func(ctx context.Context, args []string) error {
		stdout, stderr := e.progressWriters(events, phaseLifecycle, label, e.stdout, e.stderr)
		req, err := e.DockerExecRequest(ctx, observed, true, false, nil, args, dockercli.Streams{Stdout: stdout, Stderr: stderr})
		if err != nil {
			return err
		}
		return e.engine.Exec(ctx, req)
	}, command)
}

func (e *Executor) installDotfiles(ctx context.Context, observed ObservedState, cfg capdot.Config, events ui.Sink) error {
	if !cfg.Enabled() {
		return nil
	}
	targetPath, err := e.resolveDotfilesTargetPath(ctx, observed, cfg.TargetPath)
	if err != nil {
		return err
	}
	label := fmt.Sprintf("Installing dotfiles from %s", cfg.Repository)
	stdout, stderr := e.progressWriters(events, phaseDotfiles, label, e.stdout, e.stderr)
	req, err := e.DockerExecRequest(ctx, observed, true, false, nil, capdot.InstallArgs(cfg.Repository, targetPath, cfg.InstallCommand), dockercli.Streams{Stdin: strings.NewReader(capdot.InstallScript), Stdout: stdout, Stderr: stderr})
	if err != nil {
		return err
	}
	e.emitPhaseProgress(events, phaseDotfiles, label)
	return e.engine.Exec(ctx, req)
}

func (e *Executor) resolveDotfilesTargetPath(ctx context.Context, observed ObservedState, targetPath string) (string, error) {
	if !strings.HasPrefix(targetPath, "$HOME") {
		return targetPath, nil
	}
	user, err := e.effectiveExecUser(ctx, observed)
	if err != nil {
		return "", err
	}
	home, err := e.resolveExecHome(ctx, observed, user)
	if err != nil {
		return "", err
	}
	if home == "" {
		return targetPath, nil
	}
	return capdot.ResolveTargetPath(home, targetPath), nil
}

func bridgePreview(stateDir string) (*bridge.Session, error) {
	return bridgecap.Preview(stateDir, true)
}
