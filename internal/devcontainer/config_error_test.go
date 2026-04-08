package devcontainer

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathSuggestsLocations(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	_, err := resolveConfigPath(workspace, "")
	if err == nil {
		t.Fatal("expected missing config error")
	}
	message := err.Error()
	if !strings.Contains(message, "no devcontainer config found") {
		t.Fatalf("expected missing config message, got %q", message)
	}
	if !strings.Contains(message, filepath.Join(workspace, ".devcontainer", "devcontainer.json")) {
		t.Fatalf("expected .devcontainer path in error, got %q", message)
	}
	if !strings.Contains(message, filepath.Join(workspace, ".devcontainer.json")) {
		t.Fatalf("expected root devcontainer path in error, got %q", message)
	}
	if !strings.Contains(message, "rerun with --config <path>") {
		t.Fatalf("expected recovery guidance, got %q", message)
	}
}
