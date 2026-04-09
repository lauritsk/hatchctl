package app

import (
	"context"
	"io"
	"os"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/runtime"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type Service struct {
	executor      *runtime.Runner
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
	DotfilesStatus     = runtime.DotfilesStatus
	ExitError          = runtime.ExitError
)

func New(executor *runtime.Runner) *Service {
	return NewWithExecutor(executor)
}

func NewWithExecutor(executor *runtime.Runner) *Service {
	return &Service{executor: executor, buildPlans: true, lockMutations: true}
}

func NewWithExecutorWithoutMutationLock(executor *runtime.Runner) *Service {
	return &Service{executor: executor}
}

func NewDefault() *Service {
	engine := docker.NewClient("docker")
	return NewWithExecutor(runtime.NewRunner(engine))
}

func (s *Service) Up(ctx context.Context, req UpRequest) (UpResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return UpResult{}, err
	}
	return withMutationLock(s, ctx, "up", func() (workspaceplan.WorkspacePlan, error) {
		return s.maybeBuildWorkspacePlan(req.Defaults, policy, false, req.Defaults.BridgeEnabled, req.Defaults.SSHAgent, req.Defaults.Dotfiles, req.Defaults.TrustWorkspace, req.AllowHostLifecycle)
	}, func(workspacePlan workspaceplan.WorkspacePlan) (UpResult, error) {
		return s.upWithExecutor(ctx, req, workspacePlan)
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
		return s.buildWithExecutor(ctx, req, workspacePlan)
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
	if err := ensureNoActiveMutation(workspacePlan.LockProtected.StateDir); err != nil {
		return 0, err
	}
	return s.execWithExecutor(ctx, req, workspacePlan)
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
	return s.readConfigWithExecutor(ctx, req, workspacePlan)
}

func (s *Service) RunLifecycle(ctx context.Context, req RunLifecycleRequest) (RunLifecycleResult, error) {
	policy, err := devcontainer.ParseFeatureLockfilePolicy(req.Defaults.LockfilePolicy)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	return withMutationLock(s, ctx, "run", func() (workspaceplan.WorkspacePlan, error) {
		return s.maybeBuildWorkspacePlan(req.Defaults, policy, true, false, false, req.Defaults.Dotfiles, false, req.AllowHostLifecycle)
	}, func(workspacePlan workspaceplan.WorkspacePlan) (RunLifecycleResult, error) {
		return s.runLifecycleWithExecutor(ctx, req, workspacePlan)
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
	return s.bridgeDoctorWithExecutor(ctx, req, workspacePlan)
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

func ensureNoActiveMutation(stateDir string) error {
	coordination, err := storefs.ReadCoordination(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if coordination.ActiveOwner != nil {
		return &storefs.WorkspaceBusyError{StateDir: stateDir, Owner: coordination.ActiveOwner}
	}
	return nil
}
