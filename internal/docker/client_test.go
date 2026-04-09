package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/command"
)

func TestErrorExitCodeAndNotFound(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-lc", "exit 17")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit error")
	}
	dockerErr := &Error{Args: []string{"inspect", "missing"}, Stderr: "Error: No such object: missing\n", Err: err}
	if got := dockerErr.Error(); got != "docker inspect missing: Error: No such object: missing" {
		t.Fatalf("unexpected error string %q", got)
	}
	code, ok := dockerErr.ExitCode()
	if !ok || code != 17 {
		t.Fatalf("unexpected exit code %d ok=%t", code, ok)
	}
	if !IsNotFound(dockerErr) {
		t.Fatal("expected not found error")
	}
	if IsNotFound(errors.New("plain error")) {
		t.Fatal("did not expect plain error to be treated as not found")
	}
}

func TestOutputOptionsCapturesStderrOnFailure(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeDockerBinary(t, `
if [ "$1" = "failing" ]; then
  echo "bad things happened" >&2
  exit 23
fi
`))

	_, err := client.OutputOptions(context.Background(), RunOptions{Args: []string{"failing"}})
	var dockerErr *Error
	if !errors.As(err, &dockerErr) {
		t.Fatalf("expected docker error, got %v", err)
	}
	if dockerErr.Stderr != "bad things happened\n" {
		t.Fatalf("unexpected stderr %q", dockerErr.Stderr)
	}
	code, ok := dockerErr.ExitCode()
	if !ok || code != 23 {
		t.Fatalf("unexpected exit code %d ok=%t", code, ok)
	}
}

func TestInspectImageAndContainerParsingErrors(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeDockerBinary(t, `
if [ "$1" = "image" ] && [ "$2" = "inspect" ] && [ "$3" = "broken-image" ]; then
  printf '{'
  exit 0
fi
if [ "$1" = "inspect" ] && [ "$2" = "broken-container" ]; then
  printf '{'
  exit 0
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ] && [ "$3" = "empty-image" ]; then
  printf '[]'
  exit 0
fi
if [ "$1" = "inspect" ] && [ "$2" = "empty-container" ]; then
  printf '[]'
  exit 0
fi
printf 'unexpected args: %s\n' "$*" >&2
exit 99
`))

	if _, err := client.InspectImage(context.Background(), "broken-image"); err == nil || !strings.Contains(err.Error(), "parse docker image inspect") {
		t.Fatalf("expected parse image inspect error, got %v", err)
	}
	if _, err := client.InspectContainer(context.Background(), "broken-container"); err == nil || !strings.Contains(err.Error(), "parse docker inspect") {
		t.Fatalf("expected parse container inspect error, got %v", err)
	}
	if _, err := client.InspectImage(context.Background(), "empty-image"); err == nil || err.Error() != `image "empty-image" not found` {
		t.Fatalf("expected missing image error, got %v", err)
	}
	if _, err := client.InspectContainer(context.Background(), "empty-container"); err == nil || err.Error() != `container "empty-container" not found` {
		t.Fatalf("expected missing container error, got %v", err)
	}
}

func TestRunBuildStreamsCombinedOutputToStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	client := &Client{Binary: "docker", runner: stubCommandRunner{
		run: func(_ context.Context, cmd command.Command) error {
			if _, err := io.WriteString(cmd.Stdout, "#1 loading build definition\n"); err != nil {
				return err
			}
			if _, err := io.WriteString(cmd.Stderr, "#2 loading metadata\n"); err != nil {
				return err
			}
			return errors.New("boom")
		},
		combinedOutput: func(context.Context, command.Command) (string, error) {
			t.Fatal("expected build commands to stream via Run")
			return "", nil
		},
	}}

	err := client.Run(context.Background(), RunOptions{Args: []string{"build", "."}, Stdout: &stdout, Stderr: &stderr})
	var dockerErr *Error
	if !errors.As(err, &dockerErr) {
		t.Fatalf("expected docker error, got %v", err)
	}
	if got := stdout.String(); got != "#1 loading build definition\n#2 loading metadata\n" {
		t.Fatalf("unexpected streamed stdout %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected combined build output to avoid stderr target, got %q", stderr.String())
	}
	if got := dockerErr.Stderr; got != stdout.String() {
		t.Fatalf("expected captured output %q, got %q", stdout.String(), got)
	}
}

func TestShouldCombineStreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "docker build", args: []string{"build", "."}, want: true},
		{name: "docker compose build", args: []string{"compose", "build", "app"}, want: true},
		{name: "docker compose up", args: []string{"compose", "up", "-d"}, want: true},
		{name: "docker pull", args: []string{"pull", "alpine"}, want: false},
		{name: "docker compose ps", args: []string{"compose", "ps"}, want: false},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldCombineStreams(tc.args); got != tc.want {
				t.Fatalf("shouldCombineStreams(%v) = %t want %t", tc.args, got, tc.want)
			}
		})
	}
}

type stubCommandRunner struct {
	run            func(context.Context, command.Command) error
	output         func(context.Context, command.Command) (string, string, error)
	combinedOutput func(context.Context, command.Command) (string, error)
}

func (s stubCommandRunner) Run(ctx context.Context, cmd command.Command) error {
	if s.run != nil {
		return s.run(ctx, cmd)
	}
	return nil
}

func (s stubCommandRunner) Output(ctx context.Context, cmd command.Command) (string, string, error) {
	if s.output != nil {
		return s.output(ctx, cmd)
	}
	return "", "", nil
}

func (s stubCommandRunner) CombinedOutput(ctx context.Context, cmd command.Command) (string, error) {
	if s.combinedOutput != nil {
		return s.combinedOutput(ctx, cmd)
	}
	return "", nil
}

func (s stubCommandRunner) Start(command.StartOptions) (*os.Process, error) {
	return nil, errors.New("not implemented")
}

func fakeDockerBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker binary: %v", err)
	}
	return path
}
