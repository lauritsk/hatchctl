package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesParentAndReplacesContents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := WriteFileAtomic(path, []byte(`{"old":true}`), WriteOptions{Mode: 0o600, DirMode: 0o700}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteFileAtomic(path, []byte(`{"new":true}`), WriteOptions{Mode: 0o600, DirMode: 0o700}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if got := string(data); got != `{"new":true}` {
		t.Fatalf("unexpected file contents %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("unexpected file mode %o", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("unexpected dir mode %o", got)
	}
}
