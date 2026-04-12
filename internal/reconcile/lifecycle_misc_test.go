package reconcile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/bridge"
	capdot "github.com/lauritsk/hatchctl/internal/capability/dotfiles"
	"github.com/lauritsk/hatchctl/internal/command"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type recordingRunner struct {
	runFn func(context.Context, command.Command) error
	runs  []command.Command
}

func (r *recordingRunner) Run(ctx context.Context, cmd command.Command) error {
	r.runs = append(r.runs, cmd)
	if r.runFn != nil {
		return r.runFn(ctx, cmd)
	}
	return nil
}

func (*recordingRunner) Output(context.Context, command.Command) (string, string, error) {
	return "", "", nil
}

func (*recordingRunner) CombinedOutput(context.Context, command.Command) (string, error) {
	return "", nil
}

func (*recordingRunner) Start(command.StartOptions) (*os.Process, error) {
	return nil, errors.New("not implemented")
}

type reconcileEventSink struct {
	events []ui.Event
}

func (s *reconcileEventSink) Emit(event ui.Event) {
	s.events = append(s.events, event)
}

func TestRunCommandSupportsStringArrayAndObject(t *testing.T) {
	t.Parallel()

	var calls [][]string
	runner := func(_ context.Context, args []string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	if err := runCommand(context.Background(), runner, spec.LifecycleCommand{Kind: "string", Value: "echo hi", Exists: true}); err != nil {
		t.Fatalf("run string command: %v", err)
	}
	if err := runCommand(context.Background(), runner, spec.LifecycleCommand{Kind: "array", Args: []string{"echo", "there"}, Exists: true}); err != nil {
		t.Fatalf("run array command: %v", err)
	}
	if err := runCommand(context.Background(), runner, spec.LifecycleCommand{Kind: "object", Steps: map[string]spec.LifecycleCommand{"b": {Kind: "string", Value: "echo two", Exists: true}, "a": {Kind: "array", Args: []string{"echo", "one"}, Exists: true}}, Exists: true}); err != nil {
		t.Fatalf("run object command: %v", err)
	}
	if !reflect.DeepEqual(calls, [][]string{{"/bin/sh", "-lc", "echo hi"}, {"echo", "there"}, {"echo", "one"}, {"/bin/sh", "-lc", "echo two"}}) {
		t.Fatalf("unexpected run command calls %#v", calls)
	}
}

func TestRunCommandWrapsFailingStep(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	err := runCommand(context.Background(), func(_ context.Context, args []string) error {
		if len(args) > 0 && args[len(args)-1] == "fail" {
			return errBoom
		}
		return nil
	}, spec.LifecycleCommand{Kind: "object", Steps: map[string]spec.LifecycleCommand{"a": {Kind: "string", Value: "echo ok", Exists: true}, "b": {Kind: "array", Args: []string{"echo", "fail"}, Exists: true}}, Exists: true})
	if err == nil || !strings.Contains(err.Error(), "lifecycle step b") || !errors.Is(err, errBoom) {
		t.Fatalf("expected wrapped lifecycle step error, got %v", err)
	}
}

func TestRunHostLifecycleUsesRunnerAndSkipsEmpty(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	streams := commandIO{Stdin: strings.NewReader("in"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := runHostLifecycle(context.Background(), "/workspace", spec.LifecycleCommand{}, streams, runner); err != nil {
		t.Fatalf("run empty host lifecycle: %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("expected empty lifecycle to skip runner, got %#v", runner.runs)
	}
	if err := runHostLifecycle(context.Background(), "/workspace", spec.LifecycleCommand{Kind: "string", Value: "echo hi", Exists: true}, streams, runner); err != nil {
		t.Fatalf("run host lifecycle: %v", err)
	}
	if len(runner.runs) != 1 || runner.runs[0].Binary != "/bin/sh" || runner.runs[0].Dir != "/workspace" {
		t.Fatalf("unexpected host lifecycle run %#v", runner.runs)
	}
	if !reflect.DeepEqual(runner.runs[0].Args, []string{"-lc", "echo hi"}) {
		t.Fatalf("unexpected host lifecycle args %#v", runner.runs[0].Args)
	}
}

func TestDotfilesStatusHelpersAndTargetResolution(t *testing.T) {
	t.Parallel()

	state := storefs.WorkspaceState{DotfilesReady: true, DotfilesRepo: "https://github.com/example/dotfiles.git", DotfilesInstall: "install.sh", DotfilesTarget: "$HOME/.dotfiles"}
	cfg := capdot.Config{Repository: "https://github.com/example/dotfiles.git", InstallCommand: "install.sh", TargetPath: "$HOME/.dotfiles"}
	status := DotfilesStatusFromState(state, cfg)
	if status == nil || !status.Applied || status.NeedsInstall {
		t.Fatalf("unexpected dotfiles status %#v", status)
	}
	if DotfilesNeedsInstall(state, cfg) {
		t.Fatal("expected matching dotfiles state to skip install")
	}
	if got, err := (&Executor{}).resolveDotfilesTargetPath(context.Background(), ObservedState{}, "/already/absolute"); err != nil || got != "/already/absolute" {
		t.Fatalf("expected non-HOME path passthrough, got %q err=%v", got, err)
	}
	executor := &Executor{engine: &fakeExecutorEngine{execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
		return "vscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
	}}}
	observed := ObservedState{Resolved: devcontainer.ResolvedConfig{Merged: spec.MergedConfig{RemoteUser: "vscode"}}, Target: RuntimeTarget{PrimaryContainer: "container-123"}}
	if got, err := executor.resolveDotfilesTargetPath(context.Background(), observed, "$HOME/.dotfiles"); err != nil || got != "/home/vscode/.dotfiles" {
		t.Fatalf("unexpected resolved dotfiles path %q err=%v", got, err)
	}
	if lifecycleProgressLabel("postCreateCommand") != "Running postCreateCommand lifecycle hook" {
		t.Fatalf("unexpected lifecycle progress label %q", lifecycleProgressLabel("postCreateCommand"))
	}
}

func TestBridgePreviewLoadsPersistedSession(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	paths, err := storefs.EnsureWorkspaceBridgePaths(stateDir)
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	want := bridge.Session{ID: "bridge-123", Enabled: true, HelperArch: "arm64"}
	if err := storefs.WriteBridgeSession(paths.Dir, want); err != nil {
		t.Fatalf("write bridge session: %v", err)
	}
	got, err := bridgePreview(stateDir)
	if err != nil {
		t.Fatalf("bridge preview: %v", err)
	}
	if got == nil || got.ID != want.ID || got.HelperArch != want.HelperArch {
		t.Fatalf("unexpected bridge preview %#v", got)
	}
}

func TestProgressWritersEmitOnceAndRedactionHelpers(t *testing.T) {
	t.Parallel()

	sink := &reconcileEventSink{}
	executor := &Executor{}
	var out bytes.Buffer
	stdout, _ := executor.progressWriters(sink, phaseImage, "Building image", &out, io.Discard)
	if _, err := stdout.Write([]byte("step 1\n")); err != nil {
		t.Fatalf("write progress output: %v", err)
	}
	if _, err := stdout.Write([]byte("step 2\n")); err != nil {
		t.Fatalf("write progress output second time: %v", err)
	}
	if len(sink.events) != 2 || sink.events[0].Kind != ui.EventClear || sink.events[1].Kind != ui.EventProgressOutput {
		t.Fatalf("unexpected progress events %#v", sink.events)
	}
	if got, want := sink.events[1].Message, "Building image"; got != want {
		t.Fatalf("unexpected progress message %q", got)
	}
	if got := out.String(); got != "step 1\nstep 2\n" {
		t.Fatalf("unexpected progress writer output %q", got)
	}
	if got, want := executor.progressWriters(nil, phaseImage, "ignored", &out, io.Discard); got != &out || want != io.Discard {
		t.Fatal("expected nil events to preserve original writers")
	}
	redacted := RedactSensitiveMap(map[string]string{"api.token": "secret", "safe": "value", "ssh-key": "private", "custom_key": "masked"})
	if redacted["safe"] != "value" || redacted["api.token"] != redactedValue || redacted["ssh-key"] != redactedValue || redacted["custom_key"] != redactedValue {
		t.Fatalf("unexpected redacted map %#v", redacted)
	}
	if isSensitiveKey("plain") {
		t.Fatal("expected non-sensitive key to stay visible")
	}
}

func TestSessionSettersKeepObservedStateInSync(t *testing.T) {
	t.Parallel()

	session := &Session{prepared: preparedWorkspace{resolved: devcontainer.ResolvedConfig{ImageName: "image-a"}, state: storefs.WorkspaceState{ContainerID: "container-a"}, containerID: "container-a", containerInspect: &docker.ContainerInspect{ID: "container-a"}, observed: ObservedState{Resolved: devcontainer.ResolvedConfig{ImageName: "image-a"}, Target: RuntimeTarget{PrimaryContainer: "container-a"}, Control: ControlState{WorkspaceState: storefs.WorkspaceState{ContainerID: "container-a"}}, Container: &docker.ContainerInspect{ID: "container-a"}}}}

	session.SetResolved(devcontainer.ResolvedConfig{ImageName: "image-b"})
	if session.Observed().Resolved.ImageName != "image-b" {
		t.Fatalf("expected resolved config sync, got %#v", session.Observed().Resolved)
	}
	session.SetState(storefs.WorkspaceState{ContainerID: "container-b", DotfilesReady: true})
	if session.ContainerID() != "container-b" || session.Observed().Control.WorkspaceState.ContainerID != "container-b" || session.ContainerInspect() != nil {
		t.Fatalf("unexpected session state after SetState %#v", session.Observed())
	}
	inspect := &docker.ContainerInspect{ID: "container-b"}
	session.SetContainerInspect(inspect)
	if session.Observed().Container != inspect {
		t.Fatalf("expected container inspect sync, got %#v", session.Observed().Container)
	}
	session.SetContainerID("container-c")
	if session.State().ContainerID != "container-c" || session.Observed().Target.PrimaryContainer != "container-c" || session.ContainerInspect() != nil {
		t.Fatalf("unexpected session state after SetContainerID %#v", session.Observed())
	}
	if err := session.RevalidateReadTarget(context.Background()); err != nil {
		t.Fatalf("expected empty read token to skip revalidation, got %v", err)
	}
}

func TestPreparedImageAndWarningHelpers(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{ImageName: "managed-image"}
	if got := preparedImage(resolved); got != "managed-image" {
		t.Fatalf("unexpected prepared image %q", got)
	}
	resolved = devcontainer.ResolvedConfig{SourceKind: "compose", ImageName: "managed-image"}
	if got := preparedImage(resolved); got != "" {
		t.Fatalf("expected compose prepared image to stay empty, got %q", got)
	}
	var stderr bytes.Buffer
	executor := &Executor{stderr: &stderr}
	executor.emitWarning(nil, "careful")
	if stderr.String() != "warning: careful\n" {
		t.Fatalf("unexpected warning output %q", stderr.String())
	}
	sink := &reconcileEventSink{}
	executor.emitResolvedPlan(sink, devcontainer.ResolvedConfig{SourceKind: "image", ConfigPath: "/workspace/.devcontainer/devcontainer.json", WorkspaceFolder: "/workspace", StateDir: "/state", ImageName: "hatchctl-demo"})
	if len(sink.events) != 1 || sink.events[0].Kind != ui.EventDebug || !strings.Contains(sink.events[0].Message, "target-image=hatchctl-demo") {
		t.Fatalf("unexpected resolved plan event %#v", sink.events)
	}
	if eventCount := len(sink.events); eventCount != 1 {
		t.Fatalf("expected one event, got %d", eventCount)
	}
	if err := (&Executor{imageVerifier: policy.NewImageVerificationPolicyWithPrompt(false, nil)}).verifyImageReference(context.Background(), "", nil); err != nil {
		t.Fatal("expected empty reference image verification to succeed")
	}
	file, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()
	previousIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	defer func() { isTerminal = previousIsTerminal }()
	if !ShouldAllocateTTY(file, file) {
		t.Fatal("expected tty-capable streams to allocate tty")
	}
	if fd, ok := streamFileDescriptor(strings.NewReader("nope")); ok || fd != 0 {
		t.Fatal("expected non-fd stream to have no descriptor")
	}
	if !isTerminalStream(file) {
		t.Fatal("expected terminal override to mark file as tty")
	}
}
