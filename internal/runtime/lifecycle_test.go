package runtime

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type hostOnlyBackend struct {
	runHost func(context.Context, string, []string, commandIO) error
}

func (b hostOnlyBackend) RunDocker(context.Context, string, docker.RunOptions, ui.Sink) error {
	panic("unexpected RunDocker call")
}

func (b hostOnlyBackend) DockerOutput(context.Context, docker.RunOptions) (string, error) {
	panic("unexpected DockerOutput call")
}

func (b hostOnlyBackend) InspectImage(context.Context, string) (docker.ImageInspect, error) {
	panic("unexpected InspectImage call")
}

func (b hostOnlyBackend) InspectContainer(context.Context, string) (docker.ContainerInspect, error) {
	panic("unexpected InspectContainer call")
}

func (b hostOnlyBackend) RunHost(ctx context.Context, cwd string, args []string, streams commandIO) error {
	return b.runHost(ctx, cwd, args, streams)
}

func (b hostOnlyBackend) BuildImage(context.Context, string, string, []string, ui.Sink) error {
	panic("unexpected BuildImage call")
}

func (b hostOnlyBackend) StartContainer(context.Context, string, ui.Sink) error {
	panic("unexpected StartContainer call")
}

func (b hostOnlyBackend) RemoveContainer(context.Context, string, ui.Sink) error {
	panic("unexpected RemoveContainer call")
}

func (b hostOnlyBackend) ContainerStatus(context.Context, string) (string, error) {
	panic("unexpected ContainerStatus call")
}

func (b hostOnlyBackend) RunContainer(context.Context, []string) (string, error) {
	panic("unexpected RunContainer call")
}

func (b hostOnlyBackend) ComposeUp(context.Context, devcontainer.ResolvedConfig, string, ui.Sink) error {
	panic("unexpected ComposeUp call")
}

func (b hostOnlyBackend) ComposeBuild(context.Context, devcontainer.ResolvedConfig, ui.Sink) error {
	panic("unexpected ComposeBuild call")
}

func (b hostOnlyBackend) DockerExec(context.Context, string, []string, io.Reader, io.Writer, io.Writer, ui.Sink) error {
	panic("unexpected DockerExec call")
}

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
	}, commandIO{Stdin: stdin, Stdout: &stdout, Stderr: &stderr}, hostOnlyBackend{runHost: func(_ context.Context, cwd string, args []string, streams commandIO) error {
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
	}})
	if err != nil {
		t.Fatalf("run host lifecycle: %v", err)
	}
	if !called {
		t.Fatal("expected injected host runner to be called")
	}
}

func TestRunCommandUsesShellForStringCommands(t *testing.T) {
	t.Parallel()

	var got []string
	err := runCommand(context.Background(), func(_ context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}, devcontainer.LifecycleCommand{Kind: "string", Value: "echo hi", Exists: true})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if len(got) != 3 || got[0] != "/bin/sh" || got[1] != "-lc" || got[2] != "echo hi" {
		t.Fatalf("unexpected args %#v", got)
	}
}

func TestRunCommandRunsObjectStepsInSortedOrder(t *testing.T) {
	t.Parallel()

	var got []string
	err := runCommand(context.Background(), func(_ context.Context, args []string) error {
		got = append(got, args[len(args)-1])
		return nil
	}, devcontainer.LifecycleCommand{
		Kind:   "object",
		Exists: true,
		Steps: map[string]devcontainer.LifecycleCommand{
			"z-last":  {Kind: "string", Value: "echo z", Exists: true},
			"a-first": {Kind: "string", Value: "echo a", Exists: true},
		},
	})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if len(got) != 2 || got[0] != "echo a" || got[1] != "echo z" {
		t.Fatalf("unexpected command order %#v", got)
	}
}

func TestRunCommandWrapsObjectStepErrors(t *testing.T) {
	t.Parallel()

	err := runCommand(context.Background(), func(_ context.Context, args []string) error {
		if len(args) > 0 && args[len(args)-1] == "echo fail" {
			return context.DeadlineExceeded
		}
		return nil
	}, devcontainer.LifecycleCommand{
		Kind:   "object",
		Exists: true,
		Steps: map[string]devcontainer.LifecycleCommand{
			"ok":   {Kind: "string", Value: "echo ok", Exists: true},
			"fail": {Kind: "string", Value: "echo fail", Exists: true},
		},
	})
	if err == nil || err.Error() != "lifecycle step fail: context deadline exceeded" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRunCommandSkipsEmptyArraysAndCommands(t *testing.T) {
	t.Parallel()

	called := false
	runner := func(_ context.Context, _ []string) error {
		called = true
		return nil
	}
	if err := runCommand(context.Background(), runner, devcontainer.LifecycleCommand{Kind: "array", Exists: true}); err != nil {
		t.Fatalf("run empty array: %v", err)
	}
	if err := runCommand(context.Background(), runner, devcontainer.LifecycleCommand{}); err != nil {
		t.Fatalf("run empty command: %v", err)
	}
	if called {
		t.Fatal("expected runner not to be called")
	}
}
