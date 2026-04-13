package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/spec"
)

var errManagedContainerNotFound = errors.New("managed container not found")

type containerReuseRequirements struct {
	BridgeEnabled bool
	SSHAgent      bool
}

func containerBridgeModeMatches(inspect backend.ContainerInspect, bridgeEnabled bool) bool {
	return (inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true") == bridgeEnabled
}

func containerSSHAgentMatches(inspect backend.ContainerInspect, sshAgent bool) bool {
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
		if err := e.engine.StartContainer(ctx, backend.StartContainerRequest{ContainerID: plan.ContainerID, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", "", false, err
		}
		return plan.ContainerID, containerKey, false, nil
	case ContainerActionReplace:
		if plan.NeedsCleanup && plan.ContainerID != "" {
			if err := e.removeContainer(ctx, plan.ContainerID, events); err != nil {
				return "", "", false, err
			}
		}
		containerID, err := e.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, events)
		return containerID, containerKey, true, err
	case ContainerActionCreate:
		containerID, err := e.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, events)
		return containerID, containerKey, true, err
	default:
		return plan.ContainerID, containerKey, false, nil
	}
}

func (e *Executor) ensureReusableContainer(ctx context.Context, containerID string, requirements containerReuseRequirements, events ui.Sink) (string, bool, error) {
	inspect, err := e.engine.InspectContainer(ctx, containerID)
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
		if err := e.engine.StartContainer(ctx, backend.StartContainerRequest{ContainerID: inspect.ID, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", false, err
		}
	}
	return inspect.ID, true, nil
}

func (e *Executor) findContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return e.findComposeContainer(ctx, resolved)
	}
	result, err := e.engine.ListContainers(ctx, backend.ListContainersRequest{All: true, Quiet: true, Labels: resolved.Labels})
	if err != nil {
		return "", err
	}
	return e.selectBestContainerID(ctx, result)
}

func (e *Executor) removeContainer(ctx context.Context, containerID string, events ui.Sink) error {
	stdout, stderr := e.progressWriters(events, phaseContainer, fmt.Sprintf("Removing managed container %s", containerID), e.stdout, e.stderr)
	return e.engine.RemoveContainer(ctx, backend.RemoveContainerRequest{ContainerID: containerID, Force: true, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}})
}

func (e *Executor) effectiveRemoteUser(ctx context.Context, prepared preparedWorkspace) (string, error) {
	if user := effectiveRemoteUserFromContainerInspect(prepared.containerInspect, prepared.resolved); user != "" {
		return user, nil
	}
	if prepared.containerID != "" {
		inspect, err := e.engine.InspectContainer(ctx, prepared.containerID)
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
	metadata, err := e.runtimeMetadataFromContainer(ctx, prepared.resolved, inspect)
	if err != nil {
		return nil, err
	}
	merged := mergedConfigWithRuntimeMetadata(prepared.resolved, inspect.Image, metadata)
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

func (e *Executor) createContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return e.createComposeContainer(ctx, resolved, image, containerKey, events)
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
	return e.engine.RunDetachedContainer(ctx, backend.RunDetachedContainerRequest{Name: resolved.ContainerName, Labels: labels, Mounts: append([]string{resolved.WorkspaceMount, stateMount}, resolved.Merged.Mounts...), Init: resolved.Merged.Init, Privileged: resolved.Merged.Privileged, CapAdd: resolved.Merged.CapAdd, SecurityOpt: resolved.Merged.SecurityOpt, Env: env, ExtraArgs: resolved.Config.RunArgs, Image: image, Command: spec.ContainerCommand(resolved.Config)})
}

func (e *Executor) readComposeConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig) (backend.ProjectConfig, error) {
	if resolved == nil {
		return backend.ProjectConfig{}, nil
	}
	config, err := e.engine.ProjectConfig(ctx, backend.ProjectConfigRequest{Target: backend.ProjectTarget{Files: resolved.ComposeFiles, Project: resolved.ComposeProject, Service: resolved.ComposeService, Dir: resolved.ConfigDir}})
	if err != nil {
		return backend.ProjectConfig{}, err
	}
	if config.Name != "" {
		resolved.ComposeProject = config.Name
	}
	return config, nil
}

func (e *Executor) findComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	_, primary, err := e.engine.ProjectContainers(ctx, backend.ProjectContainersRequest{Target: backend.ProjectTarget{Files: resolved.ComposeFiles, Project: resolved.ComposeProject, Service: resolved.ComposeService, Dir: resolved.ConfigDir}})
	if err != nil {
		return "", err
	}
	if primary == nil {
		return "", errManagedContainerNotFound
	}
	return primary.ID, nil
}

func projectOverride(resolved devcontainer.ResolvedConfig, image string, containerKey string) (backend.ProjectOverride, error) {
	override := backend.ProjectOverride{}
	if len(resolved.Features) > 0 {
		override.PullPolicy = "never"
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
	metadataLabel, err := spec.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return backend.ProjectOverride{}, err
	}
	if metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	if containerKey != "" {
		labels[ContainerKeyLabel] = containerKey
	}
	override.Labels = labels
	override.Environment = resolved.Merged.ContainerEnv
	allMounts := append([]string{resolved.WorkspaceMount}, resolved.Merged.Mounts...)
	namedVolumes := map[string]struct{}{}
	for _, mount := range allMounts {
		mountSpec, ok := spec.ParseMountSpec(mount)
		if !ok {
			continue
		}
		if value, ok := composeMountValue(mountSpec); ok {
			override.Mounts = append(override.Mounts, value)
		}
		if source, ok := composeNamedVolume(mountSpec); ok {
			namedVolumes[source] = struct{}{}
		}
	}
	override.Init = resolved.Merged.Init
	override.Privileged = resolved.Merged.Privileged
	if user := resolved.Merged.ContainerUser; user != "" {
		override.User = user
	}
	if overrideCommandEnabled(resolved.Config.OverrideCommand) {
		override.Command = []string{"/bin/sh", "-lc", spec.KeepAliveCommand()}
	}
	if len(resolved.Merged.CapAdd) > 0 {
		override.CapAdd = append([]string(nil), resolved.Merged.CapAdd...)
	}
	if len(resolved.Merged.SecurityOpt) > 0 {
		override.SecurityOpt = append([]string(nil), resolved.Merged.SecurityOpt...)
	}
	if image != "" {
		override.Image = image
	}
	if len(namedVolumes) > 0 {
		override.NamedVolumes = sortedVolumeNames(namedVolumes)
	}
	return override, nil
}

func overrideCommandEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func composeMountValue(mountSpec spec.MountSpec) (backend.ProjectMount, bool) {
	switch mountSpec.Type {
	case "bind", "volume":
		if mountSpec.Source == "" {
			return backend.ProjectMount{}, false
		}
		mount := backend.ProjectMount{Type: mountSpec.Type, Source: mountSpec.Source, Target: mountSpec.Target}
		if mountSpec.ReadOnly {
			mount.ReadOnly = true
		}
		if mountSpec.Consistency != "" {
			mount.Consistency = mountSpec.Consistency
		}
		if mount.Type == "bind" {
			bind := &backend.ProjectBindMount{}
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
			volume := &backend.ProjectVolumeMount{}
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
		return backend.ProjectMount{}, false
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

func composeTarget(resolved devcontainer.ResolvedConfig) backend.ProjectTarget {
	return backend.ProjectTarget{Files: append([]string(nil), resolved.ComposeFiles...), Project: resolved.ComposeProject, Service: resolved.ComposeService, Dir: resolved.ConfigDir}
}

func (e *Executor) startComposeService(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, events ui.Sink) error {
	stdout, stderr := e.progressWriters(events, phaseContainer, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), e.stdout, e.stderr)
	override, err := projectOverride(resolved, image, containerKey)
	if err != nil {
		return err
	}
	return e.engine.UpProject(ctx, backend.ProjectUpRequest{
		Target:   composeTarget(resolved),
		Services: []string{resolved.ComposeService},
		NoBuild:  true,
		Detach:   true,
		Override: &override,
		StateDir: resolved.StateDir,
		Streams:  backend.Streams{Stdout: stdout, Stderr: stderr},
	})
}

func (e *Executor) createComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, events ui.Sink) (string, error) {
	if err := e.startComposeService(ctx, resolved, image, containerKey, events); err != nil {
		return "", err
	}
	return e.findComposeContainer(ctx, resolved)
}

func (e *Executor) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool, events ui.Sink) (string, bool, error) {
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
	if err := e.startComposeService(ctx, resolved, image, containerKey, events); err != nil {
		return "", false, err
	}
	containerID, err = e.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}
