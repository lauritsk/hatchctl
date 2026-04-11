package dockercli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
)

type fakeTransport struct {
	runOpts    []docker.RunOptions
	outputOpts []docker.RunOptions

	inspectImageRef     string
	inspectImageResult  docker.ImageInspect
	inspectImageErr     error
	inspectContainerID  string
	inspectContainerRes docker.ContainerInspect
	inspectContainerErr error

	runErr       error
	outputResult string
	outputErr    error
}

func (f *fakeTransport) Run(_ context.Context, opts docker.RunOptions) error {
	f.runOpts = append(f.runOpts, cloneRunOptions(opts))
	return f.runErr
}

func (f *fakeTransport) OutputOptions(_ context.Context, opts docker.RunOptions) (string, error) {
	f.outputOpts = append(f.outputOpts, cloneRunOptions(opts))
	return f.outputResult, f.outputErr
}

func (f *fakeTransport) InspectImage(context.Context, string) (docker.ImageInspect, error) {
	if f.inspectImageErr != nil {
		return docker.ImageInspect{}, f.inspectImageErr
	}
	return f.inspectImageResult, nil
}

func (f *fakeTransport) InspectContainer(_ context.Context, containerID string) (docker.ContainerInspect, error) {
	f.inspectContainerID = containerID
	if f.inspectContainerErr != nil {
		return docker.ContainerInspect{}, f.inspectContainerErr
	}
	return f.inspectContainerRes, nil
}

func TestRunAndOutputForwardDirEnvAndStreams(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{outputResult: "ok"}
	client := New(transport)
	stdin := strings.NewReader("input")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if err := client.Run(context.Background(), CommandRequest{Args: []string{"ps"}, Dir: "/workspace", Env: []string{"A=1", "B=2"}, Streams: Streams{Stdin: stdin, Stdout: stdout, Stderr: stderr}}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := client.Output(context.Background(), CommandRequest{Args: []string{"images"}, Dir: "/workspace", Env: []string{"A=1"}, Streams: Streams{Stdin: stdin}}); err != nil {
		t.Fatalf("output: %v", err)
	}
	if got := transport.runOpts[0]; !reflect.DeepEqual(got.Args, []string{"ps"}) || got.Dir != "/workspace" || !reflect.DeepEqual(got.Env, []string{"A=1", "B=2"}) || got.Stdin != stdin || got.Stdout != stdout || got.Stderr != stderr {
		t.Fatalf("unexpected run options %#v", got)
	}
	if got := transport.outputOpts[0]; !reflect.DeepEqual(got.Args, []string{"images"}) || got.Dir != "/workspace" || !reflect.DeepEqual(got.Env, []string{"A=1"}) || got.Stdin != stdin {
		t.Fatalf("unexpected output options %#v", got)
	}
}

func TestInspectHelpersDelegateToTransport(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{
		inspectImageResult:  docker.ImageInspect{Architecture: "arm64"},
		inspectContainerRes: docker.ContainerInspect{ID: "container-123"},
	}
	client := New(transport)

	image, err := client.InspectImage(context.Background(), InspectImageRequest{Reference: "demo:latest"})
	if err != nil {
		t.Fatalf("inspect image: %v", err)
	}
	container, err := client.InspectContainer(context.Background(), InspectContainerRequest{ContainerID: "container-123"})
	if err != nil {
		t.Fatalf("inspect container: %v", err)
	}
	if image.Architecture != "arm64" {
		t.Fatalf("unexpected image inspect %#v", image)
	}
	if container.ID != "container-123" || transport.inspectContainerID != "container-123" {
		t.Fatalf("unexpected container inspect %#v via %q", container, transport.inspectContainerID)
	}
}

func TestSimpleDockerCommandHelpersBuildExpectedArgs(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{outputResult: "container-123"}
	client := New(transport)
	streams := Streams{Stdout: io.Discard, Stderr: io.Discard}

	if err := client.PullImage(context.Background(), PullImageRequest{Reference: "alpine:3.23", Streams: streams}); err != nil {
		t.Fatalf("pull image: %v", err)
	}
	if err := client.StartContainer(context.Background(), StartContainerRequest{ContainerID: "container-123", Streams: streams}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	if err := client.RemoveContainer(context.Background(), RemoveContainerRequest{ContainerID: "container-123", Force: true, Streams: streams}); err != nil {
		t.Fatalf("remove container: %v", err)
	}
	if _, err := client.ListContainers(context.Background(), ListContainersRequest{All: true, Quiet: true, Filters: []string{"label=a", "status=running"}, Dir: "/workspace"}); err != nil {
		t.Fatalf("list containers: %v", err)
	}
	if _, err := client.ExecOutput(context.Background(), ExecRequest{ContainerID: "container-123", Command: []string{"pwd"}}); err != nil {
		t.Fatalf("exec output: %v", err)
	}

	if got := transport.runOpts[0].Args; !reflect.DeepEqual(got, []string{"pull", "alpine:3.23"}) {
		t.Fatalf("unexpected pull args %#v", got)
	}
	if got := transport.runOpts[1].Args; !reflect.DeepEqual(got, []string{"start", "container-123"}) {
		t.Fatalf("unexpected start args %#v", got)
	}
	if got := transport.runOpts[2].Args; !reflect.DeepEqual(got, []string{"rm", "-f", "container-123"}) {
		t.Fatalf("unexpected remove args %#v", got)
	}
	if got := transport.outputOpts[0]; !reflect.DeepEqual(got.Args, []string{"ps", "-a", "-q", "--filter", "label=a", "--filter", "status=running"}) || got.Dir != "/workspace" {
		t.Fatalf("unexpected list args %#v", got)
	}
	if got := transport.outputOpts[1].Args; !reflect.DeepEqual(got, []string{"exec", "container-123", "pwd"}) {
		t.Fatalf("unexpected exec output args %#v", got)
	}
}

func TestBuildImageBuildsTypedDockerArgs(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{}
	client := New(transport)
	if err := client.BuildImage(context.Background(), BuildImageRequest{
		ContextDir:   "/workspace",
		Dockerfile:   "/workspace/Dockerfile",
		Tag:          "hatchctl-demo",
		Labels:       map[string]string{"b": "2", "a": "1"},
		BuildArgs:    map[string]string{"Z": "last", "A": "first"},
		Target:       "dev",
		ExtraOptions: []string{"--pull"},
		Streams:      Streams{Stdout: io.Discard, Stderr: io.Discard},
	}); err != nil {
		t.Fatalf("build image: %v", err)
	}
	if len(transport.runOpts) != 1 {
		t.Fatalf("expected a single run call, got %#v", transport.runOpts)
	}
	want := []string{"build", "-f", "/workspace/Dockerfile", "-t", "hatchctl-demo", "--label", "a=1", "--label", "b=2", "--target", "dev", "--build-arg", "A=first", "--build-arg", "Z=last", "--pull", "/workspace"}
	if got := transport.runOpts[0].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected build args %#v", got)
	}
}

func TestRunDetachedContainerBuildsTypedDockerArgs(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{outputResult: "container-123\n"}
	client := New(transport)
	containerID, err := client.RunDetachedContainer(context.Background(), RunDetachedContainerRequest{
		Name:        "hatchctl-demo",
		Labels:      map[string]string{"b": "2", "a": "1"},
		Mounts:      []string{"type=bind,source=/workspace,target=/workspaces/demo", "type=bind,source=/state,target=/var/run/hatchctl"},
		Init:        true,
		Privileged:  true,
		CapAdd:      []string{"NET_ADMIN"},
		SecurityOpt: []string{"label=disable"},
		Env:         map[string]string{"B": "2", "A": "1"},
		ExtraArgs:   []string{"--network", "host"},
		Image:       "hatchctl-demo-image",
		Command:     []string{"/bin/sh", "-lc", "sleep infinity"},
	})
	if err != nil {
		t.Fatalf("run detached container: %v", err)
	}
	if containerID != "container-123\n" {
		t.Fatalf("unexpected container id %q", containerID)
	}
	if len(transport.outputOpts) != 1 {
		t.Fatalf("expected a single output call, got %#v", transport.outputOpts)
	}
	want := []string{"run", "-d", "--name", "hatchctl-demo", "--label", "a=1", "--label", "b=2", "--mount", "type=bind,source=/workspace,target=/workspaces/demo", "--mount", "type=bind,source=/state,target=/var/run/hatchctl", "--init", "--privileged", "--cap-add", "NET_ADMIN", "--security-opt", "label=disable", "-e", "A=1", "-e", "B=2", "--network", "host", "hatchctl-demo-image", "/bin/sh", "-lc", "sleep infinity"}
	if got := transport.outputOpts[0].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected run args %#v", got)
	}
}

func TestComposeRequestsBuildExpectedArgs(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{outputResult: "{}"}
	client := New(transport)
	target := ComposeTarget{Files: []string{"compose.yml", "override.yml"}, Project: "demo", Dir: "/workspace"}
	if _, err := client.ComposeConfig(context.Background(), ComposeConfigRequest{Target: target, Format: "json"}); err != nil {
		t.Fatalf("compose config: %v", err)
	}
	if err := client.ComposeBuild(context.Background(), ComposeBuildRequest{Target: target, Services: []string{"app"}}); err != nil {
		t.Fatalf("compose build: %v", err)
	}
	if err := client.ComposeUp(context.Background(), ComposeUpRequest{Target: target, Services: []string{"app"}, NoBuild: true, Detach: true}); err != nil {
		t.Fatalf("compose up: %v", err)
	}
	if got := transport.outputOpts[0].Args; !reflect.DeepEqual(got, []string{"compose", "-f", "compose.yml", "-f", "override.yml", "-p", "demo", "config", "--format", "json"}) {
		t.Fatalf("unexpected compose config args %#v", got)
	}
	if got := transport.runOpts[0].Args; !reflect.DeepEqual(got, []string{"compose", "-f", "compose.yml", "-f", "override.yml", "-p", "demo", "build", "app"}) {
		t.Fatalf("unexpected compose build args %#v", got)
	}
	if got := transport.runOpts[1].Args; !reflect.DeepEqual(got, []string{"compose", "-f", "compose.yml", "-f", "override.yml", "-p", "demo", "up", "--no-build", "-d", "app"}) {
		t.Fatalf("unexpected compose up args %#v", got)
	}
}

func TestExecBuildsTypedDockerArgs(t *testing.T) {
	t.Parallel()

	transport := &fakeTransport{}
	client := New(transport)
	if err := client.Exec(context.Background(), ExecRequest{
		ContainerID: "container-123",
		User:        "app",
		Workdir:     "/workspaces/demo",
		Interactive: true,
		TTY:         true,
		Env:         map[string]string{"B": "2", "A": "1"},
		Command:     []string{"pwd"},
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	want := []string{"exec", "-i", "-t", "-u", "app", "--workdir", "/workspaces/demo", "-e", "A=1", "-e", "B=2", "container-123", "pwd"}
	if got := transport.runOpts[0].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected exec args %#v", got)
	}
}

func cloneRunOptions(opts docker.RunOptions) docker.RunOptions {
	clone := opts
	clone.Args = append([]string(nil), opts.Args...)
	clone.Env = append([]string(nil), opts.Env...)
	return clone
}
