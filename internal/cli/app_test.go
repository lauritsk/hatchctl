package cli

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/lauritsk/hatchctl/internal/bridge"
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

	global, remaining, err := parseGlobalOptions([]string{"--verbose", "--debug", "up", "--workspace", "/tmp/demo"})
	if err != nil {
		t.Fatalf("parse global options: %v", err)
	}
	if !global.Verbose || !global.Debug {
		t.Fatalf("unexpected global options %#v", global)
	}
	if len(remaining) != 3 || remaining[0] != "up" {
		t.Fatalf("unexpected remaining args %#v", remaining)
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
		if opts.Progress == nil {
			t.Fatal("expected progress writer")
		}
		_, _ = io.WriteString(opts.Progress, "==> Resolving development container\n")
		_, _ = io.WriteString(opts.Progress, "plan source=image config=/tmp/devcontainer.json workspace=/workspace state=/tmp/state target-image=hatchctl-demo\n")
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
		if opts.Progress != nil {
			t.Fatal("expected no progress writer for json output")
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
