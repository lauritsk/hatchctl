package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestWorkspacePlannerPrepareResolvedUsesWritableResolverOptions(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t, &fakeRuntimeBackend{})
	planner := newWorkspacePlanner(runner)
	sink := &recordedSink{}
	called := false
	planner.resolve = func(_ context.Context, workspaceArg string, configArg string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
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
		if opts.VerifyImage == nil {
			t.Fatal("expected image verifier to be wired into resolve options")
		}
		return devcontainer.ResolvedConfig{
			SourceKind:      "image",
			ConfigPath:      configArg,
			WorkspaceFolder: "/workspace",
			StateDir:        "/state/workspaces/demo",
			ImageName:       "hatchctl-demo",
		}, nil
	}
	planner.resolveReadOnly = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		t.Fatal("unexpected read-only resolve")
		return devcontainer.ResolvedConfig{}, nil
	}

	resolved, err := planner.prepareResolved(context.Background(), prepareResolveOptions{
		Workspace:      "/workspace",
		ConfigPath:     "/workspace/.devcontainer/devcontainer.json",
		StateDir:       "/state",
		CacheDir:       "/cache",
		FeatureTimeout: 45 * time.Second,
		LockfilePolicy: devcontainer.FeatureLockfilePolicyUpdate,
		ProgressPhase:  phaseResolve,
		ProgressLabel:  "Resolving development container",
		Debug:          true,
		Events:         sink,
	})
	if err != nil {
		t.Fatalf("prepare resolved: %v", err)
	}
	if !called {
		t.Fatal("expected writable resolver to be called")
	}
	if resolved.ImageName != "hatchctl-demo" {
		t.Fatalf("unexpected resolved config %#v", resolved)
	}
	if len(sink.events) != 2 {
		t.Fatalf("expected progress and debug events, got %#v", sink.events)
	}
	if sink.events[0].Kind != "progress" || sink.events[0].Phase != phaseResolve || sink.events[0].Message != "Resolving development container" {
		t.Fatalf("unexpected progress event %#v", sink.events[0])
	}
	if sink.events[1].Kind != "debug" || !strings.Contains(sink.events[1].Message, "plan source=image") || !strings.Contains(sink.events[1].Message, "target-image=hatchctl-demo") {
		t.Fatalf("unexpected debug event %#v", sink.events[1])
	}
}

func TestWorkspacePlannerPrepareResolvedUsesReadOnlyResolverOptions(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t, &fakeRuntimeBackend{})
	planner := newWorkspacePlanner(runner)
	called := false
	planner.resolve = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		t.Fatal("unexpected writable resolve")
		return devcontainer.ResolvedConfig{}, nil
	}
	planner.resolveReadOnly = func(_ context.Context, workspaceArg string, configArg string, opts devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
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
		return devcontainer.ResolvedConfig{SourceKind: "image"}, nil
	}

	_, err := planner.prepareResolved(context.Background(), prepareResolveOptions{
		Workspace:      "/workspace",
		ConfigPath:     "devcontainer.json",
		StateDir:       "/state",
		CacheDir:       "/cache",
		FeatureTimeout: 30 * time.Second,
		LockfilePolicy: devcontainer.FeatureLockfilePolicyFrozen,
		ReadOnly:       true,
	})
	if err != nil {
		t.Fatalf("prepare resolved: %v", err)
	}
	if !called {
		t.Fatal("expected read-only resolver to be called")
	}
}

func TestWorkspacePlannerPrepareWorkspaceLoadsStateReconcilesAndInspectsContainer(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) != 0 && cmd.Args[0] == "ps" {
				return "replacement\n", nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
		inspectContainer: func(_ context.Context, containerID string) (docker.ContainerInspect, error) {
			switch containerID {
			case "missing":
				return docker.ContainerInspect{}, dockerNotFoundError("inspect", containerID)
			case "replacement":
				return docker.ContainerInspect{ID: "replacement", Config: docker.InspectConfig{User: "app"}, State: docker.ContainerState{Status: "running", Running: true}}, nil
			default:
				t.Fatalf("unexpected container inspect %q", containerID)
				return docker.ContainerInspect{}, nil
			}
		},
	}
	runner := newTestRunner(t, backend)
	planner := newWorkspacePlanner(runner)
	planner.resolve = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		return devcontainer.ResolvedConfig{
			Config:   devcontainer.Config{Image: "demo-image"},
			Labels:   map[string]string{"managed": "true"},
			StateDir: "/state/workspaces/demo",
		}, nil
	}
	planner.readState = func(stateDir string) (devcontainer.State, error) {
		if stateDir != "/state/workspaces/demo" {
			t.Fatalf("unexpected state dir %q", stateDir)
		}
		return devcontainer.State{ContainerID: "missing", LifecycleReady: true, BridgeEnabled: true, BridgeSessionID: "bridge-1"}, nil
	}

	prepared, err := planner.prepareWorkspace(context.Background(), prepareWorkspaceOptions{
		resolve:          prepareResolveOptions{Workspace: "/workspace"},
		loadState:        true,
		inspectContainer: true,
	})
	if err != nil {
		t.Fatalf("prepare workspace: %v", err)
	}
	if prepared.containerID != "replacement" {
		t.Fatalf("unexpected container id %q", prepared.containerID)
	}
	if prepared.state.ContainerID != "replacement" || prepared.state.LifecycleReady || !prepared.state.BridgeEnabled || prepared.state.BridgeSessionID != "bridge-1" {
		t.Fatalf("unexpected reconciled state %#v", prepared.state)
	}
	if prepared.containerInspect == nil || prepared.containerInspect.ID != "replacement" {
		t.Fatalf("expected inspected replacement container, got %#v", prepared.containerInspect)
	}
}

func TestWorkspacePlannerPrepareWorkspaceEnrichesManagedImageWithoutInspect(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		inspectImage: func(_ context.Context, image string) (docker.ImageInspect, error) {
			if image != "hatchctl-demo" {
				t.Fatalf("unexpected image inspect %q", image)
			}
			return docker.ImageInspect{}, dockerNotFoundError("image", "inspect", image)
		},
	}
	runner := newTestRunner(t, backend)
	planner := newWorkspacePlanner(runner)
	planner.resolve = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		return devcontainer.ResolvedConfig{
			Config: devcontainer.Config{RemoteEnv: map[string]string{"CONFIG": "1"}},
			Features: []devcontainer.ResolvedFeature{{
				Metadata: devcontainer.MetadataEntry{RemoteEnv: map[string]string{"FEATURE": "1"}},
			}},
			ImageName:  "hatchctl-demo",
			SourceKind: "image",
		}, nil
	}

	prepared, err := planner.prepareWorkspace(context.Background(), prepareWorkspaceOptions{enrich: true})
	if err != nil {
		t.Fatalf("prepare workspace: %v", err)
	}
	if prepared.image != "hatchctl-demo" {
		t.Fatalf("unexpected prepared image %q", prepared.image)
	}
	if prepared.resolved.Merged.RemoteEnv["CONFIG"] != "1" || prepared.resolved.Merged.RemoteEnv["FEATURE"] != "1" {
		t.Fatalf("expected managed-image enrichment fallback, got %#v", prepared.resolved.Merged.RemoteEnv)
	}
}

func TestWorkspacePlannerPrepareWorkspaceAllowsMissingContainer(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) != 0 && cmd.Args[0] == "ps" {
				return "", nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
	}
	runner := newTestRunner(t, backend)
	planner := newWorkspacePlanner(runner)
	planner.resolve = func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
		return devcontainer.ResolvedConfig{Labels: map[string]string{"managed": "true"}}, nil
	}

	prepared, err := planner.prepareWorkspace(context.Background(), prepareWorkspaceOptions{
		findContainer:         true,
		allowMissingContainer: true,
	})
	if err != nil {
		t.Fatalf("prepare workspace: %v", err)
	}
	if prepared.containerID != "" {
		t.Fatalf("expected missing container to be allowed, got %q", prepared.containerID)
	}
}
