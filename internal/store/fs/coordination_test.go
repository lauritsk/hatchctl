package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/fileutil"
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
	if _, err := os.Stat(filepath.Join(stateDir, "lock")); !os.IsNotExist(err) {
		t.Fatalf("expected lock directory removal, got %v", err)
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

func TestAcquireWorkspaceLockRecoversExpiredStaleLock(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	if err := os.Mkdir(filepath.Join(stateDir, "lock"), 0o700); err != nil {
		t.Fatalf("seed lock dir: %v", err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	data := []byte(`{
	  "version": 1,
	  "generation": 7,
	  "activeOwner": {
	    "ownerId": "stale-owner",
	    "command": "up",
	    "pid": 999999,
	    "hostname": "` + hostname + `",
	    "startedAt": "` + expired.Format(time.RFC3339Nano) + `",
	    "updatedAt": "` + expired.Format(time.RFC3339Nano) + `",
	    "leaseExpiresAt": "` + expired.Format(time.RFC3339Nano) + `"
	  }
	}`)
	if err := fileutil.WriteFile(filepath.Join(stateDir, "coordination.json"), data, 0o600); err != nil {
		t.Fatalf("seed coordination record: %v", err)
	}

	lock, err := AcquireWorkspaceLock(context.Background(), stateDir, "build")
	if err != nil {
		t.Fatalf("recover stale lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})
	record, err := ReadCoordination(stateDir)
	if err != nil {
		t.Fatalf("read coordination: %v", err)
	}
	if record.Generation != 8 {
		t.Fatalf("expected generation bump to 8, got %d", record.Generation)
	}
	if record.ActiveOwner == nil || record.ActiveOwner.Command != "build" {
		t.Fatalf("unexpected new owner %#v", record.ActiveOwner)
	}
}
