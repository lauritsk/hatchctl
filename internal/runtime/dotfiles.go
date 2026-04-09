package runtime

import (
	"context"
	"fmt"
	"strings"

	capdot "github.com/lauritsk/hatchctl/internal/capability/dotfiles"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

type DotfilesOptions struct {
	Repository     string `json:"repository,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
}

type DotfilesStatus struct {
	Configured     bool   `json:"configured"`
	Applied        bool   `json:"applied"`
	NeedsInstall   bool   `json:"needsInstall"`
	Repository     string `json:"repository,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
}

func (o DotfilesOptions) Enabled() bool {
	return capdot.Config{Repository: o.Repository}.Enabled()
}

func (o DotfilesOptions) Normalized() (DotfilesOptions, error) {
	config, err := capdot.Normalize(capdot.Config{Repository: o.Repository, InstallCommand: o.InstallCommand, TargetPath: o.TargetPath})
	if err != nil {
		return DotfilesOptions{}, err
	}
	return DotfilesOptions{Repository: config.Repository, InstallCommand: config.InstallCommand, TargetPath: config.TargetPath}, nil
}

func normalizeDotfilesRepository(value string) (string, error) {
	return capdot.NormalizeRepository(value)
}

func normalizeDotfilesTargetPath(value string) string {
	return capdot.NormalizeTargetPath(value)
}

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func dotfilesStateMatches(state devcontainer.State, opts DotfilesOptions) bool {
	return capdot.StateMatches(state, capdot.Config{Repository: opts.Repository, InstallCommand: opts.InstallCommand, TargetPath: opts.TargetPath})
}

func dotfilesStatus(state devcontainer.State, opts DotfilesOptions) *DotfilesStatus {
	status := capdot.StatusFor(state, capdot.Config{Repository: opts.Repository, InstallCommand: opts.InstallCommand, TargetPath: opts.TargetPath})
	if status == nil {
		return nil
	}
	return &DotfilesStatus{
		Configured:     status.Configured,
		Applied:        status.Applied,
		NeedsInstall:   status.NeedsInstall,
		Repository:     status.Repository,
		InstallCommand: status.InstallCommand,
		TargetPath:     status.TargetPath,
	}
}

func (r *Runner) installDotfiles(ctx context.Context, observed reconcile.ObservedState, opts DotfilesOptions, events ui.Sink) error {
	if !opts.Enabled() {
		return nil
	}
	targetPath, err := r.resolveDotfilesTargetPath(ctx, observed, opts.TargetPath)
	if err != nil {
		return err
	}
	args, err := r.dockerExecArgs(ctx, observed, true, false, nil, capdot.InstallArgs(opts.Repository, targetPath, opts.InstallCommand))
	if err != nil {
		return err
	}
	label := fmt.Sprintf("Installing dotfiles from %s", opts.Repository)
	r.emitPhaseProgress(events, phaseDotfiles, label)
	return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Phase: phaseDotfiles, Label: label, Args: args, Stdin: strings.NewReader(capdot.InstallScript), Stdout: r.stdout, Stderr: r.stderr, Events: events})
}

func (r *Runner) resolveDotfilesTargetPath(ctx context.Context, observed reconcile.ObservedState, targetPath string) (string, error) {
	if !strings.HasPrefix(targetPath, "$HOME") {
		return targetPath, nil
	}
	user, err := r.effectiveExecUser(ctx, observed)
	if err != nil {
		return "", err
	}
	home, err := r.resolveExecHome(ctx, observed, user)
	if err != nil {
		return "", err
	}
	if home == "" {
		return targetPath, nil
	}
	return capdot.ResolveTargetPath(home, targetPath), nil
}
