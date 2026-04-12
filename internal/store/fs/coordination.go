package fs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

const (
	coordinationVersion = 1
	leaseDuration       = 15 * time.Second
	renewInterval       = 5 * time.Second
)

type CoordinationRecord struct {
	Version     int          `json:"version"`
	Generation  int64        `json:"generation"`
	ActiveOwner *ActiveOwner `json:"activeOwner,omitempty"`
}

type ActiveOwner struct {
	OwnerID        string    `json:"ownerId"`
	Command        string    `json:"command"`
	PID            int       `json:"pid"`
	Hostname       string    `json:"hostname"`
	StartedAt      time.Time `json:"startedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt"`
}

type WorkspaceBusyError struct {
	StateDir string
	Owner    *ActiveOwner
}

func (e *WorkspaceBusyError) Error() string {
	if e.Owner == nil {
		return fmt.Sprintf("workspace is busy: %s", e.StateDir)
	}
	return fmt.Sprintf("workspace is busy: %s (command=%s pid=%d host=%s owner=%s)", e.StateDir, e.Owner.Command, e.Owner.PID, e.Owner.Hostname, e.Owner.OwnerID)
}

type WorkspaceLock struct {
	stateDir string
	lockFile *os.File
	owner    ActiveOwner

	mu       sync.Mutex
	released bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func AcquireWorkspaceLock(ctx context.Context, stateDir string, command string) (*WorkspaceLock, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	owner := ActiveOwner{
		OwnerID:        fmt.Sprintf("%s:%d:%s", hostname, os.Getpid(), now.Format(time.RFC3339Nano)),
		Command:        command,
		PID:            os.Getpid(),
		Hostname:       hostname,
		StartedAt:      now,
		UpdatedAt:      now,
		LeaseExpiresAt: now.Add(leaseDuration),
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lockFile, err := openWorkspaceLockFile(filepath.Join(stateDir, "lock"))
	if err != nil {
		return nil, err
	}
	if err := tryExclusiveLock(lockFile); err != nil {
		_ = lockFile.Close()
		if isLockBusy(err) {
			owner, ownerErr := activeWorkspaceOwner(stateDir)
			if ownerErr != nil {
				return nil, &WorkspaceBusyError{StateDir: stateDir}
			}
			return nil, &WorkspaceBusyError{StateDir: stateDir, Owner: owner}
		}
		return nil, err
	}
	record, readErr := readCoordination(stateDir)
	if readErr != nil {
		record = CoordinationRecord{Version: coordinationVersion}
	}
	record.Version = coordinationVersion
	record.Generation++
	record.ActiveOwner = &owner
	if err := writeCoordination(stateDir, record); err != nil {
		_ = unlockWorkspaceFile(lockFile)
		_ = lockFile.Close()
		return nil, err
	}
	leaseCtx, cancel := context.WithCancel(context.Background())
	handle := &WorkspaceLock{stateDir: stateDir, lockFile: lockFile, owner: owner, cancel: cancel, done: make(chan struct{})}
	go handle.renew(leaseCtx)
	return handle, nil
}

func CheckWorkspaceBusy(stateDir string) error {
	if _, err := os.Stat(stateDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lockFile, err := openWorkspaceLockFile(filepath.Join(stateDir, "lock"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer lockFile.Close()
	if err := tryExclusiveLock(lockFile); err != nil {
		if !isLockBusy(err) {
			return err
		}
		owner, ownerErr := activeWorkspaceOwner(stateDir)
		if ownerErr != nil {
			return &WorkspaceBusyError{StateDir: stateDir}
		}
		return &WorkspaceBusyError{StateDir: stateDir, Owner: owner}
	}
	return unlockWorkspaceFile(lockFile)
}

func ReadCoordination(stateDir string) (CoordinationRecord, error) {
	return readCoordination(stateDir)
}

func (l *WorkspaceLock) Release() error {
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		return nil
	}
	l.released = true
	l.mu.Unlock()

	if l.cancel != nil {
		l.cancel()
	}
	if l.done != nil {
		<-l.done
	}

	record, err := readCoordination(l.stateDir)
	if err != nil {
		record = CoordinationRecord{Version: coordinationVersion}
	}
	if record.ActiveOwner != nil && record.ActiveOwner.OwnerID == l.owner.OwnerID {
		record.ActiveOwner = nil
		record.Version = coordinationVersion
		if err := writeCoordination(l.stateDir, record); err != nil {
			_ = unlockWorkspaceFile(l.lockFile)
			_ = l.lockFile.Close()
			return err
		}
	}
	if err := unlockWorkspaceFile(l.lockFile); err != nil {
		_ = l.lockFile.Close()
		return err
	}
	if err := l.lockFile.Close(); err != nil {
		return err
	}
	return nil
}

func (l *WorkspaceLock) renew(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			record, err := readCoordination(l.stateDir)
			if err != nil || record.ActiveOwner == nil || record.ActiveOwner.OwnerID != l.owner.OwnerID {
				continue
			}
			now := time.Now().UTC()
			record.ActiveOwner.UpdatedAt = now
			record.ActiveOwner.LeaseExpiresAt = now.Add(leaseDuration)
			record.Version = coordinationVersion
			_ = writeCoordination(l.stateDir, record)
		}
	}
}

func readCoordination(stateDir string) (CoordinationRecord, error) {
	path := filepath.Join(stateDir, "coordination.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CoordinationRecord{Version: coordinationVersion}, err
		}
		return CoordinationRecord{}, err
	}
	var record CoordinationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return CoordinationRecord{}, err
	}
	if record.Version == 0 {
		record.Version = coordinationVersion
	}
	return record, nil
}

func writeCoordination(stateDir string, record CoordinationRecord) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(filepath.Join(stateDir, "coordination.json"), data, 0o600)
}

func activeWorkspaceOwner(stateDir string) (*ActiveOwner, error) {
	record, err := readCoordination(stateDir)
	if err != nil {
		return nil, err
	}
	return record.ActiveOwner, nil
}

func openWorkspaceLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}

func tryExclusiveLock(file *os.File) error {
	if file == nil {
		return fmt.Errorf("workspace lock file is nil")
	}
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockWorkspaceFile(file *os.File) error {
	if file == nil {
		return nil
	}
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func leaseExpired(owner *ActiveOwner, now time.Time) bool {
	if owner == nil {
		return true
	}
	return owner.LeaseExpiresAt.Before(now)
}

func sameHost(a string, b string) bool {
	return a != "" && a == b
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
