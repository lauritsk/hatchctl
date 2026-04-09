package runtime

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestReadConfigDoesNotWriteWorkspaceArtifacts(t *testing.T) {
	t.Parallel()

	workspace := writeTestWorkspaceConfig(t, `{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`)
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	runner := newTestRunner(t, &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) != 0 && cmd.Args[0] == "ps" {
				return "", nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
		inspectImage: func(_ context.Context, image string) (docker.ImageInspect, error) {
			if image != "alpine:3.20" {
				t.Fatalf("unexpected image inspect %q", image)
			}
			return docker.ImageInspect{ID: "img", Config: docker.InspectConfig{Labels: map[string]string{}}}, nil
		},
	})

	_, err := runner.ReadConfig(context.Background(), ReadConfigOptions{
		Workspace: workspace,
		StateDir:  stateDir,
		CacheDir:  cacheDir,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	assertDirEmptyOrMissing(t, stateDir)
	assertDirEmptyOrMissing(t, cacheDir)
}

func TestBridgeDoctorDoesNotWriteWorkspaceArtifacts(t *testing.T) {
	t.Parallel()

	workspace := writeTestWorkspaceConfig(t, `{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`)
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	runner := newTestRunner(t, nil)

	_, err := runner.BridgeDoctor(context.Background(), BridgeDoctorOptions{
		Workspace: workspace,
		StateDir:  stateDir,
		CacheDir:  cacheDir,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	})
	if err != nil {
		t.Fatalf("bridge doctor: %v", err)
	}

	assertDirEmptyOrMissing(t, stateDir)
	assertDirEmptyOrMissing(t, cacheDir)
}

func TestBuildDoesNotWriteWorkspaceState(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile"), []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"dockerFile":"Dockerfile","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	runner := newTestRunner(t, &fakeRuntimeBackend{
		inspectImage: func(_ context.Context, image string) (docker.ImageInspect, error) {
			return docker.ImageInspect{ID: image, Config: docker.InspectConfig{Labels: map[string]string{}}}, nil
		},
	})

	_, err := runner.Build(context.Background(), BuildOptions{
		Workspace: workspace,
		StateDir:  stateDir,
		CacheDir:  cacheDir,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(stateDir, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected build to avoid state.json, got %v", err)
	}
}

func writeTestWorkspaceConfig(t *testing.T, config string) string {
	t.Helper()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace
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
