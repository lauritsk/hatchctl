package app

import (
	"os"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/appconfig"
)

const (
	TrustWorkspaceEnvVar     = "HATCHCTL_TRUST_WORKSPACE"
	AllowHostLifecycleEnvVar = "HATCHCTL_ALLOW_HOST_LIFECYCLE"
)

type FlagValue[T any] struct {
	Value   T
	Changed bool
}

type DotfilesOptions struct {
	Repository     string
	InstallCommand string
	TargetPath     string
}

type DotfilesOptionValues struct {
	Repository     FlagValue[string]
	InstallCommand FlagValue[string]
	TargetPath     FlagValue[string]
}

type CommandDefaults struct {
	Workspace      string
	ConfigPath     string
	StateDir       string
	CacheDir       string
	FeatureTimeout time.Duration
	LockfilePolicy string
	BridgeEnabled  bool
	TrustWorkspace bool
	SSHAgent       bool
	Dotfiles       DotfilesOptions
}

type ResolveDefaultsRequest struct {
	Workspace      FlagValue[string]
	ConfigPath     FlagValue[string]
	FeatureTimeout FlagValue[time.Duration]
	LockfilePolicy FlagValue[string]
	BridgeEnabled  *FlagValue[bool]
	TrustWorkspace *FlagValue[bool]
	SSHAgent       *FlagValue[bool]
	Dotfiles       DotfilesOptionValues
}

func DefaultDotfilesOptions() DotfilesOptions {
	return DotfilesOptions{
		Repository:     os.Getenv("HATCHCTL_DOTFILES_REPOSITORY"),
		InstallCommand: os.Getenv("HATCHCTL_DOTFILES_INSTALL_COMMAND"),
		TargetPath:     os.Getenv("HATCHCTL_DOTFILES_TARGET_PATH"),
	}
}

func EnvTruthy(name string) bool {
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

func ResolveDefaults(req ResolveDefaultsRequest) (CommandDefaults, error) {
	workspaceHint := ""
	if req.Workspace.Changed {
		workspaceHint = req.Workspace.Value
	}
	config, err := appconfig.LoadForWorkspace(workspaceHint)
	if err != nil {
		return CommandDefaults{}, err
	}

	resolvedTimeout := req.FeatureTimeout.Value
	if !req.FeatureTimeout.Changed {
		timeout, err := config.FeatureTimeoutDuration()
		if err != nil {
			return CommandDefaults{}, err
		}
		if timeout > 0 {
			resolvedTimeout = timeout
		}
	}

	resolved := CommandDefaults{
		Workspace:      firstConfigured(req.Workspace.Changed, req.Workspace.Value, config.Workspace),
		ConfigPath:     firstConfigured(req.ConfigPath.Changed, req.ConfigPath.Value, config.ConfigPath),
		StateDir:       config.StateDir,
		CacheDir:       config.CacheDir,
		FeatureTimeout: resolvedTimeout,
		LockfilePolicy: req.LockfilePolicy.Value,
		Dotfiles: DotfilesOptions{
			Repository:     req.Dotfiles.Repository.Value,
			InstallCommand: req.Dotfiles.InstallCommand.Value,
			TargetPath:     req.Dotfiles.TargetPath.Value,
		},
	}

	if !req.LockfilePolicy.Changed && config.LockfilePolicy != "" {
		resolved.LockfilePolicy = config.LockfilePolicy
	}
	if !req.Dotfiles.Repository.Changed && config.Dotfiles.Repository != "" {
		resolved.Dotfiles.Repository = config.Dotfiles.Repository
	}
	if !req.Dotfiles.InstallCommand.Changed && config.Dotfiles.InstallCommand != "" {
		resolved.Dotfiles.InstallCommand = config.Dotfiles.InstallCommand
	}
	if !req.Dotfiles.TargetPath.Changed && config.Dotfiles.TargetPath != "" {
		resolved.Dotfiles.TargetPath = config.Dotfiles.TargetPath
	}
	if req.BridgeEnabled != nil {
		resolved.BridgeEnabled = req.BridgeEnabled.Value
		if !req.BridgeEnabled.Changed && config.Bridge != nil {
			resolved.BridgeEnabled = *config.Bridge
		}
	}
	if req.SSHAgent != nil {
		resolved.SSHAgent = req.SSHAgent.Value
		if !req.SSHAgent.Changed && config.SSHAgent != nil {
			resolved.SSHAgent = *config.SSHAgent
		}
	}
	if req.TrustWorkspace != nil {
		resolved.TrustWorkspace = req.TrustWorkspace.Value
	}

	return resolved, nil
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
