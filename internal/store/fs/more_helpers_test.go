package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBridgeHelperWritersAndParsers(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	paths, err := EnsureWorkspaceBridgePaths(stateDir)
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	helperPath := filepath.Join(paths.BinDir, "devcontainer-open")
	if err := WriteBridgeExecutable(helperPath, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("write bridge executable: %v", err)
	}
	assertMode(t, helperPath, 0o755)
	if err := WriteBridgePID(paths.PIDPath, 1234); err != nil {
		t.Fatalf("write bridge pid: %v", err)
	}
	assertMode(t, paths.PIDPath, 0o600)
	if pid, err := ReadBridgePID(paths.PIDPath); err != nil || pid != 1234 {
		t.Fatalf("unexpected bridge pid %d err=%v", pid, err)
	}
	if err := WriteBridgeConfig(paths.ConfigPath, map[string]any{"host": "127.0.0.1", "port": 7777}); err != nil {
		t.Fatalf("write bridge config: %v", err)
	}
	assertMode(t, paths.ConfigPath, 0o600)
	if data, err := os.ReadFile(paths.ConfigPath); err != nil || !strings.Contains(string(data), "127.0.0.1") {
		t.Fatalf("unexpected bridge config %q err=%v", string(data), err)
	}
	if err := os.WriteFile(paths.SessionPath, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("seed invalid bridge session: %v", err)
	}
	if _, err := ReadBridgeSession[map[string]any](paths.Dir); err == nil {
		t.Fatal("expected invalid bridge session json to fail")
	}
}

func TestCoordinationReadAndReleaseHelpers(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	lock, err := AcquireWorkspaceLock(t.Context(), stateDir, "test")
	if err != nil {
		t.Fatalf("acquire workspace lock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release workspace lock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release workspace lock idempotently: %v", err)
	}
	if err := CheckWorkspaceBusy(stateDir); err != nil {
		t.Fatalf("expected lock to be clear after release, got %v", err)
	}
	coordinationPath := filepath.Join(stateDir, "coordination.json")
	if err := os.WriteFile(coordinationPath, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("seed invalid coordination: %v", err)
	}
	if _, err := ReadCoordination(stateDir); err == nil {
		t.Fatal("expected invalid coordination json to fail")
	}
	if leaseExpired(nil, time.Now()) != true {
		t.Fatal("expected nil owner lease to be expired")
	}
	if sameHost("demo", "other") {
		t.Fatal("expected different hosts not to match")
	}
	if processRunning(-1) {
		t.Fatal("expected invalid pid not to be running")
	}
}

func TestASCIIWhitespaceHelpers(t *testing.T) {
	t.Parallel()

	if got := trimASCIIWhitespace([]byte(" \t\n demo \r\n")); got != "demo" {
		t.Fatalf("unexpected trimmed value %q", got)
	}
	for _, b := range []byte{' ', '\t', '\n', '\r'} {
		if !isASCIIWhitespace(b) {
			t.Fatalf("expected %q to be recognized as whitespace", b)
		}
	}
	if isASCIIWhitespace('x') {
		t.Fatal("expected non-whitespace byte to be rejected")
	}
}
