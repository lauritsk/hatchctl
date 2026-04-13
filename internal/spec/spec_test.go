package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSupportsJSONC(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "devcontainer.json")
	writeSpecTestFile(t, configPath, `{
		// comment
		"image": "mcr.microsoft.com/devcontainers/base:ubuntu",
		"workspaceFolder": "/workspaces/demo",
		"containerEnv": {
			"FOO": "bar",
		},
		"postStartCommand": "echo ready",
	}`)

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

func TestResolveWorkspaceSpecBuildsPureWorkspaceShape(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	writeSpecTestFile(t, configPath, `{
		"name": "demo",
		"dockerFile": "Dockerfile",
		"workspaceFolder": "/workspaces/demo",
		"initializeCommand": ["/bin/sh", "-lc", "echo init"],
		"postAttachCommand": {
			"a": "echo one",
			"b": "echo two"
		}
	}`)

	resolved, err := ResolveWorkspaceSpec(workspace, "")
	if err != nil {
		t.Fatalf("resolve workspace spec: %v", err)
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
	if len(resolved.Merged.Metadata) != 1 {
		t.Fatalf("expected config-derived metadata entry, got %#v", resolved.Merged.Metadata)
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

func TestParseMountSpecSupportsAliasesAndOptions(t *testing.T) {
	t.Parallel()

	spec, ok := ParseMountSpec("type=bind,src=/workspace,dst=/workspaces/demo,ro=1,bind-propagation=rshared,create-host-path=false")
	if !ok {
		t.Fatal("expected mount spec to parse")
	}
	if spec.Type != "bind" || spec.Source != "/workspace" || spec.Target != "/workspaces/demo" {
		t.Fatalf("unexpected parsed mount %#v", spec)
	}
	if !spec.ReadOnly || spec.BindPropagation != "rshared" {
		t.Fatalf("unexpected mount options %#v", spec)
	}
	if spec.CreateHostPath == nil || *spec.CreateHostPath {
		t.Fatalf("expected create-host-path=false, got %#v", spec.CreateHostPath)
	}
}

func TestResolveHelpersPreferExpectedConfigValues(t *testing.T) {
	t.Parallel()

	overrideFalse := false
	config := Config{
		DockerFile:      "Dockerfile.root",
		RemoteUser:      "remote-user",
		ContainerUser:   "container-user",
		OverrideCommand: &overrideFalse,
		Build: &BuildConfig{
			Dockerfile: "Dockerfile.build",
			Context:    "docker",
		},
	}
	if got := EffectiveDockerfile(config); got != "Dockerfile.root" {
		t.Fatalf("unexpected effective dockerfile %q", got)
	}
	if got := EffectiveContext(config); got != "docker" {
		t.Fatalf("unexpected effective context %q", got)
	}
	if got := RemoteExecUser(config); got != "remote-user" {
		t.Fatalf("unexpected remote exec user %q", got)
	}
	if got := ContainerCommand(config); got != nil {
		t.Fatalf("expected overrideCommand=false to disable command, got %#v", got)
	}

	config = Config{Build: &BuildConfig{Dockerfile: "Dockerfile.build"}, ContainerUser: "container-user"}
	if got := EffectiveDockerfile(config); got != "Dockerfile.build" {
		t.Fatalf("unexpected build dockerfile %q", got)
	}
	if got := EffectiveContext(config); got != "." {
		t.Fatalf("unexpected default context %q", got)
	}
	if got := RemoteExecUser(config); got != "container-user" {
		t.Fatalf("unexpected fallback remote user %q", got)
	}
	if got := ContainerCommand(config); !reflect.DeepEqual(got, []string{"/bin/sh", "-lc", KeepAliveCommand()}) {
		t.Fatalf("unexpected container command %#v", got)
	}
	if got := KeepAliveCommand(); got != "exec sleep infinity" {
		t.Fatalf("unexpected keepalive command %q", got)
	}
	if got := ShellQuote("it's $HOME"); got != "'it'\\''s $HOME'" {
		t.Fatalf("unexpected shell quote %q", got)
	}
}

func TestResolvedDockerfileFallsBackToContainerfile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeSpecTestFile(t, filepath.Join(configDir, "Containerfile"), "FROM alpine:3.23\n")

	if got := ResolvedDockerfile(configDir, Config{}); got != "Containerfile" {
		t.Fatalf("unexpected resolved dockerfile %q", got)
	}
}

func TestResolvedDockerfilePrefersDockerfileWhenBothExist(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeSpecTestFile(t, filepath.Join(configDir, "Dockerfile"), "FROM alpine:3.23\n")
	writeSpecTestFile(t, filepath.Join(configDir, "Containerfile"), "FROM alpine:3.23\n")

	if got := ResolvedDockerfile(configDir, Config{}); got != "Dockerfile" {
		t.Fatalf("unexpected resolved dockerfile %q", got)
	}
}

func TestNormalizeAndMergeForwardPorts(t *testing.T) {
	t.Parallel()

	ports, err := NormalizeForwardPorts([]any{3000.0, " localhost:3000 ", "service:9000", 8080.0, "service:9000"})
	if err != nil {
		t.Fatalf("normalize forward ports: %v", err)
	}
	if got := []string(ports); !reflect.DeepEqual(got, []string{"localhost:3000", "service:9000", "localhost:8080"}) {
		t.Fatalf("unexpected normalized ports %#v", got)
	}
	merged := MergeForwardPorts(ForwardPorts{"localhost:3000", ""}, ForwardPorts{"service:9000", "localhost:3000"}, nil)
	if got := []string(merged); !reflect.DeepEqual(got, []string{"localhost:3000", "service:9000"}) {
		t.Fatalf("unexpected merged forward ports %#v", got)
	}
	if merged := MergeForwardPorts(nil); merged != nil {
		t.Fatalf("expected empty merge to return nil, got %#v", merged)
	}
}

func TestNormalizeForwardPortsRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []struct {
		name  string
		value []any
		want  string
	}{
		{name: "empty-string", value: []any{"   "}, want: "cannot be empty"},
		{name: "fractional-number", value: []any{1.5}, want: "expected an integer"},
		{name: "invalid-type", value: []any{true}, want: "invalid forward port value"},
	} {
		t.Run(raw.name, func(t *testing.T) {
			t.Parallel()
			_, err := NormalizeForwardPorts(raw.value)
			if err == nil || !strings.Contains(err.Error(), raw.want) {
				t.Fatalf("expected %q error, got %v", raw.want, err)
			}
		})
	}
}

func TestForwardPortsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(ForwardPorts{"localhost:3000", "service:9000"})
	if err != nil {
		t.Fatalf("marshal forward ports: %v", err)
	}
	if string(encoded) != `[3000,"service:9000"]` {
		t.Fatalf("unexpected forward ports json %s", encoded)
	}
	var decoded ForwardPorts
	if err := json.Unmarshal([]byte(`[3000,"service:9000",3000]`), &decoded); err != nil {
		t.Fatalf("unmarshal forward ports: %v", err)
	}
	if got := []string(decoded); !reflect.DeepEqual(got, []string{"localhost:3000", "service:9000"}) {
		t.Fatalf("unexpected decoded ports %#v", got)
	}

	encoded, err = json.Marshal(ForwardPorts(nil))
	if err != nil {
		t.Fatalf("marshal nil forward ports: %v", err)
	}
	if string(encoded) != `null` {
		t.Fatalf("unexpected nil ports json %s", encoded)
	}
}

func writeSpecTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
