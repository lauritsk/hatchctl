package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lauritsk/hatchctl/internal/featurefetch"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
)

type ResolvedFeature struct {
	SourceKind    string
	Source        string
	Path          string
	Version       string
	Resolved      string
	Integrity     string
	Verification  security.VerificationResult
	Options       map[string]string
	DependsOn     []string
	InstallsAfter []string
	Metadata      spec.MetadataEntry
}

type FeatureResolveOptions struct {
	AllowNetwork   bool
	WriteLockFile  bool
	WriteStateFile bool
	StateDir       string
	HTTPTimeout    time.Duration
	LockfilePolicy FeatureLockfilePolicy
	VerifyImage    func(context.Context, string) security.VerificationResult
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
	OnCreateCommand      spec.LifecycleCommand    `json:"onCreateCommand,omitempty"`
	UpdateContentCommand spec.LifecycleCommand    `json:"updateContentCommand,omitempty"`
	PostCreateCommand    spec.LifecycleCommand    `json:"postCreateCommand,omitempty"`
	PostStartCommand     spec.LifecycleCommand    `json:"postStartCommand,omitempty"`
	PostAttachCommand    spec.LifecycleCommand    `json:"postAttachCommand,omitempty"`
	InstallsAfter        []string                 `json:"installsAfter,omitempty"`
	DependsOn            map[string]any           `json:"dependsOn,omitempty"`
	Options              map[string]featureOption `json:"options,omitempty"`
}

type featureOption struct {
	Default any `json:"default,omitempty"`
}

func validateFeatureLockfilePolicy(source string, lock FeatureLockEntry, policy FeatureLockfilePolicy) error {
	if policy != FeatureLockfilePolicyFrozen {
		return nil
	}
	if !featurefetch.IsRemoteFeatureSource(source) || lock.Integrity != "" {
		return nil
	}
	return fmt.Errorf("feature %q requires a lockfile integrity in frozen lockfile mode", source)
}

func loadFeatureManifest(featureDir string) (featureManifest, error) {
	path := filepath.Join(featureDir, "devcontainer-feature.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return featureManifest{}, err
	}
	standardized, err := standardizeJSONC(path, data)
	if err != nil {
		return featureManifest{}, err
	}
	var manifest featureManifest
	if err := json.Unmarshal(standardized, &manifest); err != nil {
		return featureManifest{}, err
	}
	return manifest, nil
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
					return nil, fmt.Errorf("feature %q dependsOn %q, but only configured features are supported", feature.Metadata.ID, dep)
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
