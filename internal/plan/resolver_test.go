package plan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestResolverMaterializeUsesWritableResolveOptions(t *testing.T) {
	t.Parallel()

	workspacePlan := WorkspacePlan{
		Immutable: ImmutableInputs{
			Workspace:      "/workspace",
			ConfigPath:     "/workspace/.devcontainer/devcontainer.json",
			FeatureTimeout: 45 * time.Second,
		},
		LockProtected: LockProtectedArtifacts{
			StateBaseDir:         "/state",
			CacheBaseDir:         "/cache",
			UsesPlanCache:        true,
			UsesFeatureLock:      true,
			UsesFeatureState:     true,
			RequiresRevalidation: true,
		},
		FeatureMaterialization: FeatureMaterializationRefresh,
	}
	resolver := NewResolver()
	called := false
	resolver.Resolve = func(_ context.Context, workspaceArg string, configArg string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		called = true
		if workspaceArg != "/workspace" || configArg != "/workspace/.devcontainer/devcontainer.json" {
			t.Fatalf("unexpected resolve args workspace=%q config=%q", workspaceArg, configArg)
		}
		if !opts.AllowNetwork || !opts.ReadPlanCache || !opts.WritePlanCache || !opts.WriteFeatureLock || !opts.WriteFeatureState {
			t.Fatalf("unexpected writable resolve options %#v", opts)
		}
		if opts.StateBaseDir != "/state" || opts.CacheBaseDir != "/cache" {
			t.Fatalf("unexpected resolve roots %#v", opts)
		}
		if opts.FeatureHTTPTimeout != 45*time.Second {
			t.Fatalf("unexpected feature timeout %s", opts.FeatureHTTPTimeout)
		}
		if opts.LockfilePolicy != devcontainer.FeatureLockfilePolicyUpdate {
			t.Fatalf("unexpected lockfile policy %q", opts.LockfilePolicy)
		}
		return devcontainer.ResolvedConfig{ImageName: "hatchctl-demo"}, nil
	}
	resolver.ResolveReadOnly = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		t.Fatal("unexpected read-only resolve")
		return devcontainer.ResolvedConfig{}, nil
	}

	resolved, err := resolver.Materialize(context.Background(), workspacePlan, nil)
	if err != nil {
		t.Fatalf("materialize plan: %v", err)
	}
	if !called {
		t.Fatal("expected writable resolver to be called")
	}
	if resolved.ImageName != "hatchctl-demo" {
		t.Fatalf("unexpected resolved config %#v", resolved)
	}
}

func TestResolverMaterializeUsesReadOnlyResolverOptions(t *testing.T) {
	t.Parallel()

	workspacePlan := WorkspacePlan{
		Immutable: ImmutableInputs{
			Workspace:      "/workspace",
			ConfigPath:     "devcontainer.json",
			FeatureTimeout: 30 * time.Second,
		},
		LockProtected: LockProtectedArtifacts{
			StateBaseDir:  "/state",
			CacheBaseDir:  "/cache",
			UsesPlanCache: true,
		},
		ReadOnly:               true,
		FeatureMaterialization: FeatureMaterializationReadonly,
	}
	resolver := NewResolver()
	called := false
	resolver.Resolve = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		t.Fatal("unexpected writable resolve")
		return devcontainer.ResolvedConfig{}, nil
	}
	resolver.ResolveReadOnly = func(_ context.Context, workspaceArg string, configArg string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		called = true
		if workspaceArg != "/workspace" || configArg != "devcontainer.json" {
			t.Fatalf("unexpected read-only resolve args workspace=%q config=%q", workspaceArg, configArg)
		}
		if opts.AllowNetwork || opts.WritePlanCache || opts.WriteFeatureLock || opts.WriteFeatureState {
			t.Fatalf("unexpected read-only resolve options %#v", opts)
		}
		if !opts.ReadPlanCache {
			t.Fatalf("expected read-only resolve to read plan cache %#v", opts)
		}
		if opts.LockfilePolicy != devcontainer.FeatureLockfilePolicyFrozen {
			t.Fatalf("unexpected lockfile policy %q", opts.LockfilePolicy)
		}
		return devcontainer.ResolvedConfig{SourceKind: "image"}, nil
	}

	_, err := resolver.Materialize(context.Background(), workspacePlan, nil)
	if err != nil {
		t.Fatalf("materialize plan: %v", err)
	}
	if !called {
		t.Fatal("expected read-only resolver to be called")
	}
}

func TestResolverMaterializeForwardsWarningHook(t *testing.T) {
	t.Parallel()

	wantWarn := func(string) {}
	resolver := NewResolver()
	resolver.Warn = wantWarn
	resolver.Resolve = func(_ context.Context, _ string, _ string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		if reflect.ValueOf(opts.Warn).Pointer() != reflect.ValueOf(wantWarn).Pointer() {
			t.Fatalf("expected warning hook to be forwarded")
		}
		return devcontainer.ResolvedConfig{}, nil
	}

	_, err := resolver.Materialize(context.Background(), WorkspacePlan{}, nil)
	if err != nil {
		t.Fatalf("materialize plan: %v", err)
	}
}

func TestResolverMaterializeUsesWorkspacePlanSpecByDefault(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"alpine:3.23","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceSpec, err := spec.ResolveWorkspaceSpec(workspace, configPath)
	if err != nil {
		t.Fatalf("resolve workspace spec: %v", err)
	}

	workspacePlan := WorkspacePlan{
		Immutable: ImmutableInputs{
			Workspace:      "/stale-workspace",
			ConfigPath:     "/stale-workspace/devcontainer.json",
			FeatureTimeout: 30 * time.Second,
			Spec:           workspaceSpec,
		},
		LockProtected: LockProtectedArtifacts{
			StateDir:      t.TempDir(),
			CacheDir:      t.TempDir(),
			UsesPlanCache: true,
		},
		ReadOnly:               true,
		FeatureMaterialization: FeatureMaterializationReadonly,
	}

	resolved, err := NewResolver().Materialize(context.Background(), workspacePlan, nil)
	if err != nil {
		t.Fatalf("materialize plan: %v", err)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("expected materialization to use planned spec config path %q, got %q", configPath, resolved.ConfigPath)
	}
	if resolved.WorkspaceFolder != workspace {
		t.Fatalf("expected materialization to use planned spec workspace %q, got %q", workspace, resolved.WorkspaceFolder)
	}
}

func TestResolverMaterializeSkipsWorkspacePlanSpecWhenCustomResolverConfigured(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	called := false
	resolver.Resolve = func(_ context.Context, workspaceArg string, configArg string, _ devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		called = true
		if workspaceArg != "/workspace" || configArg != "/workspace/devcontainer.json" {
			t.Fatalf("unexpected resolve args workspace=%q config=%q", workspaceArg, configArg)
		}
		return devcontainer.ResolvedConfig{}, nil
	}

	_, err := resolver.Materialize(context.Background(), WorkspacePlan{
		Immutable: ImmutableInputs{
			Workspace:  "/workspace",
			ConfigPath: "/workspace/devcontainer.json",
			Spec: spec.WorkspaceSpec{
				WorkspaceFolder: "/planned-workspace",
				ConfigPath:      "/planned-workspace/.devcontainer/devcontainer.json",
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("materialize plan: %v", err)
	}
	if !called {
		t.Fatal("expected custom resolver to bypass planned workspace spec")
	}
}
