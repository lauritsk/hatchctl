package plan

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
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
