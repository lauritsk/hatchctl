package runtime

import (
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
)

func buildWorkspacePlan(req workspaceplan.BuildWorkspacePlanRequest) (workspaceplan.WorkspacePlan, error) {
	return workspaceplan.BuildWorkspacePlan(req)
}

func planForUp(opts UpOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:          opts.Workspace,
		ConfigPath:         opts.ConfigPath,
		StateBaseDir:       opts.StateDir,
		CacheBaseDir:       opts.CacheDir,
		FeatureTimeout:     opts.FeatureTimeout,
		LockfilePolicy:     opts.LockfilePolicy,
		BridgeEnabled:      opts.BridgeEnabled,
		SSHAgent:           opts.SSHAgent,
		Dotfiles:           dotfilesPreference(opts.Dotfiles),
		TrustWorkspace:     opts.TrustWorkspace,
		AllowHostLifecycle: opts.AllowHostLifecycle,
	})
}

func planForBuild(opts BuildOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateBaseDir:   opts.StateDir,
		CacheBaseDir:   opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		TrustWorkspace: opts.TrustWorkspace,
	})
}

func planForExec(opts ExecOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateBaseDir:   opts.StateDir,
		CacheBaseDir:   opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		SSHAgent:       opts.SSHAgent,
	})
}

func planForReadConfig(opts ReadConfigOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateBaseDir:   opts.StateDir,
		CacheBaseDir:   opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
		SSHAgent:       opts.SSHAgent,
		Dotfiles:       dotfilesPreference(opts.Dotfiles),
	})
}

func planForLifecycle(opts RunLifecycleOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:          opts.Workspace,
		ConfigPath:         opts.ConfigPath,
		StateBaseDir:       opts.StateDir,
		CacheBaseDir:       opts.CacheDir,
		FeatureTimeout:     opts.FeatureTimeout,
		LockfilePolicy:     opts.LockfilePolicy,
		ReadOnly:           true,
		Dotfiles:           dotfilesPreference(opts.Dotfiles),
		AllowHostLifecycle: opts.AllowHostLifecycle,
	})
}

func planForBridgeDoctor(opts BridgeDoctorOptions) (workspaceplan.WorkspacePlan, error) {
	if opts.Plan.Valid() {
		return opts.Plan, nil
	}
	return buildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:      opts.Workspace,
		ConfigPath:     opts.ConfigPath,
		StateBaseDir:   opts.StateDir,
		CacheBaseDir:   opts.CacheDir,
		FeatureTimeout: opts.FeatureTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		ReadOnly:       true,
	})
}

func dotfilesPreference(opts DotfilesOptions) workspaceplan.DotfilesPreference {
	return workspaceplan.DotfilesPreference{Repository: opts.Repository, InstallCommand: opts.InstallCommand, TargetPath: opts.TargetPath}
}

func dotfilesOptionsFromPlan(workspacePlan workspaceplan.WorkspacePlan) DotfilesOptions {
	return DotfilesOptions{
		Repository:     workspacePlan.Preferences.Dotfiles.Repository,
		InstallCommand: workspacePlan.Preferences.Dotfiles.InstallCommand,
		TargetPath:     workspacePlan.Preferences.Dotfiles.TargetPath,
	}
}
