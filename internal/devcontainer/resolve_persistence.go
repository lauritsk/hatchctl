package devcontainer

import "fmt"

type resolverPersistence struct{}

func (resolverPersistence) ReadPlanCache(cacheDir string, key string, opts ResolveOptions) (ResolvedConfig, bool, error) {
	if !opts.ReadPlanCache || opts.LockfilePolicy == FeatureLockfilePolicyUpdate {
		return ResolvedConfig{}, false, nil
	}
	resolved, ok, err := readResolvedPlanCache(cacheDir, key)
	if err == nil {
		return resolved, ok, nil
	}
	warnResolve(opts, fmt.Sprintf("ignoring resolved plan cache at %q: %v", cacheDir, err))
	return ResolvedConfig{}, false, nil
}

func (resolverPersistence) WriteCachedArtifacts(configPath string, stateDir string, resolved ResolvedConfig, opts ResolveOptions) error {
	return writeResolvedArtifacts(configPath, stateDir, resolved.Features, opts)
}

func (resolverPersistence) WriteResolvedArtifacts(configPath string, stateDir string, features []ResolvedFeature, opts ResolveOptions) error {
	return writeResolvedArtifacts(configPath, stateDir, features, opts)
}

func (resolverPersistence) WritePlanCache(cacheDir string, key string, resolved ResolvedConfig, opts ResolveOptions) error {
	if !opts.WritePlanCache {
		return nil
	}
	// The resolved plan cache is only an optimization, so a persistence failure
	// should not block the workspace operation itself.
	if err := writeResolvedPlanCache(cacheDir, key, resolved); err != nil {
		warnResolve(opts, fmt.Sprintf("unable to write resolved plan cache at %q: %v", cacheDir, err))
		return nil
	}
	return nil
}

func warnResolve(opts ResolveOptions, message string) {
	if opts.Warn != nil && message != "" {
		opts.Warn(message)
	}
}

func writeResolvedArtifacts(configPath string, stateDir string, features []ResolvedFeature, opts ResolveOptions) error {
	if opts.WriteFeatureLock && opts.LockfilePolicy != FeatureLockfilePolicyFrozen {
		if err := WriteFeatureLockFile(configPath, features); err != nil {
			return err
		}
	}
	if opts.WriteFeatureState {
		if err := WriteFeatureStateFile(stateDir, features); err != nil {
			return err
		}
	}
	return nil
}
