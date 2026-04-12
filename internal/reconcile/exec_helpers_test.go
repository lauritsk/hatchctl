package reconcile

import (
	"context"
	"reflect"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestPasswdEntryFromPasswdSupportsNameAndUIDLookups(t *testing.T) {
	t.Parallel()

	passwd := "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n"
	entry, ok := passwdEntryFromPasswd(passwd, "vscode")
	if !ok || entry.Home != "/home/vscode" || entry.Shell != "/bin/bash" {
		t.Fatalf("unexpected passwd lookup by name %#v ok=%v", entry, ok)
	}
	entry, ok = passwdEntryFromPasswd(passwd, "1000:1000")
	if !ok || entry.Home != "/home/vscode" || entry.Shell != "/bin/bash" {
		t.Fatalf("unexpected passwd lookup by uid %#v ok=%v", entry, ok)
	}
	if _, ok := passwdEntryFromPasswd(passwd, "missing"); ok {
		t.Fatal("expected missing passwd entry lookup to fail")
	}
}

func TestPasswdLookupNormalizesValues(t *testing.T) {
	t.Parallel()

	if name, uid := passwdLookup(""); name != "root" || uid != "" {
		t.Fatalf("unexpected default passwd lookup %q %q", name, uid)
	}
	if name, uid := passwdLookup("1000:1000"); name != "" || uid != "1000" {
		t.Fatalf("unexpected numeric passwd lookup %q %q", name, uid)
	}
	if name, uid := passwdLookup("vscode:staff"); name != "vscode" || uid != "" {
		t.Fatalf("unexpected named passwd lookup %q %q", name, uid)
	}
}

func TestEffectiveExecUserPrefersMergedContainerAndInspect(t *testing.T) {
	t.Parallel()

	executor := &Executor{engine: &fakeExecutorEngine{inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
		if req.ContainerID != "container-123" {
			t.Fatalf("unexpected inspect request %#v", req)
		}
		return docker.ContainerInspect{Config: docker.InspectConfig{User: "inspect-user"}}, nil
	}}}
	observed := ObservedState{Resolved: devcontainer.ResolvedConfig{Merged: spec.MergedConfig{RemoteUser: "remote-user"}}, Target: RuntimeTarget{PrimaryContainer: "container-123"}}

	user, err := executor.effectiveExecUser(context.Background(), observed)
	if err != nil || user != "remote-user" {
		t.Fatalf("unexpected effective exec user %q err=%v", user, err)
	}

	observed.Resolved.Merged.RemoteUser = ""
	observed.Resolved.Merged.ContainerUser = "container-user"
	user, err = executor.effectiveExecUser(context.Background(), observed)
	if err != nil || user != "container-user" {
		t.Fatalf("unexpected container user %q err=%v", user, err)
	}

	observed.Resolved.Merged.ContainerUser = ""
	observed.Container = &docker.ContainerInspect{Config: docker.InspectConfig{User: "observed-user"}}
	user, err = executor.effectiveExecUser(context.Background(), observed)
	if err != nil || user != "observed-user" {
		t.Fatalf("unexpected observed container user %q err=%v", user, err)
	}

	observed.Container = nil
	user, err = executor.effectiveExecUser(context.Background(), observed)
	if err != nil || user != "inspect-user" {
		t.Fatalf("unexpected inspected container user %q err=%v", user, err)
	}
}

func TestExecCommandAndRemoteEnvUsePasswdFallbacks(t *testing.T) {
	t.Parallel()

	var requests []dockercli.ExecRequest
	executor := &Executor{engine: &fakeExecutorEngine{execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
		requests = append(requests, req)
		return "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
	}}}
	observed := ObservedState{Resolved: devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: spec.MergedConfig{RemoteEnv: map[string]string{"REMOTE": "1"}}}, Target: RuntimeTarget{PrimaryContainer: "container-123"}}

	command, err := executor.execCommand(context.Background(), observed, "vscode", nil)
	if err != nil {
		t.Fatalf("exec command: %v", err)
	}
	if !reflect.DeepEqual(command, []string{"/bin/bash"}) {
		t.Fatalf("unexpected shell fallback %#v", command)
	}
	env, err := executor.execRemoteEnv(context.Background(), observed, "vscode", map[string]string{"EXTRA": "2"})
	if err != nil {
		t.Fatalf("exec remote env: %v", err)
	}
	if !reflect.DeepEqual(env, map[string]string{"REMOTE": "1", "EXTRA": "2", "HOME": "/home/vscode"}) {
		t.Fatalf("unexpected exec remote env %#v", env)
	}
	if len(requests) != 2 {
		t.Fatalf("expected passwd lookups for shell and home, got %d", len(requests))
	}
	if command, err = executor.execCommand(context.Background(), observed, "vscode", []string{"pwd"}); err != nil || !reflect.DeepEqual(command, []string{"pwd"}) {
		t.Fatalf("expected explicit command passthrough, got %#v err=%v", command, err)
	}
}

func TestExecRemoteEnvPreservesExplicitHomeAndDockerExecRequest(t *testing.T) {
	t.Parallel()

	executor := &Executor{engine: &fakeExecutorEngine{execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
		return "vscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
	}}}
	observed := ObservedState{Resolved: devcontainer.ResolvedConfig{RemoteWorkspace: "/workspaces/demo", Merged: spec.MergedConfig{RemoteUser: "vscode", RemoteEnv: map[string]string{"HOME": "/custom", "REMOTE": "1"}}}, Target: RuntimeTarget{PrimaryContainer: "container-123"}}

	env, err := executor.execRemoteEnv(context.Background(), observed, "vscode", map[string]string{"EXTRA": "2"})
	if err != nil {
		t.Fatalf("exec remote env: %v", err)
	}
	if !reflect.DeepEqual(env, map[string]string{"HOME": "/custom", "REMOTE": "1", "EXTRA": "2"}) {
		t.Fatalf("unexpected env with explicit HOME %#v", env)
	}
	req, err := executor.DockerExecRequest(context.Background(), observed, true, true, map[string]string{"EXTRA": "2"}, nil, dockercli.Streams{})
	if err != nil {
		t.Fatalf("docker exec request: %v", err)
	}
	if req.ContainerID != "container-123" || req.User != "vscode" || req.Workdir != "/workspaces/demo" || !req.Interactive || !req.TTY {
		t.Fatalf("unexpected docker exec request %#v", req)
	}
	if !reflect.DeepEqual(req.Command, []string{"/bin/bash"}) {
		t.Fatalf("unexpected docker exec command %#v", req.Command)
	}
	if !reflect.DeepEqual(req.Env, map[string]string{"HOME": "/custom", "REMOTE": "1", "EXTRA": "2"}) {
		t.Fatalf("unexpected docker exec env %#v", req.Env)
	}
}

func TestExecArgsAndEffectiveRemoteUserHelpers(t *testing.T) {
	t.Parallel()

	args := execArgs(dockercli.ExecRequest{ContainerID: "container-123", User: "vscode", Workdir: "/workspaces/demo", Interactive: true, TTY: true, Env: map[string]string{"B": "2", "A": "1"}, Command: []string{"pwd"}})
	if !reflect.DeepEqual(args, []string{"exec", "-i", "-t", "-u", "vscode", "--workdir", "/workspaces/demo", "-e", "A=1", "-e", "B=2", "container-123", "pwd"}) {
		t.Fatalf("unexpected exec args %#v", args)
	}
	resolved := devcontainer.ResolvedConfig{Merged: spec.MergedConfig{ContainerUser: "container-user"}}
	if got := effectiveRemoteUserFromContainerInspect(&docker.ContainerInspect{Config: docker.InspectConfig{User: "inspect-user"}}, resolved); got != "container-user" {
		t.Fatalf("unexpected effective remote user %q", got)
	}
	resolved.Merged.ContainerUser = ""
	if got := effectiveRemoteUserFromContainerInspect(&docker.ContainerInspect{Config: docker.InspectConfig{User: "inspect-user"}}, resolved); got != "inspect-user" {
		t.Fatalf("unexpected fallback remote user %q", got)
	}
	if got := effectiveRemoteUserFromContainerInspect(nil, devcontainer.ResolvedConfig{}); got != "" {
		t.Fatalf("unexpected empty remote user %q", got)
	}
}
