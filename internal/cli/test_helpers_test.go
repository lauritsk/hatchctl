package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/bridge"
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

func stubUp(run func(context.Context, appcore.UpRequest) (appcore.UpResult, error)) stubService {
	return stubService{up: run}
}

func stubBuild(run func(context.Context, appcore.BuildRequest) (appcore.BuildResult, error)) stubService {
	return stubService{build: run}
}

func stubExec(run func(context.Context, appcore.ExecRequest) (int, error)) stubService {
	return stubService{exec: run}
}

func stubReadConfig(run func(context.Context, appcore.ReadConfigRequest) (appcore.ReadConfigResult, error)) stubService {
	return stubService{readConfig: run}
}

func stubRunLifecycle(run func(context.Context, appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error)) stubService {
	return stubService{runLifecycle: run}
}

func stubBridgeDoctor(run func(context.Context, appcore.BridgeDoctorRequest) (bridge.Report, error)) stubService {
	return stubService{bridgeDoctor: run}
}

type appHarness struct {
	t      testing.TB
	app    *App
	out    bytes.Buffer
	errOut bytes.Buffer
}

func newAppHarness(t testing.TB, service stubService) *appHarness {
	t.Helper()
	isolateConfigHome(t)
	h := &appHarness{t: t}
	h.app = NewWithService(&h.out, &h.errOut, service)
	return h
}

func newBufferedApp(t testing.TB, service service) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	isolateConfigHome(t)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return NewWithService(out, errOut, service), out, errOut
}

func (h *appHarness) run(args ...string) error {
	h.t.Helper()
	return h.app.Run(context.Background(), args)
}

func (h *appHarness) stdout() string {
	h.t.Helper()
	return h.out.String()
}

func (h *appHarness) stderr() string {
	h.t.Helper()
	return h.errOut.String()
}

func isolateConfigHome(t testing.TB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func assertContainsAll(t testing.TB, got string, want ...string) {
	t.Helper()
	for _, part := range want {
		if !strings.Contains(got, part) {
			t.Fatalf("expected output to contain %q, got %q", part, got)
		}
	}
}
