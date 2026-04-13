package devcontainer

import (
	"context"
	"path/filepath"
	"time"

	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

const (
	HostFolderLabel    = "devcontainer.local_folder"
	ConfigFileLabel    = "devcontainer.config_file"
	ImageMetadataLabel = spec.ImageMetadataLabel
	ManagedByLabel     = "devcontainer.managed_by"
	ManagedByValue     = "hatchctl"
	BridgeEnabledLabel = "devcontainer.bridge.enabled"
	SSHAgentLabel      = "devcontainer.ssh_agent.enabled"
)

type WorkspaceSpec = spec.WorkspaceSpec

type ResolvedConfig struct {
	WorkspaceFolder string
	ConfigPath      string
	ConfigDir       string
	Config          spec.Config
	Features        []ResolvedFeature
	Merged          spec.MergedConfig
	StateDir        string
	CacheDir        string
	WorkspaceMount  string
	RemoteWorkspace string
	ImageName       string
	SourceKind      string
	ContainerName   string
	ComposeFiles    []string
	ComposeService  string
	ComposeProject  string
	Labels          map[string]string
}

type ResolveOptions struct {
	AllowNetwork       bool
	ReadPlanCache      bool
	WritePlanCache     bool
	WriteFeatureLock   bool
	WriteFeatureState  bool
	Warn               func(string)
	StateBaseDir       string
	CacheBaseDir       string
	VerifyImage        func(context.Context, string) security.VerificationResult
	FeatureHTTPTimeout time.Duration
	LockfilePolicy     FeatureLockfilePolicy
}

func Resolve(ctx context.Context, workspaceArg string, configArg string) (ResolvedConfig, error) {
	return ResolveWithOptions(ctx, workspaceArg, configArg, ResolveOptions{
		AllowNetwork:      true,
		ReadPlanCache:     true,
		WritePlanCache:    true,
		WriteFeatureLock:  true,
		WriteFeatureState: true,
		LockfilePolicy:    FeatureLockfilePolicyAuto,
	})
}

func ResolveReadOnly(ctx context.Context, workspaceArg string, configArg string) (ResolvedConfig, error) {
	return ResolveReadOnlyWithOptions(ctx, workspaceArg, configArg, ResolveOptions{
		ReadPlanCache:  true,
		LockfilePolicy: FeatureLockfilePolicyFrozen,
	})
}

func ResolveWithOptions(ctx context.Context, workspaceArg string, configArg string, opts ResolveOptions) (ResolvedConfig, error) {
	return resolve(ctx, workspaceArg, configArg, opts)
}

func ResolveReadOnlyWithOptions(ctx context.Context, workspaceArg string, configArg string, opts ResolveOptions) (ResolvedConfig, error) {
	if opts.LockfilePolicy == "" {
		opts.LockfilePolicy = FeatureLockfilePolicyFrozen
	}
	return resolve(ctx, workspaceArg, configArg, opts)
}

func resolve(ctx context.Context, workspaceArg string, configArg string, opts ResolveOptions) (ResolvedConfig, error) {
	workspaceSpec, stateDir, cacheDir, err := resolveWorkspaceSpecAndDirs(workspaceArg, configArg, opts)
	if err != nil {
		return ResolvedConfig{}, err
	}
	return ResolveWorkspaceSpecWithOptions(ctx, workspaceSpec, stateDir, cacheDir, opts)
}

func ResolveWorkspaceSpecWithOptions(ctx context.Context, workspaceSpec WorkspaceSpec, stateDir string, cacheDir string, opts ResolveOptions) (ResolvedConfig, error) {
	if opts.LockfilePolicy == "" {
		opts.LockfilePolicy = FeatureLockfilePolicyAuto
	}
	if workspaceSpec.ConfigDir == "" && workspaceSpec.ConfigPath != "" {
		workspaceSpec.ConfigDir = filepath.Dir(workspaceSpec.ConfigPath)
	}
	if stateDir == "" || cacheDir == "" {
		_, resolvedStateDir, resolvedCacheDir, err := storefs.WorkspaceOutputDirs(opts.StateBaseDir, opts.CacheBaseDir, workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath)
		if err != nil {
			return ResolvedConfig{}, err
		}
		if stateDir == "" {
			stateDir = resolvedStateDir
		}
		if cacheDir == "" {
			cacheDir = resolvedCacheDir
		}
	}
	persistence := resolverPersistence{}
	cacheKey, err := resolvedPlanCacheKey(workspaceSpec.ConfigPath, workspaceSpec.ConfigDir, workspaceSpec.Config, workspaceSpec.ComposeFiles)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if cached, ok, err := loadResolvedPlanCache(persistence, workspaceSpec.ConfigPath, stateDir, cacheDir, cacheKey, opts); err != nil {
		return ResolvedConfig{}, err
	} else if ok {
		return cached, nil
	}

	resolved, err := resolveWorkspaceConfig(ctx, workspaceSpec, stateDir, cacheDir, persistence, opts)
	if err != nil {
		return ResolvedConfig{}, err
	}
	cacheKey, err = resolvedPlanCacheKey(workspaceSpec.ConfigPath, workspaceSpec.ConfigDir, workspaceSpec.Config, workspaceSpec.ComposeFiles)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if err := persistence.WritePlanCache(cacheDir, cacheKey, resolved, opts); err != nil {
		return ResolvedConfig{}, err
	}
	return resolved, nil
}

func resolveWorkspaceSpecAndDirs(workspaceArg string, configArg string, opts ResolveOptions) (WorkspaceSpec, string, string, error) {
	workspaceSpec, err := spec.ResolveWorkspaceSpec(workspaceArg, configArg)
	if err != nil {
		return WorkspaceSpec{}, "", "", err
	}
	_, stateDir, cacheDir, err := storefs.WorkspaceOutputDirs(opts.StateBaseDir, opts.CacheBaseDir, workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath)
	if err != nil {
		return WorkspaceSpec{}, "", "", err
	}
	return workspaceSpec, stateDir, cacheDir, nil
}

func loadResolvedPlanCache(persistence resolverPersistence, configPath string, stateDir string, cacheDir string, cacheKey string, opts ResolveOptions) (ResolvedConfig, bool, error) {
	cached, ok, err := persistence.ReadPlanCache(cacheDir, cacheKey, opts)
	if err != nil || !ok {
		return ResolvedConfig{}, ok, err
	}
	if err := persistence.WriteCachedArtifacts(configPath, stateDir, cached, opts); err != nil {
		return ResolvedConfig{}, false, err
	}
	return cached, true, nil
}

func resolveWorkspaceConfig(ctx context.Context, workspaceSpec WorkspaceSpec, stateDir string, cacheDir string, persistence resolverPersistence, opts ResolveOptions) (ResolvedConfig, error) {
	features, err := resolveFeaturesForWorkspace(ctx, workspaceSpec, stateDir, cacheDir, persistence, opts)
	if err != nil {
		return ResolvedConfig{}, err
	}
	return buildResolvedConfig(workspaceSpec, stateDir, cacheDir, features), nil
}

func resolveFeaturesForWorkspace(ctx context.Context, workspaceSpec WorkspaceSpec, stateDir string, cacheDir string, persistence resolverPersistence, opts ResolveOptions) ([]ResolvedFeature, error) {
	features, err := ResolveFeatures(ctx, workspaceSpec.ConfigPath, workspaceSpec.ConfigDir, storefs.FeatureCacheDir(cacheDir), workspaceSpec.Config.Features, FeatureResolveOptions{
		AllowNetwork:   opts.AllowNetwork,
		StateDir:       stateDir,
		HTTPTimeout:    opts.FeatureHTTPTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		VerifyImage:    opts.VerifyImage,
	})
	if err != nil {
		return nil, err
	}
	if err := persistence.WriteResolvedArtifacts(workspaceSpec.ConfigPath, stateDir, features, opts); err != nil {
		return nil, err
	}
	return features, nil
}

func buildResolvedConfig(workspaceSpec WorkspaceSpec, stateDir string, cacheDir string, features []ResolvedFeature) ResolvedConfig {
	return ResolvedConfig{
		WorkspaceFolder: workspaceSpec.WorkspaceFolder,
		ConfigPath:      workspaceSpec.ConfigPath,
		ConfigDir:       workspaceSpec.ConfigDir,
		Config:          workspaceSpec.Config,
		Features:        features,
		Merged:          spec.MergeMetadata(workspaceSpec.Config, FeaturesMetadata(features)),
		StateDir:        stateDir,
		CacheDir:        cacheDir,
		WorkspaceMount:  workspaceSpec.WorkspaceMount,
		RemoteWorkspace: workspaceSpec.RemoteWorkspace,
		ImageName:       storefs.ImageName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath),
		SourceKind:      workspaceSpec.SourceKind,
		ContainerName:   storefs.ContainerName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath),
		ComposeFiles:    workspaceSpec.ComposeFiles,
		ComposeService:  workspaceSpec.ComposeService,
		ComposeProject:  spec.ComposeProjectName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath),
		Labels: map[string]string{
			HostFolderLabel: workspaceSpec.WorkspaceFolder,
			ConfigFileLabel: workspaceSpec.ConfigPath,
			ManagedByLabel:  ManagedByValue,
		},
	}
}
