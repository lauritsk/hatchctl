package plan

import (
	"context"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type FeatureMaterializationMode string

const (
	FeatureMaterializationReadonly FeatureMaterializationMode = "readonly"
	FeatureMaterializationReuse    FeatureMaterializationMode = "reuse"
	FeatureMaterializationRefresh  FeatureMaterializationMode = "refresh"
)

type DotfilesPreference struct {
	Repository     string
	InstallCommand string
	TargetPath     string
}

type Preferences struct {
	BridgeEnabled bool
	SSHAgent      bool
	Dotfiles      DotfilesPreference
}

type TrustPlan struct {
	WorkspaceAllowed      bool
	WorkspaceRequired     bool
	HostLifecycleAllowed  bool
	HostLifecycleRequired bool
}

type ImmutableInputs struct {
	Workspace      string
	ConfigPath     string
	FeatureTimeout time.Duration
	Spec           spec.WorkspaceSpec
}

type LockProtectedArtifacts struct {
	StateBaseDir         string
	CacheBaseDir         string
	StateDir             string
	CacheDir             string
	UsesPlanCache        bool
	UsesFeatureLock      bool
	UsesFeatureState     bool
	RequiresRevalidation bool
}

type WorkspacePlan struct {
	Immutable              ImmutableInputs
	LockProtected          LockProtectedArtifacts
	ReadOnly               bool
	FeatureMaterialization FeatureMaterializationMode
	Preferences            Preferences
	Trust                  TrustPlan
}

type BuildWorkspacePlanRequest struct {
	Workspace          string
	ConfigPath         string
	StateBaseDir       string
	CacheBaseDir       string
	FeatureTimeout     time.Duration
	LockfilePolicy     devcontainer.FeatureLockfilePolicy
	ReadOnly           bool
	BridgeEnabled      bool
	SSHAgent           bool
	Dotfiles           DotfilesPreference
	TrustWorkspace     bool
	AllowHostLifecycle bool
}

func BuildWorkspacePlan(req BuildWorkspacePlanRequest) (WorkspacePlan, error) {
	workspaceSpec, err := spec.ResolveWorkspaceSpec(req.Workspace, req.ConfigPath)
	if err != nil {
		return WorkspacePlan{}, err
	}
	stateBaseDir, cacheBaseDir, stateDir, cacheDir, err := workspaceOutputDirs(workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath, req.StateBaseDir, req.CacheBaseDir)
	if err != nil {
		return WorkspacePlan{}, err
	}
	materialization, err := featureMaterializationMode(req.LockfilePolicy)
	if err != nil {
		return WorkspacePlan{}, err
	}
	return WorkspacePlan{
		Immutable: ImmutableInputs{
			Workspace:      workspaceSpec.WorkspaceFolder,
			ConfigPath:     workspaceSpec.ConfigPath,
			FeatureTimeout: req.FeatureTimeout,
			Spec:           workspaceSpec,
		},
		LockProtected: LockProtectedArtifacts{
			StateBaseDir:         stateBaseDir,
			CacheBaseDir:         cacheBaseDir,
			StateDir:             stateDir,
			CacheDir:             cacheDir,
			UsesPlanCache:        true,
			UsesFeatureLock:      true,
			UsesFeatureState:     !req.ReadOnly,
			RequiresRevalidation: !req.ReadOnly,
		},
		ReadOnly:               req.ReadOnly,
		FeatureMaterialization: materialization,
		Preferences: Preferences{
			BridgeEnabled: req.BridgeEnabled,
			SSHAgent:      req.SSHAgent,
			Dotfiles:      req.Dotfiles,
		},
		Trust: TrustPlan{
			WorkspaceAllowed:      req.TrustWorkspace,
			WorkspaceRequired:     policy.WorkspaceTrustRequiredForSpec(workspaceSpec),
			HostLifecycleAllowed:  req.AllowHostLifecycle,
			HostLifecycleRequired: policy.HostLifecycleTrustRequired(workspaceSpec.Config.InitializeCommand),
		},
	}, nil
}

func (p WorkspacePlan) Valid() bool {
	return p.Immutable.Spec.ConfigPath != ""
}

func (p WorkspacePlan) ResolveOptions(verifyImage func(context.Context, string) security.VerificationResult) devcontainer.ResolveOptions {
	opts := devcontainer.ResolveOptions{
		ReadPlanCache:      p.LockProtected.UsesPlanCache,
		StateBaseDir:       p.LockProtected.StateBaseDir,
		CacheBaseDir:       p.LockProtected.CacheBaseDir,
		FeatureHTTPTimeout: p.Immutable.FeatureTimeout,
		LockfilePolicy:     p.lockfilePolicy(),
		VerifyImage:        verifyImage,
	}
	if !p.ReadOnly {
		opts.AllowNetwork = true
		opts.WritePlanCache = true
		opts.WriteFeatureLock = p.LockProtected.UsesFeatureLock
		opts.WriteFeatureState = p.LockProtected.UsesFeatureState
	}
	return opts
}

func (p WorkspacePlan) lockfilePolicy() devcontainer.FeatureLockfilePolicy {
	switch p.FeatureMaterialization {
	case FeatureMaterializationReadonly:
		return devcontainer.FeatureLockfilePolicyFrozen
	case FeatureMaterializationRefresh:
		return devcontainer.FeatureLockfilePolicyUpdate
	default:
		return devcontainer.FeatureLockfilePolicyAuto
	}
}

func featureMaterializationMode(policyValue devcontainer.FeatureLockfilePolicy) (FeatureMaterializationMode, error) {
	policyValue, err := devcontainer.ParseFeatureLockfilePolicy(string(policyValue))
	if err != nil {
		return "", err
	}
	switch policyValue {
	case devcontainer.FeatureLockfilePolicyFrozen:
		return FeatureMaterializationReadonly, nil
	case devcontainer.FeatureLockfilePolicyUpdate:
		return FeatureMaterializationRefresh, nil
	default:
		return FeatureMaterializationReuse, nil
	}
}

func workspaceOutputDirs(workspace string, configPath string, stateBaseDir string, cacheBaseDir string) (string, string, string, string, error) {
	if stateBaseDir == "" || cacheBaseDir == "" {
		roots, err := storefs.DefaultOutputRoots()
		if err != nil {
			return "", "", "", "", err
		}
		if stateBaseDir == "" {
			stateBaseDir = roots.StateRoot
		}
		if cacheBaseDir == "" {
			cacheBaseDir = roots.CacheRoot
		}
	}
	stateDir := storefs.WorkspaceScopedDir(stateBaseDir, workspace, configPath)
	cacheDir := storefs.WorkspaceScopedDir(cacheBaseDir, workspace, configPath)
	return stateBaseDir, cacheBaseDir, stateDir, cacheDir, nil
}
