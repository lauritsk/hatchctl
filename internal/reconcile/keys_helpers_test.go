package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestManagedImageKeyChangesWithDockerfileAndFeatures(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	contextDir := filepath.Join(configDir, "ctx")
	featureDir := filepath.Join(workspace, "feature")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReconcileTestFile(t, filepath.Join(configDir, "Dockerfile"), "FROM alpine:3.23\n")
	writeReconcileTestFile(t, filepath.Join(contextDir, "app.txt"), "one\n")
	writeReconcileTestFile(t, filepath.Join(featureDir, "devcontainer-feature.json"), `{"id":"go"}`)

	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
		ConfigDir:       configDir,
		Config: devcontainer.Config{
			Image: "ghcr.io/example/base:1",
			Build: &devcontainer.BuildConfig{Context: "ctx"},
		},
		ImageName:  "hatchctl-demo",
		SourceKind: "dockerfile",
		Features: []devcontainer.ResolvedFeature{{
			Source:    "ghcr.io/example/go:1",
			Resolved:  "ghcr.io/example/go@sha256:abc",
			Integrity: "sha256:abc",
			Path:      featureDir,
			Options:   map[string]string{"version": "1.24"},
		}},
	}

	key1, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key: %v", err)
	}
	key2, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key second pass: %v", err)
	}
	if key1 != key2 {
		t.Fatalf("expected stable managed image key, got %q vs %q", key1, key2)
	}

	writeReconcileTestFile(t, filepath.Join(configDir, "Dockerfile"), "FROM alpine:3.24\n")
	key3, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key after dockerfile change: %v", err)
	}
	if key3 == key1 {
		t.Fatal("expected dockerfile content change to affect managed image key")
	}

	writeReconcileTestFile(t, filepath.Join(configDir, "Dockerfile"), "FROM alpine:3.23\n")
	resolved.Features[0].Options["version"] = "1.25"
	key4, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key after feature option change: %v", err)
	}
	if key4 == key1 {
		t.Fatal("expected feature option change to affect managed image key")
	}
}

func TestManagedImageKeyIncludesComposeFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	composePath := filepath.Join(workspace, "compose.yml")
	writeReconcileTestFile(t, composePath, "services:\n  app:\n    image: alpine:3.23\n")
	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigPath:      filepath.Join(workspace, ".devcontainer", "devcontainer.json"),
		ImageName:       "hatchctl-demo",
		SourceKind:      "compose",
		ComposeProject:  "demo",
		ComposeService:  "app",
		ComposeFiles:    []string{composePath},
	}

	key1, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key: %v", err)
	}
	writeReconcileTestFile(t, composePath, "services:\n  app:\n    image: alpine:3.24\n")
	key2, err := ManagedImageKey(resolved, "target-image")
	if err != nil {
		t.Fatalf("managed image key after compose change: %v", err)
	}
	if key1 == key2 {
		t.Fatal("expected compose file change to affect managed image key")
	}
}

func TestContainerKeyChangesWithRuntimeInputs(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{
		SourceKind:      "dockerfile",
		ContainerName:   "hatchctl-demo",
		ComposeProject:  "demo",
		ComposeService:  "app",
		WorkspaceMount:  "type=bind,source=/workspace,target=/workspaces/demo",
		RemoteWorkspace: "/workspaces/demo",
		Labels:          map[string]string{"devcontainer.config_file": "/workspace/.devcontainer/devcontainer.json"},
		Config:          devcontainer.Config{RunArgs: []string{"--network=host"}},
		Merged:          devcontainer.MergedConfig{ContainerUser: "vscode", Mounts: []string{"type=volume,source=data,target=/data"}, CapAdd: []string{"NET_ADMIN"}, SecurityOpt: []string{"label=disable"}, Init: true, Privileged: true, ContainerEnv: map[string]string{"A": "1"}},
	}

	key1, err := ContainerKey(resolved, "image@sha256:abc", false, false)
	if err != nil {
		t.Fatalf("container key: %v", err)
	}
	key2, err := ContainerKey(resolved, "image@sha256:abc", true, false)
	if err != nil {
		t.Fatalf("container key with bridge: %v", err)
	}
	if key1 == key2 {
		t.Fatal("expected bridge setting to affect container key")
	}
	resolved.Labels["extra"] = "value"
	key3, err := ContainerKey(resolved, "image@sha256:abc", false, false)
	if err != nil {
		t.Fatalf("container key with extra label: %v", err)
	}
	if key1 == key3 {
		t.Fatal("expected label change to affect container key")
	}
}

func TestLifecycleKeyChangesWithCommandsAndDotfiles(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{
		Config: devcontainer.Config{InitializeCommand: devcontainer.LifecycleCommand{Kind: "string", Value: "echo init", Exists: true}},
		Merged: devcontainer.MergedConfig{
			OnCreateCommands:      []devcontainer.LifecycleCommand{{Kind: "string", Value: "echo create", Exists: true}},
			UpdateContentCommands: []devcontainer.LifecycleCommand{{Kind: "array", Args: []string{"echo", "update"}, Exists: true}},
			PostCreateCommands:    []devcontainer.LifecycleCommand{{Kind: "string", Value: "echo post", Exists: true}},
			PostStartCommands:     []devcontainer.LifecycleCommand{{Kind: "string", Value: "echo start", Exists: true}},
			PostAttachCommands:    []devcontainer.LifecycleCommand{{Kind: "string", Value: "echo attach", Exists: true}},
		},
	}

	key1, err := LifecycleKey(resolved, "container-key", DotfilesConfig{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/.dotfiles"})
	if err != nil {
		t.Fatalf("lifecycle key: %v", err)
	}
	key2, err := LifecycleKey(resolved, "container-key", DotfilesConfig{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/config"})
	if err != nil {
		t.Fatalf("lifecycle key with changed dotfiles target: %v", err)
	}
	if key1 == key2 {
		t.Fatal("expected dotfiles change to affect lifecycle key")
	}
	resolved.Merged.PostAttachCommands[0].Value = "echo attached"
	key3, err := LifecycleKey(resolved, "container-key", DotfilesConfig{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/.dotfiles"})
	if err != nil {
		t.Fatalf("lifecycle key with changed command: %v", err)
	}
	if key1 == key3 {
		t.Fatal("expected lifecycle command change to affect lifecycle key")
	}
}

func writeReconcileTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
