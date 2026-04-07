package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/security"
)

func (r *Runner) enrichMergedConfig(ctx context.Context, resolved *devcontainer.ResolvedConfig, image string) error {
	inspect, err := r.docker.InspectImage(ctx, image)
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

func (r *Runner) ensureImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeImage(ctx, resolved)
	}
	if len(resolved.Features) > 0 {
		return r.ensureImageWithFeatures(ctx, resolved)
	}
	if resolved.Config.Image != "" {
		if err := security.VerifyImage(ctx, resolved.Config.Image); err != nil {
			return "", err
		}
		return resolved.Config.Image, nil
	}
	return resolved.ImageName, r.buildDockerfileImage(ctx, resolved, resolved.ImageName)
}

func (r *Runner) buildDockerfileImage(ctx context.Context, resolved devcontainer.ResolvedConfig, imageName string) error {
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
	return r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: r.stdout, Stderr: r.stderr})
}

func (r *Runner) ensureImageWithFeatures(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	baseImage := resolved.Config.Image
	if baseImage == "" {
		baseImage = resolved.ImageName + "-base"
		if err := r.buildDockerfileImage(ctx, resolved, baseImage); err != nil {
			return "", err
		}
	} else if err := security.VerifyImage(ctx, baseImage); err != nil {
		return "", err
	}
	return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage)
}

func (r *Runner) ensureFeaturesImageFromBase(ctx context.Context, resolved devcontainer.ResolvedConfig, baseImage string) (string, error) {
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
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return "", err
	}
	if err := writeFeatureBuildContext(buildDir, resolved.Features, containerUser, remoteUser, resolved.Merged.Metadata); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); err != nil {
		return "", fmt.Errorf("generated feature Dockerfile missing in %s: %w", buildDir, err)
	}
	args := []string{
		"build",
		"-f", filepath.Join(buildDir, "Dockerfile"),
		"-t", resolved.ImageName,
		"--build-arg", "BASE_IMAGE=" + baseImage,
		buildDir,
	}
	if err := r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: r.stdout, Stderr: r.stderr}); err != nil {
		entries, _ := os.ReadDir(buildDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return "", fmt.Errorf("build features image from %s with files %v: %w", buildDir, names, err)
	}
	return resolved.ImageName, nil
}

func (r *Runner) ensureComposeImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	config, err := r.readComposeConfig(ctx, resolved)
	if err != nil {
		return "", err
	}
	service, ok := config.Services[resolved.ComposeService]
	if !ok {
		return "", fmt.Errorf("compose service %q not found", resolved.ComposeService)
	}
	baseImage := service.Image
	if service.Build.Enabled() {
		if err := r.docker.Run(ctx, docker.RunOptions{Args: append(r.composeBaseArgs(resolved), "build", resolved.ComposeService), Dir: resolved.ConfigDir, Stdout: r.stdout, Stderr: r.stderr}); err != nil {
			return "", err
		}
		if baseImage == "" {
			baseImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
	}
	if baseImage != "" && !service.Build.Enabled() {
		if err := security.VerifyImage(ctx, baseImage); err != nil {
			return "", err
		}
	}
	if len(resolved.Features) > 0 {
		if baseImage == "" {
			return "", fmt.Errorf("compose service %q needs an image or build result for features", resolved.ComposeService)
		}
		return r.ensureFeaturesImageFromBase(ctx, resolved, baseImage)
	}
	if baseImage != "" {
		return baseImage, nil
	}
	return resolved.ComposeProject + "-" + resolved.ComposeService, nil
}

func (r *Runner) ensureUpdatedUIDImage(ctx context.Context, resolved devcontainer.ResolvedConfig, image string) (string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return image, nil
	}
	if resolved.Merged.UpdateRemoteUserUID != nil && !*resolved.Merged.UpdateRemoteUserUID {
		return image, nil
	}
	uid := os.Getuid()
	gid := os.Getgid()
	if uid <= 0 || gid <= 0 {
		return image, nil
	}
	inspect, err := r.docker.InspectImage(ctx, image)
	if err != nil {
		return image, err
	}
	imageUser := inspect.Config.User
	if imageUser == "" {
		imageUser = "root"
	}
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser, imageUser)
	if remoteUser == "" || remoteUser == "root" || isNumericUser(remoteUser) {
		return image, nil
	}
	derivedImage := resolved.ImageName + "-uid"
	dockerfilePath := filepath.Join(resolved.StateDir, "updateUID.Dockerfile")
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return image, err
	}
	if err := os.WriteFile(dockerfilePath, []byte(updateUIDDockerfile), 0o644); err != nil {
		return image, err
	}
	metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata)
	if err != nil {
		return image, err
	}
	args := []string{
		"build",
		"-f", dockerfilePath,
		"-t", derivedImage,
		"--build-arg", "BASE_IMAGE=" + image,
		"--build-arg", "REMOTE_USER=" + remoteUser,
		"--build-arg", fmt.Sprintf("NEW_UID=%d", uid),
		"--build-arg", fmt.Sprintf("NEW_GID=%d", gid),
		"--build-arg", "IMAGE_USER=" + imageUser,
	}
	if metadataLabel != "" {
		args = append(args, "--label", devcontainer.ImageMetadataLabel+"="+metadataLabel)
	}
	args = append(args, resolved.StateDir)
	if err := r.docker.Run(ctx, docker.RunOptions{Args: args, Stdout: r.stdout, Stderr: r.stderr}); err != nil {
		return image, err
	}
	return derivedImage, nil
}

func (r *Runner) inspectImageUser(ctx context.Context, image string) (string, error) {
	inspect, err := r.docker.InspectImage(ctx, image)
	if err != nil {
		if docker.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return inspect.Config.User, nil
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

func isManagedImage(resolved *devcontainer.ResolvedConfig, image string) bool {
	return image == resolved.ImageName || strings.HasPrefix(image, resolved.ImageName+"-")
}

func writeFeatureBuildContext(buildDir string, features []devcontainer.ResolvedFeature, containerUser string, remoteUser string, metadata []devcontainer.MetadataEntry) error {
	metadataLabel, err := devcontainer.MetadataLabelValue(metadata)
	if err != nil {
		return err
	}
	builtinEnv := map[string]string{
		"_CONTAINER_USER": containerUser,
		"_REMOTE_USER":    remoteUser,
	}
	if err := os.WriteFile(filepath.Join(buildDir, "devcontainer-features.builtin.env"), []byte(shellEnvScript(builtinEnv)), 0o644); err != nil {
		return err
	}
	var dockerfile strings.Builder
	dockerfile.WriteString("ARG BASE_IMAGE\nFROM ${BASE_IMAGE}\nUSER root\n")
	dockerfile.WriteString("RUN mkdir -p /tmp/dev-container-features\n")
	dockerfile.WriteString("COPY devcontainer-features.builtin.env /tmp/dev-container-features/devcontainer-features.builtin.env\n")
	for i, feature := range features {
		rel := fmt.Sprintf("feature-%02d", i)
		dst := filepath.Join(buildDir, rel)
		if err := copyDir(feature.Path, dst); err != nil {
			return err
		}
		if len(feature.Options) > 0 {
			if err := os.WriteFile(filepath.Join(dst, "devcontainer-features.env"), []byte(shellEnvScript(feature.Options)), 0o644); err != nil {
				return err
			}
		}
		dockerfile.WriteString("COPY " + rel + " /tmp/hatchctl-features/" + rel + "\n")
		if len(feature.Metadata.ContainerEnv) > 0 {
			for _, key := range devcontainer.SortedMapKeys(feature.Metadata.ContainerEnv) {
				dockerfile.WriteString("ENV " + key + "=" + dockerfileQuotedValue(feature.Metadata.ContainerEnv[key]) + "\n")
			}
		}
		dockerfile.WriteString("RUN if [ -f /tmp/hatchctl-features/" + rel + "/install.sh ]; then cd /tmp/hatchctl-features/" + rel + " && chmod +x ./install.sh && . /tmp/dev-container-features/devcontainer-features.builtin.env && if [ -f ./devcontainer-features.env ]; then . ./devcontainer-features.env; fi && ./install.sh; fi\n")
	}
	if metadataLabel != "" {
		dockerfile.WriteString("LABEL " + devcontainer.ImageMetadataLabel + "=" + dockerfileQuotedValue(metadataLabel) + "\n")
	}
	return os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile.String()), 0o644)
}

func shellEnvScript(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	var lines []string
	for _, key := range sortedFeatureOptionKeys(values) {
		lines = append(lines, "export "+key+"="+devcontainer.ShellQuote(values[key]))
	}
	return strings.Join(lines, "\n") + "\n"
}

func sortedFeatureOptionKeys(values map[string]string) []string {
	return devcontainer.SortedMapKeys(values)
}

func dockerfileQuotedValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "")
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
