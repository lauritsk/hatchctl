package runtime

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

var errManagedContainerNotFound = errors.New("managed container not found")

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
	return r.engineAdapter.RemoveContainer(ctx, containerID, events)
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
