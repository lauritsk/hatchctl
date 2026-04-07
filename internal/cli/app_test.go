package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/bridge"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/runtime"
)

type stubRunner struct {
	up func(context.Context, runtime.UpOptions) (runtime.UpResult, error)
}

func (s stubRunner) Up(ctx context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
	if s.up != nil {
		return s.up(ctx, opts)
	}
	return runtime.UpResult{}, nil
}

func (s stubRunner) Build(context.Context, runtime.BuildOptions) (runtime.BuildResult, error) {
	return runtime.BuildResult{}, nil
}

func (s stubRunner) Exec(context.Context, runtime.ExecOptions) (int, error) {
	return 0, nil
}

func (s stubRunner) ReadConfig(context.Context, runtime.ReadConfigOptions) (runtime.ReadConfigResult, error) {
	return runtime.ReadConfigResult{}, nil
}

func (s stubRunner) RunLifecycle(context.Context, runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error) {
	return runtime.RunLifecycleResult{}, nil
}

func (s stubRunner) BridgeDoctor(context.Context, runtime.BridgeDoctorOptions) (bridge.Report, error) {
	return bridge.Report{}, nil
}

func TestParseGlobalOptionsStripsLeadingVerboseFlags(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got runtime.UpOptions
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		got = opts
		return runtime.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"--verbose", "--debug", "up", "--workspace", "/tmp/demo"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !got.Verbose || !got.Debug {
		t.Fatalf("unexpected global options %#v", got)
	}
	if got.Workspace != "/tmp/demo" {
		t.Fatalf("unexpected workspace %q", got.Workspace)
	}
	if got.FeatureTimeout != 90*time.Second {
		t.Fatalf("unexpected default feature timeout %s", got.FeatureTimeout)
	}
}

func TestRunUpPassesFeatureTimeoutFlag(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got runtime.UpOptions
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		got = opts
		return runtime.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"up", "--feature-timeout", "45s"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.FeatureTimeout != 45*time.Second {
		t.Fatalf("unexpected feature timeout %s", got.FeatureTimeout)
	}
}

func TestRunUpUsesGlobalDebugForProgressAndPlan(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	called := false
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		called = true
		if !opts.Verbose || !opts.Debug {
			t.Fatalf("expected verbose+debug options, got %#v", opts)
		}
		if opts.Events == nil {
			t.Fatal("expected event sink")
		}
		opts.Events.Emit(ui.Event{Kind: ui.EventProgress, Message: "Resolving development container"})
		opts.Events.Emit(ui.Event{Kind: ui.EventDebug, Message: "plan source=image config=/tmp/devcontainer.json workspace=/workspace state=/tmp/state target-image=hatchctl-demo"})
		return runtime.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := app.Run(context.Background(), []string{"--debug", "up"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected up runner to be called")
	}
	if got := errOut.String(); got != "==> Resolving development container\nplan source=image config=/tmp/devcontainer.json workspace=/workspace state=/tmp/state target-image=hatchctl-demo\n" {
		t.Fatalf("unexpected progress output %q", got)
	}
	if got := out.String(); got != "Container: abc123\nImage: hatchctl-demo\nWorkspace: /workspace\nState: /tmp/state\n" {
		t.Fatalf("unexpected command output %q", got)
	}
}

func TestRunUpJSONDisablesProgressOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		if opts.Events != nil {
			t.Fatal("expected no event sink for json output")
		}
		return runtime.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := app.Run(context.Background(), []string{"--verbose", "up", "--json"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
	if got := out.String(); got == "" || got[0] != '{' {
		t.Fatalf("expected json output, got %q", got)
	}
}
