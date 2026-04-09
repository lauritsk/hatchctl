package app

import (
	"context"
	"io"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/runtime"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type Runtime interface {
	Up(context.Context, runtime.UpOptions) (runtime.UpResult, error)
	Build(context.Context, runtime.BuildOptions) (runtime.BuildResult, error)
	Exec(context.Context, runtime.ExecOptions) (int, error)
	ReadConfig(context.Context, runtime.ReadConfigOptions) (runtime.ReadConfigResult, error)
	RunLifecycle(context.Context, runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error)
	BridgeDoctor(context.Context, runtime.BridgeDoctorOptions) (bridge.Report, error)
}

type Service struct {
	runner        Runtime
	lockMutations bool
}

type GlobalOptions struct {
	Verbose bool
	Debug   bool
}

type CommandIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Events ui.Sink
}

type UpRequest struct {
	Defaults           CommandDefaults
	AllowHostLifecycle bool
	Recreate           bool
	Global             GlobalOptions
	IO                 CommandIO
}

type BuildRequest struct {
	Defaults CommandDefaults
	Global   GlobalOptions
	IO       CommandIO
}

type ExecRequest struct {
	Defaults  CommandDefaults
	Args      []string
	RemoteEnv map[string]string
	Global    GlobalOptions
	IO        CommandIO
}

type ReadConfigRequest struct {
	Defaults CommandDefaults
	Global   GlobalOptions
	IO       CommandIO
}

type RunLifecycleRequest struct {
	Defaults           CommandDefaults
	AllowHostLifecycle bool
	Phase              string
	Global             GlobalOptions
	IO                 CommandIO
}

type BridgeDoctorRequest struct {
	Defaults CommandDefaults
	Global   GlobalOptions
	IO       CommandIO
}

type (
	UpResult           = runtime.UpResult
	BuildResult        = runtime.BuildResult
	ReadConfigResult   = runtime.ReadConfigResult
	RunLifecycleResult = runtime.RunLifecycleResult
	ExitError          = runtime.ExitError
)

func New(runner Runtime) *Service {
	return &Service{runner: runner, lockMutations: true}
}

func NewWithoutMutationLock(runner Runtime) *Service {
	return &Service{runner: runner}
}

func NewDefault() *Service {
	engine := docker.NewClient("docker")
	return New(runtime.NewRunner(engine))
}

func (s *Service) Up(ctx context.Context, req UpRequest) (UpResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return UpResult{}, err
	}
	return maybeWithMutationLock(s, ctx, req.Defaults, "up", func() (UpResult, error) {
		return s.runner.Up(ctx, runtime.UpOptions{
			Workspace:          req.Defaults.Workspace,
			ConfigPath:         req.Defaults.ConfigPath,
			StateDir:           req.Defaults.StateDir,
			CacheDir:           req.Defaults.CacheDir,
			FeatureTimeout:     req.Defaults.FeatureTimeout,
			LockfilePolicy:     policy,
			Dotfiles:           req.Defaults.Dotfiles.runtime(),
			AllowHostLifecycle: req.AllowHostLifecycle,
			TrustWorkspace:     req.Defaults.TrustWorkspace,
			SSHAgent:           req.Defaults.SSHAgent,
			Recreate:           req.Recreate,
			BridgeEnabled:      req.Defaults.BridgeEnabled,
			Verbose:            req.Global.Verbose || req.Global.Debug,
			Debug:              req.Global.Debug,
			Events:             req.IO.Events,
			Stdout:             req.IO.Stdout,
			Stderr:             req.IO.Stderr,
		})
	})
}

func (s *Service) Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return BuildResult{}, err
	}
	return maybeWithMutationLock(s, ctx, req.Defaults, "build", func() (BuildResult, error) {
		return s.runner.Build(ctx, runtime.BuildOptions{
			Workspace:      req.Defaults.Workspace,
			ConfigPath:     req.Defaults.ConfigPath,
			StateDir:       req.Defaults.StateDir,
			CacheDir:       req.Defaults.CacheDir,
			FeatureTimeout: req.Defaults.FeatureTimeout,
			LockfilePolicy: policy,
			TrustWorkspace: req.Defaults.TrustWorkspace,
			Verbose:        req.Global.Verbose || req.Global.Debug,
			Debug:          req.Global.Debug,
			Events:         req.IO.Events,
			Stdout:         req.IO.Stdout,
			Stderr:         req.IO.Stderr,
		})
	})
}

func (s *Service) Exec(ctx context.Context, req ExecRequest) (int, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return 0, err
	}
	return s.runner.Exec(ctx, runtime.ExecOptions{
		Workspace:      req.Defaults.Workspace,
		ConfigPath:     req.Defaults.ConfigPath,
		StateDir:       req.Defaults.StateDir,
		CacheDir:       req.Defaults.CacheDir,
		FeatureTimeout: req.Defaults.FeatureTimeout,
		LockfilePolicy: policy,
		SSHAgent:       req.Defaults.SSHAgent,
		Verbose:        req.Global.Verbose || req.Global.Debug,
		Debug:          req.Global.Debug,
		Events:         req.IO.Events,
		Args:           req.Args,
		RemoteEnv:      req.RemoteEnv,
		Stdin:          req.IO.Stdin,
		Stdout:         req.IO.Stdout,
		Stderr:         req.IO.Stderr,
	})
}

func (s *Service) ReadConfig(ctx context.Context, req ReadConfigRequest) (ReadConfigResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return ReadConfigResult{}, err
	}
	return s.runner.ReadConfig(ctx, runtime.ReadConfigOptions{
		Workspace:      req.Defaults.Workspace,
		ConfigPath:     req.Defaults.ConfigPath,
		StateDir:       req.Defaults.StateDir,
		CacheDir:       req.Defaults.CacheDir,
		FeatureTimeout: req.Defaults.FeatureTimeout,
		LockfilePolicy: policy,
		SSHAgent:       req.Defaults.SSHAgent,
		Dotfiles:       req.Defaults.Dotfiles.runtime(),
		Verbose:        req.Global.Verbose || req.Global.Debug,
		Debug:          req.Global.Debug,
		Events:         req.IO.Events,
		Stdout:         req.IO.Stdout,
		Stderr:         req.IO.Stderr,
	})
}

func (s *Service) RunLifecycle(ctx context.Context, req RunLifecycleRequest) (RunLifecycleResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	return maybeWithMutationLock(s, ctx, req.Defaults, "run", func() (RunLifecycleResult, error) {
		return s.runner.RunLifecycle(ctx, runtime.RunLifecycleOptions{
			Workspace:          req.Defaults.Workspace,
			ConfigPath:         req.Defaults.ConfigPath,
			StateDir:           req.Defaults.StateDir,
			CacheDir:           req.Defaults.CacheDir,
			FeatureTimeout:     req.Defaults.FeatureTimeout,
			LockfilePolicy:     policy,
			Dotfiles:           req.Defaults.Dotfiles.runtime(),
			AllowHostLifecycle: req.AllowHostLifecycle,
			Verbose:            req.Global.Verbose || req.Global.Debug,
			Debug:              req.Global.Debug,
			Events:             req.IO.Events,
			Phase:              req.Phase,
			Stdout:             req.IO.Stdout,
			Stderr:             req.IO.Stderr,
		})
	})
}

func (s *Service) BridgeDoctor(ctx context.Context, req BridgeDoctorRequest) (bridge.Report, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return bridge.Report{}, err
	}
	return s.runner.BridgeDoctor(ctx, runtime.BridgeDoctorOptions{
		Workspace:      req.Defaults.Workspace,
		ConfigPath:     req.Defaults.ConfigPath,
		StateDir:       req.Defaults.StateDir,
		CacheDir:       req.Defaults.CacheDir,
		FeatureTimeout: req.Defaults.FeatureTimeout,
		LockfilePolicy: policy,
		Verbose:        req.Global.Verbose || req.Global.Debug,
		Debug:          req.Global.Debug,
		Events:         req.IO.Events,
		Stdout:         req.IO.Stdout,
		Stderr:         req.IO.Stderr,
	})
}

func (o DotfilesOptions) runtime() runtime.DotfilesOptions {
	return runtime.DotfilesOptions{Repository: o.Repository, InstallCommand: o.InstallCommand, TargetPath: o.TargetPath}
}

func mutationStateDir(defaults CommandDefaults) (string, error) {
	workspace, err := devcontainer.ResolveWorkspacePath(defaults.Workspace)
	if err != nil {
		return "", err
	}
	configPath, err := devcontainer.ResolveConfigPath(workspace, defaults.ConfigPath)
	if err != nil {
		return "", err
	}
	if defaults.StateDir != "" {
		return storefs.WorkspaceScopedDir(defaults.StateDir, workspace, configPath), nil
	}
	return storefs.WorkspaceStateDir(workspace, configPath)
}

func withMutationLock[T any](ctx context.Context, defaults CommandDefaults, command string, run func() (T, error)) (T, error) {
	var zero T
	stateDir, err := mutationStateDir(defaults)
	if err != nil {
		return zero, err
	}
	lock, err := storefs.AcquireWorkspaceLock(ctx, stateDir, command)
	if err != nil {
		return zero, err
	}
	defer lock.Release()
	return run()
}

func maybeWithMutationLock[T any](s *Service, ctx context.Context, defaults CommandDefaults, command string, run func() (T, error)) (T, error) {
	if !s.lockMutations {
		return run()
	}
	return withMutationLock(ctx, defaults, command, run)
}
