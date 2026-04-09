package runtime

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/policy"
)

type hostOnlyBackend struct {
	run func(context.Context, runtimeCommand) error
}

func (b hostOnlyBackend) Run(ctx context.Context, cmd runtimeCommand) error {
	if b.run == nil {
		panic("unexpected Run call")
	}
	return b.run(ctx, cmd)
}

func (b hostOnlyBackend) Output(context.Context, runtimeCommand) (string, error) {
	panic("unexpected Output call")
}

func (b hostOnlyBackend) InspectImage(context.Context, string) (docker.ImageInspect, error) {
	panic("unexpected InspectImage call")
}

func (b hostOnlyBackend) InspectContainer(context.Context, string) (docker.ContainerInspect, error) {
	panic("unexpected InspectContainer call")
}

func (b hostOnlyBackend) BuildImage(context.Context, dockercli.BuildImageRequest) error {
	panic("unexpected BuildImage call")
}

func (b hostOnlyBackend) RunDetachedContainer(context.Context, dockercli.RunDetachedContainerRequest) (string, error) {
	panic("unexpected RunDetachedContainer call")
}

func (b hostOnlyBackend) StartContainer(context.Context, dockercli.StartContainerRequest) error {
	panic("unexpected StartContainer call")
}

func (b hostOnlyBackend) RemoveContainer(context.Context, dockercli.RemoveContainerRequest) error {
	panic("unexpected RemoveContainer call")
}

func (b hostOnlyBackend) ListContainers(context.Context, dockercli.ListContainersRequest) (string, error) {
	panic("unexpected ListContainers call")
}

func (b hostOnlyBackend) ComposeConfig(context.Context, dockercli.ComposeConfigRequest) (string, error) {
	panic("unexpected ComposeConfig call")
}

func (b hostOnlyBackend) ComposeBuild(context.Context, dockercli.ComposeBuildRequest) error {
	panic("unexpected ComposeBuild call")
}

func (b hostOnlyBackend) ComposeUp(context.Context, dockercli.ComposeUpRequest) error {
	panic("unexpected ComposeUp call")
}

func (b hostOnlyBackend) Exec(context.Context, dockercli.ExecRequest) error {
	panic("unexpected Exec call")
}

func (b hostOnlyBackend) ExecOutput(context.Context, dockercli.ExecRequest) (string, error) {
	panic("unexpected ExecOutput call")
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
	}, commandIO{Stdin: stdin, Stdout: &stdout, Stderr: &stderr}, hostOnlyBackend{run: func(_ context.Context, cmd runtimeCommand) error {
		called = true
		if cmd.Kind != runtimeCommandHost {
			t.Fatalf("unexpected command kind %q", cmd.Kind)
		}
		if cmd.Dir != "/tmp/workspace" {
			t.Fatalf("unexpected cwd %q", cmd.Dir)
		}
		if cmd.Binary != "tool" || len(cmd.Args) != 1 || cmd.Args[0] != "arg1" {
			t.Fatalf("unexpected command %#v", cmd)
		}
		if cmd.Stdin != stdin || cmd.Stdout != &stdout || cmd.Stderr != &stderr {
			t.Fatalf("unexpected command streams %#v", cmd)
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

func TestEnsureHostLifecycleAllowedRejectsUntrustedCommands(t *testing.T) {
	t.Parallel()

	err := policy.EnsureHostLifecycleAllowed(devcontainer.LifecycleCommand{Kind: "string", Value: "echo init", Exists: true}, false)
	if err == nil || !errors.Is(err, policy.ErrHostLifecycleNotAllowed) {
		t.Fatalf("expected host lifecycle trust error, got %v", err)
	}
}

func TestEnsureHostLifecycleAllowedPermitsTrustedOrEmptyCommands(t *testing.T) {
	t.Parallel()

	if err := policy.EnsureHostLifecycleAllowed(devcontainer.LifecycleCommand{}, false); err != nil {
		t.Fatalf("empty command should be allowed: %v", err)
	}
	if err := policy.EnsureHostLifecycleAllowed(devcontainer.LifecycleCommand{Kind: "string", Value: "echo init", Exists: true}, true); err != nil {
		t.Fatalf("trusted command should be allowed: %v", err)
	}
}
