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

func TestDockerExecArgsFallsBackToContainerUserAndInjectsHome(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinary(t, logPath)), nil, nil, nil)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteEnv: map[string]string{"A": "1"}}}

	args, err := runner.dockerExecArgs(ctx, "container-123", resolved, true, false, map[string]string{"B": "2"}, []string{"id", "-un"})
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec -i -u app", "--workdir /workspaces/demo", "-e A=1", "-e B=2", "-e HOME=/home/app", "container-123 id -un"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	gotLog := string(data)
	if !strings.Contains(gotLog, "inspect|container-123") {
		t.Fatalf("expected inspect fallback, got log %q", gotLog)
	}
	if !strings.Contains(gotLog, "exec|-u|app|container-123|cat|"+passwdFilePath) {
		t.Fatalf("expected home resolution exec, got log %q", gotLog)
	}
	if strings.Contains(gotLog, "|-e|HOME=") {
		t.Fatalf("home lookup should not pre-seed HOME, got log %q", gotLog)
	}
	if strings.Contains(gotLog, "|id|-un") {
		t.Fatalf("command execution should not happen while building args, got log %q", gotLog)
	}
}

func TestDockerExecArgsPreservesExplicitHome(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinary(t, logPath)), nil, nil, nil)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteUser: "app", RemoteEnv: map[string]string{"HOME": "/custom-home"}}}

	args, err := runner.dockerExecArgs(ctx, "container-123", resolved, false, false, nil, []string{"pwd"})
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
	data, err := os.ReadFile(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read fake docker log: %v", err)
	}
	if gotLog := string(data); strings.Contains(gotLog, "cat|"+passwdFilePath) {
		t.Fatalf("did not expect home resolution lookup, got log %q", gotLog)
	}
}

func TestDockerExecArgsUsesUserShellWhenCommandMissing(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinary(t, logPath)), nil, nil, nil)
	resolved := devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	args, err := runner.dockerExecArgs(ctx, "container-123", resolved, true, true, nil, nil)
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
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinary(t, logPath)), nil, nil, nil)
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	err := runner.installDotfiles(ctx, "container-123", resolved, DotfilesOptions{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/.dotfiles"}, nil)
	if err != nil {
		t.Fatalf("install dotfiles: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	gotLog := string(data)
	if !strings.Contains(gotLog, "exec|-u|app|container-123|cat|"+passwdFilePath) {
		t.Fatalf("expected home resolution lookup, got log %q", gotLog)
	}
	if !strings.Contains(gotLog, "exec|-i|-u|app|-e|HOME=/home/app|container-123|/bin/sh|-s|--|https://github.com/example/dotfiles.git|/home/app/.dotfiles|") {
		t.Fatalf("expected dotfiles exec to inject HOME, got log %q", gotLog)
	}
}

func TestResolveDotfilesTargetPathLeavesLiteralHomeWhenUnknown(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinaryNoPasswd(t, logPath)), nil, nil, nil)
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteUser: "app"}}

	targetPath, err := runner.resolveDotfilesTargetPath(ctx, "container-123", resolved, "$HOME/.dotfiles")
	if err != nil {
		t.Fatalf("resolve dotfiles target path: %v", err)
	}
	if targetPath != "$HOME/.dotfiles" {
		t.Fatalf("unexpected target path %q", targetPath)
	}
}

func TestPasswdEntryFromPasswdReturnsHomeAndShell(t *testing.T) {
	entry, ok := passwdEntryFromPasswd("root:x:0:0:root:/root:/bin/sh\napp:x:1000:1000::/home/app:/bin/zsh\n", "app")
	if !ok {
		t.Fatal("expected passwd entry")
	}
	if entry.Home != "/home/app" || entry.Shell != "/bin/zsh" {
		t.Fatalf("unexpected passwd entry %#v", entry)
	}
}

func TestExecDoesNotWritePlanCacheArtifacts(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "docker.log")
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	runner := NewRunnerWithIO(docker.NewClient(fakeDockerBinary(t, logPath)), nil, nil, nil)

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
}

func fakeDockerBinary(t *testing.T, logPath string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"first=1\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$first\" -eq 1 ]; then\n" +
		"    printf '%s' \"$arg\" >> \"$FAKE_DOCKER_LOG\"\n" +
		"    first=0\n" +
		"  else\n" +
		"    printf '|%s' \"$arg\" >> \"$FAKE_DOCKER_LOG\"\n" +
		"  fi\n" +
		"done\n" +
		"printf '\\n' >> \"$FAKE_DOCKER_LOG\"\n" +
		"case \"${1:-}\" in\n" +
		"  ps)\n" +
		"    printf 'container-123\\n'\n" +
		"    ;;\n" +
		"  image)\n" +
		"    printf '[{\"Id\":\"img\",\"Architecture\":\"amd64\",\"Config\":{\"User\":\"app\",\"Labels\":{}}}]'\n" +
		"    ;;\n" +
		"  inspect)\n" +
		"    printf '[{\"Id\":\"container-123\",\"Name\":\"/container-123\",\"Image\":\"img\",\"Config\":{\"User\":\"app\",\"Env\":[\"HOME=/root\"]},\"State\":{\"Status\":\"running\",\"Running\":true}}]'\n" +
		"    ;;\n" +
		"  exec)\n" +
		"    case \"$*\" in\n" +
		"      *' cat /etc/passwd')\n" +
		"        printf 'root:x:0:0:root:/root:/bin/sh\napp:x:1000:1000::/home/app:/bin/sh\n'\n" +
		"        ;;\n" +
		"      *)\n" +
		"        :\n" +
		"        ;;\n" +
		"    esac\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker script: %v", err)
	}
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	return scriptPath
}

func fakeDockerBinaryNoPasswd(t *testing.T, logPath string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"first=1\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$first\" -eq 1 ]; then\n" +
		"    printf '%s' \"$arg\" >> \"$FAKE_DOCKER_LOG\"\n" +
		"    first=0\n" +
		"  else\n" +
		"    printf '|%s' \"$arg\" >> \"$FAKE_DOCKER_LOG\"\n" +
		"  fi\n" +
		"done\n" +
		"printf '\\n' >> \"$FAKE_DOCKER_LOG\"\n" +
		"case \"${1:-}\" in\n" +
		"  inspect)\n" +
		"    printf '[{\"Id\":\"container-123\",\"Name\":\"/container-123\",\"Image\":\"img\",\"Config\":{\"User\":\"app\",\"Env\":[\"HOME=/root\"]},\"State\":{\"Status\":\"running\",\"Running\":true}}]'\n" +
		"    ;;\n" +
		"  exec)\n" +
		"    :\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker script: %v", err)
	}
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	return scriptPath
}
