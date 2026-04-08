package docker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if _, err := client.InspectImage(context.Background(), "empty-image"); err == nil || err.Error() != "image empty-image not found" {
		t.Fatalf("expected missing image error, got %v", err)
	}
	if _, err := client.InspectContainer(context.Background(), "empty-container"); err == nil || err.Error() != "container empty-container not found" {
		t.Fatalf("expected missing container error, got %v", err)
	}
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
