package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
	"go.yaml.in/yaml/v3"
)

var errManagedContainerNotFound = errors.New("managed container not found")

type containerReuseRequirements struct {
	BridgeEnabled bool
	SSHAgent      bool
}

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

func containerBridgeModeMatches(inspect docker.ContainerInspect, bridgeEnabled bool) bool {
	return (inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true") == bridgeEnabled
}

func containerSSHAgentMatches(inspect docker.ContainerInspect, sshAgent bool) bool {
	if inspect.Config.Labels[devcontainer.SSHAgentLabel] == "true" {
		return sshAgent
	}
	if sshAgent {
		return capssh.HasTargetMount(inspect, capssh.ContainerSocketPath)
	}
	return !capssh.HasTargetMount(inspect, capssh.ContainerSocketPath)
}

func (e *Executor) ReconcileContainer(ctx context.Context, observed ObservedState, resolved devcontainer.ResolvedConfig, image string, imagePlan ImagePlan, bridgeEnabled bool, sshAgent bool, forceNew bool, events ui.Sink) (string, string, bool, error) {
	containerKey, err := e.desiredContainerKey(resolved, imagePlan, image, bridgeEnabled, sshAgent)
	if err != nil {
		return "", "", false, err
	}
	plan, err := PlanContainer(observed, DesiredContainer{ReuseKey: containerKey, ForceNew: forceNew})
	if err != nil {
		return "", "", false, err
	}
	switch plan.Action {
	case ContainerActionReuse:
		return plan.ContainerID, containerKey, false, nil
	case ContainerActionStart:
		stdout, stderr := e.progressWriters(events, phaseContainer, "Starting existing container "+plan.ContainerID, e.stdout, e.stderr)
		if err := e.engine.StartContainer(ctx, dockercli.StartContainerRequest{ContainerID: plan.ContainerID, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", "", false, err
		}
		return plan.ContainerID, containerKey, false, nil
	case ContainerActionReplace:
		if plan.NeedsCleanup && plan.ContainerID != "" {
			if err := e.removeContainer(ctx, plan.ContainerID, events); err != nil {
				return "", "", false, err
			}
		}
		containerID, err := e.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, "", events)
		return containerID, containerKey, true, err
	case ContainerActionCreate:
		containerID, err := e.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, "", events)
		return containerID, containerKey, true, err
	default:
		return plan.ContainerID, containerKey, false, nil
	}
}

func (e *Executor) ensureReusableContainer(ctx context.Context, containerID string, requirements containerReuseRequirements, events ui.Sink) (string, bool, error) {
	inspect, err := e.engine.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: containerID})
	if err != nil {
		return "", false, err
	}
	if !containerBridgeModeMatches(inspect, requirements.BridgeEnabled) || !containerSSHAgentMatches(inspect, requirements.SSHAgent) {
		if err := e.removeContainer(ctx, containerID, events); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	if !inspect.State.Running {
		stdout, stderr := e.progressWriters(events, phaseContainer, fmt.Sprintf("Starting existing container %s", inspect.ID), e.stdout, e.stderr)
		if err := e.engine.StartContainer(ctx, dockercli.StartContainerRequest{ContainerID: inspect.ID, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", false, err
		}
	}
	return inspect.ID, true, nil
}

func (e *Executor) findContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return e.findComposeContainer(ctx, resolved)
	}
	filters := make([]string, 0, len(resolved.Labels))
	for key, value := range resolved.Labels {
		filters = append(filters, "label="+key+"="+value)
	}
	result, err := e.engine.ListContainers(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: filters})
	if err != nil {
		return "", err
	}
	return e.selectBestContainerID(ctx, result)
}

func (e *Executor) removeContainer(ctx context.Context, containerID string, events ui.Sink) error {
	stdout, stderr := e.progressWriters(events, phaseContainer, fmt.Sprintf("Removing managed container %s", containerID), e.stdout, e.stderr)
	return e.engine.RemoveContainer(ctx, dockercli.RemoveContainerRequest{ContainerID: containerID, Force: true, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}})
}

func (e *Executor) effectiveRemoteUser(ctx context.Context, prepared preparedWorkspace) (string, error) {
	if user := effectiveRemoteUserFromContainerInspect(prepared.containerInspect, prepared.resolved); user != "" {
		return user, nil
	}
	if prepared.containerID != "" {
		inspect, err := e.engine.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: prepared.containerID})
		if err != nil {
			return "", err
		}
		return effectiveRemoteUserFromContainerInspect(&inspect, prepared.resolved), nil
	}
	return e.InspectImageUser(ctx, prepared.image)
}

func (e *Executor) readManagedContainerState(ctx context.Context, prepared preparedWorkspace) (*ManagedContainer, error) {
	if prepared.containerID == "" {
		return nil, nil
	}
	inspect := prepared.containerInspect
	if inspect == nil {
		return nil, fmt.Errorf("read managed container state for %s: container metadata is unavailable", prepared.containerID)
	}
	metadata, err := spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return nil, err
	}
	metadata, err = e.mergeSourceImageMetadata(ctx, prepared.resolved, inspect.Image, metadata)
	if err != nil {
		return nil, err
	}
	merged := spec.MergeMetadata(prepared.resolved.Config, metadata)
	effectiveUser := firstNonEmpty(merged.RemoteUser, merged.ContainerUser, inspect.Config.User)
	return &ManagedContainer{
		ID:            inspect.ID,
		Name:          strings.TrimPrefix(inspect.Name, "/"),
		Image:         inspect.Image,
		Status:        inspect.State.Status,
		Running:       inspect.State.Running,
		RemoteUser:    effectiveUser,
		ContainerEnv:  RedactSensitiveMap(envListToMap(inspect.Config.Env)),
		Labels:        RedactSensitiveMap(inspect.Config.Labels),
		ForwardPorts:  []string(merged.ForwardPorts),
		MetadataCount: len(metadata),
		BridgeEnabled: inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true",
	}, nil
}

func envListToMap(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for _, entry := range values {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (e *Executor) selectBestContainerID(ctx context.Context, output string) (string, error) {
	best, err := selectBestContainer(ctx, output, inspectContainerWithEngine(e.engine))
	if err != nil {
		return "", err
	}
	return best.ID, nil
}

func isNumericUser(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (e *Executor) createContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool, overridePath string, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return e.createComposeContainer(ctx, resolved, image, containerKey, overridePath, events)
	}
	return e.createManagedContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent)
}

func (e *Executor) createManagedContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool) (string, error) {
	stateMount := fmt.Sprintf("type=bind,source=%s,target=%s", resolved.StateDir, "/var/run/hatchctl")
	metadataLabel, err := spec.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return "", err
	}
	labels := map[string]string{}
	for key, value := range resolved.Labels {
		labels[key] = value
	}
	if metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	if containerKey != "" {
		labels[ContainerKeyLabel] = containerKey
	}
	if bridgeEnabled {
		labels[devcontainer.BridgeEnabledLabel] = "true"
	}
	if sshAgent {
		labels[devcontainer.SSHAgentLabel] = "true"
	}
	env := map[string]string{}
	for key, value := range resolved.Merged.ContainerEnv {
		env[key] = value
	}
	return e.engine.RunDetachedContainer(ctx, dockercli.RunDetachedContainerRequest{Name: resolved.ContainerName, Labels: labels, Mounts: append([]string{resolved.WorkspaceMount, stateMount}, resolved.Merged.Mounts...), Init: resolved.Merged.Init, Privileged: resolved.Merged.Privileged, CapAdd: resolved.Merged.CapAdd, SecurityOpt: resolved.Merged.SecurityOpt, Env: env, ExtraArgs: resolved.Config.RunArgs, Image: image, Command: spec.ContainerCommand(resolved.Config)})
}

func (e *Executor) readComposeConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig) (composeConfig, error) {
	if resolved == nil {
		return composeConfig{}, nil
	}
	output, err := e.engine.ComposeConfig(ctx, dockercli.ComposeConfigRequest{Target: dockercli.ComposeTarget{Files: resolved.ComposeFiles, Project: resolved.ComposeProject, Dir: resolved.ConfigDir}, Format: "json"})
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

func (e *Executor) findComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	project := resolved.ComposeProject
	if project == "" {
		config, err := e.readComposeConfig(ctx, &resolved)
		if err != nil {
			return "", err
		}
		project = firstNonEmpty(config.Name, resolved.ComposeProject)
	}
	result, err := e.engine.ListContainers(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: []string{"label=com.docker.compose.project=" + project, "label=com.docker.compose.service=" + resolved.ComposeService}})
	if err != nil {
		return "", err
	}
	return e.selectBestContainerID(ctx, result)
}

func writeComposeOverride(resolved devcontainer.ResolvedConfig, image string, containerKey string) (string, error) {
	contents, err := renderComposeOverride(resolved, image, containerKey)
	if err != nil {
		return "", err
	}
	return storefs.WriteComposeOverride(resolved.StateDir, []byte(contents))
}

func renderComposeOverride(resolved devcontainer.ResolvedConfig, image string, containerKey string) (string, error) {
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
	if resolved.Merged.ContainerEnv["SSH_AUTH_SOCK"] == capssh.ContainerSocketPath {
		labels[devcontainer.SSHAgentLabel] = "true"
	}
	if metadataLabel, err := spec.MetadataLabelValue(resolved.Merged.Metadata); err == nil && metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	if containerKey != "" {
		labels[ContainerKeyLabel] = containerKey
	}
	for _, key := range spec.SortedMapKeys(labels) {
		service.Labels = append(service.Labels, key+"="+labels[key])
	}
	for _, key := range spec.SortedMapKeys(resolved.Merged.ContainerEnv) {
		service.Environment = append(service.Environment, key+"="+resolved.Merged.ContainerEnv[key])
	}
	allMounts := append([]string{resolved.WorkspaceMount}, resolved.Merged.Mounts...)
	namedVolumes := map[string]struct{}{}
	for _, mount := range allMounts {
		mountSpec, ok := spec.ParseMountSpec(mount)
		if !ok {
			continue
		}
		if value, ok := composeMountValue(mountSpec); ok {
			service.Volumes = append(service.Volumes, value)
		}
		if source, ok := composeNamedVolume(mountSpec); ok {
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
		service.Command = []string{"/bin/sh", "-lc", spec.KeepAliveCommand()}
	}
	if len(resolved.Merged.CapAdd) > 0 {
		service.CapAdd = append([]string(nil), resolved.Merged.CapAdd...)
	}
	if len(resolved.Merged.SecurityOpt) > 0 {
		service.SecurityOpt = append([]string(nil), resolved.Merged.SecurityOpt...)
	}
	if image != "" {
		service.Image = image
	}
	override := composeOverrideFile{Services: map[string]composeOverrideService{resolved.ComposeService: service}}
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

func composeMountValue(mountSpec spec.MountSpec) (composeServiceMount, bool) {
	switch mountSpec.Type {
	case "bind", "volume":
		if mountSpec.Source == "" {
			return composeServiceMount{}, false
		}
		mount := composeServiceMount{Type: mountSpec.Type, Source: mountSpec.Source, Target: mountSpec.Target}
		if mountSpec.ReadOnly {
			mount.ReadOnly = true
		}
		if mountSpec.Consistency != "" {
			mount.Consistency = mountSpec.Consistency
		}
		if mount.Type == "bind" {
			bind := &composeBindMountOptions{}
			if mountSpec.BindPropagation != "" {
				bind.Propagation = mountSpec.BindPropagation
			}
			if mountSpec.CreateHostPath != nil {
				value := *mountSpec.CreateHostPath
				bind.CreateHostPath = &value
			}
			if mountSpec.SELinux != "" {
				bind.SELinux = mountSpec.SELinux
			}
			if bind.Propagation != "" || bind.CreateHostPath != nil || bind.SELinux != "" {
				mount.Bind = bind
			}
		}
		if mount.Type == "volume" {
			volume := &composeVolumeMountOptions{}
			if mountSpec.NoCopy {
				volume.NoCopy = true
			}
			if mountSpec.Subpath != "" {
				volume.Subpath = mountSpec.Subpath
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

func composeNamedVolume(mountSpec spec.MountSpec) (string, bool) {
	if mountSpec.Type != "volume" || mountSpec.Source == "" {
		return "", false
	}
	return mountSpec.Source, true
}

func sortedVolumeNames(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func composeTarget(resolved devcontainer.ResolvedConfig, overridePath string) dockercli.ComposeTarget {
	files := append([]string(nil), resolved.ComposeFiles...)
	if overridePath != "" {
		files = append(files, overridePath)
	}
	return dockercli.ComposeTarget{Files: files, Project: resolved.ComposeProject, Dir: resolved.ConfigDir}
}

func (e *Executor) startComposeService(ctx context.Context, resolved devcontainer.ResolvedConfig, overridePath string, events ui.Sink) error {
	stdout, stderr := e.progressWriters(events, phaseContainer, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), e.stdout, e.stderr)
	return e.engine.ComposeUp(ctx, dockercli.ComposeUpRequest{
		Target:   composeTarget(resolved, overridePath),
		Services: []string{resolved.ComposeService},
		NoBuild:  true,
		Detach:   true,
		Streams:  dockercli.Streams{Stdout: stdout, Stderr: stderr},
	})
}

func (e *Executor) createComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, overridePath string, events ui.Sink) (string, error) {
	path := overridePath
	var err error
	if path == "" {
		path, err = writeComposeOverride(resolved, image, containerKey)
		if err != nil {
			return "", err
		}
		defer os.Remove(path)
	}
	if err := e.startComposeService(ctx, resolved, path, events); err != nil {
		return "", err
	}
	return e.findComposeContainer(ctx, resolved)
}

func (e *Executor) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, bridgeEnabled bool, sshAgent bool, overridePath string, events ui.Sink) (string, bool, error) {
	containerID, err := e.findComposeContainer(ctx, resolved)
	if err != nil && !errors.Is(err, errManagedContainerNotFound) {
		return "", false, err
	}
	if err == nil && containerID != "" {
		reusedID, reused, err := e.ensureReusableContainer(ctx, containerID, containerReuseRequirements{BridgeEnabled: bridgeEnabled, SSHAgent: sshAgent}, events)
		if err != nil {
			return "", false, err
		}
		if reused {
			return reusedID, false, nil
		}
	}
	if err := e.startComposeService(ctx, resolved, overridePath, events); err != nil {
		return "", false, err
	}
	containerID, err = e.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}
