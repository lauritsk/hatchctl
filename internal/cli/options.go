package cli

import (
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
)

type globalOptions struct {
	Backend string
	Verbose bool
	Debug   bool
}

func (o *globalOptions) app() appcore.GlobalOptions {
	if o == nil {
		return appcore.GlobalOptions{}
	}
	return appcore.GlobalOptions{Verbose: o.Verbose, Debug: o.Debug}
}

func addWorkspaceFlags(cmd *cobra.Command, workspace *string, configPath *string) {
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(configPath, "config", "", "path to devcontainer.json")
}

func addResolutionFlags(cmd *cobra.Command, featureTimeout *time.Duration, lockfilePolicy *string, defaultLockfilePolicy string) {
	cmd.Flags().DurationVar(featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(lockfilePolicy, "lockfile-policy", defaultLockfilePolicy, "feature lockfile policy: auto, frozen, or update")
}

func addJSONFlag(cmd *cobra.Command, jsonOut *bool) {
	cmd.Flags().BoolVar(jsonOut, "json", false, "emit machine-readable JSON")
}

func addDotfilesFlags(cmd *cobra.Command, opts *appcore.DotfilesOptions) {
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles", opts.Repository, "dotfiles repository (user, owner/repo, host/user, host/user/repo, or git URL); env HATCHCTL_DOTFILES_REPOSITORY")
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles-repository", opts.Repository, "same as --dotfiles")
	cmd.PersistentFlags().StringVar(&opts.InstallCommand, "dotfiles-install-command", opts.InstallCommand, "dotfiles install script or command; env HATCHCTL_DOTFILES_INSTALL_COMMAND")
	cmd.PersistentFlags().StringVar(&opts.TargetPath, "dotfiles-target-path", opts.TargetPath, "dotfiles checkout path inside the container; env HATCHCTL_DOTFILES_TARGET_PATH")
}

func resolveDefaultsRequest(cmd *cobra.Command, workspace string, configPath string, featureTimeout time.Duration, lockfilePolicy string, bridgeEnabled *bool, trustWorkspace *bool, sshAgent *bool, dotfiles appcore.DotfilesOptions) appcore.ResolveDefaultsRequest {
	req := appcore.ResolveDefaultsRequest{
		Backend:        appcore.FlagValue[string]{Value: cmd.Flag("backend").Value.String(), Changed: flagChanged(cmd, "backend")},
		Workspace:      appcore.FlagValue[string]{Value: workspace, Changed: flagChanged(cmd, "workspace")},
		ConfigPath:     appcore.FlagValue[string]{Value: configPath, Changed: flagChanged(cmd, "config")},
		FeatureTimeout: appcore.FlagValue[time.Duration]{Value: featureTimeout, Changed: flagChanged(cmd, "feature-timeout")},
		LockfilePolicy: appcore.FlagValue[string]{Value: lockfilePolicy, Changed: flagChanged(cmd, "lockfile-policy")},
		Dotfiles: appcore.DotfilesOptionValues{
			Repository:     appcore.FlagValue[string]{Value: dotfiles.Repository, Changed: dotfilesRepositoryChanged(cmd)},
			InstallCommand: appcore.FlagValue[string]{Value: dotfiles.InstallCommand, Changed: flagChanged(cmd, "dotfiles-install-command")},
			TargetPath:     appcore.FlagValue[string]{Value: dotfiles.TargetPath, Changed: flagChanged(cmd, "dotfiles-target-path")},
		},
	}
	if bridgeEnabled != nil {
		value := appcore.FlagValue[bool]{Value: *bridgeEnabled, Changed: flagChanged(cmd, "bridge")}
		req.BridgeEnabled = &value
	}
	if trustWorkspace != nil {
		value := appcore.FlagValue[bool]{Value: *trustWorkspace, Changed: flagChanged(cmd, "trust-workspace")}
		req.TrustWorkspace = &value
	}
	if sshAgent != nil {
		value := appcore.FlagValue[bool]{Value: *sshAgent, Changed: flagChanged(cmd, "ssh")}
		req.SSHAgent = &value
	}
	return req
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func dotfilesRepositoryChanged(cmd *cobra.Command) bool {
	return flagChanged(cmd, "dotfiles") || flagChanged(cmd, "dotfiles-repository")
}
