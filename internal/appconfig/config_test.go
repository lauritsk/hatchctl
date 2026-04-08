package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadForWorkspaceMergesUserAndWorkspaceConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	userDir := filepath.Join(configRoot, "hatchctl")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".hatchctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "config.toml"), []byte("workspace = \"/user/workspace\"\nlockfile_policy = \"update\"\nfeature_timeout = \"45s\"\n[dotfiles]\nrepository = \"github.com/example/user\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hatchctl", "config.toml"), []byte("config = \"../custom/devcontainer.json\"\nbridge = true\nssh = true\n[dotfiles]\ntarget_path = \"~/dotfiles\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadForWorkspace(workspace)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Workspace != "/user/workspace" {
		t.Fatalf("unexpected workspace %q", config.Workspace)
	}
	if config.ConfigPath != filepath.Join(workspace, "custom", "devcontainer.json") {
		t.Fatalf("unexpected config path %q", config.ConfigPath)
	}
	if config.LockfilePolicy != "update" || config.FeatureTimeout != "45s" {
		t.Fatalf("unexpected merged config %#v", config)
	}
	if config.Bridge == nil || !*config.Bridge {
		t.Fatalf("expected workspace bridge override, got %#v", config.Bridge)
	}
	if config.SSHAgent == nil || !*config.SSHAgent {
		t.Fatalf("expected workspace ssh override, got %#v", config.SSHAgent)
	}
	if config.Dotfiles.Repository != "github.com/example/user" || config.Dotfiles.TargetPath != "~/dotfiles" {
		t.Fatalf("unexpected merged dotfiles %#v", config.Dotfiles)
	}
}
