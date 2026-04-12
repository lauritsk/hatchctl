package app

import (
	"os"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/appconfig"
	"github.com/lauritsk/hatchctl/internal/capability"
	"github.com/lauritsk/hatchctl/internal/security"
)

const (
	TrustWorkspaceEnvVar     = "HATCHCTL_TRUST_WORKSPACE"
	AllowHostLifecycleEnvVar = "HATCHCTL_ALLOW_HOST_LIFECYCLE"
)

type FlagValue[T any] struct {
	Value   T
	Changed bool
}

type DotfilesOptions = capability.Dotfiles

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
	TrustedSigners []security.TrustedSigner
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
	loaded, err := appconfig.LoadForWorkspace(workspaceHint)
	if err != nil {
		return CommandDefaults{}, err
	}
	config := loaded.Merged
	trustedWorkspace := req.TrustWorkspace != nil && req.TrustWorkspace.Value

	resolvedTimeout, err := resolveFeatureTimeout(config, req.FeatureTimeout)
	if err != nil {
		return CommandDefaults{}, err
	}

	resolved := CommandDefaults{
		Workspace:      resolveStringDefault(req.Workspace, loaded.User.Workspace, loaded.Workspace.Workspace, trustedWorkspace),
		ConfigPath:     firstConfigured(req.ConfigPath.Changed, req.ConfigPath.Value, config.ConfigPath),
		StateDir:       resolveStringDefault(FlagValue[string]{}, loaded.User.StateDir, loaded.Workspace.StateDir, trustedWorkspace),
		CacheDir:       resolveStringDefault(FlagValue[string]{}, loaded.User.CacheDir, loaded.Workspace.CacheDir, trustedWorkspace),
		FeatureTimeout: resolvedTimeout,
		LockfilePolicy: resolveLockfilePolicy(config, req.LockfilePolicy),
		Dotfiles:       resolveDotfilesDefaults(loaded, req.Dotfiles, trustedWorkspace),
		TrustedSigners: resolveTrustedSigners(loaded, trustedWorkspace),
	}

	resolveOptionalBoolDefault(&resolved.BridgeEnabled, req.BridgeEnabled, loaded.User.Bridge, loaded.Workspace.Bridge, trustedWorkspace)
	resolveOptionalBoolDefault(&resolved.SSHAgent, req.SSHAgent, loaded.User.SSHAgent, loaded.Workspace.SSHAgent, trustedWorkspace)
	if req.TrustWorkspace != nil {
		resolved.TrustWorkspace = req.TrustWorkspace.Value
	}

	return resolved, nil
}

func resolveFeatureTimeout(config appconfig.Config, flag FlagValue[time.Duration]) (time.Duration, error) {
	if flag.Changed {
		return flag.Value, nil
	}
	timeout, err := config.FeatureTimeoutDuration()
	if err != nil {
		return 0, err
	}
	if timeout > 0 {
		return timeout, nil
	}
	return flag.Value, nil
}

func resolveLockfilePolicy(config appconfig.Config, flag FlagValue[string]) string {
	if !flag.Changed && config.LockfilePolicy != "" {
		return config.LockfilePolicy
	}
	return flag.Value
}

func resolveDotfilesDefaults(loaded appconfig.LoadedConfig, flags DotfilesOptionValues, trustedWorkspace bool) DotfilesOptions {
	return DotfilesOptions{
		Repository: resolveStringDefault(flags.Repository, loaded.User.Dotfiles.Repository, loaded.Workspace.Dotfiles.Repository, trustedWorkspace),
		InstallCommand: resolveStringDefault(
			flags.InstallCommand,
			loaded.User.Dotfiles.InstallCommand,
			loaded.Workspace.Dotfiles.InstallCommand,
			trustedWorkspace,
		),
		TargetPath: resolveStringDefault(flags.TargetPath, loaded.User.Dotfiles.TargetPath, loaded.Workspace.Dotfiles.TargetPath, trustedWorkspace),
	}
}

func resolveStringDefault(flag FlagValue[string], userValue string, workspaceValue string, trustedWorkspace bool) string {
	if flag.Changed {
		return flag.Value
	}
	return preferredWorkspaceValue(userValue, workspaceValue, trustedWorkspace)
}

func resolveTrustedSigners(loaded appconfig.LoadedConfig, trustedWorkspace bool) []security.TrustedSigner {
	signers := append([]security.TrustedSigner(nil), loaded.User.Verification.TrustedSigners...)
	if trustedWorkspace && len(loaded.Workspace.Verification.TrustedSigners) > 0 {
		signers = append([]security.TrustedSigner(nil), loaded.Workspace.Verification.TrustedSigners...)
	}
	return signers
}

func resolveOptionalBoolDefault(target *bool, flag *FlagValue[bool], userValue *bool, workspaceValue *bool, trustedWorkspace bool) {
	if flag == nil {
		return
	}
	*target = flag.Value
	if flag.Changed {
		return
	}
	if trustedWorkspace && workspaceValue != nil {
		*target = *workspaceValue
		return
	}
	if userValue != nil {
		*target = *userValue
	}
}

func preferredWorkspaceValue(userValue string, workspaceValue string, trustedWorkspace bool) string {
	if trustedWorkspace && workspaceValue != "" {
		return workspaceValue
	}
	if userValue != "" {
		return userValue
	}
	if trustedWorkspace {
		return workspaceValue
	}
	return ""
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
