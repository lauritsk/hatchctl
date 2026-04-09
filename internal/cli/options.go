package cli

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lauritsk/hatchctl/internal/appconfig"
	"github.com/lauritsk/hatchctl/internal/runtime"
)

type commandDefaults struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy string
	BridgeEnabled  bool
	TrustWorkspace bool
	SSHAgent       bool
	Dotfiles       dotfilesOptions
}

type globalOptions struct {
	Verbose bool
	Debug   bool
}

type dotfilesOptions struct {
	Repository     string
	InstallCommand string
	TargetPath     string
}

func defaultDotfilesOptions() dotfilesOptions {
	return dotfilesOptions{
		Repository:     os.Getenv("HATCHCTL_DOTFILES_REPOSITORY"),
		InstallCommand: os.Getenv("HATCHCTL_DOTFILES_INSTALL_COMMAND"),
		TargetPath:     os.Getenv("HATCHCTL_DOTFILES_TARGET_PATH"),
	}
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

func addDotfilesFlags(cmd *cobra.Command, opts *dotfilesOptions) {
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles", opts.Repository, "dotfiles repository (user, owner/repo, host/user, host/user/repo, or git URL); env HATCHCTL_DOTFILES_REPOSITORY")
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles-repository", opts.Repository, "same as --dotfiles")
	cmd.PersistentFlags().StringVar(&opts.InstallCommand, "dotfiles-install-command", opts.InstallCommand, "dotfiles install script or command; env HATCHCTL_DOTFILES_INSTALL_COMMAND")
	cmd.PersistentFlags().StringVar(&opts.TargetPath, "dotfiles-target-path", opts.TargetPath, "dotfiles checkout path inside the container; env HATCHCTL_DOTFILES_TARGET_PATH")
}

func (o dotfilesOptions) runtime() runtime.DotfilesOptions {
	return runtime.DotfilesOptions{Repository: o.Repository, InstallCommand: o.InstallCommand, TargetPath: o.TargetPath}
}

func (a *App) resolveCommandDefaults(cmd *cobra.Command, workspace string, configPath string, featureTimeout time.Duration, lockfilePolicy string, bridgeEnabled *bool, trustWorkspace *bool, sshAgent *bool, dotfiles dotfilesOptions) (commandDefaults, error) {
	workspaceHint := ""
	if flagChanged(cmd, "workspace") {
		workspaceHint = workspace
	}
	config, err := appconfig.LoadForWorkspace(workspaceHint)
	if err != nil {
		return commandDefaults{}, err
	}
	resolvedTimeout := featureTimeout
	if !flagChanged(cmd, "feature-timeout") {
		if timeout, err := config.FeatureTimeoutDuration(); err != nil {
			return commandDefaults{}, err
		} else if timeout > 0 {
			resolvedTimeout = timeout
		}
	}
	resolved := commandDefaults{
		Workspace:      firstConfigured(flagChanged(cmd, "workspace"), workspace, config.Workspace),
		ConfigPath:     firstConfigured(flagChanged(cmd, "config"), configPath, config.ConfigPath),
		StateDir:       config.StateDir,
		CacheDir:       config.CacheDir,
		FeatureTimeout: resolvedTimeout,
		LockfilePolicy: lockfilePolicy,
		TrustWorkspace: trustWorkspace != nil && *trustWorkspace,
		SSHAgent:       sshAgent != nil && *sshAgent,
		Dotfiles:       dotfiles,
	}
	if !flagChanged(cmd, "lockfile-policy") && config.LockfilePolicy != "" {
		resolved.LockfilePolicy = config.LockfilePolicy
	}
	if !dotfilesRepositoryChanged(cmd) && config.Dotfiles.Repository != "" {
		resolved.Dotfiles.Repository = config.Dotfiles.Repository
	}
	if !flagChanged(cmd, "dotfiles-install-command") && config.Dotfiles.InstallCommand != "" {
		resolved.Dotfiles.InstallCommand = config.Dotfiles.InstallCommand
	}
	if !flagChanged(cmd, "dotfiles-target-path") && config.Dotfiles.TargetPath != "" {
		resolved.Dotfiles.TargetPath = config.Dotfiles.TargetPath
	}
	if bridgeEnabled != nil {
		resolved.BridgeEnabled = *bridgeEnabled
		if !flagChanged(cmd, "bridge") && config.Bridge != nil {
			resolved.BridgeEnabled = *config.Bridge
		}
	}
	if sshAgent != nil {
		resolved.SSHAgent = *sshAgent
		if !flagChanged(cmd, "ssh") && config.SSHAgent != nil {
			resolved.SSHAgent = *config.SSHAgent
		}
	}
	if trustWorkspace != nil {
		resolved.TrustWorkspace = *trustWorkspace
	}
	return resolved, nil
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func dotfilesRepositoryChanged(cmd *cobra.Command) bool {
	return flagChanged(cmd, "dotfiles") || flagChanged(cmd, "dotfiles-repository")
}

func firstConfigured(cliSet bool, cliValue string, configValue string) string {
	if cliSet {
		return cliValue
	}
	if configValue != "" {
		return configValue
	}
	return cliValue
}

func envTruthy(name string) bool {
	value, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
