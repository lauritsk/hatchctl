package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	capuid "github.com/lauritsk/hatchctl/internal/capability/uidremap"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/fileutil"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

const featureInstallHelper = `#!/bin/sh
set -eu
feature_dir=$1
builtin_env=$2
options_env=$3

set -a
. "$builtin_env"
if [ -f "$options_env" ]; then
  . "$options_env"
fi
set +a

cd "$feature_dir"
chmod +x ./install.sh
exec ./install.sh
`

func (r *Runner) enrichMergedConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig, image string) error {
	inspect, err := r.backend.InspectImage(ctx, image)
	if err != nil {
		if resolved.SourceKind == "compose" || isManagedImage(resolved, image) {
			resolved.Merged = devcontainer.MergeMetadata(resolved.Config, featureMetadata(resolved.Features))
			return nil
		}
		return err
	}
	metadata, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return err
	}
	if isManagedImage(resolved, image) {
		resolved.Merged = devcontainer.MergeMetadata(devcontainer.Config{}, metadata)
		resolved.Merged.Config = resolved.Config
		return nil
	}
	resolved.Merged = devcontainer.MergeMetadata(resolved.Config, metadata)
	return nil
}

func (r *Runner) inspectImageUser(ctx context.Context, image string) (string, error) {
	inspect, err := r.backend.InspectImage(ctx, image)
	if err != nil {
		if docker.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return inspect.Config.User, nil
}

func (r *Runner) inspectImageArchitecture(ctx context.Context, image string) (string, error) {
	inspect, err := r.backend.InspectImage(ctx, image)
	if err != nil {
		if docker.IsNotFound(err) {
			return runtime.GOARCH, nil
		}
		return "", err
	}
	if inspect.Architecture != "" {
		return inspect.Architecture, nil
	}
	return runtime.GOARCH, nil
}

func featureMetadata(features []devcontainer.ResolvedFeature) []devcontainer.MetadataEntry {
	if len(features) == 0 {
		return nil
	}
	result := make([]devcontainer.MetadataEntry, 0, len(features))
	for _, feature := range features {
		result = append(result, feature.Metadata)
	}
	return result
}

func mergeManagedImageMetadata(base []devcontainer.MetadataEntry, overlay []devcontainer.MetadataEntry) []devcontainer.MetadataEntry {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make([]devcontainer.MetadataEntry, 0, len(base)+len(overlay))
	merged = append(merged, base...)
	merged = append(merged, overlay...)
	return merged
}

func (r *Runner) imageMetadata(ctx context.Context, image string) ([]devcontainer.MetadataEntry, error) {
	inspect, err := r.backend.InspectImage(ctx, image)
	if err != nil {
		return nil, err
	}
	return devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
}

func isManagedImage(resolved *devcontainer.ResolvedConfig, image string) bool {
	return image == resolved.ImageName || strings.HasPrefix(image, resolved.ImageName+"-")
}

func writeFeatureBuildContext(buildDir string, baseImage string, features []devcontainer.ResolvedFeature, containerUser string, remoteUser string, metadata []devcontainer.MetadataEntry, imageKey string) error {
	metadataLabel, err := devcontainer.MetadataLabelValue(metadata)
	if err != nil {
		return err
	}
	builtinEnv := map[string]string{
		"_CONTAINER_USER": containerUser,
		"_REMOTE_USER":    remoteUser,
	}
	if err := fileutil.WriteFile(filepath.Join(buildDir, "devcontainer-features.builtin.env"), []byte(shellEnvFile(builtinEnv)), 0o600); err != nil {
		return err
	}
	if err := fileutil.WriteFile(filepath.Join(buildDir, "devcontainer-features-install.sh"), []byte(featureInstallHelper), 0o755); err != nil {
		return err
	}
	var dockerfile strings.Builder
	dockerfile.WriteString("FROM " + baseImage + "\nUSER root\n")
	dockerfile.WriteString("RUN mkdir -p /tmp/dev-container-features\n")
	dockerfile.WriteString("COPY devcontainer-features.builtin.env /tmp/dev-container-features/devcontainer-features.builtin.env\n")
	dockerfile.WriteString("COPY devcontainer-features-install.sh /tmp/dev-container-features/devcontainer-features-install.sh\n")
	for i, feature := range features {
		rel := fmt.Sprintf("feature-%02d", i)
		dst := filepath.Join(buildDir, rel)
		if err := copyDir(feature.Path, dst); err != nil {
			return err
		}
		if len(feature.Options) > 0 {
			if err := fileutil.WriteFile(filepath.Join(dst, "devcontainer-features.env"), []byte(shellEnvFile(feature.Options)), 0o600); err != nil {
				return err
			}
		}
		dockerfile.WriteString("COPY " + rel + " /tmp/hatchctl-features/" + rel + "\n")
		if len(feature.Metadata.ContainerEnv) > 0 {
			for _, key := range devcontainer.SortedMapKeys(feature.Metadata.ContainerEnv) {
				dockerfile.WriteString("ENV " + key + "=" + dockerfileQuotedValue(feature.Metadata.ContainerEnv[key]) + "\n")
			}
		}
		dockerfile.WriteString("RUN if [ -f /tmp/hatchctl-features/" + rel + "/install.sh ]; then /tmp/dev-container-features/devcontainer-features-install.sh /tmp/hatchctl-features/" + rel + " /tmp/dev-container-features/devcontainer-features.builtin.env /tmp/hatchctl-features/" + rel + "/devcontainer-features.env; fi\n")
	}
	if metadataLabel != "" {
		dockerfile.WriteString("LABEL " + devcontainer.ImageMetadataLabel + "=" + dockerfileQuotedValue(metadataLabel) + "\n")
	}
	if imageKey != "" {
		dockerfile.WriteString("LABEL " + reconcile.ImageKeyLabel + "=" + dockerfileQuotedValue(imageKey) + "\n")
	}
	return fileutil.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile.String()), 0o600)
}

func shellEnvFile(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	var lines []string
	for _, key := range devcontainer.SortedMapKeys(values) {
		lines = append(lines, key+"="+devcontainer.ShellQuote(values[key]))
	}
	return strings.Join(lines, "\n") + "\n"
}

func dockerfileQuotedValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "\n", "\\n", "\r", "")
	return "\"" + replacer.Replace(value) + "\""
}

func copyDir(src string, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return os.CopyFS(dst, os.DirFS(src))
}

func (r *Runner) ensureImage(ctx context.Context, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeImage(ctx, resolved, "", events)
	}
	if len(resolved.Features) > 0 {
		return r.ensureImageWithFeatures(ctx, resolved, "", events)
	}
	if resolved.Config.Image != "" {
		if err := r.verifyImageReference(ctx, resolved.Config.Image, events); err != nil {
			return "", err
		}
		return resolved.Config.Image, nil
	}
	return resolved.ImageName, r.buildDockerfileImage(ctx, resolved, resolved.ImageName, "", events)
}

func (r *Runner) buildDockerfileImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageName string, imageKey string, events ui.Sink) error {
	dockerfile := resolved.ConfigDir
	contextDir := resolved.ConfigDir
	if rel := devcontainer.EffectiveDockerfile(resolved.Config); rel != "" {
		dockerfile = filepath.Join(resolved.ConfigDir, rel)
	}
	if rel := devcontainer.EffectiveContext(resolved.Config); rel != "" {
		contextDir = filepath.Join(resolved.ConfigDir, rel)
	}
	labels := map[string]string{}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return err
	}
	if metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	if imageKey != "" {
		labels[reconcile.ImageKeyLabel] = imageKey
	}
	buildArgs := map[string]string{}
	extraOptions := []string(nil)
	target := ""
	if resolved.Config.Build != nil && resolved.Config.Build.Target != "" {
		target = resolved.Config.Build.Target
	}
	if resolved.Config.Build != nil {
		for key, value := range resolved.Config.Build.Args {
			buildArgs[key] = value
		}
		extraOptions = append(extraOptions, resolved.Config.Build.Options...)
	}
	stdout, stderr := r.progressWriters(events, phaseImage, "Building container image", r.stdout, r.stderr)
	return r.backend.BuildImage(ctx, dockercli.BuildImageRequest{ContextDir: contextDir, Dockerfile: dockerfile, Tag: imageName, Labels: labels, BuildArgs: buildArgs, Target: target, ExtraOptions: extraOptions, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}})
}

func (r *Runner) ensureImageWithFeatures(ctx context.Context, resolved devcontainer.ResolvedConfig, imageKey string, events ui.Sink) (string, error) {
	baseImage := resolved.Config.Image
	if baseImage == "" {
		baseImage = resolved.ImageName + "-base"
		if err := r.buildDockerfileImage(ctx, resolved, baseImage, "", events); err != nil {
			return "", err
		}
	} else if err := r.verifyImageReference(ctx, baseImage, events); err != nil {
		return "", err
	}
	return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage, imageKey, events)
}

func (r *Runner) ensureFeaturesImageFromBase(ctx context.Context, resolved devcontainer.ResolvedConfig, baseImage string, imageKey string, events ui.Sink) (string, error) {
	imageUser, err := r.inspectImageUser(ctx, baseImage)
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
	managedMetadata := resolved.Merged.Metadata
	if metadata, err := r.imageMetadata(ctx, baseImage); err == nil {
		if isManagedImage(&resolved, baseImage) {
			managedMetadata = metadata
		} else {
			managedMetadata = mergeManagedImageMetadata(metadata, managedMetadata)
		}
	} else if !docker.IsNotFound(err) {
		return "", err
	}
	if err := writeFeatureBuildContext(buildDir, baseImage, resolved.Features, containerUser, remoteUser, managedMetadata, imageKey); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); err != nil {
		return "", fmt.Errorf("generated feature Dockerfile missing in %s: %w", buildDir, err)
	}
	stdout, stderr := r.progressWriters(events, phaseImage, "Building features image", r.stdout, r.stderr)
	if err := r.backend.BuildImage(ctx, dockercli.BuildImageRequest{ContextDir: buildDir, Dockerfile: filepath.Join(buildDir, "Dockerfile"), Tag: resolved.ImageName, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
		entries, _ := os.ReadDir(buildDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return "", fmt.Errorf("build features image from %s with files %v: %w", buildDir, names, err)
	}
	return resolved.ImageName, nil
}

func (r *Runner) ensureComposeImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageKey string, events ui.Sink) (string, error) {
	config, err := r.readComposeConfig(ctx, &resolved)
	if err != nil {
		return "", err
	}
	service, ok := config.Services[resolved.ComposeService]
	if !ok {
		return "", fmt.Errorf("compose service %q not found", resolved.ComposeService)
	}
	baseImage := service.Image
	if service.Build.Enabled() {
		stdout, stderr := r.progressWriters(events, phaseImage, fmt.Sprintf("Building compose service %s", resolved.ComposeService), r.stdout, r.stderr)
		if err := r.backend.ComposeBuild(ctx, dockercli.ComposeBuildRequest{Target: dockercli.ComposeTarget{Files: resolved.ComposeFiles, Project: resolved.ComposeProject, Dir: resolved.ConfigDir}, Services: []string{resolved.ComposeService}, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", err
		}
		if baseImage == "" {
			baseImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
	}
	if baseImage != "" && !service.Build.Enabled() {
		if err := r.verifyImageReference(ctx, baseImage, events); err != nil {
			return "", err
		}
	}
	if len(resolved.Features) > 0 {
		if baseImage == "" {
			return "", fmt.Errorf("compose service %q needs an image or build result for features", resolved.ComposeService)
		}
		return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage, imageKey, events)
	}
	if baseImage != "" {
		return baseImage, nil
	}
	return resolved.ComposeProject + "-" + resolved.ComposeService, nil
}

func (r *Runner) ensureUpdatedUIDContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerID string, events ui.Sink) error {
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
	inspect, err := r.backend.InspectImage(ctx, image)
	if err != nil {
		return err
	}
	remoteUser, ok := capuid.Eligible(resolved, inspect)
	if !ok {
		return nil
	}
	args := capuid.ExecArgs(containerID, remoteUser, uid, gid)
	return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Phase: phaseContainer, Label: "Reconciling container user", Args: args, Stdin: strings.NewReader(capuid.UpdateScript), Stdout: r.stdout, Stderr: r.stderr, Events: events})
}
