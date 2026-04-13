package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceSpec struct {
	WorkspaceFolder string
	ConfigPath      string
	ConfigDir       string
	Config          Config
	Merged          MergedConfig
	WorkspaceMount  string
	RemoteWorkspace string
	SourceKind      string
	ComposeFiles    []string
	ComposeService  string
}

func ResolveWorkspaceSpec(workspaceArg string, configArg string) (WorkspaceSpec, error) {
	workspace, err := ResolveWorkspacePath(workspaceArg)
	if err != nil {
		return WorkspaceSpec{}, err
	}
	configPath, err := ResolveConfigPath(workspace, configArg)
	if err != nil {
		return WorkspaceSpec{}, err
	}
	config, err := Load(configPath)
	if err != nil {
		return WorkspaceSpec{}, err
	}
	configDir := filepath.Dir(configPath)
	composeFiles, err := ResolveComposeFiles(configDir, config.DockerComposeFile)
	if err != nil {
		return WorkspaceSpec{}, err
	}
	remoteWorkspace := config.WorkspaceFolder
	sourceKind := "image"
	if config.Service != "" || len(composeFiles) > 0 {
		sourceKind = "compose"
		if config.Service == "" {
			return WorkspaceSpec{}, errors.New("compose-based devcontainers must set \"service\"")
		}
		if len(composeFiles) == 0 {
			return WorkspaceSpec{}, errors.New("compose-based devcontainers must set \"dockerComposeFile\"")
		}
		if remoteWorkspace == "" {
			return WorkspaceSpec{}, errors.New("compose-based devcontainers must set \"workspaceFolder\"")
		}
	}
	if remoteWorkspace == "" {
		remoteWorkspace = filepath.ToSlash(filepath.Join("/workspaces", filepath.Base(workspace)))
	}
	workspaceMount := config.WorkspaceMount
	if workspaceMount == "" {
		workspaceMount = fmt.Sprintf("type=bind,source=%s,target=%s", workspace, remoteWorkspace)
	}
	if sourceKind != "compose" && config.Image == "" {
		sourceKind = "dockerfile"
	}
	return WorkspaceSpec{
		WorkspaceFolder: workspace,
		ConfigPath:      configPath,
		ConfigDir:       configDir,
		Config:          config,
		Merged:          MergeMetadata(config, nil),
		WorkspaceMount:  workspaceMount,
		RemoteWorkspace: remoteWorkspace,
		SourceKind:      sourceKind,
		ComposeFiles:    composeFiles,
		ComposeService:  config.Service,
	}, nil
}

func Load(configPath string) (Config, error) {
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, err
	}
	standardized, err := StandardizeJSONC(configPath, contents)
	if err != nil {
		return Config{}, err
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

func ResolveWorkspacePath(workspaceArg string) (string, error) {
	if workspaceArg == "" {
		return os.Getwd()
	}
	return filepath.Abs(workspaceArg)
}

func ResolveConfigPath(workspace string, configArg string) (string, error) {
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
	return "", fmt.Errorf("no devcontainer config found in %s\nLooked for:\n- %s\n- %s\nAdd a devcontainer config in one of those locations or rerun with --config <path>", workspace, paths[0], paths[1])
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

func ResolvedDockerfile(configDir string, config Config) string {
	if config.DockerFile != "" || (config.Build != nil && config.Build.Dockerfile != "") {
		return EffectiveDockerfile(config)
	}
	if configDir != "" {
		dockerfilePath := filepath.Join(configDir, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); err == nil {
			return "Dockerfile"
		}
		containerfilePath := filepath.Join(configDir, "Containerfile")
		if _, err := os.Stat(containerfilePath); err == nil {
			return "Containerfile"
		}
	}
	return "Dockerfile"
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
	return []string{"/bin/sh", "-lc", KeepAliveCommand()}
}

func KeepAliveCommand() string {
	return "exec sleep infinity"
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
			return "", fmt.Errorf("forwardPorts entries cannot be empty")
		}
		if strings.HasPrefix(v, "localhost:") {
			if port, ok := parseLocalhostPort(v); ok {
				return fmt.Sprintf("localhost:%d", port), nil
			}
		}
		return v, nil
	case float64:
		if v != float64(int(v)) {
			return "", fmt.Errorf("invalid forward port %v: expected an integer", v)
		}
		return fmt.Sprintf("localhost:%d", int(v)), nil
	default:
		return "", fmt.Errorf("invalid forward port value of type %T", value)
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
