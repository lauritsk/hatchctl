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
