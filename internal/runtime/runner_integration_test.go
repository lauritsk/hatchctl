package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

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

func TestUpPersistsMergedMetadataAndHonorsMergedRuntimeConfig(t *testing.T) {
	client := dockerClientForTest(t)
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseImage := "hatchctl-runtime-test-" + sanitizeName(filepath.Base(workspace))
	baseDockerfileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDockerfileDir, "Dockerfile"), []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	imageMetadata, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{
		RemoteUser:        "root",
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
		Args: []string{"build", "-t", baseImage, "--label", devcontainer.ImageMetadataLabel + "=" + imageMetadata, baseDockerfileDir},
	}); err != nil {
		t.Fatalf("build base image: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rmi", "-f", baseImage}})
	})

	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"name": "up-runtime-test",
		"image": "` + baseImage + `",
		"workspaceFolder": "/workspaces/demo",
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
