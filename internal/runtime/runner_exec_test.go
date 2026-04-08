package runtime

import (
	"context"
	"errors"
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
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteEnv: map[string]string{"A": "1"}}}

	args, err := runner.dockerExecArgs(ctx, "container-123", resolved, true, false, map[string]string{"B": "2"}, []string{"id", "-un"})
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec -i -u app", "-e A=1", "-e B=2", "-e HOME=/home/app", "container-123 id -un"} {
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
	if !strings.Contains(gotLog, "exec|-u|app|container-123|sh|-lc|"+resolveHomeCommand) {
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
	resolved := devcontainer.ResolvedConfig{Merged: devcontainer.MergedConfig{RemoteUser: "app", RemoteEnv: map[string]string{"HOME": "/custom-home"}}}

	args, err := runner.dockerExecArgs(ctx, "container-123", resolved, false, false, nil, []string{"pwd"})
	if err != nil {
		t.Fatalf("build docker exec args: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-u app") || !strings.Contains(joined, "-e HOME=/custom-home") {
		t.Fatalf("unexpected args %q", joined)
	}
	data, err := os.ReadFile(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read fake docker log: %v", err)
	}
	if gotLog := string(data); strings.Contains(gotLog, "sh|-lc|"+resolveHomeCommand) {
		t.Fatalf("did not expect home resolution lookup, got log %q", gotLog)
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
	if !strings.Contains(gotLog, "exec|-u|app|container-123|sh|-lc|"+resolveHomeCommand) {
		t.Fatalf("expected home resolution lookup, got log %q", gotLog)
	}
	if !strings.Contains(gotLog, "exec|-i|-u|app|-e|HOME=/home/app|container-123|/bin/sh|-lc|") {
		t.Fatalf("expected dotfiles exec to inject HOME, got log %q", gotLog)
	}
	if !strings.Contains(gotLog, "git clone --depth 1") || !strings.Contains(gotLog, "ln -snf \"$file\" \"$HOME/$base\"") {
		t.Fatalf("expected dotfiles install script in log, got %q", gotLog)
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
		"  inspect)\n" +
		"    printf '[{\"Id\":\"container-123\",\"Name\":\"/container-123\",\"Image\":\"img\",\"Config\":{\"User\":\"app\",\"Env\":[\"HOME=/root\"]},\"State\":{\"Status\":\"running\",\"Running\":true}}]'\n" +
		"    ;;\n" +
		"  exec)\n" +
		"    printf '/home/app'\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker script: %v", err)
	}
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	return scriptPath
}
