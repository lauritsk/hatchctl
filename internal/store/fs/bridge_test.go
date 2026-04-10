package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureWorkspaceBridgePathsCreatesExpectedLayout(t *testing.T) {
	t.Parallel()

	paths, err := EnsureWorkspaceBridgePaths(t.TempDir())
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}

	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("unexpected mode for %s: got %o want %o", path, got, want)
		}
	}

	assertMode(paths.Dir, 0o700)
	assertMode(paths.BinDir, 0o755)
	if paths.SessionPath != filepath.Join(paths.Dir, "session.json") {
		t.Fatalf("unexpected session path %q", paths.SessionPath)
	}
	if paths.StatusPath != filepath.Join(paths.Dir, "bridge-status.json") {
		t.Fatalf("unexpected status path %q", paths.StatusPath)
	}
}

func TestBridgeSessionRoundTrips(t *testing.T) {
	t.Parallel()

	paths, err := EnsureWorkspaceBridgePaths(t.TempDir())
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	type session struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	want := session{ID: "session-123", Enabled: true}
	if err := WriteBridgeSession(paths.Dir, want); err != nil {
		t.Fatalf("write bridge session: %v", err)
	}

	got, err := ReadBridgeSession[session](paths.Dir)
	if err != nil {
		t.Fatalf("read bridge session: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("unexpected bridge session: got %#v want %#v", got, want)
	}

	info, err := os.Stat(paths.SessionPath)
	if err != nil {
		t.Fatalf("stat session path: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("unexpected mode for %s: got %o want 600", paths.SessionPath, got)
	}
}

func TestReadBridgePIDTrimsWhitespaceAndInvalidContent(t *testing.T) {
	t.Parallel()

	pidPath := filepath.Join(t.TempDir(), "bridge.pid")
	if err := os.WriteFile(pidPath, []byte(" \n 1234\t\n"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	pid, err := ReadBridgePID(pidPath)
	if err != nil {
		t.Fatalf("read bridge pid: %v", err)
	}
	if pid != 1234 {
		t.Fatalf("expected trimmed pid 1234, got %d", pid)
	}

	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatalf("write invalid pid file: %v", err)
	}
	pid, err = ReadBridgePID(pidPath)
	if err != nil {
		t.Fatalf("read invalid pid file: %v", err)
	}
	if pid != 0 {
		t.Fatalf("expected invalid pid content to return 0, got %d", pid)
	}
}

func TestWriteBridgeStatusCreatesParentDir(t *testing.T) {
	t.Parallel()

	statusPath := filepath.Join(t.TempDir(), "bridge", "bridge-status.json")
	if err := WriteBridgeStatus(statusPath, map[string]any{"lastEvent": "running"}); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}
	data, err := ReadBridgeStatus(statusPath)
	if err != nil {
		t.Fatalf("read bridge status: %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected bridge status data to be written")
	}
	info, err := os.Stat(statusPath)
	if err != nil {
		t.Fatalf("stat bridge status: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("unexpected mode for %s: got %o want 600", statusPath, got)
	}
}
