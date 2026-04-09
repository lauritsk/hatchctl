package app

import (
	"context"
	"io"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
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
	buildPlans    bool
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
	return &Service{runner: runner, buildPlans: true, lockMutations: true}
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
	return withMutationLock(s, ctx, "up", func() (workspaceplan.WorkspacePlan, error) {
		return s.maybeBuildWorkspacePlan(req.Defaults, policy, false, req.Defaults.BridgeEnabled, req.Defaults.SSHAgent, req.Defaults.Dotfiles, req.Defaults.TrustWorkspace, req.AllowHostLifecycle)
	}, func(workspacePlan workspaceplan.WorkspacePlan) (UpResult, error) {
		return s.runner.Up(ctx, runtime.UpOptions{
			Plan:               workspacePlan,
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
	return withMutationLock(s, ctx, "build", func() (workspaceplan.WorkspacePlan, error) {
		return s.maybeBuildWorkspacePlan(req.Defaults, policy, false, false, false, DotfilesOptions{}, req.Defaults.TrustWorkspace, false)
	}, func(workspacePlan workspaceplan.WorkspacePlan) (BuildResult, error) {
		return s.runner.Build(ctx, runtime.BuildOptions{
			Plan:           workspacePlan,
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
	workspacePlan, err := s.maybeBuildWorkspacePlan(req.Defaults, policy, true, false, req.Defaults.SSHAgent, DotfilesOptions{}, false, false)
	if err != nil {
		return 0, err
	}
	return s.runner.Exec(ctx, runtime.ExecOptions{
		Plan:           workspacePlan,
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
	workspacePlan, err := s.maybeBuildWorkspacePlan(req.Defaults, policy, true, false, req.Defaults.SSHAgent, req.Defaults.Dotfiles, false, false)
	if err != nil {
		return ReadConfigResult{}, err
	}
	return s.runner.ReadConfig(ctx, runtime.ReadConfigOptions{
		Plan:           workspacePlan,
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
	return withMutationLock(s, ctx, "run", func() (workspaceplan.WorkspacePlan, error) {
		return s.maybeBuildWorkspacePlan(req.Defaults, policy, true, false, false, req.Defaults.Dotfiles, false, req.AllowHostLifecycle)
	}, func(workspacePlan workspaceplan.WorkspacePlan) (RunLifecycleResult, error) {
		return s.runner.RunLifecycle(ctx, runtime.RunLifecycleOptions{
			Plan:               workspacePlan,
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
	workspacePlan, err := s.maybeBuildWorkspacePlan(req.Defaults, policy, true, false, false, DotfilesOptions{}, false, false)
	if err != nil {
		return bridge.Report{}, err
	}
	return s.runner.BridgeDoctor(ctx, runtime.BridgeDoctorOptions{
		Plan:           workspacePlan,
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

func buildWorkspacePlan(defaults CommandDefaults, lockfilePolicy devcontainer.FeatureLockfilePolicy, readOnly bool, bridgeEnabled bool, sshAgent bool, dotfiles DotfilesOptions, trustWorkspace bool, allowHostLifecycle bool) (workspaceplan.WorkspacePlan, error) {
	return workspaceplan.BuildWorkspacePlan(workspaceplan.BuildWorkspacePlanRequest{
		Workspace:          defaults.Workspace,
		ConfigPath:         defaults.ConfigPath,
		StateBaseDir:       defaults.StateDir,
		CacheBaseDir:       defaults.CacheDir,
		FeatureTimeout:     defaults.FeatureTimeout,
		LockfilePolicy:     lockfilePolicy,
		ReadOnly:           readOnly,
		BridgeEnabled:      bridgeEnabled,
		SSHAgent:           sshAgent,
		Dotfiles:           workspaceplan.DotfilesPreference{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath},
		TrustWorkspace:     trustWorkspace,
		AllowHostLifecycle: allowHostLifecycle,
	})
}

func (s *Service) maybeBuildWorkspacePlan(defaults CommandDefaults, lockfilePolicy devcontainer.FeatureLockfilePolicy, readOnly bool, bridgeEnabled bool, sshAgent bool, dotfiles DotfilesOptions, trustWorkspace bool, allowHostLifecycle bool) (workspaceplan.WorkspacePlan, error) {
	if !s.buildPlans {
		return workspaceplan.WorkspacePlan{}, nil
	}
	return buildWorkspacePlan(defaults, lockfilePolicy, readOnly, bridgeEnabled, sshAgent, dotfiles, trustWorkspace, allowHostLifecycle)
}

func withMutationLock[T any](s *Service, ctx context.Context, command string, buildPlan func() (workspaceplan.WorkspacePlan, error), run func(workspaceplan.WorkspacePlan) (T, error)) (T, error) {
	var zero T
	workspacePlan, err := buildPlan()
	if err != nil {
		return zero, err
	}
	if !s.lockMutations {
		return run(workspacePlan)
	}
	lock, err := storefs.AcquireWorkspaceLock(ctx, workspacePlan.LockProtected.StateDir, command)
	if err != nil {
		return zero, err
	}
	defer lock.Release()
	if workspacePlan.LockProtected.RequiresRevalidation {
		workspacePlan, err = buildPlan()
		if err != nil {
			return zero, err
		}
	}
	return run(workspacePlan)
}
