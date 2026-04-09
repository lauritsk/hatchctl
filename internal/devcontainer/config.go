package devcontainer

import (
	"context"
	"path/filepath"
	"time"

	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
)

const (
	HostFolderLabel    = "devcontainer.local_folder"
	ConfigFileLabel    = "devcontainer.config_file"
	ManagedByLabel     = "devcontainer.managed_by"
	ManagedByValue     = "hatchctl"
	BridgeEnabledLabel = "devcontainer.bridge.enabled"
	SSHAgentLabel      = "devcontainer.ssh_agent.enabled"
)

type (
	Config           = spec.Config
	BuildConfig      = spec.BuildConfig
	LifecycleCommand = spec.LifecycleCommand
	ForwardPorts     = spec.ForwardPorts
	LifecycleStep    = spec.LifecycleStep
	WorkspaceSpec    = spec.WorkspaceSpec
)

type ResolvedConfig struct {
	WorkspaceFolder string
	ConfigPath      string
	ConfigDir       string
	Config          Config
	Features        []ResolvedFeature
	Merged          MergedConfig
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
	persistence := resolverPersistence{}
	workspaceSpec, err := spec.ResolveWorkspaceSpec(workspaceArg, configArg)
	if err != nil {
		return ResolvedConfig{}, err
	}
	stateDir, cacheDir, err := workspaceOutputDirs(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath, opts)
	if err != nil {
		return ResolvedConfig{}, err
	}
	cacheKey, err := resolvedPlanCacheKey(workspaceSpec.ConfigPath, workspaceSpec.ConfigDir, workspaceSpec.Config, workspaceSpec.ComposeFiles)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if cached, ok, err := persistence.ReadPlanCache(cacheDir, cacheKey, opts); err != nil {
		return ResolvedConfig{}, err
	} else if ok {
		if err := persistence.WriteCachedArtifacts(workspaceSpec.ConfigPath, stateDir, cached, opts); err != nil {
			return ResolvedConfig{}, err
		}
		return cached, nil
	}

	imageName := ImageName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath)
	containerName := ContainerName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath)
	labels := map[string]string{
		HostFolderLabel: workspaceSpec.WorkspaceFolder,
		ConfigFileLabel: workspaceSpec.ConfigPath,
		ManagedByLabel:  ManagedByValue,
	}

	features, err := ResolveFeatures(ctx, workspaceSpec.ConfigPath, workspaceSpec.ConfigDir, filepath.Join(cacheDir, "features-cache"), workspaceSpec.Config.Features, FeatureResolveOptions{
		AllowNetwork:   opts.AllowNetwork,
		StateDir:       stateDir,
		HTTPTimeout:    opts.FeatureHTTPTimeout,
		LockfilePolicy: opts.LockfilePolicy,
		VerifyImage:    opts.VerifyImage,
	})
	if err != nil {
		return ResolvedConfig{}, err
	}
	if err := persistence.WriteResolvedArtifacts(workspaceSpec.ConfigPath, stateDir, features, opts); err != nil {
		return ResolvedConfig{}, err
	}
	metadata := make([]MetadataEntry, 0, len(features))
	for _, feature := range features {
		metadata = append(metadata, feature.Metadata)
	}

	resolved := ResolvedConfig{
		WorkspaceFolder: workspaceSpec.WorkspaceFolder,
		ConfigPath:      workspaceSpec.ConfigPath,
		ConfigDir:       workspaceSpec.ConfigDir,
		Config:          workspaceSpec.Config,
		Features:        features,
		Merged:          MergeMetadata(workspaceSpec.Config, metadata),
		StateDir:        stateDir,
		CacheDir:        cacheDir,
		WorkspaceMount:  workspaceSpec.WorkspaceMount,
		RemoteWorkspace: workspaceSpec.RemoteWorkspace,
		ImageName:       imageName,
		SourceKind:      workspaceSpec.SourceKind,
		ContainerName:   containerName,
		ComposeFiles:    workspaceSpec.ComposeFiles,
		ComposeService:  workspaceSpec.ComposeService,
		ComposeProject:  ComposeProjectName(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath),
		Labels:          labels,
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

func workspaceOutputDirs(workspace string, configPath string, opts ResolveOptions) (string, string, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return "", "", err
	}
	stateRoot := roots.StateRoot
	if opts.StateBaseDir != "" {
		stateRoot = opts.StateBaseDir
	}
	cacheRoot := roots.CacheRoot
	if opts.CacheBaseDir != "" {
		cacheRoot = opts.CacheBaseDir
	}
	return workspaceScopedDir(stateRoot, workspace, configPath), workspaceScopedDir(cacheRoot, workspace, configPath), nil
}

func Load(configPath string) (Config, error) {
	return spec.Load(configPath)
}

func resolveWorkspace(workspaceArg string) (string, error) {
	return spec.ResolveWorkspacePath(workspaceArg)
}

func ResolveWorkspacePath(workspaceArg string) (string, error) {
	return spec.ResolveWorkspacePath(workspaceArg)
}

func resolveConfigPath(workspace string, configArg string) (string, error) {
	return spec.ResolveConfigPath(workspace, configArg)
}

func ResolveConfigPath(workspace string, configArg string) (string, error) {
	return spec.ResolveConfigPath(workspace, configArg)
}

func EffectiveDockerfile(config Config) string {
	return spec.EffectiveDockerfile(config)
}

func EffectiveContext(config Config) string {
	return spec.EffectiveContext(config)
}

func ContainerCommand(config Config) []string {
	return spec.ContainerCommand(config)
}

func KeepAliveCommand() string {
	return spec.KeepAliveCommand()
}

func RemoteExecUser(config Config) string {
	return spec.RemoteExecUser(config)
}

func ShellQuote(value string) string {
	return spec.ShellQuote(value)
}

func NormalizeForwardPorts(raw []any) (ForwardPorts, error) {
	return spec.NormalizeForwardPorts(raw)
}

func MergeForwardPorts(entries ...ForwardPorts) ForwardPorts {
	return spec.MergeForwardPorts(entries...)
}
