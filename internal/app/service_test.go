package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/runtime"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type stubRuntime struct {
	build func(context.Context, runtime.BuildOptions) (runtime.BuildResult, error)
	exec  func(context.Context, runtime.ExecOptions) (int, error)
}

func (s stubRuntime) Up(context.Context, runtime.UpOptions) (runtime.UpResult, error) {
	panic("unexpected Up call")
}

func (s stubRuntime) Build(ctx context.Context, opts runtime.BuildOptions) (runtime.BuildResult, error) {
	if s.build == nil {
		panic("unexpected Build call")
	}
	return s.build(ctx, opts)
}

func (s stubRuntime) Exec(ctx context.Context, opts runtime.ExecOptions) (int, error) {
	if s.exec == nil {
		panic("unexpected Exec call")
	}
	return s.exec(ctx, opts)
}

func (s stubRuntime) ReadConfig(context.Context, runtime.ReadConfigOptions) (runtime.ReadConfigResult, error) {
	panic("unexpected ReadConfig call")
}

func (s stubRuntime) RunLifecycle(context.Context, runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error) {
	panic("unexpected RunLifecycle call")
}

func (s stubRuntime) BridgeDoctor(context.Context, runtime.BridgeDoctorOptions) (bridge.Report, error) {
	panic("unexpected BridgeDoctor call")
}

func TestBuildReturnsBusyWhenWorkspaceLockIsHeld(t *testing.T) {
	t.Parallel()

	workspace, defaults := testWorkspaceDefaults(t)
	stateDir, err := mutationStateDir(defaults)
	if err != nil {
		t.Fatalf("compute mutation state dir: %v", err)
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatalf("seed workspace lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	called := false
	service := New(stubRuntime{build: func(_ context.Context, opts runtime.BuildOptions) (runtime.BuildResult, error) {
		called = true
		if opts.Workspace != workspace {
			t.Fatalf("unexpected workspace %q", opts.Workspace)
		}
		return runtime.BuildResult{}, nil
	}})

	_, err = service.Build(context.Background(), BuildRequest{Defaults: defaults, IO: CommandIO{Stdout: io.Discard, Stderr: io.Discard}})
	var busyErr *storefs.WorkspaceBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("expected workspace busy error, got %v", err)
	}
	if called {
		t.Fatal("expected build runner to be skipped while lock is held")
	}
}

func TestExecBypassesWorkspaceMutationLock(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)
	stateDir, err := mutationStateDir(defaults)
	if err != nil {
		t.Fatalf("compute mutation state dir: %v", err)
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), stateDir, "build")
	if err != nil {
		t.Fatalf("seed workspace lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	called := false
	service := New(stubRuntime{exec: func(_ context.Context, opts runtime.ExecOptions) (int, error) {
		called = true
		if opts.Workspace != defaults.Workspace {
			t.Fatalf("unexpected workspace %q", opts.Workspace)
		}
		return 0, nil
	}})

	if _, err := service.Exec(context.Background(), ExecRequest{Defaults: defaults, IO: CommandIO{Stdout: io.Discard, Stderr: io.Discard}}); err != nil {
		t.Fatalf("exec with active mutation lock: %v", err)
	}
	if !called {
		t.Fatal("expected exec runner to be called without taking the exclusive lock")
	}
}

func testWorkspaceDefaults(t *testing.T) (string, CommandDefaults) {
	t.Helper()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, CommandDefaults{Workspace: workspace, ConfigPath: configPath, LockfilePolicy: "auto"}
}
