package plan

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/security"
)

type Resolver struct {
	Resolve         func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	ResolveReadOnly func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
}

func NewResolver() *Resolver {
	return (&Resolver{}).withDefaults()
}

func (r *Resolver) Clone() *Resolver {
	if r == nil {
		return NewResolver()
	}
	clone := *r
	return clone.withDefaults()
}

func (r *Resolver) Materialize(ctx context.Context, workspacePlan WorkspacePlan, verifyImage func(context.Context, string) security.VerificationResult) (devcontainer.ResolvedConfig, error) {
	r = r.withDefaults()
	resolveOpts := workspacePlan.ResolveOptions(verifyImage)
	if workspacePlan.ReadOnly {
		return r.ResolveReadOnly(ctx, workspacePlan.Immutable.Workspace, workspacePlan.Immutable.ConfigPath, resolveOpts)
	}
	return r.Resolve(ctx, workspacePlan.Immutable.Workspace, workspacePlan.Immutable.ConfigPath, resolveOpts)
}

func (r *Resolver) withDefaults() *Resolver {
	if r.Resolve == nil {
		r.Resolve = devcontainer.ResolveWithOptions
	}
	if r.ResolveReadOnly == nil {
		r.ResolveReadOnly = devcontainer.ResolveReadOnlyWithOptions
	}
	return r
}
