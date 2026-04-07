package devcontainer

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	resolved, err := Resolve(context.Background(), workspace, "")
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
		ForwardPorts:  ForwardPorts{"localhost:3000", "service:9000"},
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
		ForwardPorts:    ForwardPorts{"localhost:3000", "localhost:8080"},
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
	if got := []string(merged.ForwardPorts); strings.Join(got, ",") != "localhost:3000,localhost:8080,service:9000" {
		t.Fatalf("unexpected merged forward ports %#v", got)
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

func TestMetadataLabelValueUsesObjectForSingleEntryAndArrayForMultiple(t *testing.T) {
	t.Parallel()

	single, err := MetadataLabelValue([]MetadataEntry{{RemoteUser: "root"}})
	if err != nil {
		t.Fatalf("marshal single metadata: %v", err)
	}
	if single != `{"remoteUser":"root"}` {
		t.Fatalf("unexpected single metadata label %q", single)
	}

	multi, err := MetadataLabelValue([]MetadataEntry{{RemoteUser: "root"}, {RemoteUser: "vscode"}})
	if err != nil {
		t.Fatalf("marshal multiple metadata entries: %v", err)
	}
	if multi != `[{"remoteUser":"root"},{"remoteUser":"vscode"}]` {
		t.Fatalf("unexpected multi metadata label %q", multi)
	}
}

func TestLoadNormalizesForwardPorts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "devcontainer.json")
	contents := `{
		"image": "alpine:3.20",
		"forwardPorts": [3000, "localhost:3000", "service:9000"]
	}`
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := []string(config.ForwardPorts); strings.Join(got, ",") != "localhost:3000,service:9000" {
		t.Fatalf("unexpected normalized forward ports %#v", got)
	}
}

func TestResolveComposeConfigParsesComposeFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(configDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{
		"dockerComposeFile": ["compose.yaml"],
		"service": "app",
		"workspaceFolder": "/workspace"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve compose config: %v", err)
	}
	if resolved.SourceKind != "compose" {
		t.Fatalf("unexpected source kind %q", resolved.SourceKind)
	}
	if len(resolved.ComposeFiles) != 1 || resolved.ComposeFiles[0] != composePath {
		t.Fatalf("unexpected compose files %#v", resolved.ComposeFiles)
	}
	if resolved.ComposeService != "app" {
		t.Fatalf("unexpected compose service %q", resolved.ComposeService)
	}
	if resolved.ComposeProject == "" {
		t.Fatal("expected compose project name")
	}
}

func TestResolveComposeConfigRequiresServiceAndWorkspaceFolder(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(configDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"compose.yaml"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), workspace, ""); err == nil || !strings.Contains(err.Error(), "require service") {
		t.Fatalf("expected missing service error, got %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"compose.yaml","service":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), workspace, ""); err == nil || !strings.Contains(err.Error(), "require workspaceFolder") {
		t.Fatalf("expected missing workspaceFolder error, got %v", err)
	}
}

func TestResolveSupportsContainerfile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "Containerfile"), []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{
		"dockerFile": "Containerfile",
		"workspaceFolder": "/workspace"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve containerfile config: %v", err)
	}
	if resolved.SourceKind != "dockerfile" {
		t.Fatalf("unexpected source kind %q", resolved.SourceKind)
	}
	if got := EffectiveDockerfile(resolved.Config); got != "Containerfile" {
		t.Fatalf("unexpected effective dockerfile %q", got)
	}
}

func TestResolveComposeConfigSupportsCommonComposeFilenames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			workspace := t.TempDir()
			configDir := filepath.Join(workspace, ".devcontainer")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			composePath := filepath.Join(configDir, name)
			if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(configDir, "devcontainer.json")
			if err := os.WriteFile(configPath, []byte(`{
				"dockerComposeFile": "`+name+`",
				"service": "app",
				"workspaceFolder": "/workspace"
			}`), 0o644); err != nil {
				t.Fatal(err)
			}

			resolved, err := Resolve(context.Background(), workspace, "")
			if err != nil {
				t.Fatalf("resolve compose config: %v", err)
			}
			if len(resolved.ComposeFiles) != 1 || resolved.ComposeFiles[0] != composePath {
				t.Fatalf("unexpected compose files %#v", resolved.ComposeFiles)
			}
		})
	}
}

func TestResolveWritesFeatureLockFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(configDir, "feature-a")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(`{
		"id": "feature-a",
		"options": {"version": {"default": "stable"}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"image": "alpine:3.20",
		"workspaceFolder": "/workspace",
		"features": {"./feature-a": {"version": "1.2.3"}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve config with features: %v", err)
	}
	lockPath := filepath.Join(resolved.StateDir, "features-lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read features lock file: %v", err)
	}
	if !strings.Contains(string(data), `"id": "feature-a"`) || !strings.Contains(string(data), `"VERSION": "1.2.3"`) {
		t.Fatalf("unexpected feature lock contents %s", string(data))
	}
}

func TestResolveReadOnlyDoesNotPersistFeatureFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(configDir, "feature-a")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(`{"id":"feature-a"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{
		"image": "alpine:3.20",
		"workspaceFolder": "/workspace",
		"features": {"./feature-a": true}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveReadOnly(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve read only: %v", err)
	}
	if len(resolved.Features) != 1 || resolved.Features[0].Metadata.ID != "feature-a" {
		t.Fatalf("unexpected resolved features %#v", resolved.Features)
	}
	if _, err := os.Stat(FeatureLockFilePath(configPath)); !os.IsNotExist(err) {
		t.Fatalf("expected no config lockfile, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(resolved.StateDir, "features-lock.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no state feature file, got %v", err)
	}
}

func TestResolveReadOnlyFailsForUncachedRemoteFeatures(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected network access", http.StatusInternalServerError)
	}))
	defer server.Close()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{
		"image": "alpine:3.20",
		"workspaceFolder": "/workspace",
		"features": {"`+server.URL+`/feature.tgz": true}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveReadOnly(context.Background(), workspace, ""); err == nil || !strings.Contains(err.Error(), "requires a lockfile integrity in frozen lockfile mode") {
		t.Fatalf("expected uncached remote feature error, got %v", err)
	}
}

func TestResolveSupportsBuildDockerfileAndContextFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	contextDir := filepath.Join(workspace, "container-context")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "Containerfile"), []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "marker.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"build": {
			"dockerfile": "Containerfile",
			"context": "../container-context"
		},
		"workspaceFolder": "/workspace"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve build config: %v", err)
	}
	if got := EffectiveDockerfile(resolved.Config); got != "Containerfile" {
		t.Fatalf("unexpected build dockerfile %q", got)
	}
	if got := EffectiveContext(resolved.Config); got != "../container-context" {
		t.Fatalf("unexpected build context %q", got)
	}
}

func TestResolvePrefersDotDevcontainerConfigOverRootConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".devcontainer.json"), []byte(`{"image":"alpine:3.20","workspaceFolder":"/root-config"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	preferredPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(preferredPath, []byte(`{"image":"alpine:3.20","workspaceFolder":"/preferred-config"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve preferred config: %v", err)
	}
	if resolved.ConfigPath != preferredPath || resolved.RemoteWorkspace != "/preferred-config" {
		t.Fatalf("unexpected resolved config %#v", resolved)
	}
}

func TestResolveSupportsComposeFileArraysWithRealFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(configDir, "compose.yml")
	second := filepath.Join(configDir, "docker-compose.override.yml")
	if err := os.WriteFile(first, []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("services:\n  app:\n    environment:\n      EXTRA: one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"dockerComposeFile": ["compose.yml", "docker-compose.override.yml"],
		"service": "app",
		"workspaceFolder": "/workspace"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("resolve compose array config: %v", err)
	}
	if got := strings.Join(resolved.ComposeFiles, ","); got != first+","+second {
		t.Fatalf("unexpected compose files %q", got)
	}
}
