package devcontainer

type resolverPersistence struct{}

func (resolverPersistence) ReadPlanCache(cacheDir string, key string, opts ResolveOptions) (ResolvedConfig, bool, error) {
	if !opts.ReadPlanCache || opts.LockfilePolicy == FeatureLockfilePolicyUpdate {
		return ResolvedConfig{}, false, nil
	}
	return readResolvedPlanCache(cacheDir, key)
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
		return nil
	}
	return nil
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
