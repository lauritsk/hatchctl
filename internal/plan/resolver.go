package plan

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/security"
)

type Resolver struct {
	ResolveWorkspaceSpec func(context.Context, devcontainer.WorkspaceSpec, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	Resolve              func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	ResolveReadOnly      func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	Warn                 func(string)
}

func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Clone() *Resolver {
	if r == nil {
		return NewResolver()
	}
	clone := *r
	return &clone
}

func (r *Resolver) Materialize(ctx context.Context, workspacePlan WorkspacePlan, verifyImage func(context.Context, string) security.VerificationResult) (devcontainer.ResolvedConfig, error) {
	usePlannedWorkspaceSpec := r != nil && r.Resolve == nil && r.ResolveReadOnly == nil
	r = r.withDefaults()
	resolveOpts := workspacePlan.ResolveOptions(verifyImage)
	resolveOpts.Warn = r.Warn
	if usePlannedWorkspaceSpec && workspacePlan.Immutable.Spec.ConfigPath != "" {
		return r.ResolveWorkspaceSpec(ctx, workspacePlan.Immutable.Spec, workspacePlan.LockProtected.StateDir, workspacePlan.LockProtected.CacheDir, resolveOpts)
	}
	if workspacePlan.ReadOnly {
		return r.ResolveReadOnly(ctx, workspacePlan.Immutable.Workspace, workspacePlan.Immutable.ConfigPath, resolveOpts)
	}
	return r.Resolve(ctx, workspacePlan.Immutable.Workspace, workspacePlan.Immutable.ConfigPath, resolveOpts)
}

func (r *Resolver) withDefaults() *Resolver {
	if r.ResolveWorkspaceSpec == nil {
		r.ResolveWorkspaceSpec = devcontainer.ResolveWorkspaceSpecWithOptions
	}
	if r.Resolve == nil {
		r.Resolve = devcontainer.ResolveWithOptions
	}
	if r.ResolveReadOnly == nil {
		r.ResolveReadOnly = devcontainer.ResolveReadOnlyWithOptions
	}
	return r
}
