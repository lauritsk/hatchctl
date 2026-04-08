package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/runtime"
)

type stubRunner struct {
	up           func(context.Context, runtime.UpOptions) (runtime.UpResult, error)
	build        func(context.Context, runtime.BuildOptions) (runtime.BuildResult, error)
	exec         func(context.Context, runtime.ExecOptions) (int, error)
	readConfig   func(context.Context, runtime.ReadConfigOptions) (runtime.ReadConfigResult, error)
	runLifecycle func(context.Context, runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error)
	bridgeDoctor func(context.Context, runtime.BridgeDoctorOptions) (bridge.Report, error)
}

func (s stubRunner) Up(ctx context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
	if s.up != nil {
		return s.up(ctx, opts)
	}
	return runtime.UpResult{}, nil
}

func (s stubRunner) Build(ctx context.Context, opts runtime.BuildOptions) (runtime.BuildResult, error) {
	if s.build != nil {
		return s.build(ctx, opts)
	}
	return runtime.BuildResult{}, nil
}

func (s stubRunner) Exec(ctx context.Context, opts runtime.ExecOptions) (int, error) {
	if s.exec != nil {
		return s.exec(ctx, opts)
	}
	return 0, nil
}

func (s stubRunner) ReadConfig(ctx context.Context, opts runtime.ReadConfigOptions) (runtime.ReadConfigResult, error) {
	if s.readConfig != nil {
		return s.readConfig(ctx, opts)
	}
	return runtime.ReadConfigResult{}, nil
}

func (s stubRunner) RunLifecycle(ctx context.Context, opts runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error) {
	if s.runLifecycle != nil {
		return s.runLifecycle(ctx, opts)
	}
	return runtime.RunLifecycleResult{}, nil
}

func (s stubRunner) BridgeDoctor(ctx context.Context, opts runtime.BridgeDoctorOptions) (bridge.Report, error) {
	if s.bridgeDoctor != nil {
		return s.bridgeDoctor(ctx, opts)
	}
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

func TestRunUpPassesDotfilesFlags(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got runtime.UpOptions
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		got = opts
		return runtime.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"up", "--dotfiles", "github.com/lauritsk/dotfiles", "--dotfiles-install-command", "install", "--dotfiles-target-path", "~/dotfiles"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Dotfiles.Repository != "github.com/lauritsk/dotfiles" || got.Dotfiles.InstallCommand != "install" || got.Dotfiles.TargetPath != "~/dotfiles" {
		t.Fatalf("unexpected dotfiles options %#v", got.Dotfiles)
	}
}

func TestRunUpAcceptsExplicitDotfilesRepositoryFlag(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got runtime.UpOptions
	app := NewWithRunner(&out, &errOut, stubRunner{up: func(_ context.Context, opts runtime.UpOptions) (runtime.UpResult, error) {
		got = opts
		return runtime.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"up", "--dotfiles-repository", "github.com/lauritsk/dotfiles"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Dotfiles.Repository != "github.com/lauritsk/dotfiles" {
		t.Fatalf("unexpected dotfiles repository %#v", got.Dotfiles)
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

func TestRunExecRequiresCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{})

	err := app.Run(context.Background(), []string{"exec"})
	if err == nil || err.Error() != "exec requires a command" {
		t.Fatalf("expected missing command error, got %v", err)
	}
}

func TestRunExecJSONCapturesOutputAndEnv(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got runtime.ExecOptions
	app := NewWithRunner(&out, &errOut, stubRunner{exec: func(_ context.Context, opts runtime.ExecOptions) (int, error) {
		got = opts
		_, _ = opts.Stdout.Write([]byte("command output\n"))
		_, _ = opts.Stderr.Write([]byte("warning output\n"))
		return 0, nil
	}})

	err := app.Run(context.Background(), []string{"exec", "--json", "--env", "A=1", "--env", "EMPTY", "--env", "PAIR=a=b", "--", "sh", "-lc", "echo hi"})
	if err != nil {
		t.Fatalf("run app: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
	if got.LockfilePolicy != devcontainer.FeatureLockfilePolicyAuto {
		t.Fatalf("unexpected lockfile policy %q", got.LockfilePolicy)
	}
	if strings.Join(got.Args, " ") != "sh -lc echo hi" {
		t.Fatalf("unexpected exec args %#v", got.Args)
	}
	if got.RemoteEnv["A"] != "1" || got.RemoteEnv["EMPTY"] != "" || got.RemoteEnv["PAIR"] != "a=b" {
		t.Fatalf("unexpected remote env %#v", got.RemoteEnv)
	}
	if got.Events != nil {
		t.Fatal("expected no event sink for json output")
	}
	if gotOut := out.String(); !strings.Contains(gotOut, `"exitCode": 0`) || !strings.Contains(gotOut, `"stdout": "command output\n"`) || !strings.Contains(gotOut, `"stderr": "warning output\n"`) {
		t.Fatalf("unexpected json output %q", gotOut)
	}
	if gotOut := out.String(); !strings.Contains(gotOut, `"command": [`) || !strings.Contains(gotOut, `"echo hi"`) {
		t.Fatalf("expected command in json output, got %q", gotOut)
	}
}

func TestRunExecReturnsExitErrorForNonZeroCode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{exec: func(_ context.Context, _ runtime.ExecOptions) (int, error) {
		return 7, nil
	}})

	err := app.Run(context.Background(), []string{"exec", "--", "false"})
	var exitErr runtime.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 7 {
		t.Fatalf("expected exit error code 7, got %v", err)
	}
}

func TestRunConfigUsesFrozenLockfilePolicy(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	called := false
	app := NewWithRunner(&out, &errOut, stubRunner{readConfig: func(_ context.Context, opts runtime.ReadConfigOptions) (runtime.ReadConfigResult, error) {
		called = true
		if opts.LockfilePolicy != devcontainer.FeatureLockfilePolicyFrozen {
			t.Fatalf("unexpected lockfile policy %q", opts.LockfilePolicy)
		}
		return runtime.ReadConfigResult{ConfigPath: "/tmp/devcontainer.json", WorkspaceFolder: "/workspace", WorkspaceMount: "type=bind", SourceKind: "image", Dotfiles: &runtime.DotfilesStatus{Configured: true, Applied: false, NeedsInstall: true, Repository: "https://github.com/lauritsk/dotfiles.git", TargetPath: "$HOME/.dotfiles"}}, nil
	}})

	if err := app.Run(context.Background(), []string{"config"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected config runner to be called")
	}
	if got := out.String(); !strings.Contains(got, "Dotfiles") {
		t.Fatalf("expected dotfiles status in output, got %q", got)
	}
}

func TestRunLifecyclePassesPhase(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{runLifecycle: func(_ context.Context, opts runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error) {
		if opts.Phase != "attach" {
			t.Fatalf("unexpected phase %q", opts.Phase)
		}
		return runtime.RunLifecycleResult{ContainerID: "abc123", Phase: opts.Phase}, nil
	}})

	if err := app.Run(context.Background(), []string{"run", "--phase", "attach"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got := out.String(); got != "Ran lifecycle commands for abc123 (attach).\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunBridgeDoctorUsesFrozenLockfilePolicy(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	called := false
	app := NewWithRunner(&out, &errOut, stubRunner{bridgeDoctor: func(_ context.Context, opts runtime.BridgeDoctorOptions) (bridge.Report, error) {
		called = true
		if opts.LockfilePolicy != devcontainer.FeatureLockfilePolicyFrozen {
			t.Fatalf("unexpected lockfile policy %q", opts.LockfilePolicy)
		}
		return bridge.Report{ID: "session", Enabled: true, Status: "running"}, nil
	}})

	if err := app.Run(context.Background(), []string{"bridge", "doctor"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected doctor runner to be called")
	}
}

func TestRunBridgeServeRequiresFlags(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{})

	err := app.Run(context.Background(), []string{"bridge", "serve"})
	if err == nil || err.Error() != "bridge serve requires --state-dir and --container-id" {
		t.Fatalf("expected missing flag error, got %v", err)
	}
}

func TestRunRejectsInvalidLockfilePolicy(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithRunner(&out, &errOut, stubRunner{})

	err := app.Run(context.Background(), []string{"up", "--lockfile-policy", "bogus"})
	if err == nil || err.Error() != `unsupported lockfile policy "bogus"` {
		t.Fatalf("expected lockfile policy error, got %v", err)
	}
}
