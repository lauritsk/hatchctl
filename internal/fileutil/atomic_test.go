package fileutil

import (
	"os"
	"path/filepath"
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
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no temp file, got err=%v", err)
	}
}

func TestReadFileRecoversTempFileWhenPrimaryMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bridge-status.json")
	if err := os.WriteFile(path+".tmp", []byte("recover"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	data, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read recovered file: %v", err)
	}
	if string(data) != "recover" {
		t.Fatalf("unexpected recovered contents %q", string(data))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected recovered primary file: %v", err)
	}
}

func TestRemoveFileRemovesPrimaryAndTemp(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resolved-plan.json")
	if err := os.WriteFile(path, []byte("main"), 0o600); err != nil {
		t.Fatalf("write primary file: %v", err)
	}
	if err := os.WriteFile(path+".tmp", []byte("tmp"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	if err := RemoveFile(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected primary file removed, got err=%v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed, got err=%v", err)
	}
}
