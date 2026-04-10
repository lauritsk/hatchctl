package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestBuildReturnsBusyWhenWorkspaceLockIsHeld(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)
	workspacePlan, err := buildWorkspacePlan(defaults, devcontainer.FeatureLockfilePolicyAuto, false, false, false, DotfilesOptions{}, false, false)
	if err != nil {
		t.Fatalf("build workspace plan: %v", err)
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), workspacePlan.LockProtected.StateDir, "up")
	if err != nil {
		t.Fatalf("seed workspace lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	service := New(&reconcile.Executor{})

	_, err = service.Build(context.Background(), BuildRequest{Defaults: defaults})
	var busyErr *storefs.WorkspaceBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("expected workspace busy error, got %v", err)
	}
}

func TestExecReturnsBusyWhenWorkspaceLockIsHeld(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)
	workspacePlan, err := buildWorkspacePlan(defaults, devcontainer.FeatureLockfilePolicyAuto, false, false, false, DotfilesOptions{}, false, false)
	if err != nil {
		t.Fatalf("build workspace plan: %v", err)
	}
	lock, err := storefs.AcquireWorkspaceLock(context.Background(), workspacePlan.LockProtected.StateDir, "build")
	if err != nil {
		t.Fatalf("seed workspace lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	service := New(&reconcile.Executor{})

	_, err = service.Exec(context.Background(), ExecRequest{Defaults: defaults})
	var busyErr *storefs.WorkspaceBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("expected workspace busy error, got %v", err)
	}
}

func TestReadConfigRejectsInvalidLockfilePolicy(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)
	defaults.LockfilePolicy = "bogus"

	service := New(&reconcile.Executor{})
	_, err := service.ReadConfig(context.Background(), ReadConfigRequest{Defaults: defaults})
	if err == nil {
		t.Fatal("expected invalid lockfile policy error")
	}
}

func TestRunLifecycleRejectsInvalidPhase(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)

	service := New(&reconcile.Executor{})
	_, err := service.RunLifecycle(context.Background(), RunLifecycleRequest{Defaults: defaults, Phase: "bogus"})
	if !errors.Is(err, reconcile.ErrInvalidLifecyclePhase) {
		t.Fatalf("expected invalid lifecycle phase error, got %v", err)
	}
}

func TestBridgeDoctorRejectsInvalidLockfilePolicy(t *testing.T) {
	t.Parallel()

	_, defaults := testWorkspaceDefaults(t)
	defaults.LockfilePolicy = "bogus"

	service := New(&reconcile.Executor{})
	_, err := service.BridgeDoctor(context.Background(), BridgeDoctorRequest{Defaults: defaults})
	if err == nil {
		t.Fatal("expected invalid lockfile policy error")
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
	if err := os.WriteFile(configPath, []byte(`{"image":"alpine:3.23","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, CommandDefaults{Workspace: workspace, ConfigPath: configPath, LockfilePolicy: "auto"}
}
