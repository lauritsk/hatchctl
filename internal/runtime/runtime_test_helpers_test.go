package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
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

func (b *fakeRuntimeBackend) BuildImage(ctx context.Context, req dockercli.BuildImageRequest) error {
	return b.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: buildImageArgs(req), Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (b *fakeRuntimeBackend) RunDetachedContainer(ctx context.Context, req dockercli.RunDetachedContainerRequest) (string, error) {
	return b.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: runDetachedContainerArgs(req), Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (b *fakeRuntimeBackend) StartContainer(ctx context.Context, req dockercli.StartContainerRequest) error {
	return b.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: []string{"start", req.ContainerID}, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (b *fakeRuntimeBackend) RemoveContainer(ctx context.Context, req dockercli.RemoveContainerRequest) error {
	args := []string{"rm"}
	if req.Force {
		args = append(args, "-f")
	}
	args = append(args, req.ContainerID)
	return b.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (b *fakeRuntimeBackend) ListContainers(ctx context.Context, req dockercli.ListContainersRequest) (string, error) {
	args := []string{"ps"}
	if req.All {
		args = append(args, "-a")
	}
	if req.Quiet {
		args = append(args, "-q")
	}
	for _, filter := range req.Filters {
		args = append(args, "--filter", filter)
	}
	return b.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Dir: req.Dir})
}

func (b *fakeRuntimeBackend) ComposeConfig(ctx context.Context, req dockercli.ComposeConfigRequest) (string, error) {
	args := composeBaseArgsForTest(req.Target)
	args = append(args, "config")
	if req.Format != "" {
		args = append(args, "--format", req.Format)
	}
	return b.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Dir: req.Target.Dir})
}

func (b *fakeRuntimeBackend) ComposeBuild(ctx context.Context, req dockercli.ComposeBuildRequest) error {
	args := composeBaseArgsForTest(req.Target)
	args = append(args, "build")
	args = append(args, req.Services...)
	return b.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Dir: req.Target.Dir, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (b *fakeRuntimeBackend) ComposeUp(ctx context.Context, req dockercli.ComposeUpRequest) error {
	args := composeBaseArgsForTest(req.Target)
	args = append(args, "up")
	if req.NoBuild {
		args = append(args, "--no-build")
	}
	if req.Detach {
		args = append(args, "-d")
	}
	args = append(args, req.Services...)
	return b.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args, Dir: req.Target.Dir, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
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

func buildImageArgs(req dockercli.BuildImageRequest) []string {
	args := []string{"build", "-f", req.Dockerfile, "-t", req.Tag}
	for _, key := range sortedKeysForTest(req.Labels) {
		args = append(args, "--label", key+"="+req.Labels[key])
	}
	if req.Target != "" {
		args = append(args, "--target", req.Target)
	}
	for _, key := range sortedKeysForTest(req.BuildArgs) {
		args = append(args, "--build-arg", key+"="+req.BuildArgs[key])
	}
	args = append(args, req.ExtraOptions...)
	args = append(args, req.ContextDir)
	return args
}

func runDetachedContainerArgs(req dockercli.RunDetachedContainerRequest) []string {
	args := []string{"run", "-d", "--name", req.Name}
	for _, key := range sortedKeysForTest(req.Labels) {
		args = append(args, "--label", key+"="+req.Labels[key])
	}
	for _, mount := range req.Mounts {
		args = append(args, "--mount", mount)
	}
	if req.Init {
		args = append(args, "--init")
	}
	if req.Privileged {
		args = append(args, "--privileged")
	}
	for _, cap := range req.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, sec := range req.SecurityOpt {
		args = append(args, "--security-opt", sec)
	}
	for _, key := range sortedKeysForTest(req.Env) {
		args = append(args, "-e", key+"="+req.Env[key])
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, req.Image)
	args = append(args, req.Command...)
	return args
}

func composeBaseArgsForTest(target dockercli.ComposeTarget) []string {
	args := []string{"compose"}
	for _, file := range target.Files {
		args = append(args, "-f", file)
	}
	if target.Project != "" {
		args = append(args, "-p", target.Project)
	}
	return args
}

func sortedKeysForTest(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
