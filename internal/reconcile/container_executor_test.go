package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	docker "github.com/lauritsk/hatchctl/internal/backend/testdocker"
	dockercli "github.com/lauritsk/hatchctl/internal/backend/testdockercli"
	capssh "github.com/lauritsk/hatchctl/internal/capability/sshagent"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestContainerBridgeModeMatches(t *testing.T) {
	t.Parallel()

	inspect := backend.ContainerInspect{Config: backend.InspectConfig{Labels: map[string]string{devcontainer.BridgeEnabledLabel: "true"}}}
	if !containerBridgeModeMatches(inspect, true) {
		t.Fatal("expected bridge-enabled container to match enabled requirement")
	}
	if containerBridgeModeMatches(inspect, false) {
		t.Fatal("expected bridge-enabled container not to match disabled requirement")
	}
}

func TestContainerSSHAgentMatches(t *testing.T) {
	t.Parallel()

	withLabel := backend.ContainerInspect{Config: backend.InspectConfig{Labels: map[string]string{devcontainer.SSHAgentLabel: "true"}}}
	if !containerSSHAgentMatches(withLabel, true) {
		t.Fatal("expected ssh-agent label to satisfy ssh requirement")
	}
	if containerSSHAgentMatches(withLabel, false) {
		t.Fatal("expected ssh-agent label to fail when ssh is disabled")
	}

	withMount := backend.ContainerInspect{Mounts: []backend.ContainerMount{{Destination: capssh.ContainerSocketPath}}}
	if !containerSSHAgentMatches(withMount, true) {
		t.Fatal("expected ssh-agent mount to satisfy ssh requirement")
	}
	if containerSSHAgentMatches(withMount, false) {
		t.Fatal("expected ssh-agent mount to fail when ssh is disabled")
	}

	withoutMount := backend.ContainerInspect{}
	if !containerSSHAgentMatches(withoutMount, false) {
		t.Fatal("expected missing ssh-agent mount to satisfy disabled ssh requirement")
	}
}

func TestEnvListToMapSkipsMalformedEntries(t *testing.T) {
	t.Parallel()

	got := envListToMap([]string{"FOO=bar", "BROKEN", "EMPTY=", "A=B=C"})
	if got["FOO"] != "bar" || got["EMPTY"] != "" || got["A"] != "B=C" {
		t.Fatalf("unexpected env map %#v", got)
	}
	if _, ok := got["BROKEN"]; ok {
		t.Fatalf("expected malformed entry to be skipped, got %#v", got)
	}
	if envListToMap(nil) != nil {
		t.Fatal("expected nil env list to stay nil")
	}
}

func TestReadComposeConfigUpdatesResolvedProject(t *testing.T) {
	t.Parallel()

	executor := NewExecutorWithIO(nil, nil, nil, nil)
	executor.engine = &fakeExecutorEngine{
		composeConfigFunc: func(_ context.Context, req dockercli.ComposeConfigRequest) (string, error) {
			return `{"name":"resolved-project","services":{"app":{"image":"alpine:3.23"}}}`, nil
		},
	}
	resolved := devcontainer.ResolvedConfig{ConfigDir: "/workspace/.devcontainer", ComposeFiles: []string{"compose.yml"}}

	config, err := executor.readComposeConfig(context.Background(), &resolved)
	if err != nil {
		t.Fatalf("read compose config: %v", err)
	}
	if config.Name != "resolved-project" || resolved.ComposeProject != "resolved-project" {
		t.Fatalf("expected compose project to update from config, got config=%#v resolved=%#v", config, resolved)
	}
	if config.Services["app"].Image != "alpine:3.23" {
		t.Fatalf("unexpected compose config %#v", config)
	}
}

func TestReadComposeConfigStripsLeadingNoise(t *testing.T) {
	t.Parallel()

	executor := NewExecutorWithIO(nil, nil, nil, nil)
	executor.engine = &fakeExecutorEngine{
		composeConfigFunc: func(_ context.Context, req dockercli.ComposeConfigRequest) (string, error) {
			return "warning: plugin emitted noise\n{\"name\":\"resolved-project\"}", nil
		},
	}
	resolved := devcontainer.ResolvedConfig{ConfigDir: "/workspace/.devcontainer", ComposeFiles: []string{"compose.yml"}}

	config, err := executor.readComposeConfig(context.Background(), &resolved)
	if err != nil {
		t.Fatalf("read compose config with noise: %v", err)
	}
	if config.Name != "resolved-project" || resolved.ComposeProject != "resolved-project" {
		t.Fatalf("unexpected compose config %#v resolved=%#v", config, resolved)
	}
}

func TestProjectOverrideIncludesDerivedSettings(t *testing.T) {
	t.Parallel()

	trueValue := true
	resolved := devcontainer.ResolvedConfig{
		SourceKind:     "compose",
		StateDir:       "/state",
		ComposeService: "app",
		WorkspaceMount: "type=bind,source=/workspace,target=/workspaces/demo,consistency=cached",
		Features:       []devcontainer.ResolvedFeature{{Metadata: spec.MetadataEntry{ID: "mise"}}},
		Labels:         map[string]string{"devcontainer.local_folder": "/workspace"},
		Config:         spec.Config{OverrideCommand: &trueValue},
		Merged: spec.MergedConfig{
			ContainerEnv: map[string]string{
				"DEVCONTAINER_BRIDGE_ENABLED": "true",
				"SSH_AUTH_SOCK":               capssh.ContainerSocketPath,
				"FOO":                         "bar",
			},
			Mounts: []string{
				"type=volume,source=deps,target=/deps,volume-nocopy=true",
				"type=bind,source=/tmp/cache,target=/cache,bind-propagation=rshared",
			},
			ContainerUser: "vscode",
			CapAdd:        []string{"SYS_PTRACE"},
			SecurityOpt:   []string{"seccomp=unconfined"},
			Metadata:      []spec.MetadataEntry{{ID: "mise"}},
		},
	}

	override, err := projectOverride(resolved, "managed-image", "container-key")
	if err != nil {
		t.Fatalf("project override: %v", err)
	}
	if override.PullPolicy != "never" || override.Image != "managed-image" || override.User != "vscode" {
		t.Fatalf("unexpected override basics %#v", override)
	}
	if override.Labels[devcontainer.BridgeEnabledLabel] != "true" || override.Labels[devcontainer.SSHAgentLabel] != "true" || override.Labels[ContainerKeyLabel] != "container-key" {
		t.Fatalf("unexpected labels %#v", override.Labels)
	}
	if override.Environment["FOO"] != "bar" || len(override.Mounts) != 3 {
		t.Fatalf("unexpected override env/mounts %#v", override)
	}
	if len(override.CapAdd) != 1 || override.CapAdd[0] != "SYS_PTRACE" || len(override.SecurityOpt) != 1 || override.SecurityOpt[0] != "seccomp=unconfined" {
		t.Fatalf("unexpected override security %#v", override)
	}
}

func TestProjectOverrideReturnsMetadataEncodingError(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{
		SourceKind:     "compose",
		ComposeService: "app",
		WorkspaceMount: "type=bind,source=/workspace,target=/workspaces/demo",
		Merged: spec.MergedConfig{
			Metadata: []spec.MetadataEntry{{Customizations: map[string]any{"bad": func() {}}}},
		},
	}

	if _, err := projectOverride(resolved, "managed-image", "container-key"); err == nil {
		t.Fatal("expected metadata encoding error")
	}
}

func TestEnsureReusableContainerRemovesMismatchedContainer(t *testing.T) {
	t.Parallel()

	removed := false
	executor := NewExecutorWithIO(nil, nil, nil, nil)
	executor.engine = &fakeExecutorEngine{
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			if req.ContainerID != "container-123" {
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{
				ID: "container-123",
				Config: docker.InspectConfig{Labels: map[string]string{
					devcontainer.BridgeEnabledLabel: "false",
				}},
				State: docker.ContainerState{Running: true},
			}, nil
		},
		removeContainerFunc: func(_ context.Context, req dockercli.RemoveContainerRequest) error {
			removed = true
			if req.ContainerID != "container-123" || !req.Force {
				t.Fatalf("unexpected remove request %#v", req)
			}
			return nil
		},
	}

	id, reused, err := executor.ensureReusableContainer(context.Background(), "container-123", containerReuseRequirements{BridgeEnabled: true}, nil)
	if err != nil {
		t.Fatalf("ensure reusable container: %v", err)
	}
	if id != "" || reused {
		t.Fatalf("expected mismatched container to be discarded, got id=%q reused=%v", id, reused)
	}
	if !removed {
		t.Fatal("expected mismatched container to be removed")
	}
}

func TestFindComposeContainerResolvesProjectFromComposeConfig(t *testing.T) {
	t.Parallel()

	executor := NewExecutorWithIO(nil, nil, nil, nil)
	executor.engine = &fakeExecutorEngine{
		composeConfigFunc: func(_ context.Context, req dockercli.ComposeConfigRequest) (string, error) {
			return `{"name":"resolved-project"}`, nil
		},
		listContainersFunc: func(_ context.Context, req dockercli.ListContainersRequest) (string, error) {
			filters := strings.Join(req.Filters, ",")
			if !strings.Contains(filters, "label=com.docker.compose.project=resolved-project") {
				t.Fatalf("expected compose project filter, got %#v", req.Filters)
			}
			return "container-123\n", nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			return docker.ContainerInspect{ID: req.ContainerID, Config: docker.InspectConfig{Labels: map[string]string{"com.docker.compose.service": "app"}}, State: docker.ContainerState{Running: true}}, nil
		},
	}

	resolved := devcontainer.ResolvedConfig{ConfigDir: "/workspace/.devcontainer", ComposeFiles: []string{"compose.yml"}, ComposeProject: "resolved-project", ComposeService: "app", SourceKind: "compose"}
	id, err := executor.findComposeContainer(context.Background(), resolved)
	if err != nil {
		t.Fatalf("find compose container: %v", err)
	}
	if id != "container-123" {
		t.Fatalf("expected compose container id container-123, got %q", id)
	}
}

func TestCreateComposeContainerWritesTemporaryOverrideAndRemovesIt(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var overridePath string
	executor := NewExecutorWithIO(nil, nil, nil, nil)
	executor.engine = &fakeExecutorEngine{
		composeUpFunc: func(_ context.Context, req dockercli.ComposeUpRequest) error {
			if len(req.Target.Files) != 2 {
				t.Fatalf("expected compose target files plus override, got %#v", req.Target.Files)
			}
			overridePath = req.Target.Files[1]
			data, err := os.ReadFile(overridePath)
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), "image: managed-image") {
				t.Fatalf("expected override file to contain image override, got:\n%s", string(data))
			}
			return nil
		},
		listContainersFunc: func(_ context.Context, req dockercli.ListContainersRequest) (string, error) {
			return "container-123\n", nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			return docker.ContainerInspect{ID: req.ContainerID, Config: docker.InspectConfig{Labels: map[string]string{"com.docker.compose.service": "app"}}, State: docker.ContainerState{Running: true}}, nil
		},
	}

	resolved := devcontainer.ResolvedConfig{
		SourceKind:     "compose",
		StateDir:       stateDir,
		ConfigDir:      "/workspace/.devcontainer",
		ComposeFiles:   []string{"compose.yml"},
		ComposeProject: "demo",
		ComposeService: "app",
		WorkspaceMount: "type=bind,source=/workspace,target=/workspaces/demo",
		Merged:         spec.MergedConfig{},
	}

	id, err := executor.createComposeContainer(context.Background(), resolved, "managed-image", "container-key", nil)
	if err != nil {
		t.Fatalf("create compose container: %v", err)
	}
	if id != "container-123" {
		t.Fatalf("expected compose container id container-123, got %q", id)
	}
	if overridePath == "" {
		t.Fatal("expected temporary compose override to be created")
	}
	if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
		t.Fatalf("expected temporary override to be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "project-service.override.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected workspace override path to be cleaned up, got %v", err)
	}
}
