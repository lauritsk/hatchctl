package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWorkspace(t *testing.T) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return workspace, configDir
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixtureWorkspace(t *testing.T, fixture string) string {
	t.Helper()
	source := filepath.Join("testdata", fixture)
	workspace := t.TempDir()
	if err := filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(workspace, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture workspace %s: %v", fixture, err)
	}
	return workspace
}

func rewriteFixturePlaceholders(t *testing.T, path string, replacements map[string]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture file %s: %v", path, err)
	}
	contents := string(data)
	for oldValue, newValue := range replacements {
		contents = strings.ReplaceAll(contents, oldValue, newValue)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("rewrite fixture file %s: %v", path, err)
	}
}
