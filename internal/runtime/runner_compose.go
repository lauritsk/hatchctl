package runtime

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/fileutil"
	"go.yaml.in/yaml/v3"
)

type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Image string        `json:"image"`
	Build *composeBuild `json:"build"`
}

type composeBuild struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile"`
	Target     string            `json:"target"`
	Args       map[string]string `json:"args"`
}

type composeOverrideFile struct {
	Services map[string]composeOverrideService `yaml:"services"`
	Volumes  map[string]composeOverrideVolume  `yaml:"volumes,omitempty"`
}

type composeOverrideService struct {
	PullPolicy  string                `yaml:"pull_policy,omitempty"`
	Labels      []string              `yaml:"labels,omitempty"`
	Environment []string              `yaml:"environment,omitempty"`
	Volumes     []composeServiceMount `yaml:"volumes,omitempty"`
	Init        bool                  `yaml:"init,omitempty"`
	Privileged  bool                  `yaml:"privileged,omitempty"`
	User        string                `yaml:"user,omitempty"`
	Command     []string              `yaml:"command,omitempty"`
	CapAdd      []string              `yaml:"cap_add,omitempty"`
	SecurityOpt []string              `yaml:"security_opt,omitempty"`
	Image       string                `yaml:"image,omitempty"`
}

type composeServiceMount struct {
	Type        string                     `yaml:"type,omitempty"`
	Source      string                     `yaml:"source,omitempty"`
	Target      string                     `yaml:"target,omitempty"`
	ReadOnly    bool                       `yaml:"read_only,omitempty"`
	Consistency string                     `yaml:"consistency,omitempty"`
	Bind        *composeBindMountOptions   `yaml:"bind,omitempty"`
	Volume      *composeVolumeMountOptions `yaml:"volume,omitempty"`
}

type composeBindMountOptions struct {
	Propagation    string `yaml:"propagation,omitempty"`
	CreateHostPath *bool  `yaml:"create_host_path,omitempty"`
	SELinux        string `yaml:"selinux,omitempty"`
}

type composeVolumeMountOptions struct {
	NoCopy  bool   `yaml:"nocopy,omitempty"`
	Subpath string `yaml:"subpath,omitempty"`
}

type composeOverrideVolume struct{}

func (b *composeBuild) Enabled() bool {
	return b != nil && b.Context != ""
}

func composeBaseArgs(resolved devcontainer.ResolvedConfig) []string {
	args := []string{"compose"}
	for _, file := range resolved.ComposeFiles {
		args = append(args, "-f", file)
	}
	if resolved.ComposeProject != "" {
		args = append(args, "-p", resolved.ComposeProject)
	}
	return args
}

func composeArgs(resolved devcontainer.ResolvedConfig, overridePath string) []string {
	args := composeBaseArgs(resolved)
	if overridePath != "" {
		args = append(args, "-f", overridePath)
	}
	return args
}

func (r *Runner) readComposeConfig(ctx context.Context, resolved devcontainer.ResolvedConfig) (composeConfig, error) {
	args := append(composeBaseArgs(resolved), "config", "--format", "json")
	output, err := r.backend.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Dir: resolved.ConfigDir})
	if err != nil {
		return composeConfig{}, err
	}
	var config composeConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return composeConfig{}, err
	}
	if config.Name != "" {
		resolved.ComposeProject = config.Name
	}
	return config, nil
}

func (r *Runner) findComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	project := resolved.ComposeProject
	if project == "" {
		config, err := r.readComposeConfig(ctx, resolved)
		if err != nil {
			return "", err
		}
		project = firstNonEmpty(config.Name, resolved.ComposeProject)
	}
	args := []string{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + project, "--filter", "label=com.docker.compose.service=" + resolved.ComposeService}
	result, err := r.backend.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args})
	if err != nil {
		return "", err
	}
	return r.selectBestContainerID(ctx, result)
}

func writeComposeOverride(resolved devcontainer.ResolvedConfig, image string) (string, error) {
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return "", err
	}
	path := devcontainer.ComposeOverrideFile(resolved.StateDir)
	contents, err := renderComposeOverride(resolved, image)
	if err != nil {
		return "", err
	}
	if err := fileutil.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func renderComposeOverride(resolved devcontainer.ResolvedConfig, image string) (string, error) {
	service := composeOverrideService{}
	if len(resolved.Features) > 0 {
		service.PullPolicy = "never"
	}
	labels := map[string]string{}
	for key, value := range resolved.Labels {
		labels[key] = value
	}
	if resolved.Merged.ContainerEnv["DEVCONTAINER_BRIDGE_ENABLED"] == "true" {
		labels[devcontainer.BridgeEnabledLabel] = "true"
	}
	if metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata); err == nil && metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	for _, key := range devcontainer.SortedMapKeys(labels) {
		service.Labels = append(service.Labels, key+"="+labels[key])
	}
	for _, key := range devcontainer.SortedMapKeys(resolved.Merged.ContainerEnv) {
		service.Environment = append(service.Environment, key+"="+resolved.Merged.ContainerEnv[key])
	}
	allMounts := append([]string{resolved.WorkspaceMount}, resolved.Merged.Mounts...)
	namedVolumes := map[string]struct{}{}
	for _, mount := range allMounts {
		if value, ok := composeMountValue(mount); ok {
			service.Volumes = append(service.Volumes, value)
		}
		if source, ok := composeNamedVolume(mount); ok {
			namedVolumes[source] = struct{}{}
		}
	}
	if resolved.Merged.Init {
		service.Init = true
	}
	if resolved.Merged.Privileged {
		service.Privileged = true
	}
	if user := resolved.Merged.ContainerUser; user != "" {
		service.User = user
	}
	if overrideCommandEnabled(resolved.Config.OverrideCommand) {
		service.Command = []string{"/bin/sh", "-lc", devcontainer.KeepAliveCommand()}
	}
	if len(resolved.Merged.CapAdd) > 0 {
		service.CapAdd = slices.Clone(resolved.Merged.CapAdd)
	}
	if len(resolved.Merged.SecurityOpt) > 0 {
		service.SecurityOpt = slices.Clone(resolved.Merged.SecurityOpt)
	}
	if image != "" {
		service.Image = image
	}
	override := composeOverrideFile{
		Services: map[string]composeOverrideService{resolved.ComposeService: service},
	}
	if len(namedVolumes) > 0 {
		override.Volumes = make(map[string]composeOverrideVolume, len(namedVolumes))
		for _, name := range sortedVolumeNames(namedVolumes) {
			override.Volumes[name] = composeOverrideVolume{}
		}
	}
	data, err := yaml.Marshal(override)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func overrideCommandEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func composeMountValue(raw string) (composeServiceMount, bool) {
	spec, ok := devcontainer.ParseMountSpec(raw)
	if !ok {
		return composeServiceMount{}, false
	}
	switch spec.Type {
	case "bind", "volume":
		if spec.Source == "" {
			return composeServiceMount{}, false
		}
		mount := composeServiceMount{
			Type:   spec.Type,
			Source: spec.Source,
			Target: spec.Target,
		}
		if spec.ReadOnly {
			mount.ReadOnly = true
		}
		if spec.Consistency != "" {
			mount.Consistency = spec.Consistency
		}
		if mount.Type == "bind" {
			bind := &composeBindMountOptions{}
			if spec.BindPropagation != "" {
				bind.Propagation = spec.BindPropagation
			}
			if spec.CreateHostPath != nil {
				value := *spec.CreateHostPath
				bind.CreateHostPath = &value
			}
			if spec.SELinux != "" {
				bind.SELinux = spec.SELinux
			}
			if bind.Propagation != "" || bind.CreateHostPath != nil || bind.SELinux != "" {
				mount.Bind = bind
			}
		}
		if mount.Type == "volume" {
			volume := &composeVolumeMountOptions{}
			if spec.NoCopy {
				volume.NoCopy = true
			}
			if spec.Subpath != "" {
				volume.Subpath = spec.Subpath
			}
			if volume.NoCopy || volume.Subpath != "" {
				mount.Volume = volume
			}
		}
		return mount, true
	default:
		return composeServiceMount{}, false
	}
}

func composeNamedVolume(raw string) (string, bool) {
	spec, ok := devcontainer.ParseMountSpec(raw)
	if !ok || spec.Type != "volume" || spec.Source == "" {
		return "", false
	}
	return spec.Source, true
}

func isTrue(value string) bool {
	return strings.EqualFold(value, "true") || value == "1"
}

func sortedVolumeNames(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
