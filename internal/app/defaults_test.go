package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveDefaultsIgnoresUntrustedWorkspaceHostSettings(t *testing.T) {
	workspace := writeConfigFixtures(t)

	got, err := ResolveDefaults(resolveDefaultsRequestForTest(workspace, false))
	if err != nil {
		t.Fatalf("resolve defaults: %v", err)
	}

	if got.ConfigPath != filepath.Join(workspace, "custom", "devcontainer.json") {
		t.Fatalf("unexpected config path %q", got.ConfigPath)
	}
	if got.StateDir != "/user/state" || got.CacheDir != "/user/cache" {
		t.Fatalf("expected untrusted workspace paths to keep user defaults, got state=%q cache=%q", got.StateDir, got.CacheDir)
	}
	if got.BridgeEnabled {
		t.Fatalf("expected untrusted workspace config to leave bridge disabled, got %#v", got)
	}
	if got.SSHAgent {
		t.Fatalf("expected untrusted workspace config to leave ssh disabled, got %#v", got)
	}
	if got.Dotfiles.Repository != "github.com/example/user" || got.Dotfiles.InstallCommand != "user-install" || got.Dotfiles.TargetPath != "~/user-dotfiles" {
		t.Fatalf("unexpected untrusted dotfiles defaults %#v", got.Dotfiles)
	}
	if len(got.TrustedSigners) != 1 || got.TrustedSigners[0].Subject != "user@example.com" {
		t.Fatalf("unexpected untrusted trusted signers %#v", got.TrustedSigners)
	}
	if got.FeatureTimeout != 45*time.Second || got.LockfilePolicy != "update" {
		t.Fatalf("unexpected non-host defaults %#v", got)
	}
}

func TestResolveDefaultsAppliesTrustedWorkspaceHostSettings(t *testing.T) {
	workspace := writeConfigFixtures(t)

	got, err := ResolveDefaults(resolveDefaultsRequestForTest(workspace, true))
	if err != nil {
		t.Fatalf("resolve defaults: %v", err)
	}

	if !got.BridgeEnabled || !got.SSHAgent {
		t.Fatalf("expected trusted workspace host settings, got %#v", got)
	}
	if got.StateDir != filepath.Join(workspace, "workspace-state") || got.CacheDir != filepath.Join(workspace, "workspace-cache") {
		t.Fatalf("expected trusted workspace paths, got state=%q cache=%q", got.StateDir, got.CacheDir)
	}
	if got.Dotfiles.Repository != "github.com/example/workspace" || got.Dotfiles.InstallCommand != "workspace-install" || got.Dotfiles.TargetPath != "~/workspace-dotfiles" {
		t.Fatalf("unexpected trusted dotfiles defaults %#v", got.Dotfiles)
	}
	if len(got.TrustedSigners) != 1 || got.TrustedSigners[0].Subject != "workspace@example.com" {
		t.Fatalf("unexpected trusted signers %#v", got.TrustedSigners)
	}
}

func writeConfigFixtures(t *testing.T) string {
	t.Helper()
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
	if err := os.WriteFile(filepath.Join(userDir, "config.toml"), []byte("state_dir = \"/user/state\"\ncache_dir = \"/user/cache\"\nlockfile_policy = \"update\"\nfeature_timeout = \"45s\"\nbridge = false\nssh = false\n[dotfiles]\nrepository = \"github.com/example/user\"\ninstall_command = \"user-install\"\ntarget_path = \"~/user-dotfiles\"\n[[verification.trusted_signers]]\nissuer = \"https://issuer.user.example\"\nsubject = \"user@example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hatchctl", "config.toml"), []byte("config = \"../custom/devcontainer.json\"\nstate_dir = \"../workspace-state\"\ncache_dir = \"../workspace-cache\"\nbridge = true\nssh = true\n[dotfiles]\nrepository = \"github.com/example/workspace\"\ninstall_command = \"workspace-install\"\ntarget_path = \"~/workspace-dotfiles\"\n[[verification.trusted_signers]]\nissuer = \"https://issuer.workspace.example\"\nsubject = \"workspace@example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func TestResolveDefaultsIgnoresUntrustedWorkspacePathOverride(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	if err := os.MkdirAll(filepath.Join(workspace, ".hatchctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantTrusted := filepath.Join(workspace, "trusted-workspace")
	if err := os.WriteFile(filepath.Join(workspace, ".hatchctl", "config.toml"), []byte("workspace = \"../trusted-workspace\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveDefaults(ResolveDefaultsRequest{TrustWorkspace: &FlagValue[bool]{Value: false}})
	if err != nil {
		t.Fatalf("resolve defaults: %v", err)
	}
	if got.Workspace != "" {
		t.Fatalf("expected untrusted workspace path override to be ignored, got %q", got.Workspace)
	}

	got, err = ResolveDefaults(ResolveDefaultsRequest{TrustWorkspace: &FlagValue[bool]{Value: true}})
	if err != nil {
		t.Fatalf("resolve defaults with trust: %v", err)
	}
	if got.Workspace != wantTrusted {
		t.Fatalf("expected trusted workspace path %q, got %q", wantTrusted, got.Workspace)
	}
}

func resolveDefaultsRequestForTest(workspace string, trustWorkspace bool) ResolveDefaultsRequest {
	return ResolveDefaultsRequest{
		Workspace:      FlagValue[string]{Value: workspace, Changed: true},
		ConfigPath:     FlagValue[string]{},
		FeatureTimeout: FlagValue[time.Duration]{},
		LockfilePolicy: FlagValue[string]{Value: "auto"},
		BridgeEnabled:  &FlagValue[bool]{Value: false},
		TrustWorkspace: &FlagValue[bool]{Value: trustWorkspace},
		SSHAgent:       &FlagValue[bool]{Value: false},
		Dotfiles:       DotfilesOptionValues{},
	}
}
