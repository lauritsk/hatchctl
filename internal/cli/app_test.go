package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

type stubService struct {
	up           func(context.Context, appcore.UpRequest) (appcore.UpResult, error)
	build        func(context.Context, appcore.BuildRequest) (appcore.BuildResult, error)
	exec         func(context.Context, appcore.ExecRequest) (int, error)
	readConfig   func(context.Context, appcore.ReadConfigRequest) (appcore.ReadConfigResult, error)
	runLifecycle func(context.Context, appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error)
	bridgeDoctor func(context.Context, appcore.BridgeDoctorRequest) (bridge.Report, error)
}

func (s stubService) Up(ctx context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
	if s.up != nil {
		return s.up(ctx, opts)
	}
	return appcore.UpResult{}, nil
}

func (s stubService) Build(ctx context.Context, opts appcore.BuildRequest) (appcore.BuildResult, error) {
	if s.build != nil {
		return s.build(ctx, opts)
	}
	return appcore.BuildResult{}, nil
}

func (s stubService) Exec(ctx context.Context, opts appcore.ExecRequest) (int, error) {
	if s.exec != nil {
		return s.exec(ctx, opts)
	}
	return 0, nil
}

func (s stubService) ReadConfig(ctx context.Context, opts appcore.ReadConfigRequest) (appcore.ReadConfigResult, error) {
	if s.readConfig != nil {
		return s.readConfig(ctx, opts)
	}
	return appcore.ReadConfigResult{}, nil
}

func (s stubService) RunLifecycle(ctx context.Context, opts appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error) {
	if s.runLifecycle != nil {
		return s.runLifecycle(ctx, opts)
	}
	return appcore.RunLifecycleResult{}, nil
}

func (s stubService) BridgeDoctor(ctx context.Context, opts appcore.BridgeDoctorRequest) (bridge.Report, error) {
	if s.bridgeDoctor != nil {
		return s.bridgeDoctor(ctx, opts)
	}
	return bridge.Report{}, nil
}

func TestParseGlobalOptionsStripsLeadingVerboseFlags(t *testing.T) {
	var got appcore.UpRequest
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := h.run("--verbose", "--debug", "up", "--workspace", "/tmp/demo"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !got.Global.Verbose || !got.Global.Debug {
		t.Fatalf("unexpected global options %#v", got)
	}
	if got.Defaults.Workspace != "/tmp/demo" {
		t.Fatalf("unexpected workspace %q", got.Defaults.Workspace)
	}
	if got.Defaults.FeatureTimeout != 90*time.Second {
		t.Fatalf("unexpected default feature timeout %s", got.Defaults.FeatureTimeout)
	}
}

func TestRunUpPassesFeatureTimeoutFlag(t *testing.T) {
	var got appcore.UpRequest
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := h.run("up", "--feature-timeout", "45s"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Defaults.FeatureTimeout != 45*time.Second {
		t.Fatalf("unexpected feature timeout %s", got.Defaults.FeatureTimeout)
	}
}

func TestRunUpPassesDotfilesFlags(t *testing.T) {
	var got appcore.UpRequest
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := h.run("up", "--dotfiles", "github.com/lauritsk/dotfiles", "--dotfiles-install-command", "install", "--dotfiles-target-path", "~/dotfiles"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Defaults.Dotfiles.Repository != "github.com/lauritsk/dotfiles" || got.Defaults.Dotfiles.InstallCommand != "install" || got.Defaults.Dotfiles.TargetPath != "~/dotfiles" {
		t.Fatalf("unexpected dotfiles options %#v", got.Defaults.Dotfiles)
	}
}

func TestRunUpAcceptsExplicitDotfilesRepositoryFlag(t *testing.T) {
	var got appcore.UpRequest
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := h.run("up", "--dotfiles-repository", "github.com/lauritsk/dotfiles"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Defaults.Dotfiles.Repository != "github.com/lauritsk/dotfiles" {
		t.Fatalf("unexpected dotfiles repository %#v", got.Defaults.Dotfiles)
	}
}

func TestRunBuildRejectsDotfilesFlags(t *testing.T) {
	h := newAppHarness(t, stubService{})

	err := h.run("build", "--dotfiles", "github.com/example/dotfiles")
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --dotfiles") {
		t.Fatalf("expected unknown dotfiles flag error, got %v", err)
	}
}

func TestRunUpUsesGlobalDebugForProgressAndPlan(t *testing.T) {
	called := false
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		called = true
		if opts.Global.Verbose || !opts.Global.Debug {
			t.Fatalf("expected verbose+debug options, got %#v", opts)
		}
		if opts.IO.Events == nil {
			t.Fatal("expected event sink")
		}
		opts.IO.Events.Emit(ui.Event{Kind: ui.EventProgress, Message: "Resolving development container"})
		opts.IO.Events.Emit(ui.Event{Kind: ui.EventDebug, Message: "plan source=image config=/tmp/devcontainer.json workspace=/workspace state=/tmp/state target-image=hatchctl-demo"})
		return appcore.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := h.run("--debug", "up"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected up runner to be called")
	}
	assertContainsAll(t, h.stderr(), "==> Resolving development container\n", "plan source=image", "target-image=hatchctl-demo")
	assertContainsAll(t, h.stdout(), "Container: abc123\n", "Image: hatchctl-demo\n", "Workspace: /workspace\n", "\nNext:\n", "  hatchctl exec\n", "  hatchctl exec -- pwd\n", "  hatchctl exec -- go test ./...\n")
}

func TestRunUpPrintsSuggestedExecCommands(t *testing.T) {
	h := newAppHarness(t, stubService{up: func(_ context.Context, _ appcore.UpRequest) (appcore.UpResult, error) {
		return appcore.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := h.run("up", "--workspace", "../my project", "--config", "dev/container.json", "--feature-timeout", "45s", "--lockfile-policy", "frozen"); err != nil {
		t.Fatalf("run app: %v", err)
	}

	assertContainsAll(t, h.stdout(),
		"Container: abc123\n",
		"Image: hatchctl-demo\n",
		"Workspace: /workspace\n",
		"  hatchctl exec --workspace \"../my project\" --config \"dev/container.json\" --feature-timeout 45s --lockfile-policy frozen\n",
		"  hatchctl exec --workspace \"../my project\" --config \"dev/container.json\" --feature-timeout 45s --lockfile-policy frozen -- pwd\n",
		"  hatchctl exec --workspace \"../my project\" --config \"dev/container.json\" --feature-timeout 45s --lockfile-policy frozen -- go test ./...\n",
	)
}

func TestRunUpWithSSHPrintsSuggestedExecCommandsWithSSH(t *testing.T) {
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		if !opts.Defaults.SSHAgent {
			t.Fatal("expected ssh-agent passthrough to be enabled")
		}
		return appcore.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := h.run("up", "--ssh"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got := h.stdout(); !strings.Contains(got, "hatchctl exec --ssh") {
		t.Fatalf("expected suggested ssh exec commands, got %q", got)
	}
}

func TestRunUpJSONDisablesProgressOutput(t *testing.T) {
	h := newAppHarness(t, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		if opts.IO.Events != nil {
			t.Fatal("expected no event sink for json output")
		}
		return appcore.UpResult{ContainerID: "abc123", Image: "hatchctl-demo", RemoteWorkspaceFolder: "/workspace", StateDir: "/tmp/state"}, nil
	}})

	if err := h.run("--verbose", "up", "--json"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if h.stderr() != "" {
		t.Fatalf("expected no stderr output, got %q", h.stderr())
	}
	if got := h.stdout(); got == "" || got[0] != '{' {
		t.Fatalf("expected json output, got %q", got)
	}
}

func TestRunExecAllowsMissingCommand(t *testing.T) {
	called := false
	h := newAppHarness(t, stubService{exec: func(_ context.Context, opts appcore.ExecRequest) (int, error) {
		called = true
		if len(opts.Args) != 0 {
			t.Fatalf("expected no exec args, got %#v", opts.Args)
		}
		return 0, nil
	}})

	if err := h.run("exec"); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected exec runner to be called")
	}
}

func TestRunExecJSONRequiresCommand(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, appcore.NewWithExecutorWithoutMutationLock(&reconcile.Executor{}))

	err := app.Run(context.Background(), []string{"exec", "--json"})
	if err == nil || !strings.Contains(err.Error(), "missing command for exec --json") || !strings.Contains(err.Error(), "hatchctl exec --json -- <command>") {
		t.Fatalf("expected missing command error, got %v", err)
	}
}

func TestExecHelpExplainsShellAndSeparator(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, appcore.NewWithExecutorWithoutMutationLock(&reconcile.Executor{}))

	if err := app.Run(context.Background(), []string{"exec", "--help"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Open the remote user's default shell in the managed devcontainer") {
		t.Fatalf("expected updated exec help text, got %q", got)
	}
	if !strings.Contains(got, "hatchctl exec") || !strings.Contains(got, "Use `--` to separate hatchctl flags") {
		t.Fatalf("expected shell and separator guidance, got %q", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunExecJSONCapturesOutputAndEnv(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	var got appcore.ExecRequest
	app := NewWithService(&out, &errOut, stubService{exec: func(_ context.Context, opts appcore.ExecRequest) (int, error) {
		got = opts
		_, _ = opts.IO.Stdout.Write([]byte("command output\n"))
		_, _ = opts.IO.Stderr.Write([]byte("warning output\n"))
		return 0, nil
	}})

	err := app.Run(context.Background(), []string{"exec", "--json", "--env", "A=1", "--env", "EMPTY", "--env", "PAIR=a=b", "--", "sh", "-lc", "echo hi"})
	if err != nil {
		t.Fatalf("run app: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
	if got.Defaults.LockfilePolicy != string(devcontainer.FeatureLockfilePolicyAuto) {
		t.Fatalf("unexpected lockfile policy %q", got.Defaults.LockfilePolicy)
	}
	if strings.Join(got.Args, " ") != "sh -lc echo hi" {
		t.Fatalf("unexpected exec args %#v", got.Args)
	}
	if got.RemoteEnv["A"] != "1" || got.RemoteEnv["EMPTY"] != "" || got.RemoteEnv["PAIR"] != "a=b" {
		t.Fatalf("unexpected remote env %#v", got.RemoteEnv)
	}
	if got.IO.Events != nil {
		t.Fatal("expected no event sink for json output")
	}
	if gotOut := out.String(); !strings.Contains(gotOut, `"exitCode": 0`) || !strings.Contains(gotOut, `"stdout": "command output\n"`) || !strings.Contains(gotOut, `"stderr": "warning output\n"`) {
		t.Fatalf("unexpected json output %q", gotOut)
	}
	if gotOut := out.String(); !strings.Contains(gotOut, `"args": [`) || !strings.Contains(gotOut, `"echo hi"`) {
		t.Fatalf("expected command in json output, got %q", gotOut)
	}
}

func TestRunExecPassesSSHFlag(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{exec: func(_ context.Context, opts appcore.ExecRequest) (int, error) {
		if !opts.Defaults.SSHAgent {
			t.Fatal("expected ssh-agent passthrough flag")
		}
		return 0, nil
	}})

	if err := app.Run(context.Background(), []string{"exec", "--ssh", "--", "pwd"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
}

func TestRunBuildJSONKeepsStdoutClean(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{build: func(_ context.Context, opts appcore.BuildRequest) (appcore.BuildResult, error) {
		if opts.IO.Stdout == nil || opts.IO.Stderr == nil {
			t.Fatalf("expected managed build writers, got stdout=%v stderr=%v", opts.IO.Stdout, opts.IO.Stderr)
		}
		if _, err := opts.IO.Stdout.Write([]byte("build output\n")); err != nil {
			t.Fatalf("write build stdout: %v", err)
		}
		if _, err := opts.IO.Stderr.Write([]byte("build warning\n")); err != nil {
			t.Fatalf("write build stderr: %v", err)
		}
		return appcore.BuildResult{Image: "hatchctl-demo"}, nil
	}})

	if err := app.Run(context.Background(), []string{"build", "--json"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got := out.String(); !strings.Contains(got, `"image": "hatchctl-demo"`) {
		t.Fatalf("expected json build output, got %q", got)
	}
	if strings.Contains(out.String(), "build output") || strings.Contains(out.String(), "build warning") {
		t.Fatalf("expected command output to stay off stdout, got %q", out.String())
	}
	if got := errOut.String(); got != "build output\nbuild warning\n" {
		t.Fatalf("unexpected redirected command output %q", got)
	}
}

func TestRunExecReturnsExitErrorForNonZeroCode(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{exec: func(_ context.Context, _ appcore.ExecRequest) (int, error) {
		return 7, nil
	}})

	err := app.Run(context.Background(), []string{"exec", "--", "false"})
	var exitErr appcore.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 7 {
		t.Fatalf("expected exit error code 7, got %v", err)
	}
}

func TestExecWritersUsesRawTerminalStreamsForInteractiveExec(t *testing.T) {
	t.Parallel()

	renderer := ui.NewRenderer(&bytes.Buffer{}, &bytes.Buffer{}, false)
	stdout, stderr := execWriters(renderer, true)

	if stdout != os.Stdout {
		t.Fatalf("expected interactive stdout to use raw os.Stdout, got %T", stdout)
	}
	if stderr != os.Stderr {
		t.Fatalf("expected interactive stderr to use raw os.Stderr, got %T", stderr)
	}
}

func TestExecWritersKeepsManagedStreamsForNonInteractiveExec(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer
	renderer := ui.NewRenderer(&out, &errOut, false)
	stdout, stderr := execWriters(renderer, false)

	if stdout == os.Stdout {
		t.Fatal("expected non-interactive stdout to stay managed")
	}
	if stderr == os.Stderr {
		t.Fatal("expected non-interactive stderr to stay managed")
	}
	if _, err := stdout.Write([]byte("command output\n")); err != nil {
		t.Fatalf("write managed stdout: %v", err)
	}
	if _, err := stderr.Write([]byte("command error\n")); err != nil {
		t.Fatalf("write managed stderr: %v", err)
	}
	if got := out.String(); got != "command output\n" {
		t.Fatalf("unexpected managed stdout %q", got)
	}
	if got := errOut.String(); got != "command error\n" {
		t.Fatalf("unexpected managed stderr %q", got)
	}
}

func TestShouldUseRawExecStreamsRejectsNonTerminalFiles(t *testing.T) {
	t.Parallel()

	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("create temp stdin: %v", err)
	}
	defer stdin.Close()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("create temp stdout: %v", err)
	}
	defer stdout.Close()

	if shouldUseRawExecStreams(stdin, stdout) {
		t.Fatal("expected raw exec streams to require real terminals")
	}
}

func TestRunConfigUsesFrozenLockfilePolicy(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	called := false
	app := NewWithService(&out, &errOut, stubService{readConfig: func(_ context.Context, opts appcore.ReadConfigRequest) (appcore.ReadConfigResult, error) {
		called = true
		if opts.Defaults.LockfilePolicy != string(devcontainer.FeatureLockfilePolicyFrozen) {
			t.Fatalf("unexpected lockfile policy %q", opts.Defaults.LockfilePolicy)
		}
		return appcore.ReadConfigResult{ConfigPath: "/tmp/devcontainer.json", WorkspaceFolder: "/workspace", WorkspaceMount: "type=bind", SourceKind: "image", Dotfiles: &appcore.DotfilesStatus{Configured: true, Applied: false, NeedsInstall: true, Repository: "https://github.com/lauritsk/dotfiles.git", TargetPath: "$HOME/.dotfiles"}}, nil
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

func TestRunConfigPassesSSHFlag(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{readConfig: func(_ context.Context, opts appcore.ReadConfigRequest) (appcore.ReadConfigResult, error) {
		if !opts.Defaults.SSHAgent {
			t.Fatal("expected ssh-agent passthrough flag")
		}
		return appcore.ReadConfigResult{ConfigPath: "/tmp/devcontainer.json", WorkspaceFolder: "/workspace", WorkspaceMount: "type=bind", SourceKind: "image"}, nil
	}})

	if err := app.Run(context.Background(), []string{"config", "--ssh"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
}

func TestRunLifecyclePassesPhase(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{runLifecycle: func(_ context.Context, opts appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error) {
		if opts.Phase != "attach" {
			t.Fatalf("unexpected phase %q", opts.Phase)
		}
		return appcore.RunLifecycleResult{ContainerID: "abc123", Phase: opts.Phase}, nil
	}})

	if err := app.Run(context.Background(), []string{"run", "--phase", "attach"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	assertContainsAll(t, out.String(), `Lifecycle phase "attach" completed`, "container abc123")
}

func TestLifecycleAliasPassesPhase(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{runLifecycle: func(_ context.Context, opts appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error) {
		if opts.Phase != "start" {
			t.Fatalf("unexpected phase %q", opts.Phase)
		}
		return appcore.RunLifecycleResult{ContainerID: "abc123", Phase: opts.Phase}, nil
	}})

	if err := app.Run(context.Background(), []string{"lifecycle", "--phase", "start"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	assertContainsAll(t, out.String(), `Lifecycle phase "start" completed`, "container abc123")
}

func TestRunLifecycleRejectsInvalidPhase(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, appcore.NewWithExecutorWithoutMutationLock(&reconcile.Executor{}))

	err := app.Run(context.Background(), []string{"run", "--phase", "bogus"})
	if err == nil || !strings.Contains(err.Error(), `invalid lifecycle phase "bogus"`) {
		t.Fatalf("expected invalid phase error, got %v", err)
	}
}

func TestRunBridgeDoctorUsesFrozenLockfilePolicy(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	called := false
	app := NewWithService(&out, &errOut, stubService{bridgeDoctor: func(_ context.Context, opts appcore.BridgeDoctorRequest) (bridge.Report, error) {
		called = true
		if opts.Defaults.LockfilePolicy != string(devcontainer.FeatureLockfilePolicyFrozen) {
			t.Fatalf("unexpected lockfile policy %q", opts.Defaults.LockfilePolicy)
		}
		return bridge.Report{ID: "session", Enabled: true, Status: "running"}, nil
	}})

	if err := app.Run(context.Background(), []string{"bridge", "doctor"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !called {
		t.Fatal("expected doctor runner to be called")
	}
	if got := out.String(); !strings.Contains(got, "Bridge enabled: true") || !strings.Contains(got, "Current status: running") {
		t.Fatalf("unexpected bridge doctor output %q", got)
	}
}

func TestBridgeHelpHidesInternalServeCommand(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, appcore.NewWithExecutorWithoutMutationLock(&reconcile.Executor{}))

	if err := app.Run(context.Background(), []string{"bridge", "--help"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "serve") {
		t.Fatalf("expected bridge help to hide internal serve command, got %q", got)
	}
	if !strings.Contains(got, "doctor") {
		t.Fatalf("expected bridge doctor in help output, got %q", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunBridgeServeRequiresFlags(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, stubService{})

	err := app.Run(context.Background(), []string{"bridge", "serve"})
	if err == nil || !strings.Contains(err.Error(), "missing required flags") || !strings.Contains(err.Error(), "--state-dir") || !strings.Contains(err.Error(), "--container-id") {
		t.Fatalf("expected missing flag error, got %v", err)
	}
}

func TestRunUpLoadsWorkspaceConfigTomlDefaults(t *testing.T) {
	isolateConfigHome(t)
	var out bytes.Buffer
	var errOut bytes.Buffer
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".hatchctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hatchctl", "config.toml"), []byte("config = \"../.devcontainer/devcontainer.json\"\nfeature_timeout = \"45s\"\nlockfile_policy = \"update\"\nbridge = true\nssh = true\n[dotfiles]\nrepository = \"github.com/example/dotfiles\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got appcore.UpRequest
	app := NewWithService(&out, &errOut, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"up", "--workspace", workspace}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if got.Defaults.ConfigPath != filepath.Join(workspace, ".devcontainer", "devcontainer.json") {
		t.Fatalf("unexpected config path %q", got.Defaults.ConfigPath)
	}
	if got.Defaults.FeatureTimeout != 45*time.Second || got.Defaults.LockfilePolicy != string(devcontainer.FeatureLockfilePolicyUpdate) {
		t.Fatalf("unexpected config defaults %#v", got)
	}
	if got.Defaults.BridgeEnabled || got.Defaults.SSHAgent || got.Defaults.Dotfiles.Repository != "" {
		t.Fatalf("unexpected merged workspace config %#v", got)
	}
	if got.Defaults.Workspace != workspace {
		t.Fatalf("unexpected workspace %q", got.Defaults.Workspace)
	}
}

func TestRunUpAppliesTrustedWorkspaceConfigTomlHostDefaults(t *testing.T) {
	isolateConfigHome(t)
	var out bytes.Buffer
	var errOut bytes.Buffer
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".hatchctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hatchctl", "config.toml"), []byte("bridge = true\nssh = true\n[dotfiles]\nrepository = \"github.com/example/dotfiles\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got appcore.UpRequest
	app := NewWithService(&out, &errOut, stubService{up: func(_ context.Context, opts appcore.UpRequest) (appcore.UpResult, error) {
		got = opts
		return appcore.UpResult{}, nil
	}})

	if err := app.Run(context.Background(), []string{"up", "--workspace", workspace, "--trust-workspace"}); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if !got.Defaults.TrustWorkspace || !got.Defaults.BridgeEnabled || !got.Defaults.SSHAgent || got.Defaults.Dotfiles.Repository != "github.com/example/dotfiles" {
		t.Fatalf("unexpected trusted workspace config %#v", got)
	}
}

func TestRunRejectsInvalidLockfilePolicy(t *testing.T) {
	isolateConfigHome(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewWithService(&out, &errOut, appcore.NewWithExecutorWithoutMutationLock(&reconcile.Executor{}))

	err := app.Run(context.Background(), []string{"up", "--lockfile-policy", "bogus"})
	if err == nil || !strings.Contains(err.Error(), `invalid lockfile policy "bogus"`) || !strings.Contains(err.Error(), "expected auto, frozen, or update") {
		t.Fatalf("expected lockfile policy error, got %v", err)
	}
}
