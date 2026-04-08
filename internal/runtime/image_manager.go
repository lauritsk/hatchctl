package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

type runtimeImageManager struct {
	runner *Runner
}

func (m *runtimeImageManager) EnsureImage(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return m.ensureComposeImage(ctx, resolved, events)
	}
	if len(resolved.Features) > 0 {
		return m.ensureImageWithFeatures(ctx, resolved, events)
	}
	if resolved.Config.Image != "" {
		if err := m.runner.verifyImageReference(ctx, resolved.Config.Image, events); err != nil {
			return "", err
		}
		return resolved.Config.Image, nil
	}
	return resolved.ImageName, m.buildDockerfileImage(ctx, resolved, resolved.ImageName, events)
}

func (m *runtimeImageManager) buildDockerfileImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageName string, events ui.Sink) error {
	dockerfile := resolved.ConfigDir
	contextDir := resolved.ConfigDir
	if rel := devcontainer.EffectiveDockerfile(resolved.Config); rel != "" {
		dockerfile = filepath.Join(resolved.ConfigDir, rel)
	}
	if rel := devcontainer.EffectiveContext(resolved.Config); rel != "" {
		contextDir = filepath.Join(resolved.ConfigDir, rel)
	}
	args := []string{"build", "-f", dockerfile, "-t", imageName}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return err
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
	if resolved.Config.Build != nil && resolved.Config.Build.Target != "" {
		args = append(args, "--target", resolved.Config.Build.Target)
	}
	if resolved.Config.Build != nil {
		for key, value := range resolved.Config.Build.Args {
			args = append(args, "--build-arg", key+"="+value)
		}
		args = append(args, resolved.Config.Build.Options...)
	}
	args = append(args, contextDir)
	return m.runner.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: "Building container image", Args: args, Dir: "", Stdout: m.runner.stdout, Stderr: m.runner.stderr, Events: events})
}

func (m *runtimeImageManager) ensureImageWithFeatures(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, error) {
	baseImage := resolved.Config.Image
	if baseImage == "" {
		baseImage = resolved.ImageName + "-base"
		if err := m.buildDockerfileImage(ctx, resolved, baseImage, events); err != nil {
			return "", err
		}
	} else if err := m.runner.verifyImageReference(ctx, baseImage, events); err != nil {
		return "", err
	}
	return m.ensureFeaturesImageFromBase(ctx, resolved, baseImage, events)
}

func (m *runtimeImageManager) ensureFeaturesImageFromBase(ctx context.Context, resolved devcontainer.ResolvedConfig, baseImage string, events ui.Sink) (string, error) {
	imageUser, err := m.runner.inspectImageUser(ctx, baseImage)
	if err != nil {
		return "", err
	}
	containerUser := firstNonEmpty(resolved.Merged.ContainerUser, imageUser, "root")
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, containerUser)
	buildDir := filepath.Join(resolved.StateDir, "features-build")
	if err := os.RemoveAll(buildDir); err != nil {
		return "", err
	}
	if err := ensureDir(buildDir); err != nil {
		return "", err
	}
	if err := writeFeatureBuildContext(buildDir, baseImage, resolved.Features, containerUser, remoteUser, resolved.Merged.Metadata); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); err != nil {
		return "", fmt.Errorf("generated feature Dockerfile missing in %s: %w", buildDir, err)
	}
	args := []string{"build", "-f", filepath.Join(buildDir, "Dockerfile"), "-t", resolved.ImageName, buildDir}
	if err := m.runner.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: "Building features image", Args: args, Dir: "", Stdout: m.runner.stdout, Stderr: m.runner.stderr, Events: events}); err != nil {
		entries, _ := os.ReadDir(buildDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return "", fmt.Errorf("build features image from %s with files %v: %w", buildDir, names, err)
	}
	return resolved.ImageName, nil
}

func (m *runtimeImageManager) ensureComposeImage(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, error) {
	config, err := m.runner.readComposeConfig(ctx, resolved)
	if err != nil {
		return "", err
	}
	service, ok := config.Services[resolved.ComposeService]
	if !ok {
		return "", fmt.Errorf("compose service %q not found", resolved.ComposeService)
	}
	baseImage := service.Image
	if service.Build.Enabled() {
		if err := m.runner.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: fmt.Sprintf("Building compose service %s", resolved.ComposeService), Args: append(composeBaseArgs(resolved), "build", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: m.runner.stdout, Stderr: m.runner.stderr, Events: events}); err != nil {
			return "", err
		}
		if baseImage == "" {
			baseImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
	}
	if baseImage != "" && !service.Build.Enabled() {
		if err := m.runner.verifyImageReference(ctx, baseImage, events); err != nil {
			return "", err
		}
	}
	if len(resolved.Features) > 0 {
		if baseImage == "" {
			return "", fmt.Errorf("compose service %q needs an image or build result for features", resolved.ComposeService)
		}
		return m.ensureFeaturesImageFromBase(ctx, resolved, baseImage, events)
	}
	if baseImage != "" {
		return baseImage, nil
	}
	return resolved.ComposeProject + "-" + resolved.ComposeService, nil
}

func (m *runtimeImageManager) EnsureUpdatedUIDContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerID string, events ui.Sink) error {
	if containerID == "" {
		return nil
	}
	if resolved.Merged.UpdateRemoteUserUID != nil && !*resolved.Merged.UpdateRemoteUserUID {
		return nil
	}
	uid := os.Getuid()
	gid := os.Getgid()
	if uid <= 0 || gid <= 0 {
		return nil
	}
	inspect, err := m.runner.backend.InspectImage(ctx, image)
	if err != nil {
		return err
	}
	imageUser := inspect.Config.User
	if imageUser == "" {
		imageUser = "root"
	}
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser, imageUser)
	if remoteUser == "" || remoteUser == "root" || isNumericUser(remoteUser) {
		return nil
	}
	args := []string{"exec", "-u", "root", containerID, "sh", "-lc", updateUIDCommand, "sh", remoteUser, fmt.Sprintf("%d", uid), fmt.Sprintf("%d", gid)}
	return m.runner.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: "Reconciling container user", Args: args, Stdout: m.runner.stdout, Stderr: m.runner.stderr, Events: events})
}
