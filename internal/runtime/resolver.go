package runtime

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type workspaceResolver interface {
	Resolve(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
	ResolveReadOnly(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error)
}

type devcontainerResolver struct{}

func (devcontainerResolver) Resolve(ctx context.Context, workspace string, configPath string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
	return devcontainer.ResolveWithOptions(ctx, workspace, configPath, opts)
}

func (devcontainerResolver) ResolveReadOnly(ctx context.Context, workspace string, configPath string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
	return devcontainer.ResolveReadOnlyWithOptions(ctx, workspace, configPath, opts)
}
