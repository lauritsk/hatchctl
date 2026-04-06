package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSupportsJSONC(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "devcontainer.json")
	contents := `{
		// comment
		"image": "mcr.microsoft.com/devcontainers/base:ubuntu",
		"workspaceFolder": "/workspaces/demo",
		"containerEnv": {
			"FOO": "bar",
		},
		"postStartCommand": "echo ready",
	}`
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Image != "mcr.microsoft.com/devcontainers/base:ubuntu" {
		t.Fatalf("unexpected image %q", config.Image)
	}
	if config.WorkspaceFolder != "/workspaces/demo" {
		t.Fatalf("unexpected workspace folder %q", config.WorkspaceFolder)
	}
	if config.ContainerEnv["FOO"] != "bar" {
		t.Fatalf("unexpected container env %#v", config.ContainerEnv)
	}
	if config.PostStartCommand.Empty() {
		t.Fatal("expected postStartCommand to be parsed")
	}
}

func TestResolveFindsDefaultConfigAndBuildsRuntimeShape(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	contents := `{
		"name": "demo",
		"dockerFile": "Dockerfile",
		"workspaceFolder": "/workspaces/demo",
		"initializeCommand": ["/bin/sh", "-lc", "echo init"],
		"postAttachCommand": {
			"a": "echo one",
			"b": "echo two"
		}
	}`
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(workspace, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("unexpected config path %q", resolved.ConfigPath)
	}
	if resolved.SourceKind != "dockerfile" {
		t.Fatalf("unexpected source kind %q", resolved.SourceKind)
	}
	if resolved.RemoteWorkspace != "/workspaces/demo" {
		t.Fatalf("unexpected remote workspace %q", resolved.RemoteWorkspace)
	}
	if !strings.Contains(resolved.WorkspaceMount, workspace) {
		t.Fatalf("workspace mount %q does not reference workspace", resolved.WorkspaceMount)
	}
	if resolved.Config.InitializeCommand.Empty() {
		t.Fatal("expected initializeCommand to be populated")
	}
	steps := resolved.Config.PostAttachCommand.SortedSteps()
	if len(steps) != 2 {
		t.Fatalf("expected 2 attach steps, got %d", len(steps))
	}
	if steps[0].Name != "a" || steps[1].Name != "b" {
		t.Fatalf("unexpected lifecycle step order %#v", steps)
	}
	if resolved.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("unexpected labels %#v", resolved.Labels)
	}
}

func TestMergeMetadataMatchesExpectedPrecedence(t *testing.T) {
	t.Parallel()

	falseValue := false
	trueValue := true
	merged := MergeMetadata(Config{
		RemoteUser:    "config-remote",
		ContainerUser: "config-container",
		RemoteEnv: map[string]string{
			"BASE":   "config",
			"CONFIG": "yes",
		},
		ContainerEnv: map[string]string{
			"KEEP":   "config",
			"CONFIG": "yes",
		},
		Mounts: []string{
			"type=volume,target=/config-only",
			"type=bind,source=/config,target=/shared",
		},
		CapAdd:          []string{"SYS_PTRACE"},
		SecurityOpt:     []string{"seccomp=unconfined"},
		OverrideCommand: &falseValue,
		OnCreateCommand: LifecycleCommand{Kind: "string", Value: "config-create", Exists: true},
	}, []MetadataEntry{{
		RemoteUser:      "image-remote",
		ContainerUser:   "image-container",
		RemoteEnv:       map[string]string{"BASE": "image", "IMAGE": "yes"},
		ContainerEnv:    map[string]string{"KEEP": "image", "IMAGE": "yes"},
		Mounts:          []string{"type=bind,source=/image,target=/shared", "type=volume,target=/image-only"},
		CapAdd:          []string{"NET_ADMIN"},
		SecurityOpt:     []string{"label=disable"},
		OverrideCommand: &trueValue,
		OnCreateCommand: LifecycleCommand{Kind: "string", Value: "image-create", Exists: true},
	}})

	if merged.RemoteUser != "config-remote" {
		t.Fatalf("unexpected remote user %q", merged.RemoteUser)
	}
	if merged.ContainerUser != "config-container" {
		t.Fatalf("unexpected container user %q", merged.ContainerUser)
	}
	if merged.RemoteEnv["BASE"] != "config" || merged.RemoteEnv["IMAGE"] != "yes" || merged.RemoteEnv["CONFIG"] != "yes" {
		t.Fatalf("unexpected remote env %#v", merged.RemoteEnv)
	}
	if merged.ContainerEnv["KEEP"] != "config" || merged.ContainerEnv["IMAGE"] != "yes" || merged.ContainerEnv["CONFIG"] != "yes" {
		t.Fatalf("unexpected container env %#v", merged.ContainerEnv)
	}
	if len(merged.Mounts) != 3 {
		t.Fatalf("unexpected mounts %#v", merged.Mounts)
	}
	if merged.Mounts[2] != "type=bind,source=/config,target=/shared" {
		t.Fatalf("expected config mount to override shared target, got %#v", merged.Mounts)
	}
	if len(merged.CapAdd) != 2 || len(merged.SecurityOpt) != 2 {
		t.Fatalf("unexpected merged security values %#v %#v", merged.CapAdd, merged.SecurityOpt)
	}
	if merged.OverrideCommand == nil || *merged.OverrideCommand {
		t.Fatalf("unexpected overrideCommand %#v", merged.OverrideCommand)
	}
	if len(merged.OnCreateCommands) != 2 {
		t.Fatalf("unexpected onCreate commands %#v", merged.OnCreateCommands)
	}
	if merged.OnCreateCommands[0].Value != "image-create" || merged.OnCreateCommands[1].Value != "config-create" {
		t.Fatalf("unexpected lifecycle order %#v", merged.OnCreateCommands)
	}
}

func TestMetadataFromLabelSupportsSingleAndArray(t *testing.T) {
	t.Parallel()

	entries, err := MetadataFromLabel(`[ {"remoteUser":"vscode"}, {"remoteUser":"dev"} ]`)
	if err != nil {
		t.Fatalf("parse array metadata: %v", err)
	}
	if len(entries) != 2 || entries[1].RemoteUser != "dev" {
		t.Fatalf("unexpected metadata entries %#v", entries)
	}

	entries, err = MetadataFromLabel(`{"remoteEnv":{"A":"B"}}`)
	if err != nil {
		t.Fatalf("parse single metadata: %v", err)
	}
	if len(entries) != 1 || entries[0].RemoteEnv["A"] != "B" {
		t.Fatalf("unexpected single metadata %#v", entries)
	}
}
