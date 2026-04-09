package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

const testPasswd = "root:x:0:0:root:/root:/bin/sh\napp:x:1000:1000::/home/app:/bin/sh\n"

func TestDockerExecArgsFallsBackToContainerUserAndInjectsHome(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) >= 5 && cmd.Args[0] == "exec" && cmd.Args[len(cmd.Args)-2] == "cat" && cmd.Args[len(cmd.Args)-1] == passwdFilePath {
				return testPasswd, nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
		inspectContainer: func(_ context.Context, containerID string) (docker.ContainerInspect, error) {
			return docker.ContainerInspect{ID: containerID, Config: docker.InspectConfig{User: "app"}}, nil
		},
	}
	runner := newTestRunner(t, backend)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteEnv: map[string]string{"A": "1"}}}

	args, err := runner.dockerExecArgs(context.Background(), "container-123", resolved, true, false, map[string]string{"B": "2"}, []string{"id", "-un"})
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec -i -u app", "--workdir /workspaces/demo", "-e A=1", "-e B=2", "-e HOME=/home/app", "container-123 id -un"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
	if len(backend.inspectContainerID) != 1 || backend.inspectContainerID[0] != "container-123" {
		t.Fatalf("expected single container inspect, got %#v", backend.inspectContainerID)
	}
	if len(backend.outputCommands) != 1 {
		t.Fatalf("expected one passwd lookup, got %#v", backend.outputCommands)
	}
	if got := strings.Join(backend.outputCommands[0].Args, " "); !strings.Contains(got, "exec -u app container-123 cat "+passwdFilePath) {
		t.Fatalf("expected passwd lookup, got %q", got)
	}
	if len(backend.runCommands) != 0 {
		t.Fatalf("did not expect docker exec run during arg building, got %#v", backend.runCommands)
	}
}

func TestDockerExecArgsPreservesExplicitHome(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{}
	runner := newTestRunner(t, backend)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteUser: "app", RemoteEnv: map[string]string{"HOME": "/custom-home"}}}

	args, err := runner.dockerExecArgs(context.Background(), "container-123", resolved, false, false, nil, []string{"pwd"})
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-u app") || !strings.Contains(joined, "-e HOME=/custom-home") {
		t.Fatalf("unexpected args %q", joined)
	}
	if !strings.Contains(joined, "--workdir /workspaces/demo") {
		t.Fatalf("expected workdir in args %q", joined)
	}
	if len(backend.inspectContainerID) != 0 || len(backend.outputCommands) != 0 {
		t.Fatalf("did not expect inspect or passwd lookup, got inspect=%#v output=%#v", backend.inspectContainerID, backend.outputCommands)
	}
}

func TestDockerExecArgsUsesUserShellWhenCommandMissing(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) >= 5 && cmd.Args[0] == "exec" && cmd.Args[len(cmd.Args)-2] == "cat" && cmd.Args[len(cmd.Args)-1] == passwdFilePath {
				return testPasswd, nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
	}
	runner := newTestRunner(t, backend)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	args, err := runner.dockerExecArgs(context.Background(), "container-123", resolved, true, true, nil, nil)
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-t", "--workdir /workspaces/demo", "-e HOME=/home/app", "container-123 /bin/sh"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
}

func TestInstallDotfilesUsesResolvedHome(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) >= 5 && cmd.Args[0] == "exec" && cmd.Args[len(cmd.Args)-2] == "cat" && cmd.Args[len(cmd.Args)-1] == passwdFilePath {
				return testPasswd, nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
	}
	runner := newTestRunner(t, backend)
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	err := runner.installDotfiles(context.Background(), "container-123", resolved, DotfilesOptions{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/.dotfiles"}, nil)
	if err != nil {
		t.Fatalf("install dotfiles: %v", err)
	}
	if len(backend.outputCommands) != 2 {
		t.Fatalf("expected two passwd lookups, got %#v", backend.outputCommands)
	}
	for _, cmd := range backend.outputCommands {
		if got := strings.Join(cmd.Args, " "); !strings.Contains(got, "exec -u app container-123 cat "+passwdFilePath) {
			t.Fatalf("expected passwd lookup, got %q", got)
		}
	}
	if len(backend.runCommands) != 1 {
		t.Fatalf("expected one docker exec run, got %#v", backend.runCommands)
	}
	if got := strings.Join(backend.runCommands[0].Args, " "); !strings.Contains(got, "exec -i -u app -e HOME=/home/app container-123 /bin/sh -s -- https://github.com/example/dotfiles.git /home/app/.dotfiles") {
		t.Fatalf("expected dotfiles install command, got %q", got)
	}
}

func TestResolveDotfilesTargetPathLeavesLiteralHomeWhenUnknown(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			if len(cmd.Args) >= 5 && cmd.Args[0] == "exec" && cmd.Args[len(cmd.Args)-2] == "cat" && cmd.Args[len(cmd.Args)-1] == passwdFilePath {
				return "", nil
			}
			t.Fatalf("unexpected output command %#v", cmd.Args)
			return "", nil
		},
	}
	runner := newTestRunner(t, backend)
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	targetPath, err := runner.resolveDotfilesTargetPath(context.Background(), "container-123", resolved, "$HOME/.dotfiles")
	if err != nil {
		t.Fatalf("resolve dotfiles target path: %v", err)
	}
	if targetPath != "$HOME/.dotfiles" {
		t.Fatalf("unexpected target path %q", targetPath)
	}
}

func TestPasswdEntryFromPasswdReturnsHomeAndShell(t *testing.T) {
	t.Parallel()

	entry, ok := passwdEntryFromPasswd("root:x:0:0:root:/root:/bin/sh\napp:x:1000:1000::/home/app:/bin/zsh\n", "app")
	if !ok {
		t.Fatal("expected passwd entry")
	}
	if entry.Home != "/home/app" || entry.Shell != "/bin/zsh" {
		t.Fatalf("unexpected passwd entry %#v", entry)
	}
}

func TestExecDoesNotWritePlanCacheArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := &fakeRuntimeBackend{
		output: func(_ context.Context, cmd runtimeCommand) (string, error) {
			switch {
			case len(cmd.Args) != 0 && cmd.Args[0] == "ps":
				return "container-123\n", nil
			case len(cmd.Args) >= 5 && cmd.Args[0] == "exec" && cmd.Args[len(cmd.Args)-2] == "cat" && cmd.Args[len(cmd.Args)-1] == passwdFilePath:
				return testPasswd, nil
			default:
				t.Fatalf("unexpected output command %#v", cmd.Args)
				return "", nil
			}
		},
		inspectImage: func(_ context.Context, image string) (docker.ImageInspect, error) {
			if image != "alpine:3.20" {
				t.Fatalf("unexpected image inspect %q", image)
			}
			return docker.ImageInspect{ID: "img", Config: docker.InspectConfig{User: "app", Labels: map[string]string{}}}, nil
		},
		inspectContainer: func(_ context.Context, containerID string) (docker.ContainerInspect, error) {
			if containerID != "container-123" {
				t.Fatalf("unexpected container inspect %q", containerID)
			}
			return docker.ContainerInspect{
				ID:    containerID,
				Name:  "/container-123",
				Image: "img",
				Config: docker.InspectConfig{
					User: "app",
					Env:  []string{"HOME=/root"},
				},
				State: docker.ContainerState{Status: "running", Running: true},
			}, nil
		},
	}
	runner := newTestRunner(t, backend)
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")

	code, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		StateDir:  stateDir,
		CacheDir:  cacheDir,
		Args:      []string{"pwd"},
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	cacheEntries, err := os.ReadDir(cacheDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read cache dir: %v", err)
	}
	if len(cacheEntries) != 0 {
		t.Fatalf("expected exec to avoid cache writes, found %d entries", len(cacheEntries))
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read state dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected exec to avoid state writes, found %d entries", len(entries))
	}
	if len(backend.runCommands) != 1 {
		t.Fatalf("expected a single docker exec command, got %#v", backend.runCommands)
	}
	if got := strings.Join(backend.runCommands[0].Args, " "); !strings.Contains(got, "exec -u app --workdir /workspaces/demo -e HOME=/home/app container-123 pwd") {
		t.Fatalf("unexpected exec command %q", got)
	}
}
