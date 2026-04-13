//go:build integration

package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	backendfactory "github.com/lauritsk/hatchctl/internal/backend/factory"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/reconcile"
	"github.com/lauritsk/hatchctl/internal/spec"
)

var integrationBackends = []string{"docker", "podman"}

func integrationClient(t *testing.T, name string) backend.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	client, err := backendfactory.New(name)
	if err != nil {
		t.Fatalf("new integration backend %s: %v", name, err)
	}
	cmd := exec.CommandContext(context.Background(), name, "info", "--format", "{{json .}}")
	if _, err := cmd.Output(); err != nil {
		t.Skipf("%s unavailable: %v", name, err)
	}
	return client
}

func integrationService(client backend.Client) *Service {
	return NewWithExecutor(reconcile.NewExecutor(client))
}

func integrationIO() CommandIO {
	return CommandIO{Stdout: io.Discard, Stderr: io.Discard}
}

func TestBuildPersistsMetadataLabel(t *testing.T) {
	for _, name := range integrationBackends {
		t.Run(name, func(t *testing.T) {
			client := integrationClient(t, name)
			service := integrationService(client)
			ctx := context.Background()
			workspace := t.TempDir()
			configDir := filepath.Join(workspace, ".devcontainer")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			buildFile := client.BuildDefinitionFileName()
			if err := os.WriteFile(filepath.Join(configDir, buildFile), []byte("FROM alpine:3.23\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{
				"name": "build-label-test",
				"dockerFile": "`+buildFile+`",
				"workspaceFolder": "/workspaces/demo",
				"remoteUser": "root",
				"remoteEnv": {"BUILD_REMOTE": "1"},
				"containerEnv": {"BUILD_CONTAINER": "1"}
			}`), 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := service.Build(ctx, BuildRequest{Defaults: CommandDefaults{Backend: name, Workspace: workspace, LockfilePolicy: "auto"}, IO: integrationIO()})
			if err != nil {
				t.Fatalf("build image: %v", err)
			}
			t.Cleanup(func() {
				_ = exec.CommandContext(ctx, name, "rmi", "-f", result.Image).Run()
			})

			inspect, err := client.InspectImage(ctx, result.Image)
			if err != nil {
				t.Fatalf("inspect image: %v", err)
			}
			entries, err := spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
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
		})
	}
}

func TestReadConfigDoesNotWriteWorkspaceArtifacts(t *testing.T) {
	for _, name := range integrationBackends {
		t.Run(name, func(t *testing.T) {
			client := integrationClient(t, name)
			service := integrationService(client)
			ctx := context.Background()
			workspace := t.TempDir()
			configDir := filepath.Join(workspace, ".devcontainer")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"alpine:3.23","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			stateDir := filepath.Join(t.TempDir(), "state")
			cacheDir := filepath.Join(t.TempDir(), "cache")

			_, err := service.ReadConfig(ctx, ReadConfigRequest{Defaults: CommandDefaults{Backend: name, Workspace: workspace, StateDir: stateDir, CacheDir: cacheDir, LockfilePolicy: "frozen"}, IO: integrationIO()})
			if err != nil {
				t.Fatalf("read config: %v", err)
			}

			assertDirEmptyOrMissing(t, stateDir)
			assertDirEmptyOrMissing(t, cacheDir)
		})
	}
}

func TestBuildDoesNotWriteWorkspaceState(t *testing.T) {
	for _, name := range integrationBackends {
		t.Run(name, func(t *testing.T) {
			client := integrationClient(t, name)
			service := integrationService(client)
			ctx := context.Background()
			workspace := t.TempDir()
			configDir := filepath.Join(workspace, ".devcontainer")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			buildFile := client.BuildDefinitionFileName()
			if err := os.WriteFile(filepath.Join(configDir, buildFile), []byte("FROM alpine:3.23\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"dockerFile":"`+buildFile+`","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			stateDir := filepath.Join(t.TempDir(), "state")
			cacheDir := filepath.Join(t.TempDir(), "cache")

			result, err := service.Build(ctx, BuildRequest{Defaults: CommandDefaults{Backend: name, Workspace: workspace, StateDir: stateDir, CacheDir: cacheDir, LockfilePolicy: "auto"}, IO: integrationIO()})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			t.Cleanup(func() {
				_ = exec.CommandContext(ctx, name, "rmi", "-f", result.Image).Run()
			})

			if _, err := os.Stat(filepath.Join(stateDir, "state.json")); !os.IsNotExist(err) {
				t.Fatalf("expected build to avoid state.json, got %v", err)
			}
		})
	}
}

func assertDirEmptyOrMissing(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read dir %s: %v", path, err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected %s to stay empty, found %d entries", path, len(entries))
	}
}
