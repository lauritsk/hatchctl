package fileutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileWritesAtomicallyWithPermissions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFile(path, []byte("ok\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("unexpected file contents %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("unexpected file mode %#o", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("expected no temp files, found %q", entry.Name())
		}
	}
}

func TestReadFileReturnsNotExistWhenPrimaryMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bridge-status.json")
	if _, err := ReadFile(path); !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestRemoveFileRemovesPrimary(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resolved-plan.json")
	if err := os.WriteFile(path, []byte("main"), 0o600); err != nil {
		t.Fatalf("write primary file: %v", err)
	}

	if err := RemoveFile(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected primary file removed, got err=%v", err)
	}
}
