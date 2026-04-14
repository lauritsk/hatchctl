package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	capuid "github.com/lauritsk/hatchctl/internal/capability/uidremap"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
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

func (e *Executor) planDesiredImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (DesiredImage, error) {
	if resolved.SourceKind == "compose" {
		config, err := e.readComposeConfig(ctx, &resolved)
		if err != nil {
			return DesiredImage{}, err
		}
		service, ok := config.Services[resolved.ComposeService]
		if !ok {
			return DesiredImage{}, nil
		}
		if len(resolved.Features) > 0 {
			key, err := ManagedImageKey(resolved, resolved.ImageName)
			if err != nil {
				return DesiredImage{}, err
			}
			return DesiredImage{TargetImage: resolved.ImageName, BuildMode: ImageBuildModeFeatures, ReuseKey: key}, nil
		}
		targetImage := service.Image
		if targetImage == "" {
			targetImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
		if service.Build.Enabled() {
			return DesiredImage{TargetImage: targetImage, BuildMode: ImageBuildModeProject}, nil
		}
		return DesiredImage{TargetImage: targetImage, BuildMode: ImageBuildModeNone, Verify: targetImage != ""}, nil
	}
	if len(resolved.Features) > 0 {
		key, err := ManagedImageKey(resolved, resolved.ImageName)
		if err != nil {
			return DesiredImage{}, err
		}
		return DesiredImage{TargetImage: resolved.ImageName, BuildMode: ImageBuildModeFeatures, ReuseKey: key}, nil
	}
	if resolved.Config.Image != "" {
		return DesiredImage{TargetImage: resolved.Config.Image, BuildMode: ImageBuildModeNone, Verify: true}, nil
	}
	key, err := ManagedImageKey(resolved, resolved.ImageName)
	if err != nil {
		return DesiredImage{}, err
	}
	return DesiredImage{TargetImage: resolved.ImageName, BuildMode: ImageBuildModeBuild, ReuseKey: key}, nil
}

func (e *Executor) ReconcileImage(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, ImagePlan, error) {
	desired, err := e.planDesiredImage(ctx, resolved)
	if err != nil {
		return "", ImagePlan{}, err
	}
	observed, err := NewObserver(e.observerBackend()).Observe(ctx, ObserveRequest{
		Plan:         workspacePlan,
		Resolved:     resolved,
		ImageRef:     desired.TargetImage,
		ObserveImage: desired.TargetImage != "",
	})
	if err != nil {
		return "", ImagePlan{}, err
	}
	plan := PlanImage(desired, observed.Image)
	switch plan.Action {
	case ImageActionUseTarget:
		if plan.Verify && plan.TargetImage != "" {
			if err := e.verifyImageReference(ctx, plan.TargetImage, events); err != nil {
				return "", ImagePlan{}, err
			}
		}
		if plan.TargetImage != "" {
			if err := e.ensureLocalImage(ctx, plan.TargetImage, events); err != nil {
				return "", ImagePlan{}, err
			}
		}
		return plan.TargetImage, plan, nil
	case ImageActionReuseTarget:
		return plan.TargetImage, plan, nil
	case ImageActionBuildTarget:
		image, err := e.buildManagedImage(ctx, resolved, plan, events)
		if err != nil {
			return "", ImagePlan{}, err
		}
		return image, plan, nil
	default:
		return plan.TargetImage, plan, nil
	}
}

func (e *Executor) EnrichMergedConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig, image string) error {
	if resolved == nil {
		return nil
	}
	metadata, err := e.runtimeMetadataFromImage(ctx, *resolved, image)
	if err != nil {
		return err
	}
	resolved.Merged = mergedConfigWithRuntimeMetadata(*resolved, image, metadata)
	return nil
}

func (e *Executor) InspectImageUser(ctx context.Context, image string) (string, error) {
	inspect, err := e.engine.InspectImage(ctx, image)
	if err != nil {
		if backend.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return inspect.Config.User, nil
}

func (e *Executor) InspectImageArchitecture(ctx context.Context, image string) (string, error) {
	inspect, err := e.engine.InspectImage(ctx, image)
	if err != nil {
		if backend.IsNotFound(err) {
			return stdruntime.GOARCH, nil
		}
		return "", err
	}
	if inspect.Architecture != "" {
		return inspect.Architecture, nil
	}
	return stdruntime.GOARCH, nil
}

func mergeManagedImageMetadata(base []spec.MetadataEntry, overlay []spec.MetadataEntry) []spec.MetadataEntry {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make([]spec.MetadataEntry, 0, len(base)+len(overlay))
	merged = append(merged, base...)
	merged = append(merged, overlay...)
	return merged
}

func (e *Executor) imageMetadata(ctx context.Context, image string) ([]spec.MetadataEntry, error) {
	inspect, err := e.engine.InspectImage(ctx, image)
	if err != nil {
		return nil, err
	}
	return spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
}

func (e *Executor) mergeSourceImageMetadata(ctx context.Context, resolved devcontainer.ResolvedConfig, runtimeImage string, metadata []spec.MetadataEntry) ([]spec.MetadataEntry, error) {
	if resolved.Config.Image == "" || resolved.Config.Image == runtimeImage {
		return metadata, nil
	}
	sourceMetadata, err := e.imageMetadata(ctx, resolved.Config.Image)
	if err != nil {
		if backend.IsNotFound(err) {
			return metadata, nil
		}
		return nil, err
	}
	return mergeManagedImageMetadata(sourceMetadata, metadata), nil
}

func isManagedImage(resolved *devcontainer.ResolvedConfig, image string) bool {
	return image == resolved.ImageName || strings.HasPrefix(image, resolved.ImageName+"-")
}

func writeFeatureBuildContext(buildDir string, definitionFileName string, baseImage string, features []devcontainer.ResolvedFeature, containerUser string, remoteUser string, metadata []spec.MetadataEntry, imageKey string) error {
	metadataLabel, err := spec.MetadataLabelValue(metadata)
	if err != nil {
		return err
	}
	builtinEnv := map[string]string{
		"_CONTAINER_USER": containerUser,
		"_REMOTE_USER":    remoteUser,
	}
	if err := storefs.WriteFeatureBuildFile(filepath.Join(buildDir, "devcontainer-features.builtin.env"), []byte(shellEnvFile(builtinEnv)), 0o600); err != nil {
		return err
	}
	if err := storefs.WriteFeatureBuildFile(filepath.Join(buildDir, "devcontainer-features-install.sh"), []byte(featureInstallHelper), 0o755); err != nil {
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
		if err := storefs.CopyFeatureSource(feature.Path, dst); err != nil {
			return err
		}
		if len(feature.Options) > 0 {
			if err := storefs.WriteFeatureBuildFile(filepath.Join(dst, "devcontainer-features.env"), []byte(shellEnvFile(feature.Options)), 0o600); err != nil {
				return err
			}
		}
		dockerfile.WriteString("COPY " + rel + " /tmp/hatchctl-features/" + rel + "\n")
		if len(feature.Metadata.ContainerEnv) > 0 {
			for _, key := range spec.SortedMapKeys(feature.Metadata.ContainerEnv) {
				dockerfile.WriteString("ENV " + key + "=" + dockerfileQuotedValue(feature.Metadata.ContainerEnv[key]) + "\n")
			}
		}
		dockerfile.WriteString("RUN if [ -f /tmp/hatchctl-features/" + rel + "/install.sh ]; then /tmp/dev-container-features/devcontainer-features-install.sh /tmp/hatchctl-features/" + rel + " /tmp/dev-container-features/devcontainer-features.builtin.env /tmp/hatchctl-features/" + rel + "/devcontainer-features.env; fi\n")
	}
	if metadataLabel != "" {
		dockerfile.WriteString("LABEL " + devcontainer.ImageMetadataLabel + "=" + dockerfileQuotedValue(metadataLabel) + "\n")
	}
	if imageKey != "" {
		dockerfile.WriteString("LABEL " + ImageKeyLabel + "=" + dockerfileQuotedValue(imageKey) + "\n")
	}
	return storefs.WriteFeatureBuildFile(filepath.Join(buildDir, definitionFileName), []byte(dockerfile.String()), 0o600)
}

func shellEnvFile(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	var lines []string
	for _, key := range spec.SortedMapKeys(values) {
		lines = append(lines, key+"="+spec.ShellQuote(values[key]))
	}
	return strings.Join(lines, "\n") + "\n"
}

func dockerfileQuotedValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "\n", "\\n", "\r", "")
	return "\"" + replacer.Replace(value) + "\""
}

func (e *Executor) buildDockerfileImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageName string, imageKey string, events ui.Sink) error {
	dockerfile := resolved.ConfigDir
	contextDir := resolved.ConfigDir
	if rel := spec.ResolvedDockerfile(resolved.ConfigDir, resolved.Config); rel != "" {
		dockerfile = filepath.Join(resolved.ConfigDir, rel)
	}
	if rel := spec.EffectiveContext(resolved.Config); rel != "" {
		contextDir = filepath.Join(resolved.ConfigDir, rel)
	}
	labels := map[string]string{}
	metadataLabel, err := spec.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return err
	}
	if metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	if imageKey != "" {
		labels[ImageKeyLabel] = imageKey
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
	stdout, stderr := e.progressWriters(events, phaseImage, "Building container image", e.stdout, e.stderr)
	return e.engine.BuildImage(ctx, backend.BuildImageRequest{ContextDir: contextDir, DefinitionFile: dockerfile, Tag: imageName, Labels: labels, BuildArgs: buildArgs, Target: target, ExtraOptions: extraOptions, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}})
}

func (e *Executor) ensureImageWithFeatures(ctx context.Context, resolved devcontainer.ResolvedConfig, imageKey string, events ui.Sink) (string, error) {
	baseImage := resolved.Config.Image
	if baseImage == "" {
		baseImage = resolved.ImageName + "-base"
		if err := e.buildDockerfileImage(ctx, resolved, baseImage, "", events); err != nil {
			return "", err
		}
	} else if err := e.verifyImageReference(ctx, baseImage, events); err != nil {
		return "", err
	} else if err := e.ensureLocalImage(ctx, baseImage, events); err != nil {
		return "", err
	}
	return e.ensureFeaturesImageFromBase(ctx, resolved, baseImage, imageKey, events)
}

func (e *Executor) ensureLocalImage(ctx context.Context, image string, events ui.Sink) error {
	_, err := e.engine.InspectImage(ctx, image)
	if err == nil {
		return nil
	}
	if !backend.IsNotFound(err) {
		return err
	}
	stdout, stderr := e.progressWriters(events, phaseImage, "Pulling source image", e.stdout, e.stderr)
	return e.engine.PullImage(ctx, backend.PullImageRequest{Reference: image, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}})
}

func (e *Executor) ensureFeaturesImageFromBase(ctx context.Context, resolved devcontainer.ResolvedConfig, baseImage string, imageKey string, events ui.Sink) (string, error) {
	imageUser, err := e.InspectImageUser(ctx, baseImage)
	if err != nil {
		return "", err
	}
	containerUser := firstNonEmptyString(resolved.Merged.ContainerUser, imageUser, "root")
	remoteUser := firstNonEmptyString(resolved.Merged.RemoteUser, containerUser)
	buildDir, err := storefs.ResetFeatureBuildDir(resolved.StateDir)
	if err != nil {
		return "", err
	}
	managedMetadata := resolved.Merged.Metadata
	if metadata, err := e.imageMetadata(ctx, baseImage); err == nil {
		if isManagedImage(&resolved, baseImage) {
			managedMetadata = metadata
		} else {
			managedMetadata = mergeManagedImageMetadata(metadata, managedMetadata)
		}
	} else if !backend.IsNotFound(err) {
		return "", err
	}
	if err := writeFeatureBuildContext(buildDir, e.engine.BuildDefinitionFileName(), baseImage, resolved.Features, containerUser, remoteUser, managedMetadata, imageKey); err != nil {
		return "", err
	}
	definitionFile := filepath.Join(buildDir, e.engine.BuildDefinitionFileName())
	if _, err := os.Stat(definitionFile); err != nil {
		return "", fmt.Errorf("generated feature build definition missing in %s: %w", buildDir, err)
	}
	stdout, stderr := e.progressWriters(events, phaseImage, "Building features image", e.stdout, e.stderr)
	if err := e.engine.BuildImage(ctx, backend.BuildImageRequest{ContextDir: buildDir, DefinitionFile: definitionFile, Tag: resolved.ImageName, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
		entries, _ := os.ReadDir(buildDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return "", fmt.Errorf("build features image from %s with files %v: %w", buildDir, names, err)
	}
	return resolved.ImageName, nil
}

func (e *Executor) ensureComposeImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageKey string, events ui.Sink) (string, error) {
	config, err := e.readComposeConfig(ctx, &resolved)
	if err != nil {
		return "", err
	}
	service, ok := config.Services[resolved.ComposeService]
	if !ok {
		return "", fmt.Errorf("compose service %q not found", resolved.ComposeService)
	}
	baseImage := service.Image
	if service.Build.Enabled() {
		stdout, stderr := e.progressWriters(events, phaseImage, fmt.Sprintf("Building compose service %s", resolved.ComposeService), e.stdout, e.stderr)
		if err := e.engine.BuildProject(ctx, backend.ProjectBuildRequest{Target: composeTarget(resolved), Services: []string{resolved.ComposeService}, Streams: backend.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", err
		}
		if baseImage == "" {
			baseImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
	}
	if baseImage != "" && !service.Build.Enabled() {
		if err := e.verifyImageReference(ctx, baseImage, events); err != nil {
			return "", err
		}
	}
	if len(resolved.Features) > 0 {
		if baseImage == "" {
			return "", fmt.Errorf("compose service %q needs an image or build result for features", resolved.ComposeService)
		}
		return e.ensureFeaturesImageFromBase(ctx, resolved, baseImage, imageKey, events)
	}
	if baseImage != "" {
		return baseImage, nil
	}
	return resolved.ComposeProject + "-" + resolved.ComposeService, nil
}

func (e *Executor) buildManagedImage(ctx context.Context, resolved devcontainer.ResolvedConfig, plan ImagePlan, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return e.ensureComposeImage(ctx, resolved, plan.ReuseKey, events)
	}
	if plan.BuildMode == ImageBuildModeFeatures {
		return e.ensureImageWithFeatures(ctx, resolved, plan.ReuseKey, events)
	}
	return resolved.ImageName, e.buildDockerfileImage(ctx, resolved, resolved.ImageName, plan.ReuseKey, events)
}

func (e *Executor) desiredContainerKey(resolved devcontainer.ResolvedConfig, imagePlan ImagePlan, image string, bridgeEnabled bool, sshAgent bool) (string, error) {
	imageIdentity := image
	if imagePlan.ReuseKey != "" {
		imageIdentity = imagePlan.ReuseKey
	}
	return ContainerKey(resolved, imageIdentity, bridgeEnabled, sshAgent)
}

func (e *Executor) EnsureUpdatedUIDContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, image string, containerID string, events ui.Sink) error {
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
	inspect, err := e.engine.InspectImage(ctx, image)
	if err != nil {
		return err
	}
	remoteUser, ok := capuid.Eligible(resolved, inspect)
	if !ok {
		return nil
	}
	stdout, stderr := e.progressWriters(events, phaseContainer, "Reconciling container user", e.stdout, e.stderr)
	return e.engine.Exec(ctx, backend.ExecRequest{ContainerID: containerID, User: "root", Interactive: true, Command: []string{"sh", "-s", "--", remoteUser, fmt.Sprintf("%d", uid), fmt.Sprintf("%d", gid)}, Streams: backend.Streams{Stdin: strings.NewReader(capuid.UpdateScript), Stdout: stdout, Stderr: stderr}})
}
