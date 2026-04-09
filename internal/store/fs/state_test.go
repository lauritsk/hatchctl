package fs

import (
	"os"
	"path/filepath"
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
