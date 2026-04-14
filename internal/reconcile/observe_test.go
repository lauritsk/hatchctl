package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type fakeBackend struct {
	listContainers    func(context.Context, backend.ListContainersRequest) (string, error)
	inspectImage      func(context.Context, string) (backend.ImageInspect, error)
	inspectCont       func(context.Context, string) (backend.ContainerInspect, error)
	projectContainers func(context.Context, backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error)
}

func (f fakeBackend) InspectImage(ctx context.Context, image string) (backend.ImageInspect, error) {
	if f.inspectImage == nil {
		return backend.ImageInspect{}, nil
	}
	return f.inspectImage(ctx, image)
}

func (f fakeBackend) InspectContainer(ctx context.Context, containerID string) (backend.ContainerInspect, error) {
	if f.inspectCont == nil {
		return backend.ContainerInspect{}, nil
	}
	return f.inspectCont(ctx, containerID)
}

func (f fakeBackend) ListContainers(ctx context.Context, req backend.ListContainersRequest) (string, error) {
	if f.listContainers == nil {
		return "", nil
	}
	return f.listContainers(ctx, req)
}

func (f fakeBackend) ProjectContainers(ctx context.Context, req backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error) {
	if f.projectContainers == nil {
		return nil, nil, nil
	}
	return f.projectContainers(ctx, req)
}

func TestObserveManagedContainerCombinesControlAndEngineState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{ContainerID: "missing", LifecycleReady: true, BridgeEnabled: true, BridgeSessionID: "bridge-1"}); err != nil {
		t.Fatal(err)
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "resolved-plan.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := fakeBackend{
		listContainers: func(_ context.Context, req backend.ListContainersRequest) (string, error) {
			if len(req.Labels) != 2 {
				t.Fatalf("unexpected labels %#v", req.Labels)
			}
			return "dead\nlive\n", nil
		},
		inspectCont: func(_ context.Context, containerID string) (backend.ContainerInspect, error) {
			switch containerID {
			case "missing":
				return backend.ContainerInspect{}, &backenddocker.Error{Stderr: "No such object", Err: os.ErrNotExist}
			case "dead":
				return backend.ContainerInspect{ID: "dead", State: backend.ContainerState{Status: "exited", Running: false}}, nil
			case "live":
				return backend.ContainerInspect{ID: "live", State: backend.ContainerState{Status: "running", Running: true}}, nil
			default:
				t.Fatalf("unexpected inspect %q", containerID)
				return backend.ContainerInspect{}, nil
			}
		},
	}
	observed, err := NewObserver(backend).Observe(context.Background(), ObserveRequest{
		Plan: workspaceplan.WorkspacePlan{LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir, CacheDir: cacheDir}},
		Resolved: devcontainer.ResolvedConfig{
			StateDir:      stateDir,
			CacheDir:      cacheDir,
			ContainerName: "hatchctl-demo",
			SourceKind:    "image",
			ImageName:     "hatchctl-demo-image",
			Labels:        map[string]string{"a": "1", "b": "2"},
		},
		LoadControlState: true,
		ObserveTarget:    true,
		InspectTarget:    true,
	})
	if err != nil {
		t.Fatalf("observe managed target: %v", err)
	}
	if observed.Target.Kind != TargetKindManagedContainer || observed.Target.PrimaryContainer != "live" {
		t.Fatalf("unexpected target %#v", observed.Target)
	}
	if observed.Container == nil || observed.Container.ID != "live" {
		t.Fatalf("unexpected primary container %#v", observed.Container)
	}
	if observed.Control.WorkspaceState.ContainerID != "live" || observed.Control.WorkspaceState.LifecycleReady {
		t.Fatalf("unexpected normalized state %#v", observed.Control.WorkspaceState)
	}
	if !observed.Control.PlanCacheExists || observed.Control.Coordination.Generation == 0 {
		t.Fatalf("expected combined control state, got %#v", observed.Control)
	}
	if observed.ReadTarget.PrimaryContainer != "live" || observed.ReadTarget.CoordinationGeneration != observed.Control.Coordination.Generation {
		t.Fatalf("unexpected read token %#v", observed.ReadTarget)
	}
}

func TestObserveComposeTargetIncludesProjectContainers(t *testing.T) {
	t.Parallel()

	backend := fakeBackend{
		projectContainers: func(_ context.Context, req backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error) {
			app := backend.ContainerInspect{ID: "app", State: backend.ContainerState{Status: "running", Running: true}}
			db := backend.ContainerInspect{ID: "db", State: backend.ContainerState{Status: "running", Running: true}}
			return []backend.ContainerInspect{db, app}, &app, nil
		},
	}
	observed, err := NewObserver(backend).Observe(context.Background(), ObserveRequest{
		Resolved:      devcontainer.ResolvedConfig{SourceKind: "compose", ComposeProject: "demo", ComposeService: "app", ContainerName: "hatchctl-demo", ImageName: "demo-app"},
		ObserveTarget: true,
	})
	if err != nil {
		t.Fatalf("observe compose target: %v", err)
	}
	if observed.Target.Kind != TargetKindComposeService || observed.Target.PrimaryContainer != "app" {
		t.Fatalf("unexpected target %#v", observed.Target)
	}
	if len(observed.Target.Containers) != 2 {
		t.Fatalf("expected project containers in observation, got %#v", observed.Target.Containers)
	}
}

func TestRevalidateReadTokenDetectsCoordinationChanges(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	backend := fakeBackend{
		inspectCont: func(_ context.Context, containerID string) (backend.ContainerInspect, error) {
			return backend.ContainerInspect{ID: containerID}, nil
		},
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	observer := NewObserver(backend)
	observed, err := observer.Observe(context.Background(), ObserveRequest{
		Resolved:         devcontainer.ResolvedConfig{StateDir: stateDir},
		LoadControlState: true,
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	observed.ReadTarget.PrimaryContainer = "container-123"
	lock, err = storefs.AcquireWorkspaceLock(context.Background(), stateDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := observer.RevalidateReadToken(context.Background(), observed); !errors.Is(err, ErrObservedStateStale) {
		t.Fatalf("expected stale observed state, got %v", err)
	}
}

func TestRevalidateReadTokenDetectsManagedContainerIdentityChanges(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	observer := NewObserver(fakeBackend{
		inspectCont: func(_ context.Context, containerID string) (backend.ContainerInspect, error) {
			return backend.ContainerInspect{ID: containerID, Name: "/other-container"}, nil
		},
	})
	observed := ObservedState{
		Resolved: devcontainer.ResolvedConfig{StateDir: stateDir},
		ReadTarget: ReadToken{
			TargetKind:             TargetKindManagedContainer,
			ContainerName:          "expected-container",
			PrimaryContainer:       "container-123",
			CoordinationGeneration: 1,
		},
	}

	if err := observer.RevalidateReadToken(context.Background(), observed); !errors.Is(err, ErrObservedStateStale) {
		t.Fatalf("expected stale observed state, got %v", err)
	}
}

func TestRevalidateReadTokenDetectsComposeIdentityChanges(t *testing.T) {
	t.Parallel()

	observer := NewObserver(fakeBackend{
		projectContainers: func(_ context.Context, req backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error) {
			db := backend.ContainerInspect{ID: "container-123"}
			return []backend.ContainerInspect{db}, nil, nil
		},
	})
	observed := ObservedState{
		Resolved: devcontainer.ResolvedConfig{StateDir: t.TempDir()},
		ReadTarget: ReadToken{
			TargetKind:       TargetKindComposeService,
			ComposeProject:   "demo",
			ComposeService:   "app",
			PrimaryContainer: "container-123",
		},
	}

	if err := observer.RevalidateReadToken(context.Background(), observed); !errors.Is(err, ErrObservedStateStale) {
		t.Fatalf("expected stale observed state, got %v", err)
	}
}
