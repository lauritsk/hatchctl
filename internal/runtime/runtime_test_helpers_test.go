package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
)

type recordedSink struct {
	events []ui.Event
}

func (s *recordedSink) Emit(event ui.Event) {
	s.events = append(s.events, event)
}

type fakeRuntimeBackend struct {
	run              func(context.Context, runtimeCommand) error
	output           func(context.Context, runtimeCommand) (string, error)
	inspectImage     func(context.Context, string) (docker.ImageInspect, error)
	inspectContainer func(context.Context, string) (docker.ContainerInspect, error)

	runCommands        []runtimeCommand
	outputCommands     []runtimeCommand
	inspectImageRefs   []string
	inspectContainerID []string
}

func (b *fakeRuntimeBackend) Run(ctx context.Context, cmd runtimeCommand) error {
	b.runCommands = append(b.runCommands, cloneRuntimeCommand(cmd))
	if b.run != nil {
		return b.run(ctx, cmd)
	}
	return nil
}

func (b *fakeRuntimeBackend) Output(ctx context.Context, cmd runtimeCommand) (string, error) {
	b.outputCommands = append(b.outputCommands, cloneRuntimeCommand(cmd))
	if b.output != nil {
		return b.output(ctx, cmd)
	}
	return "", nil
}

func (b *fakeRuntimeBackend) InspectImage(ctx context.Context, image string) (docker.ImageInspect, error) {
	b.inspectImageRefs = append(b.inspectImageRefs, image)
	if b.inspectImage != nil {
		return b.inspectImage(ctx, image)
	}
	return docker.ImageInspect{}, nil
}

func (b *fakeRuntimeBackend) InspectContainer(ctx context.Context, containerID string) (docker.ContainerInspect, error) {
	b.inspectContainerID = append(b.inspectContainerID, containerID)
	if b.inspectContainer != nil {
		return b.inspectContainer(ctx, containerID)
	}
	return docker.ContainerInspect{}, nil
}

func cloneRuntimeCommand(cmd runtimeCommand) runtimeCommand {
	clone := cmd
	clone.Args = append([]string(nil), cmd.Args...)
	clone.Env = append([]string(nil), cmd.Env...)
	return clone
}

func newTestRunner(t testing.TB, backend runtimeBackend) *Runner {
	t.Helper()
	if backend == nil {
		backend = &fakeRuntimeBackend{}
	}
	stdin := bytes.NewBuffer(nil)
	runner := &Runner{
		stdin:         stdin,
		stdout:        io.Discard,
		stderr:        io.Discard,
		backend:       backend,
		imageVerifier: policy.NewImageVerificationPolicy(stdin, io.Discard),
	}
	runner.planner = workspaceplan.NewResolver()
	return runner
}

func dockerNotFoundError(args ...string) error {
	object := "object"
	if len(args) != 0 {
		object = args[len(args)-1]
	}
	return &docker.Error{
		Args:   append([]string(nil), args...),
		Stderr: "Error: No such object: " + object + "\n",
		Err:    errors.New("exit status 1"),
	}
}

var _ runtimeBackend = (*fakeRuntimeBackend)(nil)
