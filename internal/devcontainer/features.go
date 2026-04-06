package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

type ResolvedFeature struct {
	Source        string
	Path          string
	Options       map[string]string
	DependsOn     []string
	InstallsAfter []string
	Metadata      MetadataEntry
}

type featureManifest struct {
	ID                   string                   `json:"id"`
	ContainerEnv         map[string]string        `json:"containerEnv,omitempty"`
	Mounts               []string                 `json:"mounts,omitempty"`
	Init                 *bool                    `json:"init,omitempty"`
	Privileged           *bool                    `json:"privileged,omitempty"`
	CapAdd               []string                 `json:"capAdd,omitempty"`
	SecurityOpt          []string                 `json:"securityOpt,omitempty"`
	Customizations       map[string]any           `json:"customizations,omitempty"`
	OnCreateCommand      LifecycleCommand         `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand         `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand         `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand         `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand         `json:"postAttachCommand,omitempty"`
	InstallsAfter        []string                 `json:"installsAfter,omitempty"`
	DependsOn            map[string]any           `json:"dependsOn,omitempty"`
	Options              map[string]featureOption `json:"options,omitempty"`
}

type featureOption struct {
	Default any `json:"default,omitempty"`
}

func ResolveFeatures(configDir string, values map[string]any) ([]ResolvedFeature, error) {
	if len(values) == 0 {
		return nil, nil
	}
	features := make([]ResolvedFeature, 0, len(values))
	byAlias := map[string]int{}
	for source, raw := range values {
		options, enabled := featureValueOptions(raw)
		if !enabled {
			continue
		}
		resolvedPath, err := resolveFeaturePath(configDir, source)
		if err != nil {
			return nil, err
		}
		manifest, err := loadFeatureManifest(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("load feature %q: %w", source, err)
		}
		if manifest.ID == "" {
			return nil, fmt.Errorf("load feature %q: missing id in devcontainer-feature.json", source)
		}
		feature := ResolvedFeature{
			Source:        source,
			Path:          resolvedPath,
			Options:       materializeFeatureOptions(manifest, options),
			DependsOn:     sortedKeys(manifest.DependsOn),
			InstallsAfter: append([]string(nil), manifest.InstallsAfter...),
			Metadata: MetadataEntry{
				ID:                   manifest.ID,
				Init:                 manifest.Init,
				Privileged:           manifest.Privileged,
				CapAdd:               cloneSlice(manifest.CapAdd),
				SecurityOpt:          cloneSlice(manifest.SecurityOpt),
				Mounts:               cloneSlice(manifest.Mounts),
				ContainerEnv:         cloneMap(manifest.ContainerEnv),
				Customizations:       manifest.Customizations,
				OnCreateCommand:      manifest.OnCreateCommand,
				UpdateContentCommand: manifest.UpdateContentCommand,
				PostCreateCommand:    manifest.PostCreateCommand,
				PostStartCommand:     manifest.PostStartCommand,
				PostAttachCommand:    manifest.PostAttachCommand,
			},
		}
		features = append(features, feature)
		idx := len(features) - 1
		byAlias[source] = idx
		byAlias[manifest.ID] = idx
	}
	return orderFeatures(features, byAlias)
}

func resolveFeaturePath(configDir string, source string) (string, error) {
	path := source
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir, path)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("feature %q not found; only local file-path features are supported", source)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("feature %q must resolve to a directory", source)
	}
	return resolved, nil
}

func loadFeatureManifest(featureDir string) (featureManifest, error) {
	path := filepath.Join(featureDir, "devcontainer-feature.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return featureManifest{}, err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return featureManifest{}, fmt.Errorf("parse jsonc %s: %w", path, err)
	}
	var manifest featureManifest
	if err := json.Unmarshal(standardized, &manifest); err != nil {
		return featureManifest{}, err
	}
	return manifest, nil
}

func featureValueOptions(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, true
	case bool:
		return nil, value
	case string:
		return map[string]any{"version": value}, true
	case map[string]any:
		return value, true
	default:
		return nil, true
	}
}

func materializeFeatureOptions(manifest featureManifest, overrides map[string]any) map[string]string {
	result := map[string]string{}
	for key, option := range manifest.Options {
		if option.Default != nil {
			result[featureOptionEnvName(key)] = fmt.Sprint(option.Default)
		}
	}
	if len(manifest.Options) > 0 {
		if _, ok := manifest.Options["version"]; ok {
			if raw, ok := overrides["version"]; ok {
				result[featureOptionEnvName("version")] = fmt.Sprint(raw)
			}
		}
	}
	for key, value := range overrides {
		result[featureOptionEnvName(key)] = fmt.Sprint(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func featureOptionEnvName(key string) string {
	var b strings.Builder
	for i, r := range key {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			if i == 0 && r >= '0' && r <= '9' {
				b.WriteByte('_')
			}
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r - ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func sortedKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func orderFeatures(features []ResolvedFeature, byAlias map[string]int) ([]ResolvedFeature, error) {
	if len(features) <= 1 {
		return features, nil
	}
	incoming := make([]int, len(features))
	edges := make([][]int, len(features))
	for i, feature := range features {
		deps := append([]string(nil), feature.DependsOn...)
		deps = append(deps, feature.InstallsAfter...)
		seen := map[int]struct{}{}
		for _, dep := range deps {
			idx, ok := byAlias[dep]
			if !ok || idx == i {
				if contains(feature.DependsOn, dep) {
					return nil, fmt.Errorf("feature %q dependsOn %q, but only configured local features are supported", feature.Metadata.ID, dep)
				}
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			edges[idx] = append(edges[idx], i)
			incoming[i]++
		}
	}
	ready := make([]int, 0, len(features))
	for i := range features {
		if incoming[i] == 0 {
			ready = append(ready, i)
		}
	}
	result := make([]ResolvedFeature, 0, len(features))
	for len(ready) > 0 {
		sort.Slice(ready, func(i int, j int) bool {
			return features[ready[i]].Metadata.ID < features[ready[j]].Metadata.ID
		})
		current := ready[0]
		ready = ready[1:]
		result = append(result, features[current])
		for _, next := range edges[current] {
			incoming[next]--
			if incoming[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	if len(result) != len(features) {
		return nil, fmt.Errorf("feature dependency cycle detected")
	}
	return result, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
