package devcontainer

import (
	"context"
	"fmt"
	"slices"

	"github.com/lauritsk/hatchctl/internal/featurefetch"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
)

type resolvedFeatureSource struct {
	Path         string
	Kind         string
	Resolved     string
	Integrity    string
	Version      string
	Verification security.VerificationResult
}

func ResolveFeatures(ctx context.Context, configPath string, configDir string, cacheDir string, values map[string]any, opts FeatureResolveOptions) ([]ResolvedFeature, error) {
	return resolveFeatures(ctx, configPath, configDir, cacheDir, values, opts)
}

func resolveFeatures(ctx context.Context, configPath string, configDir string, cacheDir string, values map[string]any, opts FeatureResolveOptions) ([]ResolvedFeature, error) {
	if len(values) == 0 {
		return nil, nil
	}
	policy, err := ParseFeatureLockfilePolicy(string(opts.LockfilePolicy))
	if err != nil {
		return nil, err
	}
	lockFile, _, err := ReadFeatureLockFile(configPath)
	if err != nil {
		return nil, err
	}
	features := make([]ResolvedFeature, 0, len(values))
	byAlias := map[string]int{}
	for source, raw := range values {
		feature, err := resolveFeature(ctx, configDir, cacheDir, source, raw, lockFile[source], policy, opts)
		if err != nil {
			return nil, err
		}
		if feature == nil {
			continue
		}
		features = append(features, *feature)
		idx := len(features) - 1
		byAlias[source] = idx
		byAlias[feature.Metadata.ID] = idx
	}
	return orderFeatures(features, byAlias)
}

func resolveFeature(ctx context.Context, configDir string, cacheDir string, source string, raw any, lock FeatureLockEntry, policy FeatureLockfilePolicy, opts FeatureResolveOptions) (*ResolvedFeature, error) {
	options, enabled, err := resolveFeatureValueOptions(source, raw)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	if err := validateFeatureLockfilePolicy(source, lock, policy); err != nil {
		return nil, err
	}
	resolvedSource, err := resolveFeatureSource(ctx, configDir, cacheDir, source, lock, policy, opts)
	if err != nil {
		return nil, err
	}
	manifest, err := loadFeatureManifest(resolvedSource.Path)
	if err != nil {
		return nil, fmt.Errorf("load feature %q: %w", source, err)
	}
	if manifest.ID == "" {
		return nil, fmt.Errorf("load feature %q: missing id in devcontainer-feature.json", source)
	}
	materialized, err := materializeFeatureOptions(source, manifest, options)
	if err != nil {
		return nil, err
	}
	feature := &ResolvedFeature{
		SourceKind:    resolvedSource.Kind,
		Source:        source,
		Path:          resolvedSource.Path,
		Version:       resolvedSource.Version,
		Resolved:      resolvedSource.Resolved,
		Integrity:     resolvedSource.Integrity,
		Verification:  resolvedSource.Verification,
		Options:       materialized,
		DependsOn:     sortedKeys(manifest.DependsOn),
		InstallsAfter: slices.Clone(manifest.InstallsAfter),
		Metadata: spec.MetadataEntry{
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
	return feature, nil
}

func resolveFeatureSource(ctx context.Context, configDir string, cacheDir string, source string, lock FeatureLockEntry, policy FeatureLockfilePolicy, opts FeatureResolveOptions) (resolvedFeatureSource, error) {
	resolved, err := featurefetch.ResolveSource(ctx, configDir, cacheDir, source, lock, string(policy), featurefetch.ResolveOptions{
		AllowNetwork: opts.AllowNetwork,
		HTTPTimeout:  opts.HTTPTimeout,
		VerifyImage:  opts.VerifyImage,
	})
	if err != nil {
		return resolvedFeatureSource{}, err
	}
	return resolvedFeatureSource{
		Path:         resolved.Path,
		Kind:         resolved.Kind,
		Resolved:     resolved.Resolved,
		Integrity:    resolved.Integrity,
		Version:      resolved.Version,
		Verification: resolved.Verification,
	}, nil
}
