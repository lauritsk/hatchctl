package dockercli

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
)

type fakeTransport struct {
	runOpts    []docker.RunOptions
	outputOpts []docker.RunOptions

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
	return docker.ImageInspect{}, errors.New("unexpected inspect image")
}

func (f *fakeTransport) InspectContainer(context.Context, string) (docker.ContainerInspect, error) {
	return docker.ContainerInspect{}, errors.New("unexpected inspect container")
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
