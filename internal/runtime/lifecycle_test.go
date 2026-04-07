package runtime

import (
	"bytes"
	"context"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestRunHostLifecycleUsesInjectedRunnerAndStreams(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdin := bytes.NewBufferString("input")
	called := false
	err := runHostLifecycle(context.Background(), "/tmp/workspace", devcontainer.LifecycleCommand{
		Kind:   "array",
		Args:   []string{"tool", "arg1"},
		Exists: true,
	}, commandIO{Stdin: stdin, Stdout: &stdout, Stderr: &stderr}, func(_ context.Context, cwd string, args []string, streams commandIO) error {
		called = true
		if cwd != "/tmp/workspace" {
			t.Fatalf("unexpected cwd %q", cwd)
		}
		if len(args) != 2 || args[0] != "tool" || args[1] != "arg1" {
			t.Fatalf("unexpected args %#v", args)
		}
		if streams.Stdin != stdin || streams.Stdout != &stdout || streams.Stderr != &stderr {
			t.Fatalf("unexpected command streams %#v", streams)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run host lifecycle: %v", err)
	}
	if !called {
		t.Fatal("expected injected host runner to be called")
	}
}
