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
	lockDir  string
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
	lockDir := filepath.Join(stateDir, "lock")
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			record, readErr := readCoordination(stateDir)
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				_ = os.Remove(lockDir)
				return nil, readErr
			}
			record.Version = coordinationVersion
			record.Generation++
			record.ActiveOwner = &owner
			if err := writeCoordination(stateDir, record); err != nil {
				_ = os.Remove(lockDir)
				return nil, err
			}
			leaseCtx, cancel := context.WithCancel(context.Background())
			handle := &WorkspaceLock{stateDir: stateDir, lockDir: lockDir, owner: owner, cancel: cancel, done: make(chan struct{})}
			go handle.renew(leaseCtx)
			return handle, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		record, readErr := readCoordination(stateDir)
		if readErr != nil {
			return nil, readErr
		}
		if record.ActiveOwner != nil && !leaseExpired(record.ActiveOwner, now) {
			return nil, &WorkspaceBusyError{StateDir: stateDir, Owner: record.ActiveOwner}
		}
		if record.ActiveOwner != nil && sameHost(record.ActiveOwner.Hostname, hostname) && processRunning(record.ActiveOwner.PID) {
			return nil, &WorkspaceBusyError{StateDir: stateDir, Owner: record.ActiveOwner}
		}
		if err := os.Remove(lockDir); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		now = time.Now().UTC()
		owner.UpdatedAt = now
		owner.LeaseExpiresAt = now.Add(leaseDuration)
	}
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
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if record.ActiveOwner != nil && record.ActiveOwner.OwnerID == l.owner.OwnerID {
		record.ActiveOwner = nil
		record.Version = coordinationVersion
		if err := writeCoordination(l.stateDir, record); err != nil {
			return err
		}
	}
	if err := os.Remove(l.lockDir); err != nil && !os.IsNotExist(err) {
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
