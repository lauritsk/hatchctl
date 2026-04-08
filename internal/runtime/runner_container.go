package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

var errManagedContainerNotFound = errors.New("managed container not found")

func (r *Runner) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, overridePath string, events ui.Sink) (string, bool, error) {
	containerID, err := r.findComposeContainer(ctx, resolved)
	if err == nil && containerID != "" {
		matches, matchErr := r.containerBridgeModeMatches(ctx, containerID, resolved.Merged.ContainerEnv["DEVCONTAINER_BRIDGE_ENABLED"] == "true")
		if matchErr != nil {
			return "", false, matchErr
		}
		if !matches {
			if err := r.removeContainer(ctx, containerID, events); err != nil {
				return "", false, err
			}
			containerID = ""
		} else {
			status, statusErr := r.docker.Output(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
			if statusErr == nil && status == "running" {
				return containerID, false, nil
			}
		}
	}
	if err := r.docker.Run(ctx, r.progressDockerRunOptions(events, fmt.Sprintf("Starting compose service %s", resolved.ComposeService), docker.RunOptions{Args: append(r.composeArgs(resolved, overridePath), "up", "--no-build", "-d", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: r.stdout, Stderr: r.stderr})); err != nil {
		return "", false, err
	}
	containerID, err = r.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}

func (r *Runner) ensureContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, bridgeEnabled bool, overridePath string, events ui.Sink) (string, bool, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeContainer(ctx, resolved, overridePath, events)
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err == nil && containerID != "" {
		matches, matchErr := r.containerBridgeModeMatches(ctx, containerID, bridgeEnabled)
		if matchErr != nil {
			return "", false, matchErr
		}
		if !matches {
			if err := r.removeContainer(ctx, containerID, events); err != nil {
				return "", false, err
			}
		} else {
			status, statusErr := r.docker.Output(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
			if statusErr == nil && status != "running" {
				if err := r.docker.Run(ctx, r.progressDockerRunOptions(events, fmt.Sprintf("Starting existing container %s", containerID), docker.RunOptions{Args: []string{"start", containerID}, Stdout: r.stdout, Stderr: r.stderr})); err != nil {
					return "", false, err
				}
			}
			return containerID, false, nil
		}
	}

	stateMount := fmt.Sprintf("type=bind,source=%s,target=%s", resolved.StateDir, "/var/run/hatchctl")
	args := []string{"run", "-d", "--name", resolved.ContainerName}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return "", false, err
	}
	for key, value := range resolved.Labels {
		args = append(args, "--label", key+"="+value)
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
	if bridgeEnabled {
		args = append(args, "--label", devcontainer.BridgeEnabledLabel+"=true")
	}
	args = append(args, "--mount", resolved.WorkspaceMount, "--mount", stateMount)
	if resolved.Merged.Init {
		args = append(args, "--init")
	}
	if resolved.Merged.Privileged {
		args = append(args, "--privileged")
	}
	for _, cap := range resolved.Merged.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, sec := range resolved.Merged.SecurityOpt {
		args = append(args, "--security-opt", sec)
	}
	for _, key := range devcontainer.SortedMapKeys(resolved.Merged.ContainerEnv) {
		value := resolved.Merged.ContainerEnv[key]
		args = append(args, "-e", key+"="+value)
	}
	for _, mount := range resolved.Merged.Mounts {
		args = append(args, "--mount", mount)
	}
	args = append(args, resolved.Config.RunArgs...)
	args = append(args, image)
	args = append(args, devcontainer.ContainerCommand(resolved.Config)...)

	containerID, err = r.docker.Output(ctx, args...)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}

func (r *Runner) containerBridgeModeMatches(ctx context.Context, containerID string, bridgeEnabled bool) (bool, error) {
	inspect, err := r.docker.InspectContainer(ctx, containerID)
	if err != nil {
		return false, err
	}
	return (inspect.Config.Labels[devcontainer.BridgeEnabledLabel] == "true") == bridgeEnabled, nil
}

func (r *Runner) findContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.findComposeContainer(ctx, resolved)
	}
	args := []string{"ps", "-aq"}
	for key, value := range resolved.Labels {
		args = append(args, "--filter", "label="+key+"="+value)
	}
	result, err := r.docker.Output(ctx, args...)
	if err != nil {
		return "", err
	}
	return r.selectBestContainerID(ctx, result)
}

func (r *Runner) removeContainer(ctx context.Context, containerID string, events ui.Sink) error {
	return r.docker.Run(ctx, r.progressDockerRunOptions(events, fmt.Sprintf("Removing managed container %s", containerID), docker.RunOptions{Args: []string{"rm", "-f", containerID}, Stdout: r.stdout, Stderr: r.stderr}))
}

func (r *Runner) reconcileState(ctx context.Context, resolved devcontainer.ResolvedConfig, state devcontainer.State) (devcontainer.State, error) {
	if state.ContainerID != "" {
		if _, err := r.docker.InspectContainer(ctx, state.ContainerID); err == nil {
			return state, nil
		} else if !docker.IsNotFound(err) {
			return devcontainer.State{}, err
		}
	}
	containerID, err := r.findContainer(ctx, resolved)
	if err != nil {
		if errors.Is(err, errManagedContainerNotFound) {
			return devcontainer.State{BridgeEnabled: state.BridgeEnabled, BridgeSessionID: state.BridgeSessionID}, nil
		}
		return devcontainer.State{}, err
	}
	state.ContainerID = containerID
	state.LifecycleReady = false
	return state, nil
}

func (r *Runner) applyBridgeConfig(resolved *devcontainer.ResolvedConfig, enabled bool, helperArch string) (*bridge.Report, error) {
	report, merged, err := bridge.Apply(resolved.StateDir, enabled, helperArch, resolved.Merged)
	if err != nil {
		return nil, err
	}
	resolved.Merged = merged
	return (*bridge.Report)(report), nil
}

func (r *Runner) previewBridgeConfig(resolved *devcontainer.ResolvedConfig, enabled bool) (*bridge.Report, error) {
	report, merged, err := bridge.Preview(resolved.StateDir, enabled, resolved.Merged)
	if err != nil {
		return nil, err
	}
	resolved.Merged = merged
	return (*bridge.Report)(report), nil
}

func (r *Runner) effectiveRemoteUser(ctx context.Context, prepared preparedWorkspace) (string, error) {
	if user := effectiveRemoteUserFromContainerInspect(prepared.containerInspect, prepared.resolved); user != "" {
		return user, nil
	}
	if prepared.containerID != "" {
		inspect, err := r.docker.InspectContainer(ctx, prepared.containerID)
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
		return nil, errors.New("managed container inspect not loaded")
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
		ContainerEnv:  envListToMap(inspect.Config.Env),
		Labels:        inspect.Config.Labels,
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
		inspect, err := r.docker.InspectContainer(ctx, id)
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
