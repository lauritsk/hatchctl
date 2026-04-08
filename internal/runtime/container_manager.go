package runtime

import (
	"context"
	"fmt"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

type runtimeContainerManager struct {
	runner *Runner
}

func (m *runtimeContainerManager) EnsureContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, bridgeEnabled bool, overridePath string, events ui.Sink) (string, bool, error) {
	if resolved.SourceKind == "compose" {
		return m.ensureComposeContainer(ctx, resolved, overridePath, events)
	}
	containerID, err := m.runner.findContainer(ctx, resolved)
	if err == nil && containerID != "" {
		matches, matchErr := m.runner.containerBridgeModeMatches(ctx, containerID, bridgeEnabled)
		if matchErr != nil {
			return "", false, matchErr
		}
		if !matches {
			if err := m.runner.removeContainer(ctx, containerID, events); err != nil {
				return "", false, err
			}
		} else {
			status, statusErr := m.runner.backend.ContainerStatus(ctx, containerID)
			if statusErr == nil && status != "running" {
				if err := m.runner.backend.StartContainer(ctx, containerID, events); err != nil {
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
		args = append(args, "-e", key+"="+resolved.Merged.ContainerEnv[key])
	}
	for _, mount := range resolved.Merged.Mounts {
		args = append(args, "--mount", mount)
	}
	args = append(args, resolved.Config.RunArgs...)
	args = append(args, image)
	args = append(args, devcontainer.ContainerCommand(resolved.Config)...)

	containerID, err = m.runner.backend.RunContainer(ctx, args)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}

func (m *runtimeContainerManager) ensureComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, overridePath string, events ui.Sink) (string, bool, error) {
	containerID, err := m.runner.findComposeContainer(ctx, resolved)
	if err == nil && containerID != "" {
		matches, matchErr := m.runner.containerBridgeModeMatches(ctx, containerID, resolved.Merged.ContainerEnv["DEVCONTAINER_BRIDGE_ENABLED"] == "true")
		if matchErr != nil {
			return "", false, matchErr
		}
		if !matches {
			if err := m.runner.removeContainer(ctx, containerID, events); err != nil {
				return "", false, err
			}
			containerID = ""
		} else {
			status, statusErr := m.runner.backend.ContainerStatus(ctx, containerID)
			if statusErr == nil && status == "running" {
				return containerID, false, nil
			}
		}
	}
	if err := m.runner.backend.ComposeUp(ctx, resolved, overridePath, events); err != nil {
		return "", false, err
	}
	containerID, err = m.runner.findComposeContainer(ctx, resolved)
	if err != nil {
		return "", false, err
	}
	return containerID, true, nil
}
