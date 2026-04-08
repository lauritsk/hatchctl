package devcontainer

type resolverPersistence struct{}

func (resolverPersistence) ReadPlanCache(stateDir string, key string, opts ResolveOptions) (ResolvedConfig, bool, error) {
	if !opts.ReadPlanCache || opts.LockfilePolicy == FeatureLockfilePolicyUpdate {
		return ResolvedConfig{}, false, nil
	}
	return readResolvedPlanCache(stateDir, key)
}

func (resolverPersistence) WriteCachedArtifacts(configPath string, stateDir string, resolved ResolvedConfig, opts ResolveOptions) error {
	return writeResolvedArtifacts(configPath, stateDir, resolved.Features, opts)
}

func (resolverPersistence) WriteResolvedArtifacts(configPath string, stateDir string, features []ResolvedFeature, opts ResolveOptions) error {
	return writeResolvedArtifacts(configPath, stateDir, features, opts)
}

func (resolverPersistence) WritePlanCache(stateDir string, key string, resolved ResolvedConfig, opts ResolveOptions) error {
	if !opts.WritePlanCache {
		return nil
	}
	return writeResolvedPlanCache(stateDir, key, resolved)
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
