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
	ForwardPorts         ForwardPorts      `json:"forwardPorts,omitempty"`
	InitializeCommand    LifecycleCommand  `json:"initializeCommand,omitempty"`
	OnCreateCommand      LifecycleCommand  `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand  `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand  `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand  `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand  `json:"postAttachCommand,omitempty"`
	WaitFor              string            `json:"waitFor,omitempty"`
	OverrideCommand      *bool             `json:"overrideCommand,omitempty"`
	UpdateRemoteUserUID  *bool             `json:"updateRemoteUserUID,omitempty"`
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

type ForwardPorts []string

func (p ForwardPorts) MarshalJSON() ([]byte, error) {
	values := make([]any, 0, len(p))
	for _, port := range p {
		if port == "" {
			continue
		}
		if normalized, ok := parseLocalhostPort(port); ok {
			values = append(values, normalized)
			continue
		}
		values = append(values, port)
	}
	if len(values) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(values)
}

func (p *ForwardPorts) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*p = nil
		return nil
	}
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	ports, err := NormalizeForwardPorts(raw)
	if err != nil {
		return err
	}
	*p = ports
	return nil
}

func (c LifecycleCommand) MarshalJSON() ([]byte, error) {
	switch c.Kind {
	case "string":
		return json.Marshal(c.Value)
	case "array":
		return json.Marshal(c.Args)
	case "object":
		if c.Steps == nil {
			return []byte("{}"), nil
		}
		return json.Marshal(c.Steps)
	default:
		return []byte("null"), nil
	}
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
	Features        []ResolvedFeature
	Merged          MergedConfig
	StateDir        string
	WorkspaceMount  string
	RemoteWorkspace string
	ImageName       string
	SourceKind      string
	ContainerName   string
	ComposeFiles    []string
	ComposeService  string
	ComposeProject  string
	Labels          map[string]string
}

type ResolveOptions struct {
	AllowNetwork      bool
	WriteFeatureLock  bool
	WriteFeatureState bool
}

func Resolve(workspaceArg string, configArg string) (ResolvedConfig, error) {
	return resolve(workspaceArg, configArg, ResolveOptions{
		AllowNetwork:      true,
		WriteFeatureLock:  true,
		WriteFeatureState: true,
	})
}

func ResolveReadOnly(workspaceArg string, configArg string) (ResolvedConfig, error) {
	return resolve(workspaceArg, configArg, ResolveOptions{})
}

func resolve(workspaceArg string, configArg string, opts ResolveOptions) (ResolvedConfig, error) {
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

	configDir := filepath.Dir(configPath)
	remoteWorkspace := config.WorkspaceFolder
	sourceKind := "image"
	composeFiles, err := ResolveComposeFiles(configDir, config.DockerComposeFile)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if config.Service != "" || len(composeFiles) > 0 {
		sourceKind = "compose"
		if config.Service == "" {
			return ResolvedConfig{}, errors.New("compose-based devcontainers require service")
		}
		if len(composeFiles) == 0 {
			return ResolvedConfig{}, errors.New("compose-based devcontainers require dockerComposeFile")
		}
		if remoteWorkspace == "" {
			return ResolvedConfig{}, errors.New("compose-based devcontainers require workspaceFolder")
		}
	}
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
	if sourceKind != "compose" && config.Image == "" {
		sourceKind = "dockerfile"
	}

	containerName := ContainerName(workspace, configPath)
	labels := map[string]string{
		HostFolderLabel: workspace,
		ConfigFileLabel: configPath,
		ManagedByLabel:  ManagedByValue,
	}

	features, err := ResolveFeatures(configPath, configDir, filepath.Join(stateDir, "features-cache"), config.Features, FeatureResolveOptions{
		AllowNetwork:   opts.AllowNetwork,
		WriteLockFile:  opts.WriteFeatureLock,
		WriteStateFile: opts.WriteFeatureState,
		StateDir:       stateDir,
	})
	if err != nil {
		return ResolvedConfig{}, err
	}
	metadata := make([]MetadataEntry, 0, len(features))
	for _, feature := range features {
		metadata = append(metadata, feature.Metadata)
	}

	return ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigPath:      configPath,
		ConfigDir:       configDir,
		Config:          config,
		Features:        features,
		Merged:          MergeMetadata(config, metadata),
		StateDir:        stateDir,
		WorkspaceMount:  workspaceMount,
		RemoteWorkspace: remoteWorkspace,
		ImageName:       imageName,
		SourceKind:      sourceKind,
		ContainerName:   containerName,
		ComposeFiles:    composeFiles,
		ComposeService:  config.Service,
		ComposeProject:  ComposeProjectName(workspace, configPath),
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

func NormalizeForwardPorts(raw []any) (ForwardPorts, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, value := range raw {
		port, err := normalizeForwardPort(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		result = append(result, port)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return ForwardPorts(result), nil
}

func normalizeForwardPort(value any) (string, error) {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "", fmt.Errorf("forward port cannot be empty")
		}
		if strings.HasPrefix(v, "localhost:") {
			if port, ok := parseLocalhostPort(v); ok {
				return fmt.Sprintf("localhost:%d", port), nil
			}
		}
		return v, nil
	case float64:
		if v != float64(int(v)) {
			return "", fmt.Errorf("forward port %v must be an integer", v)
		}
		return fmt.Sprintf("localhost:%d", int(v)), nil
	default:
		return "", fmt.Errorf("unsupported forward port type %T", value)
	}
}

func MergeForwardPorts(entries ...ForwardPorts) ForwardPorts {
	seen := map[string]struct{}{}
	var result []string
	for _, entry := range entries {
		for _, port := range entry {
			if port == "" {
				continue
			}
			if _, ok := seen[port]; ok {
				continue
			}
			seen[port] = struct{}{}
			result = append(result, port)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return ForwardPorts(result)
}

func parseLocalhostPort(value string) (int, bool) {
	if !strings.HasPrefix(value, "localhost:") {
		return 0, false
	}
	var port int
	_, err := fmt.Sscanf(value, "localhost:%d", &port)
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}
