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

func (r *Runner) inspectImageArchitecture(ctx context.Context, image string) (string, error) {
	inspect, err := r.docker.InspectImage(ctx, image)
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

func isManagedImage(resolved *devcontainer.ResolvedConfig, image string) bool {
	return image == resolved.ImageName || strings.HasPrefix(image, resolved.ImageName+"-")
}

func writeFeatureBuildContext(buildDir string, baseImage string, features []devcontainer.ResolvedFeature, containerUser string, remoteUser string, metadata []devcontainer.MetadataEntry) error {
	metadataLabel, err := devcontainer.MetadataLabelValue(metadata)
	if err != nil {
		return err
	}
	builtinEnv := map[string]string{
		"_CONTAINER_USER": containerUser,
		"_REMOTE_USER":    remoteUser,
	}
	if err := os.WriteFile(filepath.Join(buildDir, "devcontainer-features.builtin.env"), []byte(shellEnvScript(builtinEnv)), 0o600); err != nil {
		return err
	}
	var dockerfile strings.Builder
	dockerfile.WriteString("FROM " + baseImage + "\nUSER root\n")
	dockerfile.WriteString("RUN mkdir -p /tmp/dev-container-features\n")
	dockerfile.WriteString("COPY devcontainer-features.builtin.env /tmp/dev-container-features/devcontainer-features.builtin.env\n")
	for i, feature := range features {
		rel := fmt.Sprintf("feature-%02d", i)
		dst := filepath.Join(buildDir, rel)
		if err := copyDir(feature.Path, dst); err != nil {
			return err
		}
		if len(feature.Options) > 0 {
			if err := os.WriteFile(filepath.Join(dst, "devcontainer-features.env"), []byte(shellEnvScript(feature.Options)), 0o600); err != nil {
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
	return os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile.String()), 0o600)
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
