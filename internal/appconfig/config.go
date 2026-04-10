package appconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/lauritsk/hatchctl/internal/security"
)

type Config struct {
	Workspace      string             `toml:"workspace"`
	ConfigPath     string             `toml:"config"`
	StateDir       string             `toml:"state_dir"`
	CacheDir       string             `toml:"cache_dir"`
	FeatureTimeout string             `toml:"feature_timeout"`
	LockfilePolicy string             `toml:"lockfile_policy"`
	Bridge         *bool              `toml:"bridge"`
	SSHAgent       *bool              `toml:"ssh"`
	Dotfiles       DotfilesConfig     `toml:"dotfiles"`
	Verification   VerificationConfig `toml:"verification"`
	loadedFrom     string
}

type DotfilesConfig struct {
	Repository     string `toml:"repository"`
	InstallCommand string `toml:"install_command"`
	TargetPath     string `toml:"target_path"`
}

type VerificationConfig struct {
	TrustedSigners []security.TrustedSigner `toml:"trusted_signers"`
}

type LoadedConfig struct {
	User      Config
	Workspace Config
	Merged    Config
}

func LoadForWorkspace(workspaceHint string) (LoadedConfig, error) {
	var loaded LoadedConfig
	if path, ok, err := userConfigPath(); err != nil {
		return LoadedConfig{}, err
	} else if ok {
		cfg, err := Load(path)
		if err != nil {
			return LoadedConfig{}, err
		}
		loaded.User = cfg
		loaded.Merged = merge(loaded.Merged, cfg)
	}

	workspace := workspaceHint
	if workspace == "" {
		workspace = loaded.Merged.Workspace
	}
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return LoadedConfig{}, err
		}
		workspace = cwd
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return LoadedConfig{}, err
	}

	if path, ok, err := workspaceConfigPath(workspace); err != nil {
		return LoadedConfig{}, err
	} else if ok {
		cfg, err := Load(path)
		if err != nil {
			return LoadedConfig{}, err
		}
		loaded.Workspace = cfg
		loaded.Merged = merge(loaded.Merged, cfg)
	}
	return loaded, nil
}

func Load(path string) (Config, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	cfg.loadedFrom = path
	resolveRelativePaths(&cfg)
	return cfg, nil
}

func (c Config) FeatureTimeoutDuration() (time.Duration, error) {
	if c.FeatureTimeout == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(c.FeatureTimeout)
	if err != nil {
		source := "config"
		if c.loadedFrom != "" {
			source = c.loadedFrom
		}
		return 0, fmt.Errorf("parse feature_timeout in %s: %w", source, err)
	}
	return duration, nil
}

func merge(base Config, override Config) Config {
	merged := base
	if override.Workspace != "" {
		merged.Workspace = override.Workspace
	}
	if override.ConfigPath != "" {
		merged.ConfigPath = override.ConfigPath
	}
	if override.StateDir != "" {
		merged.StateDir = override.StateDir
	}
	if override.CacheDir != "" {
		merged.CacheDir = override.CacheDir
	}
	if override.FeatureTimeout != "" {
		merged.FeatureTimeout = override.FeatureTimeout
	}
	if override.LockfilePolicy != "" {
		merged.LockfilePolicy = override.LockfilePolicy
	}
	if override.Bridge != nil {
		value := *override.Bridge
		merged.Bridge = &value
	}
	if override.SSHAgent != nil {
		value := *override.SSHAgent
		merged.SSHAgent = &value
	}
	if override.Dotfiles.Repository != "" {
		merged.Dotfiles.Repository = override.Dotfiles.Repository
	}
	if override.Dotfiles.InstallCommand != "" {
		merged.Dotfiles.InstallCommand = override.Dotfiles.InstallCommand
	}
	if override.Dotfiles.TargetPath != "" {
		merged.Dotfiles.TargetPath = override.Dotfiles.TargetPath
	}
	if len(override.Verification.TrustedSigners) > 0 {
		merged.Verification.TrustedSigners = append([]security.TrustedSigner(nil), override.Verification.TrustedSigners...)
	}
	return merged
}

func resolveRelativePaths(cfg *Config) {
	if cfg.loadedFrom == "" {
		return
	}
	base := filepath.Dir(cfg.loadedFrom)
	cfg.Workspace = resolveRelativePath(base, cfg.Workspace)
	cfg.ConfigPath = resolveRelativePath(base, cfg.ConfigPath)
	cfg.StateDir = resolveRelativePath(base, cfg.StateDir)
	cfg.CacheDir = resolveRelativePath(base, cfg.CacheDir)
}

func resolveRelativePath(base string, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(base, value))
}

func userConfigPath() (string, bool, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(root, "hatchctl", "config.toml")
	if _, err := os.Stat(path); err == nil {
		return path, true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	} else {
		return "", false, err
	}
}

func workspaceConfigPath(workspace string) (string, bool, error) {
	path := filepath.Join(workspace, ".hatchctl", "config.toml")
	if _, err := os.Stat(path); err == nil {
		return path, true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	} else {
		return "", false, err
	}
}
