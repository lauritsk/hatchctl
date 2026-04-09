package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

var errManagedContainerNotFound = errors.New("managed container not found")

type containerReuseRequirements struct {
	BridgeEnabled bool
	SSHAgent      bool
}

func containerBridgeModeMatches(inspect docker.ContainerInspect, bridgeEnabled bool) bool {
	return (inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true") == bridgeEnabled
}

func containerSSHAgentMatches(inspect docker.ContainerInspect, sshAgent bool) bool {
	if inspect.Config.Labels[devcontainer.SSHAgentLabel] == "true" {
		return sshAgent
	}
	if sshAgent {
		return containerHasMountTarget(inspect, sshAgentContainerSocketPath)
	}
	return !containerHasMountTarget(inspect, sshAgentContainerSocketPath)
}

func (r *Runner) ensureReusableContainer(ctx context.Context, containerID string, requirements containerReuseRequirements, events ui.Sink) (string, bool, error) {
	inspect, err := r.backend.InspectContainer(ctx, containerID)
	if err != nil {
		return "", false, err
	}
	if !containerBridgeModeMatches(inspect, requirements.BridgeEnabled) || !containerSSHAgentMatches(inspect, requirements.SSHAgent) {
		if err := r.removeContainer(ctx, containerID, events); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	if !inspect.State.Running {
		stdout, stderr := r.progressWriters(events, phaseContainer, fmt.Sprintf("Starting existing container %s", inspect.ID), r.stdout, r.stderr)
		if err := r.backend.StartContainer(ctx, dockercli.StartContainerRequest{ContainerID: inspect.ID, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", false, err
		}
	}
	return inspect.ID, true, nil
}

func (r *Runner) findContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.findComposeContainer(ctx, resolved)
	}
	filters := make([]string, 0, len(resolved.Labels))
	for key, value := range resolved.Labels {
		filters = append(filters, "label="+key+"="+value)
	}
	result, err := r.backend.ListContainers(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: filters})
	if err != nil {
		return "", err
	}
	return r.selectBestContainerID(ctx, result)
}

func (r *Runner) removeContainer(ctx context.Context, containerID string, events ui.Sink) error {
	stdout, stderr := r.progressWriters(events, phaseContainer, fmt.Sprintf("Removing managed container %s", containerID), r.stdout, r.stderr)
	return r.backend.RemoveContainer(ctx, dockercli.RemoveContainerRequest{ContainerID: containerID, Force: true, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}})
}

func (r *Runner) effectiveRemoteUser(ctx context.Context, prepared preparedWorkspace) (string, error) {
	if user := effectiveRemoteUserFromContainerInspect(prepared.containerInspect, prepared.resolved); user != "" {
		return user, nil
	}
	if prepared.containerID != "" {
		inspect, err := r.backend.InspectContainer(ctx, prepared.containerID)
		if err != nil {
			return "", err
		}
		return effectiveRemoteUserFromContainerInspect(&inspect, prepared.resolved), nil
	}
	return r.inspectImageUser(ctx, prepared.image)
}

func (r *Runner) readManagedContainerState(prepared preparedWorkspace) (*ManagedContainer, error) {
	if prepared.containerID == "" {
		return nil, nil
	}
	inspect := prepared.containerInspect
	if inspect == nil {
		return nil, fmt.Errorf("read managed container state for %s: container metadata is unavailable", prepared.containerID)
	}
	metadata, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return nil, err
	}
	merged := devcontainer.MergeMetadata(prepared.resolved.Config, metadata)
	effectiveUser := firstNonEmpty(merged.RemoteUser, merged.ContainerUser, inspect.Config.User)
	return &ManagedContainer{
		ID:            inspect.ID,
		Name:          strings.TrimPrefix(inspect.Name, "/"),
		Image:         inspect.Image,
		Status:        inspect.State.Status,
		Running:       inspect.State.Running,
		RemoteUser:    effectiveUser,
		ContainerEnv:  redactSensitiveMap(envListToMap(inspect.Config.Env)),
		Labels:        redactSensitiveMap(inspect.Config.Labels),
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

func (r *Runner) selectBestContainerID(ctx context.Context, output string) (string, error) {
	ids := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		ids = append(ids, line)
	}
	if len(ids) == 0 {
		return "", errManagedContainerNotFound
	}
	type candidate struct {
		id      string
		running bool
	}
	candidates := make([]candidate, 0, len(ids))
	for _, id := range ids {
		inspect, err := r.backend.InspectContainer(ctx, id)
		if err != nil {
			if docker.IsNotFound(err) {
				continue
			}
			return "", err
		}
		candidates = append(candidates, candidate{id: inspect.ID, running: inspect.State.Running})
	}
	if len(candidates) == 0 {
		return "", errManagedContainerNotFound
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].running != candidates[j].running {
			return candidates[i].running
		}
		return candidates[i].id < candidates[j].id
	})
	return candidates[0].id, nil
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

func (r *Runner) createContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool, overridePath string, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.createComposeContainer(ctx, resolved, image, containerKey, overridePath, events)
	}
	return r.createManagedContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent)
}

func (r *Runner) createManagedContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, bridgeEnabled bool, sshAgent bool) (string, error) {
	stateMount := fmt.Sprintf("type=bind,source=%s,target=%s", resolved.StateDir, "/var/run/hatchctl")
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
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
		labels[reconcile.ContainerKeyLabel] = containerKey
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
	return r.backend.RunDetachedContainer(ctx, dockercli.RunDetachedContainerRequest{
		Name:        resolved.ContainerName,
		Labels:      labels,
		Mounts:      append([]string{resolved.WorkspaceMount, stateMount}, resolved.Merged.Mounts...),
		Init:        resolved.Merged.Init,
		Privileged:  resolved.Merged.Privileged,
		CapAdd:      resolved.Merged.CapAdd,
		SecurityOpt: resolved.Merged.SecurityOpt,
		Env:         env,
		ExtraArgs:   resolved.Config.RunArgs,
		Image:       image,
		Command:     devcontainer.ContainerCommand(resolved.Config),
	})
}

func (r *Runner) createComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerKey string, overridePath string, events ui.Sink) (string, error) {
	path := overridePath
	var err error
	if path == "" {
		path, err = writeComposeOverride(resolved, image, containerKey)
		if err != nil {
			return "", err
		}
		defer os.Remove(path)
	}
	stdout, stderr := r.progressWriters(events, phaseContainer, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), r.stdout, r.stderr)
	target := dockercli.ComposeTarget{Files: append([]string(nil), resolved.ComposeFiles...), Project: resolved.ComposeProject, Dir: resolved.ConfigDir}
	if path != "" {
		target.Files = append(target.Files, path)
	}
	if err := r.backend.ComposeUp(ctx, dockercli.ComposeUpRequest{Target: target, Services: []string{resolved.ComposeService}, NoBuild: true, Detach: true, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
		return "", err
	}
	return r.findComposeContainer(ctx, resolved)
}

func (r *Runner) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, bridgeEnabled bool, sshAgent bool, overridePath string, events ui.Sink) (string, bool, error) {
	containerID, err := r.findComposeContainer(ctx, resolved)
	if err != nil && !errors.Is(err, errManagedContainerNotFound) {
		return "", false, err
	}
	if err == nil && containerID != "" {
		reusedID, reused, err := r.ensureReusableContainer(ctx, containerID, containerReuseRequirements{BridgeEnabled: bridgeEnabled, SSHAgent: sshAgent}, events)
		if err != nil {
			return "", false, err
		}
		if reused {
			return reusedID, false, nil
		}
	}
	stdout, stderr := r.progressWriters(events, phaseContainer, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), r.stdout, r.stderr)
	target := dockercli.ComposeTarget{Files: append([]string(nil), resolved.ComposeFiles...), Project: resolved.ComposeProject, Dir: resolved.ConfigDir}
	if overridePath != "" {
		target.Files = append(target.Files, overridePath)
	}
	if err := r.backend.ComposeUp(ctx, dockercli.ComposeUpRequest{Target: target, Services: []string{resolved.ComposeService}, NoBuild: true, Detach: true, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
		return "", false, err
	}
	containerID, err = r.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}
