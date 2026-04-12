package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireWorkspaceLockWritesAndClearsCoordination(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	lock, err := AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	record, err := ReadCoordination(stateDir)
	if err != nil {
		t.Fatalf("read coordination: %v", err)
	}
	if record.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", record.Generation)
	}
	if record.ActiveOwner == nil || record.ActiveOwner.Command != "up" {
		t.Fatalf("unexpected active owner %#v", record.ActiveOwner)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	record, err = ReadCoordination(stateDir)
	if err != nil {
		t.Fatalf("read coordination after release: %v", err)
	}
	if record.ActiveOwner != nil {
		t.Fatalf("expected active owner to be cleared, got %#v", record.ActiveOwner)
	}
	if err := CheckWorkspaceBusy(stateDir); err != nil {
		t.Fatalf("expected no active lock after release, got %v", err)
	}
}

func TestAcquireWorkspaceLockRejectsConcurrentMutation(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	lock, err := AcquireWorkspaceLock(context.Background(), stateDir, "build")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	_, err = AcquireWorkspaceLock(context.Background(), stateDir, "up")
	var busyErr *WorkspaceBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("expected busy error, got %v", err)
	}
	if busyErr.Owner == nil || busyErr.Owner.Command != "build" {
		t.Fatalf("unexpected busy owner %#v", busyErr.Owner)
	}
}

func TestAcquireWorkspaceLockReturnsBusyWhenCoordinationIsMissing(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	lock, err := AcquireWorkspaceLock(context.Background(), stateDir, "up")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})
	if err := os.Remove(filepath.Join(stateDir, "coordination.json")); err != nil {
		t.Fatalf("remove coordination record: %v", err)
	}

	_, err = AcquireWorkspaceLock(context.Background(), stateDir, "build")
	var busyErr *WorkspaceBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("expected busy error with missing coordination, got %v", err)
	}
}

func TestAcquireWorkspaceLockRecoversInvalidCoordination(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "coordination.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("seed invalid coordination record: %v", err)
	}

	lock, err := AcquireWorkspaceLock(context.Background(), stateDir, "build")
	if err != nil {
		t.Fatalf("recover invalid coordination: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})
	record, err := ReadCoordination(stateDir)
	if err != nil {
		t.Fatalf("read coordination: %v", err)
	}
	if record.Generation != 1 {
		t.Fatalf("expected generation reset to 1, got %d", record.Generation)
	}
	if record.ActiveOwner == nil || record.ActiveOwner.Command != "build" {
		t.Fatalf("unexpected new owner %#v", record.ActiveOwner)
	}
}
