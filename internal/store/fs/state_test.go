package fs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnsureWorkspaceStateDirUsesOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(t.TempDir(), "state")
	if err := EnsureWorkspaceStateDir(stateDir); err != nil {
		t.Fatalf("ensure workspace state dir: %v", err)
	}
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("unexpected mode for %s: got %o want 700", stateDir, got)
	}
}

func TestReadWorkspaceStateReturnsEmptyStateWhenMissing(t *testing.T) {
	t.Parallel()

	state, err := ReadWorkspaceState(t.TempDir())
	if err != nil {
		t.Fatalf("read missing state: %v", err)
	}
	if !reflect.DeepEqual(state, WorkspaceState{}) {
		t.Fatalf("expected empty state, got %#v", state)
	}
}

func TestWriteWorkspaceStateRoundTripsAndSetsSchemaVersion(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	want := WorkspaceState{
		ContainerID:     "container-123",
		ContainerKey:    "container-key",
		TrustedRefs:     []string{"ghcr.io/example/feature@sha256:abc123"},
		LifecycleReady:  true,
		LifecycleKey:    "lifecycle-key",
		BridgeEnabled:   true,
		BridgeSessionID: "bridge-session",
		DotfilesReady:   true,
		DotfilesRepo:    "https://github.com/example/dotfiles.git",
		DotfilesTarget:  "/home/vscode/.dotfiles",
	}

	if err := WriteWorkspaceState(stateDir, want); err != nil {
		t.Fatalf("write workspace state: %v", err)
	}

	got, err := ReadWorkspaceState(stateDir)
	if err != nil {
		t.Fatalf("read workspace state: %v", err)
	}
	if got.Version != workspaceStateSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", workspaceStateSchemaVersion, got.Version)
	}
	want.Version = workspaceStateSchemaVersion
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected round trip state: got %#v want %#v", got, want)
	}
}

func TestReadWorkspaceStateAcceptsJSONC(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	path := filepath.Join(stateDir, "state.json")
	data := []byte("{\n  // comment\n  \"containerId\": \"container-123\",\n  \"trustedRefs\": [\"ghcr.io/example/image@sha256:def456\"]\n}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed jsonc state: %v", err)
	}

	got, err := ReadWorkspaceState(stateDir)
	if err != nil {
		t.Fatalf("read workspace state: %v", err)
	}
	if got.ContainerID != "container-123" {
		t.Fatalf("expected container id to round trip, got %#v", got)
	}
	if len(got.TrustedRefs) != 1 || got.TrustedRefs[0] != "ghcr.io/example/image@sha256:def456" {
		t.Fatalf("unexpected trusted refs %#v", got.TrustedRefs)
	}
	if got.Version != workspaceStateSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", workspaceStateSchemaVersion, got.Version)
	}
}
