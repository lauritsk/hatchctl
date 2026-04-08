package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

var cachedIntegrationFixtures struct {
	mu                sync.Mutex
	plainImage        string
	plainWithCMDImage string
	appUserImage      string
	bridgeHelper      []byte
}

func TestBuildPersistsMetadataLabel(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	dockerfilePath := filepath.Join(configDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := `{
		"name": "build-label-test",
		"dockerFile": "Dockerfile",
		"workspaceFolder": "/workspaces/demo",
		"remoteUser": "root",
		"remoteEnv": {"BUILD_REMOTE": "1"},
		"containerEnv": {"BUILD_CONTAINER": "1"}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	result, err := runner.Build(ctx, BuildOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("build image: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", result.Image}})
	})

	inspect, err := client.InspectImage(ctx, result.Image)
	if err != nil {
		t.Fatalf("inspect image: %v", err)
	}
	entries, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		t.Fatalf("parse metadata label: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 metadata entry, got %#v", entries)
	}
	if entries[0].RemoteUser != "root" {
		t.Fatalf("unexpected remote user %#v", entries[0])
	}
	if entries[0].RemoteEnv["BUILD_REMOTE"] != "1" {
		t.Fatalf("unexpected remote env %#v", entries[0].RemoteEnv)
	}
	if entries[0].ContainerEnv["BUILD_CONTAINER"] != "1" {
		t.Fatalf("unexpected container env %#v", entries[0].ContainerEnv)
	}
}

func TestUpInstallsDotfilesOnceAndReportsStatus(t *testing.T) {
	if os.Getenv("HATCHCTL_RUN_DOTFILES_INTEGRATION") == "" {
		t.Skip("set HATCHCTL_RUN_DOTFILES_INTEGRATION=1 to run dotfiles integration coverage")
	}

	client := dockerClientForTest(t)
	requireIntegrationCommands(t, "git")
	ctx := context.Background()
	workspace := t.TempDir()
	networkName := "hatchctl-dotfiles-net-" + workspaceKey(workspace)
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := "hatchctl-dotfiles-test-" + workspaceKey(workspace)
	baseDockerfileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDockerfileDir, "Dockerfile"), []byte("FROM alpine:3.20\nRUN apk add --no-cache git git-daemon\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.Run(ctx, docker.RunOptions{Args: []string{"build", "-t", baseImage, baseDockerfileDir}}); err != nil {
		t.Fatalf("build base image: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", baseImage}})
	})
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"image": "`+baseImage+`",
		"workspaceFolder": "/workspaces/demo"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dotfilesRepo := filepath.Join(workspace, "dotfiles-repo")
	initGitRepoForTest(t, dotfilesRepo, map[string]string{
		"install": "#!/bin/sh\nset -eu\nmkdir -p \"$HOME/.config/hatchctl-dotfiles\"\necho run >> \"$HOME/.config/hatchctl-dotfiles/count\"\n",
	})
	dotfilesBareRepo := filepath.Join(workspace, "dotfiles-repo.git")
	cloneGitRepoBareForTest(t, dotfilesRepo, dotfilesBareRepo)
	gitServerName := startGitDaemonForTest(t, client, ctx, networkName, baseImage, dotfilesBareRepo)
	waitForGitRepoForTest(t, client, ctx, networkName, baseImage, gitServerName, "git://dotfiles/dotfiles.git")

	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
		"image": "`+baseImage+`",
		"workspaceFolder": "/workspaces/demo",
		"runArgs": ["--network", "`+networkName+`"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	dotfiles := DotfilesOptions{Repository: "git://dotfiles/dotfiles.git"}
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, Dotfiles: dotfiles})
	if err != nil {
		t.Fatalf("up with dotfiles: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace, Dotfiles: dotfiles})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if configResult.Dotfiles == nil || !configResult.Dotfiles.Configured || !configResult.Dotfiles.Applied || configResult.Dotfiles.NeedsInstall {
		t.Fatalf("unexpected dotfiles status %#v", configResult.Dotfiles)
	}

	assertDotfilesInstallCount(t, runner, ctx, workspace, 1)

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, Dotfiles: dotfiles})
	if err != nil {
		t.Fatalf("second up with dotfiles: %v", err)
	}
	if !strings.HasPrefix(upResult.ContainerID, second.ContainerID) && !strings.HasPrefix(second.ContainerID, upResult.ContainerID) {
		t.Fatalf("expected container reuse, first=%q second=%q", upResult.ContainerID, second.ContainerID)
	}
	assertDotfilesInstallCount(t, runner, ctx, workspace, 1)

	state, err := devcontainer.ReadState(configResult.StateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !state.DotfilesReady || state.DotfilesRepo != "git://dotfiles/dotfiles.git" || state.DotfilesTarget != "$HOME/.dotfiles" {
		t.Fatalf("unexpected dotfiles state %#v", state)
	}
}

func TestUpPersistsMergedMetadataAndHonorsMergedRuntimeConfig(t *testing.T) {
	setBridgeHelperEnv(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := sharedAlpineBaseImage(t, client, ctx)
	metadataImage := metadataImageTagForKey(workspaceKey(workspace))
	imageMetadata, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{
		RemoteUser:        "root",
		ForwardPorts:      devcontainer.ForwardPorts{"localhost:7000", "api:9000"},
		RemoteEnv:         map[string]string{"FROM_IMAGE": "1", "SHARED": "image"},
		ContainerEnv:      map[string]string{"IMAGE_CONTAINER": "1", "SHARED_CONTAINER": "image"},
		OnCreateCommand:   devcontainer.LifecycleCommand{Kind: "string", Value: "echo image-onCreate >> /workspaces/demo/events", Exists: true},
		PostStartCommand:  devcontainer.LifecycleCommand{Kind: "string", Value: "echo image-postStart >> /workspaces/demo/events", Exists: true},
		PostAttachCommand: devcontainer.LifecycleCommand{Kind: "string", Value: "echo image-postAttach >> /workspaces/demo/events", Exists: true},
	}})
	if err != nil {
		t.Fatalf("marshal image metadata: %v", err)
	}
	if err := client.Run(ctx, docker.RunOptions{
		Args: []string{"build", "-t", metadataImage, "--label", devcontainer.ImageMetadataLabel + "=" + imageMetadata, sharedDockerBuildContext(t, "FROM "+baseImage+"\n")},
	}); err != nil {
		t.Fatalf("build base image: %v", err)
	}
	baseImage = metadataImage
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", baseImage}})
	})

	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"name": "up-runtime-test",
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo",
		"forwardPorts": [7000, 8080, "api:9000"],
		"remoteEnv": {
			"SHARED": "config",
			"CONFIG_ONLY": "1"
		},
		"containerEnv": {
			"SHARED_CONTAINER": "config",
			"CONFIG_CONTAINER": "1"
		},
		"onCreateCommand": "echo config-onCreate >> /workspaces/demo/events",
		"updateContentCommand": "echo config-updateContent >> /workspaces/demo/events",
		"postCreateCommand": "echo config-postCreate >> /workspaces/demo/events",
		"postStartCommand": "echo config-postStart >> /workspaces/demo/events",
		"postAttachCommand": "echo config-postAttach >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	var containerID string
	var bridgeStateDir string
	t.Cleanup(func() {
		if containerID != "" {
			_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", containerID}})
		}
		if bridgeStateDir != "" {
			_ = bridge.Stop(bridgeStateDir)
		}
	})

	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	containerID = upResult.ContainerID
	bridgeStateDir = upResult.StateDir
	if upResult.Bridge == nil || !upResult.Bridge.Enabled {
		t.Fatalf("expected bridge report, got %#v", upResult.Bridge)
	}

	inspect, err := client.InspectContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("inspect container: %v", err)
	}
	entries, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		t.Fatalf("parse container metadata label: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 metadata entries, got %#v", entries)
	}
	if entries[0].RemoteEnv["FROM_IMAGE"] != "1" || entries[1].RemoteEnv["CONFIG_ONLY"] != "1" {
		t.Fatalf("unexpected persisted entries %#v", entries)
	}
	if got := []string(entries[0].ForwardPorts); strings.Join(got, ",") != "localhost:7000,api:9000" {
		t.Fatalf("unexpected image forward ports %#v", entries[0].ForwardPorts)
	}
	if got := []string(entries[1].ForwardPorts); strings.Join(got, ",") != "localhost:7000,localhost:8080,api:9000" {
		t.Fatalf("unexpected config forward ports %#v", entries[1].ForwardPorts)
	}

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := strings.Join(configResult.ForwardPorts, ","); got != "localhost:7000,api:9000,localhost:8080" {
		t.Fatalf("unexpected merged forward ports %q", got)
	}
	if configResult.Bridge == nil || !configResult.Bridge.Enabled {
		t.Fatalf("expected bridge in config result, got %#v", configResult.Bridge)
	}
	if configResult.ManagedContainer == nil {
		t.Fatal("expected managed container state in config result")
	}
	if configResult.ManagedContainer.ID != containerID {
		t.Fatalf("unexpected managed container id %q", configResult.ManagedContainer.ID)
	}
	if !configResult.ManagedContainer.Running || configResult.ManagedContainer.Status != "running" {
		t.Fatalf("unexpected managed container state %#v", configResult.ManagedContainer)
	}
	if configResult.ManagedContainer.RemoteUser != "root" {
		t.Fatalf("unexpected managed container user %#v", configResult.ManagedContainer)
	}
	if got := configResult.ManagedContainer.ContainerEnv["BROWSER"]; got != "/var/run/hatchctl/bridge/bin/devcontainer-open" {
		t.Fatalf("unexpected managed container env %#v", configResult.ManagedContainer.ContainerEnv)
	}
	if _, ok := configResult.ManagedContainer.ContainerEnv["DEVCONTAINER_BRIDGE_HELPER_SOCKET"]; ok {
		t.Fatalf("unexpected legacy helper socket env %#v", configResult.ManagedContainer.ContainerEnv)
	}
	if !configResult.ManagedContainer.BridgeEnabled {
		t.Fatalf("expected managed container bridge to be enabled %#v", configResult.ManagedContainer)
	}
	if got := strings.Join(configResult.ManagedContainer.ForwardPorts, ","); got != "localhost:7000,api:9000,localhost:8080" {
		t.Fatalf("unexpected managed container forward ports %q", got)
	}

	workspace2 := t.TempDir()
	configDir2 := filepath.Join(workspace2, ".devcontainer")
	if err := os.MkdirAll(configDir2, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath2 := filepath.Join(configDir2, "devcontainer.json")
	if err := os.WriteFile(configPath2, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configResult2, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace2})
	if err != nil {
		t.Fatalf("read config without container: %v", err)
	}
	if configResult2.ManagedContainer != nil {
		t.Fatalf("expected no managed container state, got %#v", configResult2.ManagedContainer)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", `printf '%s|%s|%s|%s|%s|%s' "$FROM_IMAGE" "$SHARED" "$CONFIG_ONLY" "$IMAGE_CONTAINER" "$SHARED_CONTAINER" "$CONFIG_CONTAINER"`},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec merged env check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "1|config|1|1|config|1" {
		t.Fatalf("unexpected merged env output %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"id", "-un"},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec user check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected user check exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "root" {
		t.Fatalf("unexpected exec user %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", `printf '%s|%s|%s' "$BROWSER" "$DEVCONTAINER_BRIDGE_ENABLED" "${DEVCONTAINER_BRIDGE_HELPER_SOCKET:-}"`},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec bridge env check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected bridge env exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "/var/run/hatchctl/bridge/bin/devcontainer-open|true|" {
		t.Fatalf("unexpected bridge env output %q", got)
	}

	eventsData, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read lifecycle events: %v", err)
	}
	events := strings.Fields(strings.TrimSpace(string(eventsData)))
	want := []string{
		"image-onCreate",
		"config-onCreate",
		"config-updateContent",
		"config-postCreate",
		"image-postStart",
		"config-postStart",
		"image-postAttach",
		"config-postAttach",
	}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected lifecycle order %v want %v", events, want)
	}
}

func TestReadConfigAndExecFallBackToImageUser(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := sharedAppUserImage(t, client, ctx)

	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"name": "image-user-fallback-test",
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo"
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	var containerID string
	t.Cleanup(func() {
		if containerID != "" {
			_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", containerID}})
		}
	})

	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	containerID = upResult.ContainerID

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if configResult.ImageUser != "app" {
		t.Fatalf("unexpected image user %q", configResult.ImageUser)
	}
	if configResult.RemoteUser != "app" {
		t.Fatalf("unexpected resolved remote user %q", configResult.RemoteUser)
	}
	if configResult.ManagedContainer == nil || configResult.ManagedContainer.RemoteUser != "app" {
		t.Fatalf("unexpected managed container %#v", configResult.ManagedContainer)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"id", "-un"},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec image-user check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected image-user exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "app" {
		t.Fatalf("unexpected exec user %q", got)
	}
}

func TestLifecycleAndExecUseResolvedHomeForImageUser(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := sharedAppUserImage(t, client, ctx)

	config := `{
		"name": "home-fallback-test",
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo",
		"postCreateCommand": "printf %s \"$HOME\" > /workspaces/demo/post-create-home"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})

	data, err := os.ReadFile(filepath.Join(workspace, "post-create-home"))
	if err != nil {
		t.Fatalf("read post-create HOME: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "/home/app" {
		t.Fatalf("unexpected postCreate HOME %q", got)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", "printf %s \"$HOME\""},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec HOME check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected HOME exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "/home/app" {
		t.Fatalf("unexpected exec HOME %q", got)
	}
}

func TestExecStreamsStdinWithoutTTYAndReturnsExitCode(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)

	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})

	stdin := bytes.NewBuffer([]byte{0x00, 0x01, 0x02, 0x7f, 0x80, 0xff})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"cat"},
		Stdin:     stdin,
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec cat: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exec cat exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), []byte{0x00, 0x01, 0x02, 0x7f, 0x80, 0xff}) {
		t.Fatalf("unexpected echoed stdin %v", stdout.Bytes())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", "[ ! -t 1 ]"},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec non-tty check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("expected non-tty stdout with buffered writers, exit=%d stderr=%s", exitCode, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", "exit 123"},
		Stdout:    io.Discard,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec exit code check returned error: %v", err)
	}
	if exitCode != 123 {
		t.Fatalf("unexpected exit code %d (stderr: %s)", exitCode, stderr.String())
	}
}

func TestExecWithoutCommandStartsInRemoteWorkspace(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)

	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Stdin:     strings.NewReader("pwd\nexit\n"),
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec default shell: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected default shell exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "/workspaces/demo") {
		t.Fatalf("expected default shell to start in workspace, got %q", stdout.String())
	}
}

func TestUpUpdatesNamedNonRootUserUID(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := "hatchctl-update-uid-test-" + workspaceKey(workspace)
	baseDockerfileDir := t.TempDir()
	dockerfile := "FROM alpine:3.20\nRUN addgroup -g 1234 app && adduser -D -u 1234 -G app app\nUSER app\n"
	if err := os.WriteFile(filepath.Join(baseDockerfileDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.Run(ctx, docker.RunOptions{Args: []string{"build", "-t", baseImage, baseDockerfileDir}}); err != nil {
		t.Fatalf("build base image: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", baseImage}})
	})

	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("build image: %v", err)
	}
	if buildResult.Image != baseImage {
		t.Fatalf("expected base image, got %q", buildResult.Image)
	}

	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("up container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"id", "-u"},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec uid check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected uid exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != fmt.Sprintf("%d", os.Getuid()) {
		t.Fatalf("unexpected updated uid %q want %d", got, os.Getuid())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode, err = runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"id", "-un"},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("exec user check: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected user exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "app" {
		t.Fatalf("unexpected user name %q", got)
	}
}

func TestUpReconcilesMissingStateContainerToExistingManagedContainer(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.20",
		"workspaceFolder": "/workspaces/demo",
		"postStartCommand": "echo started >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	baseImage := sharedAlpineBaseImage(t, client, ctx)
	if err := os.WriteFile(configPath, []byte(strings.ReplaceAll(config, "alpine:3.20", baseImage)), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := devcontainer.Resolve(ctx, workspace, "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	stateDir := resolved.StateDir
	containerName := resolved.ContainerName
	labels := []string{
		"--label", devcontainer.HostFolderLabel + "=" + workspace,
		"--label", devcontainer.ConfigFileLabel + "=" + configPath,
		"--label", devcontainer.ManagedByLabel + "=" + devcontainer.ManagedByValue,
	}
	containerID, err := client.Output(ctx, append([]string{"run", "-d", "--name", containerName}, append(labels, "--mount", resolved.WorkspaceMount, baseImage, "/bin/sh", "-lc", "trap 'exit 0' TERM INT; while sleep 1000; do :; done")...)...)
	if err != nil {
		t.Fatalf("create managed container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", containerID}})
	})
	if err := devcontainer.WriteState(stateDir, devcontainer.State{ContainerID: "missing-container", LifecycleReady: true}); err != nil {
		t.Fatalf("write stale state: %v", err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("up with reconciled state: %v", err)
	}
	if !strings.HasPrefix(containerID, upResult.ContainerID) && !strings.HasPrefix(upResult.ContainerID, containerID) {
		t.Fatalf("unexpected reconciled container id %q want %q", upResult.ContainerID, containerID)
	}
	state, err := devcontainer.ReadState(stateDir)
	if err != nil {
		t.Fatalf("read reconciled state: %v", err)
	}
	if !strings.HasPrefix(containerID, state.ContainerID) && !strings.HasPrefix(state.ContainerID, containerID) {
		t.Fatalf("unexpected stored container id %q want %q", state.ContainerID, containerID)
	}
	if !state.LifecycleReady {
		t.Fatalf("expected lifecycle ready after reconciliation %#v", state)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read lifecycle events: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "started" {
		t.Fatalf("unexpected lifecycle output %q", got)
	}
}

func TestUpRecreateRemovesReconciledManagedContainer(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	baseImage := sharedAlpineBaseImage(t, client, ctx)
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := devcontainer.Resolve(ctx, workspace, "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	labels := []string{
		"--label", devcontainer.HostFolderLabel + "=" + workspace,
		"--label", devcontainer.ConfigFileLabel + "=" + configPath,
		"--label", devcontainer.ManagedByLabel + "=" + devcontainer.ManagedByValue,
	}
	oldContainerID, err := client.Output(ctx, append([]string{"run", "-d", "--name", resolved.ContainerName}, append(labels, "--mount", resolved.WorkspaceMount, baseImage, "/bin/sh", "-lc", "trap 'exit 0' TERM INT; while sleep 1000; do :; done")...)...)
	if err != nil {
		t.Fatalf("create managed container: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", oldContainerID}})
	})
	if err := devcontainer.WriteState(resolved.StateDir, devcontainer.State{ContainerID: "missing-container", LifecycleReady: true}); err != nil {
		t.Fatalf("write stale state: %v", err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("up recreate: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
	})
	if upResult.ContainerID == oldContainerID {
		t.Fatalf("expected recreated container id to differ from %q", oldContainerID)
	}
	if _, err := client.InspectContainer(ctx, oldContainerID); err == nil {
		t.Fatalf("expected old container %q to be removed", oldContainerID)
	}
}

func TestUpStartsBridgeOnFirstRunAndReusesSession(t *testing.T) {
	skipBridgeLifecycleIntegrationOnDarwin(t)
	setBridgeHelperEnv(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	first, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("first up: %v", err)
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
		t.Fatalf("unexpected bridge report %#v", first.Bridge)
	}

	sessionPath := filepath.Join(first.StateDir, "bridge", "session.json")
	configJSONPath := filepath.Join(first.StateDir, "bridge", "bridge-config.json")
	statusPath := filepath.Join(first.StateDir, "bridge", "bridge-status.json")
	var session struct {
		ID string `json:"id"`
	}
	readJSONFile(t, sessionPath, &session)
	if session.ID == "" {
		t.Fatalf("unexpected session %#v", session)
	}
	if runtime.GOOS == "darwin" {
		var bridgeConfig struct {
			SessionID   string `json:"sessionId"`
			ContainerID string `json:"containerId"`
		}
		readJSONFile(t, configJSONPath, &bridgeConfig)
		if bridgeConfig.SessionID != session.ID || bridgeConfig.ContainerID != first.ContainerID {
			t.Fatalf("unexpected bridge config %#v session=%#v", bridgeConfig, session)
		}
		var status struct {
			SessionID   string `json:"sessionId"`
			ContainerID string `json:"containerId"`
			LastEvent   string `json:"lastEvent"`
		}
		readJSONFile(t, statusPath, &status)
		if status.SessionID != session.ID || status.ContainerID != first.ContainerID || status.LastEvent != "running" {
			t.Fatalf("unexpected bridge status %#v", status)
		}
	}

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("second up: %v", err)
	}
	if !strings.HasPrefix(first.ContainerID, second.ContainerID) && !strings.HasPrefix(second.ContainerID, first.ContainerID) {
		t.Fatalf("expected container reuse, first=%q second=%q", first.ContainerID, second.ContainerID)
	}
	if second.Bridge == nil || second.Bridge.ID != session.ID {
		t.Fatalf("expected bridge session reuse, got %#v", second.Bridge)
	}
	state, err := devcontainer.ReadState(first.StateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !state.BridgeEnabled || state.BridgeSessionID != session.ID {
		t.Fatalf("unexpected stored bridge state %#v", state)
	}
}

func TestUpEnablingBridgeRecreatesExistingManagedContainer(t *testing.T) {
	skipBridgeLifecycleIntegrationOnDarwin(t)
	setBridgeHelperEnv(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"`+baseImage+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	first, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true})
	if err != nil {
		t.Fatalf("first up: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", first.ContainerID}})
	})

	second, err := runner.Up(ctx, UpOptions{Workspace: workspace, BridgeEnabled: true})
	if err != nil {
		t.Fatalf("second up with bridge: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", second.ContainerID}})
		_ = bridge.Stop(second.StateDir)
	})
	if second.ContainerID == first.ContainerID {
		t.Fatalf("expected bridge enablement to recreate container, reused %q", second.ContainerID)
	}
	if _, err := client.InspectContainer(ctx, first.ContainerID); err == nil {
		t.Fatalf("expected old container %q to be removed", first.ContainerID)
	}
	inspect, err := client.InspectContainer(ctx, second.ContainerID)
	if err != nil {
		t.Fatalf("inspect recreated container: %v", err)
	}
	if inspect.Config.Labels[devcontainer.BridgeEnabledLabel] != "true" {
		t.Fatalf("expected bridge label on recreated container %#v", inspect.Config.Labels)
	}
	if got := envMap(inspect.Config.Env)["BROWSER"]; got != "/var/run/hatchctl/bridge/bin/devcontainer-open" {
		t.Fatalf("expected bridge env on recreated container %#v", inspect.Config.Env)
	}
	state, err := devcontainer.ReadState(second.StateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !state.BridgeEnabled || state.BridgeSessionID == "" {
		t.Fatalf("unexpected stored bridge state %#v", state)
	}
}

func setBridgeHelperEnv(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hatchctl")
	data := sharedBridgeHelperBinary(t)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("write per-test bridge helper: %v", err)
	}
	t.Setenv("HATCHCTL_BRIDGE_HELPER", path)
}

func skipBridgeLifecycleIntegrationOnDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" && os.Getenv("HATCHCTL_RUN_BRIDGE_INTEGRATION") == "" {
		t.Skip("disabled on darwin by default; set HATCHCTL_RUN_BRIDGE_INTEGRATION=1 to run manually")
	}
}

func skipHeavyDockerIntegrationOnDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" && os.Getenv("HATCHCTL_RUN_HEAVY_DOCKER_INTEGRATION") == "" {
		t.Skip("disabled on darwin by default; set HATCHCTL_RUN_HEAVY_DOCKER_INTEGRATION=1 to run manually")
	}
}

func TestBuildConsumesLocalFeaturesFromImageSource(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedAlpineBaseImage(t, client, ctx)
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", devcontainer.ImageName(workspace, filepath.Join(configDir, "devcontainer.json"))}})
	})
	writeLocalFeature(t, filepath.Join(configDir, "feature-a"), `{
		"id": "feature-a",
		"containerEnv": {"FEATURE_A_ENV": "1"},
		"customizations": {"vscode": {"extensions": ["feature.a"]}}
	}`, "#!/bin/sh\nset -eu\nmkdir -p /usr/local/bin\nprintf '#!/bin/sh\necho feature-a\n' > /usr/local/bin/feature-a\nchmod +x /usr/local/bin/feature-a\n")
	writeLocalFeature(t, filepath.Join(configDir, "feature-b"), `{
		"id": "feature-b",
		"dependsOn": {"feature-a": true},
		"containerEnv": {"FEATURE_B_ENV": "1"}
	}`, "#!/bin/sh\nset -eu\nprintf '%s|%s' \"$VERSION\" \"$OTHER_OPTION\" > /usr/local/share/feature-b-options\n")
	config := `{
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo",
		"features": {
			"./feature-b": {"version": "2.0.0", "other-option": true},
			"./feature-a": true
		},
		"postCreateCommand": "echo config-postCreate >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("build image with features: %v", err)
	}
	inspect, err := client.InspectImage(ctx, buildResult.Image)
	if err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
	entries, err := devcontainer.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		t.Fatalf("parse feature metadata label: %v", err)
	}
	if len(entries) != 3 || entries[0].ID != "feature-a" || entries[1].ID != "feature-b" {
		t.Fatalf("unexpected feature metadata %#v", entries)
	}
	if got := envMap(inspect.Config.Env)["FEATURE_B_ENV"]; got != "1" {
		t.Fatalf("expected image env from features, got %#v", inspect.Config.Env)
	}
}

func TestBuildTreatsFeatureOptionValuesAsData(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseImage := sharedDataDirBaseImage(t, client, ctx)
	derivedImage := devcontainer.ImageName(workspace, filepath.Join(configDir, "devcontainer.json"))
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", derivedImage}})
	})
	writeLocalFeature(t, filepath.Join(configDir, "feature-a"), `{
		"id": "feature-a",
		"options": {
			"dangerous": {"default": "safe"}
		}
	}`, "#!/bin/sh\nset -eu\nprintf '%s' \"$DANGEROUS\" > /usr/local/share/feature-dangerous\n")
	config := `{
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo",
		"features": {
			"./feature-a": {
				"dangerous": "$(touch /tmp/pwned) literal 'quoted' value"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	buildResult, err := runner.Build(ctx, BuildOptions{Workspace: workspace, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("build image with dangerous feature option: %v", err)
	}

	output, err := client.Output(ctx, "run", "--rm", buildResult.Image, "sh", "-lc", "cat /usr/local/share/feature-dangerous; printf '|'; if [ -e /tmp/pwned ]; then printf pwned; else printf clean; fi")
	if err != nil {
		t.Fatalf("inspect built image state: %v", err)
	}
	if got := strings.TrimSpace(output); got != "$(touch /tmp/pwned) literal 'quoted' value|clean" {
		t.Fatalf("unexpected dangerous option handling %q", got)
	}
}

func TestUpConsumesLocalFeaturesFromDockerfileSource(t *testing.T) {
	skipHeavyDockerIntegrationOnDarwin(t)
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile"), []byte("FROM alpine:3.20\nRUN mkdir -p /usr/local/share\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLocalFeature(t, filepath.Join(configDir, "feature-a"), `{
		"id": "feature-a",
		"mounts": ["type=volume,source=feature-a,target=/feature-mount"],
		"onCreateCommand": "echo feature-a-onCreate >> /workspaces/demo/events",
		"postCreateCommand": "echo feature-a-postCreate >> /workspaces/demo/events"
	}`, "#!/bin/sh\nset -eu\nprintf '%s' \"$VERSION\" > /usr/local/share/feature-a-version\n")
	writeLocalFeature(t, filepath.Join(configDir, "feature-b"), `{
		"id": "feature-b",
		"dependsOn": {"feature-a": true},
		"containerEnv": {"FEATURE_B_ENV": "1"},
		"mounts": ["type=volume,source=feature-b,target=/feature-mount"],
		"postStartCommand": "echo feature-b-postStart >> /workspaces/demo/events"
	}`, "#!/bin/sh\nset -eu\nprintf '%s|%s' \"$VERSION\" \"$OTHER_OPTION\" > /usr/local/share/feature-b-options\n")
	config := `{
		"dockerFile": "Dockerfile",
		"workspaceFolder": "/workspaces/demo",
		"features": {
			"./feature-b": {"version": "2.1.0", "other-option": true},
			"./feature-a": "1.5.0"
		},
		"mounts": ["type=volume,source=config,target=/config-only"],
		"onCreateCommand": "echo config-onCreate >> /workspaces/demo/events",
		"postCreateCommand": "echo config-postCreate >> /workspaces/demo/events",
		"postStartCommand": "echo config-postStart >> /workspaces/demo/events"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(client)
	upResult, err := runner.Up(ctx, UpOptions{Workspace: workspace, Recreate: true, TrustWorkspace: true})
	if err != nil {
		t.Fatalf("up with dockerfile features: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", upResult.ContainerID}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", upResult.Image}})
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", devcontainer.ImageName(workspace, filepath.Join(configDir, "devcontainer.json")) + "-base"}})
	})

	configResult, err := runner.ReadConfig(ctx, ReadConfigOptions{Workspace: workspace})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := strings.Join(configResult.Mounts, ","); got != "type=volume,source=feature-b,target=/feature-mount,type=volume,source=config,target=/config-only" {
		t.Fatalf("unexpected merged mounts %q", got)
	}
	if configResult.MetadataCount != 3 {
		t.Fatalf("unexpected metadata count %d", configResult.MetadataCount)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{Workspace: workspace, Args: []string{"sh", "-lc", "cat /usr/local/share/feature-a-version && printf '|' && cat /usr/local/share/feature-b-options && printf '|%s' \"$FEATURE_B_ENV\""}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("exec feature verification: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected feature verification exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.5.0|2.1.0|true|1" {
		t.Fatalf("unexpected feature output %q", got)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "events"))
	if err != nil {
		t.Fatalf("read lifecycle events: %v", err)
	}
	if got := strings.Join(strings.Fields(string(data)), ","); got != "feature-a-onCreate,config-onCreate,feature-a-postCreate,config-postCreate,feature-b-postStart,config-postStart" {
		t.Fatalf("unexpected lifecycle order %q", got)
	}
}

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
	if upResult.Image != devcontainer.ImageName(workspace, filepath.Join(configDir, "devcontainer.json")) {
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
	if buildResult.Image != devcontainer.ImageName(workspace, filepath.Join(configDir, "devcontainer.json")) {
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

func dockerClientForTest(t *testing.T) *docker.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}
	client := docker.NewClient("docker")
	if _, err := client.Output(context.Background(), "version", "--format", "{{.Server.Version}}"); err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	return client
}

func requireIntegrationCommands(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s unavailable: %v", name, err)
		}
	}
}

func sharedAlpineBaseImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.plainImage, "base", "FROM alpine:3.20\n")
}

func sharedAlpineWithCommandImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.plainWithCMDImage, "base-cmd", "FROM alpine:3.20\nCMD [\"/bin/sh\",\"-lc\",\"trap 'exit 0' TERM INT; while sleep 1000; do :; done\"]\n")
}

func sharedAppUserImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.appUserImage, "app-user", "FROM alpine:3.20\nRUN adduser -D -u 1000 app\nUSER app\n")
}

func sharedDataDirBaseImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, nil, metadataImageTagForKey("data-dir"), "FROM alpine:3.20\nRUN mkdir -p /usr/local/share\n")
}

func sharedTaggedImage(t *testing.T, client *docker.Client, ctx context.Context, slot *string, key string, dockerfile string) string {
	t.Helper()
	tag := metadataImageTagForKey(key)
	if slot != nil {
		cachedIntegrationFixtures.mu.Lock()
		if *slot == "" {
			*slot = tag
		}
		tag = *slot
		cachedIntegrationFixtures.mu.Unlock()
	}
	buildImageIfMissing(t, client, ctx, tag, dockerfile)
	return tag
}

func buildImageIfMissing(t *testing.T, client *docker.Client, ctx context.Context, tag string, dockerfile string) {
	t.Helper()
	if _, err := client.InspectImage(ctx, tag); err == nil {
		return
	}
	if err := client.Run(ctx, docker.RunOptions{Args: []string{"build", "-t", tag, sharedDockerBuildContext(t, dockerfile)}}); err != nil {
		t.Fatalf("build shared image %q: %v", tag, err)
	}
}

func sharedDockerBuildContext(t *testing.T, dockerfile string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sharedBridgeHelperBinary(t *testing.T) []byte {
	t.Helper()
	requireIntegrationCommands(t, "go")
	cachedIntegrationFixtures.mu.Lock()
	defer cachedIntegrationFixtures.mu.Unlock()
	if len(cachedIntegrationFixtures.bridgeHelper) > 0 {
		return append([]byte(nil), cachedIntegrationFixtures.bridgeHelper...)
	}
	path := filepath.Join(t.TempDir(), "hatchctl")
	cmd := exec.Command("go", "build", "-o", path, "./cmd/hatchctl")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bridge helper: %v: %s", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read built bridge helper: %v", err)
	}
	cachedIntegrationFixtures.bridgeHelper = data
	return append([]byte(nil), data...)
}

func metadataImageTagForKey(key string) string {
	return "hatchctl-test-" + sanitizeName(key)
}

func workspaceKey(workspace string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(workspace))
	return fmt.Sprintf("%s-%x", sanitizeName(filepath.Base(workspace)), hash.Sum64())
}

func readJSONFile(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func writeLocalFeature(t *testing.T, dir string, manifest string, install string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte(install), 0o755); err != nil {
		t.Fatal(err)
	}
}

func envMap(values []string) map[string]string {
	result := map[string]string{}
	for _, value := range values {
		key, raw, ok := strings.Cut(value, "=")
		if ok {
			result[key] = raw
		}
	}
	return result
}

func assertDotfilesInstallCount(t *testing.T, runner *Runner, ctx context.Context, workspace string, want int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", `wc -l < "$HOME/.config/hatchctl-dotfiles/count"`},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("read dotfiles install count: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected count exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != fmt.Sprintf("%d", want) {
		t.Fatalf("unexpected dotfiles install count %q want %d", got, want)
	}
}

func initGitRepoForTest(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if name == "install" || strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
		}
	}
	runGit("init")
	runGit("add", ".")
	runGit(
		"-c", "user.name=Test User",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"commit", "-m", "init",
	)
}

func cloneGitRepoBareForTest(t *testing.T, src string, dst string) {
	t.Helper()
	cmd := exec.Command("git", "clone", "--bare", src, dst)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone --bare: %v\n%s", err, string(output))
	}
}

func startGitDaemonForTest(t *testing.T, client *docker.Client, ctx context.Context, networkName string, image string, repoPath string) string {
	t.Helper()
	if _, err := client.Output(ctx, "network", "create", networkName); err != nil {
		t.Fatalf("create docker network: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"network", "rm", networkName}})
	})

	serverName := networkName + "-git"
	if _, err := client.Output(ctx,
		"run", "-d",
		"--name", serverName,
		"--network", networkName,
		"--network-alias", "dotfiles",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=/srv/git/dotfiles.git,readonly", repoPath),
		image,
		"sh", "-lc", "exec git daemon --reuseaddr --base-path=/srv/git --export-all --verbose /srv/git",
	); err != nil {
		t.Fatalf("start git daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", serverName}})
	})
	return serverName
}

func waitForGitRepoForTest(t *testing.T, client *docker.Client, ctx context.Context, networkName string, image string, serverName string, repoURL string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := client.Run(ctx, docker.RunOptions{Args: []string{
			"run", "--rm",
			"--network", networkName,
			image,
			"sh", "-lc", "git ls-remote " + quoteShell(repoURL) + " >/dev/null 2>&1",
		}})
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			logs, logErr := client.CombinedOutput(ctx, "logs", serverName)
			if logErr != nil {
				logs = fmt.Sprintf("(unable to read logs: %v)", logErr)
			}
			t.Fatalf("wait for git repo %s: %v\nserver logs:\n%s", repoURL, err, logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func sanitizeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return fmt.Sprintf("tmp-%d", os.Getpid())
	}
	return result
}
