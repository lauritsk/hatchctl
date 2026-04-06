package devcontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

const (
	HostFolderLabel    = "devcontainer.local_folder"
	ConfigFileLabel    = "devcontainer.config_file"
	ManagedByLabel     = "devcontainer.managed_by"
	ManagedByValue     = "hatchctl"
	BridgeEnabledLabel = "devcontainer.bridge.enabled"
)

type Config struct {
	Name                 string            `json:"name,omitempty"`
	Image                string            `json:"image,omitempty"`
	DockerFile           string            `json:"dockerFile,omitempty"`
	WorkspaceFolder      string            `json:"workspaceFolder,omitempty"`
	WorkspaceMount       string            `json:"workspaceMount,omitempty"`
	Mounts               []string          `json:"mounts,omitempty"`
	ContainerEnv         map[string]string `json:"containerEnv,omitempty"`
	RemoteEnv            map[string]string `json:"remoteEnv,omitempty"`
	ContainerUser        string            `json:"containerUser,omitempty"`
	RemoteUser           string            `json:"remoteUser,omitempty"`
	RunArgs              []string          `json:"runArgs,omitempty"`
	ForwardPorts         []any             `json:"forwardPorts,omitempty"`
	InitializeCommand    LifecycleCommand  `json:"initializeCommand,omitempty"`
	OnCreateCommand      LifecycleCommand  `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand  `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand  `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand  `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand  `json:"postAttachCommand,omitempty"`
	WaitFor              string            `json:"waitFor,omitempty"`
	OverrideCommand      *bool             `json:"overrideCommand,omitempty"`
	Init                 *bool             `json:"init,omitempty"`
	Privileged           *bool             `json:"privileged,omitempty"`
	CapAdd               []string          `json:"capAdd,omitempty"`
	SecurityOpt          []string          `json:"securityOpt,omitempty"`
	Build                *BuildConfig      `json:"build,omitempty"`
	Customizations       map[string]any    `json:"customizations,omitempty"`
	Features             map[string]any    `json:"features,omitempty"`
	DockerComposeFile    any               `json:"dockerComposeFile,omitempty"`
	Service              string            `json:"service,omitempty"`
	Raw                  map[string]any    `json:"-"`
}

type BuildConfig struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Target     string            `json:"target,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	CacheFrom  any               `json:"cacheFrom,omitempty"`
	Options    []string          `json:"options,omitempty"`
}

type LifecycleCommand struct {
	Kind   string
	Value  string
	Args   []string
	Steps  map[string]LifecycleCommand
	Exists bool
}

func (c *LifecycleCommand) UnmarshalJSON(data []byte) error {
	c.Exists = true
	if string(data) == "null" {
		*c = LifecycleCommand{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Kind = "string"
		c.Value = s
		return nil
	}

	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		c.Kind = "array"
		c.Args = arr
		return nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	c.Kind = "object"
	c.Steps = make(map[string]LifecycleCommand, len(obj))
	for key, raw := range obj {
		var child LifecycleCommand
		if err := json.Unmarshal(raw, &child); err != nil {
			return fmt.Errorf("parse lifecycle command %q: %w", key, err)
		}
		c.Steps[key] = child
	}
	return nil
}

func (c LifecycleCommand) Empty() bool {
	return !c.Exists || c.Kind == ""
}

func (c LifecycleCommand) SortedSteps() []LifecycleStep {
	if c.Kind != "object" {
		return nil
	}
	keys := make([]string, 0, len(c.Steps))
	for key := range c.Steps {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	steps := make([]LifecycleStep, 0, len(keys))
	for _, key := range keys {
		steps = append(steps, LifecycleStep{Name: key, Command: c.Steps[key]})
	}
	return steps
}

type LifecycleStep struct {
	Name    string
	Command LifecycleCommand
}

type ResolvedConfig struct {
	WorkspaceFolder string
	ConfigPath      string
	ConfigDir       string
	Config          Config
	Merged          MergedConfig
	StateDir        string
	WorkspaceMount  string
	RemoteWorkspace string
	ImageName       string
	SourceKind      string
	ContainerName   string
	Labels          map[string]string
}

func Resolve(workspaceArg string, configArg string) (ResolvedConfig, error) {
	workspace, err := resolveWorkspace(workspaceArg)
	if err != nil {
		return ResolvedConfig{}, err
	}

	configPath, err := resolveConfigPath(workspace, configArg)
	if err != nil {
		return ResolvedConfig{}, err
	}

	config, err := Load(configPath)
	if err != nil {
		return ResolvedConfig{}, err
	}

	if config.Service != "" || config.DockerComposeFile != nil {
		return ResolvedConfig{}, errors.New("compose-based devcontainers are not implemented yet in hatchctl")
	}

	configDir := filepath.Dir(configPath)
	remoteWorkspace := config.WorkspaceFolder
	if remoteWorkspace == "" {
		remoteWorkspace = filepath.ToSlash(filepath.Join("/workspaces", filepath.Base(workspace)))
	}

	workspaceMount := config.WorkspaceMount
	if workspaceMount == "" {
		workspaceMount = fmt.Sprintf("type=bind,source=%s,target=%s", workspace, remoteWorkspace)
	}

	stateDir, err := WorkspaceStateDir(workspace, configPath)
	if err != nil {
		return ResolvedConfig{}, err
	}

	imageName := ImageName(workspace, configPath)
	sourceKind := "image"
	if config.Image == "" {
		sourceKind = "dockerfile"
	}

	containerName := ContainerName(workspace, configPath)
	labels := map[string]string{
		HostFolderLabel: workspace,
		ConfigFileLabel: configPath,
		ManagedByLabel:  ManagedByValue,
	}

	return ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigPath:      configPath,
		ConfigDir:       configDir,
		Config:          config,
		Merged:          MergeMetadata(config, nil),
		StateDir:        stateDir,
		WorkspaceMount:  workspaceMount,
		RemoteWorkspace: remoteWorkspace,
		ImageName:       imageName,
		SourceKind:      sourceKind,
		ContainerName:   containerName,
		Labels:          labels,
	}, nil
}

func Load(configPath string) (Config, error) {
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, err
	}
	standardized, err := hujson.Standardize(contents)
	if err != nil {
		return Config{}, fmt.Errorf("parse jsonc %s: %w", configPath, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(standardized, &raw); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(standardized, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Raw = raw
	return cfg, nil
}

func resolveWorkspace(workspaceArg string) (string, error) {
	if workspaceArg == "" {
		return os.Getwd()
	}
	return filepath.Abs(workspaceArg)
}

func resolveConfigPath(workspace string, configArg string) (string, error) {
	if configArg != "" {
		return filepath.Abs(configArg)
	}
	paths := []string{
		filepath.Join(workspace, ".devcontainer", "devcontainer.json"),
		filepath.Join(workspace, ".devcontainer.json"),
	}
	for _, candidate := range paths {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("dev container config not found in %s", workspace)
}

func EffectiveDockerfile(config Config) string {
	switch {
	case config.DockerFile != "":
		return config.DockerFile
	case config.Build != nil && config.Build.Dockerfile != "":
		return config.Build.Dockerfile
	default:
		return "Dockerfile"
	}
}

func EffectiveContext(config Config) string {
	if config.Build != nil && config.Build.Context != "" {
		return config.Build.Context
	}
	return "."
}

func ContainerCommand(config Config) []string {
	override := true
	if config.OverrideCommand != nil {
		override = *config.OverrideCommand
	}
	if !override {
		return nil
	}
	return []string{"/bin/sh", "-lc", "trap 'exit 0' TERM INT; while sleep 1000; do :; done"}
}

func RemoteExecUser(config Config) string {
	if config.RemoteUser != "" {
		return config.RemoteUser
	}
	if config.ContainerUser != "" {
		return config.ContainerUser
	}
	return ""
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
