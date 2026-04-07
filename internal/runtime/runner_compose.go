package runtime

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Image string        `json:"image"`
	Build *composeBuild `json:"build"`
}

type composeBuild struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile"`
	Target     string            `json:"target"`
	Args       map[string]string `json:"args"`
}

func (b *composeBuild) Enabled() bool {
	return b != nil && b.Context != ""
}

func (r *Runner) composeBaseArgs(resolved devcontainer.ResolvedConfig) []string {
	args := []string{"compose"}
	for _, file := range resolved.ComposeFiles {
		args = append(args, "-f", file)
	}
	if resolved.ComposeProject != "" {
		args = append(args, "-p", resolved.ComposeProject)
	}
	return args
}

func (r *Runner) composeArgs(resolved devcontainer.ResolvedConfig, overridePath string) []string {
	args := r.composeBaseArgs(resolved)
	if overridePath != "" {
		args = append(args, "-f", overridePath)
	}
	return args
}

func (r *Runner) readComposeConfig(ctx context.Context, resolved devcontainer.ResolvedConfig) (composeConfig, error) {
	args := append(r.composeBaseArgs(resolved), "config", "--format", "json")
	output, err := r.docker.OutputOptions(ctx, docker.RunOptions{Args: args, Dir: resolved.ConfigDir})
	if err != nil {
		return composeConfig{}, err
	}
	var config composeConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return composeConfig{}, err
	}
	if config.Name != "" {
		resolved.ComposeProject = config.Name
	}
	return config, nil
}

func (r *Runner) findComposeContainer(ctx context.Context, resolved devcontainer.ResolvedConfig) (string, error) {
	project := resolved.ComposeProject
	if project == "" {
		config, err := r.readComposeConfig(ctx, resolved)
		if err != nil {
			return "", err
		}
		project = firstNonEmpty(config.Name, resolved.ComposeProject)
	}
	args := []string{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + project, "--filter", "label=com.docker.compose.service=" + resolved.ComposeService}
	result, err := r.docker.Output(ctx, args...)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errManagedContainerNotFound
}

func writeComposeOverride(resolved devcontainer.ResolvedConfig, image string) (string, error) {
	if err := os.MkdirAll(resolved.StateDir, 0o755); err != nil {
		return "", err
	}
	path := devcontainer.ComposeOverrideFile(resolved.StateDir)
	if err := os.WriteFile(path, []byte(renderComposeOverride(resolved, image)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func renderComposeOverride(resolved devcontainer.ResolvedConfig, image string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  ")
	b.WriteString(resolved.ComposeService)
	b.WriteString(":\n")
	if len(resolved.Features) > 0 {
		b.WriteString("    pull_policy: never\n")
	}
	b.WriteString("    labels:\n")
	labels := map[string]string{}
	for key, value := range resolved.Labels {
		labels[key] = value
	}
	if resolved.Merged.ContainerEnv["DEVCONTAINER_BRIDGE_ENABLED"] == "true" {
		labels[devcontainer.BridgeEnabledLabel] = "true"
	}
	if metadataLabel, err := devcontainer.MetadataLabelValue(resolved.Merged.Metadata); err == nil && metadataLabel != "" {
		labels[devcontainer.ImageMetadataLabel] = metadataLabel
	}
	for _, key := range sortedStringKeys(labels) {
		b.WriteString("      - ")
		b.WriteString(yamlQuoted(key + "=" + labels[key]))
		b.WriteString("\n")
	}
	if len(resolved.Merged.ContainerEnv) > 0 {
		b.WriteString("    environment:\n")
		for _, key := range devcontainer.SortedMapKeys(resolved.Merged.ContainerEnv) {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(key + "=" + resolved.Merged.ContainerEnv[key]))
			b.WriteString("\n")
		}
	}
	b.WriteString("    volumes:\n")
	allMounts := append([]string{resolved.WorkspaceMount}, resolved.Merged.Mounts...)
	namedVolumes := map[string]struct{}{}
	for _, mount := range allMounts {
		if value, ok := composeMountValue(mount); ok {
			b.WriteString("      - ")
			b.WriteString(value)
			b.WriteString("\n")
		}
		if source, ok := composeNamedVolume(mount); ok {
			namedVolumes[source] = struct{}{}
		}
	}
	if resolved.Merged.Init {
		b.WriteString("    init: true\n")
	}
	if resolved.Merged.Privileged {
		b.WriteString("    privileged: true\n")
	}
	if user := resolved.Merged.ContainerUser; user != "" {
		b.WriteString("    user: ")
		b.WriteString(yamlQuoted(user))
		b.WriteString("\n")
	}
	if overrideCommandEnabled(resolved.Config.OverrideCommand) {
		b.WriteString("    command: [\"/bin/sh\", \"-lc\", \"trap 'exit 0' TERM INT; while sleep 1000; do :; done\"]\n")
	}
	if len(resolved.Merged.CapAdd) > 0 {
		b.WriteString("    cap_add:\n")
		for _, value := range resolved.Merged.CapAdd {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(value))
			b.WriteString("\n")
		}
	}
	if len(resolved.Merged.SecurityOpt) > 0 {
		b.WriteString("    security_opt:\n")
		for _, value := range resolved.Merged.SecurityOpt {
			b.WriteString("      - ")
			b.WriteString(yamlQuoted(value))
			b.WriteString("\n")
		}
	}
	if image != "" {
		b.WriteString("    image: ")
		b.WriteString(yamlQuoted(image))
		b.WriteString("\n")
	}
	if len(namedVolumes) > 0 {
		b.WriteString("volumes:\n")
		for _, name := range sortedVolumeNames(namedVolumes) {
			b.WriteString("  ")
			b.WriteString(name)
			b.WriteString(":\n")
		}
	}
	return b.String()
}

func overrideCommandEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func composeMountValue(raw string) (string, bool) {
	parts := map[string]string{}
	for _, segment := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if !ok {
			continue
		}
		parts[key] = value
	}
	target := parts["target"]
	if target == "" {
		return "", false
	}
	switch parts["type"] {
	case "bind", "volume":
		source := parts["source"]
		if source == "" {
			return "", false
		}
		return yamlQuoted(source + ":" + target), true
	default:
		return "", false
	}
}

func composeNamedVolume(raw string) (string, bool) {
	parts := map[string]string{}
	for _, segment := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if !ok {
			continue
		}
		parts[key] = value
	}
	if parts["type"] != "volume" || parts["source"] == "" {
		return "", false
	}
	return parts["source"], true
}

func yamlQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sortedStringKeys(values map[string]string) []string {
	keys := slices.Collect(mapsKeys(values))
	sort.Strings(keys)
	return keys
}

func sortedVolumeNames(values map[string]struct{}) []string {
	keys := slices.Collect(mapsKeys(values))
	sort.Strings(keys)
	return keys
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mapsKeys[K comparable, V any](values map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for key := range values {
			if !yield(key) {
				return
			}
		}
	}
}
