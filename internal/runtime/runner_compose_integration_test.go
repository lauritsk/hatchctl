//go:build integration

package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestComposeBuildAndUpSingleService(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	image := "hatchctl-compose-test-" + workspaceKey(workspace)
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile"), []byte("FROM alpine:3.20\nRUN mkdir -p /usr/local/share\nCMD [\"/bin/sh\",\"-lc\",\"echo base-command && exit 0\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := "services:\n  app:\n    image: " + image + "\n    build:\n      context: .\n      dockerfile: Dockerfile\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n"
	if err := os.WriteFile(filepath.Join(configDir, "compose.yaml"), []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	config := `{
		"dockerComposeFile": "compose.yaml",
		"service": "app",
		"workspaceFolder": "/workspaces/demo",
		"containerEnv": {"COMPOSE_OVERRIDE_ENV": "yes"},
		"mounts": ["type=bind,source=` + workspace + `,target=/extra-workspace"],
		"postStartCommand": "echo compose-start >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("compose build: %v", err)
	}
	if buildResult.Image != image {
		t.Fatalf("unexpected compose build image %q", buildResult.Image)
	}
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("compose up: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", image}})
	})

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("compose read config: %v", err)
	}
	if configResult.SourceKind != "compose" {
		t.Fatalf("unexpected source kind %q", configResult.SourceKind)
	}
	if configResult.ManagedContainer == nil || !configResult.ManagedContainer.Running {
		t.Fatalf("expected running compose container, got %#v", configResult.ManagedContainer)
	}
	if got := configResult.ManagedContainer.ContainerEnv["COMPOSE_OVERRIDE_ENV"]; got != "yes" {
		t.Fatalf("expected compose override env, got %#v", configResult.ManagedContainer.ContainerEnv)
	}
	overridePath := devcontainer.ComposeOverrideFile(configResult.StateDir)
	if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
		t.Fatalf("expected no persisted compose override file, got %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"sh", "-lc", "printf '%s|' \"$COMPOSE_OVERRIDE_ENV\"; pwd; printf '|'; test -d /extra-workspace && printf mounted"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("compose exec: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 || strings.ReplaceAll(strings.TrimSpace(stdout.String()), "\n", "") != "yes|/workspaces/demo|mounted" {
		t.Fatalf("unexpected compose exec output %q exit=%d stderr=%s", stdout.String(), exitCode, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read compose lifecycle events: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "compose-start" {
		t.Fatalf("unexpected compose lifecycle output %q", got)
	}

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("compose second up: %v", err)
	}
	if !strings.HasPrefix(upResult.ContainerID, second.ContainerID) && !strings.HasPrefix(second.ContainerID, upResult.ContainerID) {
		t.Fatalf("expected compose container reuse, first=%q second=%q", upResult.ContainerID, second.ContainerID)
	}
}

func TestComposeUpStartsBridgeAndReusesSession(t *testing.T) {
	skipBridgeLifecycleIntegrationOnDarwin(t)
	setBridgeHelperEnv(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineWithCommandImage(t, client, ctx)
	composePath := filepath.Join(configDir, "docker-compose.yml")
	composeYAML := "services:\n  app:\n    image: " + baseImage + "\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n"
	if err := os.WriteFile(composePath, []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"docker-compose.yml","service":"app","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	first, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("compose up with bridge: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", first.ContainerID}})
		_ = bridge.Stop(first.StateDir)
	})
	expectedBridgeStatus := "scaffolded"
	if runtime.GOOS == "darwin" {
		expectedBridgeStatus = "running"
	}
	if first.Bridge == nil || first.Bridge.Status != expectedBridgeStatus {
		t.Fatalf("unexpected compose bridge report %#v", first.Bridge)
	}

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("compose read config: %v", err)
	}
	if configResult.Bridge == nil || configResult.Bridge.Status != expectedBridgeStatus {
		t.Fatalf("unexpected compose bridge config report %#v", configResult.Bridge)
	}
	if configResult.ManagedContainer == nil || !configResult.ManagedContainer.BridgeEnabled {
		t.Fatalf("expected compose bridge-enabled container %#v", configResult.ManagedContainer)
	}
	if got := configResult.ManagedContainer.ContainerEnv["BROWSER"]; got != "/var/run/hatchctl/bridge/bin/devcontainer-open" {
		t.Fatalf("unexpected compose bridge env %#v", configResult.ManagedContainer.ContainerEnv)
	}
	state, err := devcontainer.ReadState(first.StateDir)
	if err != nil {
		t.Fatalf("read compose state: %v", err)
	}
	if !state.BridgeEnabled || state.BridgeSessionID == "" {
		t.Fatalf("unexpected compose bridge state %#v", state)
	}

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("compose second up with bridge: %v", err)
	}
	if !strings.HasPrefix(first.ContainerID, second.ContainerID) && !strings.HasPrefix(second.ContainerID, first.ContainerID) {
		t.Fatalf("expected compose bridge container reuse first=%q second=%q", first.ContainerID, second.ContainerID)
	}
	if second.Bridge == nil || second.Bridge.ID != state.BridgeSessionID {
		t.Fatalf("expected compose bridge session reuse %#v state=%#v", second.Bridge, state)
	}
}

func TestComposeUpEnablingBridgeRecreatesExistingManagedContainer(t *testing.T) {
	skipBridgeLifecycleIntegrationOnDarwin(t)
	setBridgeHelperEnv(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineWithCommandImage(t, client, ctx)
	composePath := filepath.Join(configDir, "docker-compose.yml")
	composeYAML := "services:\n  app:\n    image: " + baseImage + "\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n"
	if err := os.WriteFile(composePath, []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"docker-compose.yml","service":"app","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	first, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("first compose up: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", first.ContainerID}})
	})

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("second compose up with bridge: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", second.ContainerID}})
		_ = bridge.Stop(second.StateDir)
	})
	if second.ContainerID == first.ContainerID {
		t.Fatalf("expected compose bridge enablement to recreate container, reused %q", second.ContainerID)
	}
	if _, err := client.InspectContainer(ctx, first.ContainerID); err == nil {
		t.Fatalf("expected old compose container %q to be removed", first.ContainerID)
	}
	inspect, err := client.InspectContainer(ctx, second.ContainerID)
	if err != nil {
		t.Fatalf("inspect recreated compose container: %v", err)
	}
	if inspect.Config.Labels[devcontainer.BridgeEnabledLabel] != "true" {
		t.Fatalf("expected bridge label on recreated compose container %#v", inspect.Config.Labels)
	}
	if got := envMap(inspect.Config.Env)["BROWSER"]; got != "/var/run/hatchctl/bridge/bin/devcontainer-open" {
		t.Fatalf("expected bridge env on recreated compose container %#v", inspect.Config.Env)
	}
}

func TestReadConfigDoesNotPersistComposeOverrideFile(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(configDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"dockerComposeFile": "compose.yaml",
		"service": "app",
		"workspaceFolder": "/workspaces/demo",
		"containerEnv": {"COMPOSE_OVERRIDE_ENV": "yes"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	result, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := result.ContainerEnv["COMPOSE_OVERRIDE_ENV"]; got != "yes" {
		t.Fatalf("unexpected container env %#v", result.ContainerEnv)
	}
	if _, err := os.Stat(devcontainer.ComposeOverrideFile(result.StateDir)); !os.IsNotExist(err) {
		t.Fatalf("expected no persisted compose override, got %v", err)
	}
}

func TestBridgeDoctorDoesNotScaffoldBridgeState(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := devcontainer.ResolveReadOnly(ctx, workspace, "")
	if err != nil {
		t.Fatalf("resolve read only: %v", err)
	}
	runner := NewRunner(client)
	report, err := runner.BridgeDoctor(ctx, BridgeDoctorOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("bridge doctor: %v", err)
	}
	if report.Enabled {
		t.Fatalf("expected disabled bridge report %#v", report)
	}
	if _, err := os.Stat(filepath.Join(resolved.StateDir, "bridge")); !os.IsNotExist(err) {
		t.Fatalf("expected no scaffolded bridge dir, got %v", err)
	}
}

func TestComposeUpUpdatesNamedNonRootUserUID(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := "hatchctl-compose-update-uid-test-" + workspaceKey(workspace)
	baseDockerfileDir := t.TempDir()
	dockerfile := "FROM alpine:3.20\nRUN addgroup -g 1234 app && adduser -D -u 1234 -G app app\nUSER app\n"
	if err := os.WriteFile(filepath.Join(baseDockerfileDir, "Containerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.Run(ctx, docker.RunOptions{Args: []string{"build", "-f", filepath.Join(baseDockerfileDir, "Containerfile"), "-t", baseImage, baseDockerfileDir}}); err != nil {
		t.Fatalf("build compose base image: %v", err)
	}
	composePath := filepath.Join(configDir, "docker-compose.yaml")
	composeYAML := "services:\n  app:\n    image: " + baseImage + "\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n    command: [\"/bin/sh\",\"-lc\",\"trap 'exit 0' TERM INT; while sleep 1000; do :; done\"]\n"
	if err := os.WriteFile(composePath, []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"docker-compose.yaml","service":"app","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("build compose image: %v", err)
	}
	if buildResult.Image != baseImage {
		t.Fatalf("expected compose base image, got %q", buildResult.Image)
	}
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("compose up uid image: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", baseImage}})
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"id", "-u"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("compose exec uid check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected compose uid exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != fmt.Sprintf("%d", os.Getuid()) {
		t.Fatalf("unexpected compose updated uid %q want %d", got, os.Getuid())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"id", "-un"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("compose exec user check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected compose user exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "app" {
		t.Fatalf("unexpected compose user name %q", got)
	}
}

func TestComposeImageServiceConsumesLocalFeatures(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)
	composeYAML := "services:\n  app:\n    image: " + baseImage + "\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n    command: [\"/bin/sh\",\"-lc\",\"echo base-compose && exit 0\"]\n"
	if err := os.WriteFile(filepath.Join(configDir, "compose.yaml"), []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLocalFeature(t, filepath.Join(configDir, "feature-a"), `{
		"id": "feature-a",
		"containerEnv": {"COMPOSE_FEATURE_A": "1"},
		"mounts": ["type=volume,source=compose-feature-a,target=/compose-feature-mount"],
		"postCreateCommand": "echo feature-a-postCreate >> /workspaces/demo/events"
	}`, "#!/bin/sh\nset -eu\nprintf '#!/bin/sh\necho compose-feature-a\n' > /usr/local/bin/compose-feature-a\nchmod +x /usr/local/bin/compose-feature-a\n")
	writeLocalFeature(t, filepath.Join(configDir, "feature-b"), `{
		"id": "feature-b",
		"dependsOn": {"feature-a": true},
		"containerEnv": {"COMPOSE_FEATURE_B": "1"},
		"options": {
			"version": {"default": "latest"},
			"extra-flag": {"default": false}
		},
		"postStartCommand": "echo feature-b-postStart >> /workspaces/demo/events"
	}`, "#!/bin/sh\nset -eu\nprintf '%s|%s' \"$VERSION\" \"$EXTRA_FLAG\" > /usr/local/share/compose-feature-options\n")
	config := `{
		"dockerComposeFile": "compose.yaml",
		"service": "app",
		"workspaceFolder": "/workspaces/demo",
		"features": {
			"./feature-b": {"version": "2.2.0", "extra-flag": true},
			"./feature-a": true
		},
		"postStartCommand": "echo config-postStart >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("compose image service up with features: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", upResult.Image}})
	})
	if upResult.Image != storefs.ImageName(workspace, filepath.Join(configDir, "devcontainer.json")) {
		t.Fatalf("unexpected derived image %q", upResult.Image)
	}

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("compose read config: %v", err)
	}
	if configResult.MetadataCount != 3 {
		t.Fatalf("unexpected metadata count %d", configResult.MetadataCount)
	}
	if got := strings.Join(configResult.Mounts, ","); got != "type=volume,source=compose-feature-a,target=/compose-feature-mount" {
		t.Fatalf("unexpected compose feature mounts %q", got)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"sh", "-lc", "compose-feature-a && printf '|'; cat /usr/local/share/compose-feature-options; printf '|%s|%s|' \"$COMPOSE_FEATURE_A\" \"$COMPOSE_FEATURE_B\"; test -d /compose-feature-mount && printf mounted"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("compose feature exec: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected compose feature exit code %d stderr=%s", exitCode, stderr.String())
	}
	if strings.ReplaceAll(strings.TrimSpace(stdout.String()), "\n", "") != "compose-feature-a|2.2.0|true|1|1|mounted" {
		t.Fatalf("unexpected compose feature output %q", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read compose feature lifecycle events: %v", err)
	}
	if got := strings.Join(strings.Fields(string(data)), ","); got != "feature-a-postCreate,feature-b-postStart,config-postStart" {
		t.Fatalf("unexpected compose feature lifecycle order %q", got)
	}
}

func TestComposeDockerfileServiceConsumesLocalFeatures(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	serviceImage := "hatchctl-compose-feature-dockerfile-" + workspaceKey(workspace)
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile"), []byte("FROM alpine:3.20\nRUN mkdir -p /usr/local/share\nCMD [\"/bin/sh\",\"-lc\",\"echo base-compose-build && exit 0\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := "services:\n  app:\n    image: " + serviceImage + "\n    build:\n      context: .\n      dockerfile: Dockerfile\n    working_dir: /workspaces/demo\n    volumes:\n      - ..:/workspaces/demo\n"
	if err := os.WriteFile(filepath.Join(configDir, "compose.yaml"), []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLocalFeature(t, filepath.Join(configDir, "feature-a"), `{
		"id": "feature-a",
		"options": {
			"version": {"default": "latest"}
		},
		"postCreateCommand": "echo compose-dockerfile-feature-postCreate >> /workspaces/demo/events"
	}`, "#!/bin/sh\nset -eu\nprintf '%s' \"$VERSION\" > /usr/local/share/compose-dockerfile-feature-version\n")
	config := `{
		"dockerComposeFile": "compose.yaml",
		"service": "app",
		"workspaceFolder": "/workspaces/demo",
		"features": {
			"./feature-a": "4.0.0"
		},
		"postStartCommand": "echo compose-dockerfile-config-postStart >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("compose dockerfile feature build: %v", err)
	}
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("compose dockerfile feature up: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", upResult.Image}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", serviceImage}})
	})
	if buildResult.Image != storefs.ImageName(workspace, filepath.Join(configDir, "devcontainer.json")) {
		t.Fatalf("unexpected compose dockerfile feature image %q", buildResult.Image)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"sh", "-lc", "cat /usr/local/share/compose-dockerfile-feature-version"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("compose dockerfile feature exec: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 || strings.TrimSpace(stdout.String()) != "4.0.0" {
		t.Fatalf("unexpected compose dockerfile feature output %q exit=%d stderr=%s", stdout.String(), exitCode, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read compose dockerfile feature lifecycle events: %v", err)
	}
	if got := strings.Join(strings.Fields(string(data)), ","); got != "compose-dockerfile-feature-postCreate,compose-dockerfile-config-postStart" {
		t.Fatalf("unexpected compose dockerfile lifecycle order %q", got)
	}
}
