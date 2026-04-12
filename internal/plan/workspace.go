package plan

import (
	"context"
	"time"

	"github.com/lauritsk/hatchctl/internal/capability"
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

type DotfilesPreference = capability.Dotfiles

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

type DesiredState struct {
	Resolved devcontainer.ResolvedConfig
}

type WorkspacePlan struct {
	Immutable              ImmutableInputs
	LockProtected          LockProtectedArtifacts
	Desired                DesiredState
	ReadOnly               bool
	FeatureMaterialization FeatureMaterializationMode
	Preferences            Preferences
	Capabilities           capability.Set
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
	roots, stateDir, cacheDir, err := storefs.WorkspaceOutputDirs(req.StateBaseDir, req.CacheBaseDir, workspaceSpec.WorkspaceFolder, workspaceSpec.ConfigPath)
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
			StateBaseDir:         roots.StateRoot,
			CacheBaseDir:         roots.CacheRoot,
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
		Capabilities: capability.Set{
			SSHAgent: capability.SSHAgent{Enabled: req.SSHAgent},
			UIDRemap: capability.UIDRemap{Enabled: workspaceSpec.Merged.UpdateRemoteUserUID == nil || *workspaceSpec.Merged.UpdateRemoteUserUID},
			Dotfiles: capability.Dotfiles{Repository: req.Dotfiles.Repository, InstallCommand: req.Dotfiles.InstallCommand, TargetPath: req.Dotfiles.TargetPath},
			Bridge:   capability.Bridge{Enabled: req.BridgeEnabled},
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

func (p WorkspacePlan) WithResolved(resolved devcontainer.ResolvedConfig) WorkspacePlan {
	p.Desired.Resolved = resolved
	return p
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
