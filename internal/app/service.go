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
	"github.com/lauritsk/hatchctl/internal/reconcile"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type Service struct {
	executor      *reconcile.Executor
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

type workspacePlanOptions struct {
	ReadOnly           bool
	BridgeEnabled      bool
	SSHAgent           bool
	Dotfiles           DotfilesOptions
	TrustWorkspace     bool
	AllowHostLifecycle bool
}

type (
	UpResult           = reconcile.UpResult
	BuildResult        = reconcile.BuildResult
	ReadConfigResult   = reconcile.ReadConfigResult
	RunLifecycleResult = reconcile.RunLifecycleResult
	DotfilesStatus     = reconcile.DotfilesStatus
	ExitError          = reconcile.ExitError
)

func New(executor *reconcile.Executor) *Service {
	return NewWithExecutor(executor)
}

func NewWithExecutor(executor *reconcile.Executor) *Service {
	return &Service{executor: executor, lockMutations: true}
}

func NewWithExecutorWithoutMutationLock(executor *reconcile.Executor) *Service {
	return &Service{executor: executor}
}

func NewDefault() *Service {
	engine := docker.NewClient("docker")
	return NewWithExecutor(reconcile.NewExecutor(engine))
}

func (s *Service) Up(ctx context.Context, req UpRequest) (UpResult, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{
		BridgeEnabled:      req.Defaults.BridgeEnabled,
		SSHAgent:           req.Defaults.SSHAgent,
		Dotfiles:           req.Defaults.Dotfiles,
		TrustWorkspace:     req.Defaults.TrustWorkspace,
		AllowHostLifecycle: req.AllowHostLifecycle,
	})
	if err != nil {
		return UpResult{}, err
	}
	return withMutationLock(s, ctx, "up", func() (workspaceplan.WorkspacePlan, error) {
		return buildPlan()
	}, func(workspacePlan workspaceplan.WorkspacePlan) (UpResult, error) {
		return s.executor.Up(ctx, workspacePlan, reconcile.UpOptions{Recreate: req.Recreate, Debug: req.Global.Debug, IO: commandStreams(req.IO)})
	})
}

func (s *Service) Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{
		TrustWorkspace: req.Defaults.TrustWorkspace,
	})
	if err != nil {
		return BuildResult{}, err
	}
	return withMutationLock(s, ctx, "build", func() (workspaceplan.WorkspacePlan, error) {
		return buildPlan()
	}, func(workspacePlan workspaceplan.WorkspacePlan) (BuildResult, error) {
		return s.executor.Build(ctx, workspacePlan, reconcile.BuildOptions{Debug: req.Global.Debug, IO: commandStreams(req.IO)})
	})
}

func (s *Service) Exec(ctx context.Context, req ExecRequest) (int, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{ReadOnly: true, SSHAgent: req.Defaults.SSHAgent})
	if err != nil {
		return 0, err
	}
	workspacePlan, err := buildPlan()
	if err != nil {
		return 0, err
	}
	if err := ensureNoActiveMutation(workspacePlan.LockProtected.StateDir); err != nil {
		return 0, err
	}
	return s.executor.Exec(ctx, workspacePlan, reconcile.ExecOptions{Args: req.Args, RemoteEnv: req.RemoteEnv, Debug: req.Global.Debug, IO: commandStreams(req.IO)})
}

func (s *Service) ReadConfig(ctx context.Context, req ReadConfigRequest) (ReadConfigResult, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{ReadOnly: true, SSHAgent: req.Defaults.SSHAgent, Dotfiles: req.Defaults.Dotfiles})
	if err != nil {
		return ReadConfigResult{}, err
	}
	workspacePlan, err := buildPlan()
	if err != nil {
		return ReadConfigResult{}, err
	}
	return s.executor.ReadConfig(ctx, workspacePlan, reconcile.ReadConfigOptions{Debug: req.Global.Debug, IO: commandStreams(req.IO)})
}

func (s *Service) RunLifecycle(ctx context.Context, req RunLifecycleRequest) (RunLifecycleResult, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{ReadOnly: true, Dotfiles: req.Defaults.Dotfiles, AllowHostLifecycle: req.AllowHostLifecycle})
	if err != nil {
		return RunLifecycleResult{}, err
	}
	phase, err := reconcile.NormalizeLifecyclePhase(req.Phase)
	if err != nil {
		return RunLifecycleResult{}, err
	}
	req.Phase = phase
	return withMutationLock(s, ctx, "run", func() (workspaceplan.WorkspacePlan, error) {
		return buildPlan()
	}, func(workspacePlan workspaceplan.WorkspacePlan) (RunLifecycleResult, error) {
		return s.executor.RunLifecycle(ctx, workspacePlan, reconcile.RunLifecycleOptions{Phase: req.Phase, Debug: req.Global.Debug, IO: commandStreams(req.IO)})
	})
}

func (s *Service) BridgeDoctor(ctx context.Context, req BridgeDoctorRequest) (bridge.Report, error) {
	buildPlan, err := s.workspacePlanBuilder(req.Defaults, workspacePlanOptions{ReadOnly: true})
	if err != nil {
		return bridge.Report{}, err
	}
	workspacePlan, err := buildPlan()
	if err != nil {
		return bridge.Report{}, err
	}
	return s.executor.BridgeDoctor(ctx, workspacePlan, reconcile.BridgeDoctorOptions{Debug: req.Global.Debug, IO: commandStreams(req.IO)})
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
		Dotfiles:           dotfiles,
		TrustWorkspace:     trustWorkspace,
		AllowHostLifecycle: allowHostLifecycle,
	})
}

func (s *Service) workspacePlanBuilder(defaults CommandDefaults, opts workspacePlanOptions) (func() (workspaceplan.WorkspacePlan, error), error) {
	s.executor.SetTrustedSigners(defaults.TrustedSigners)
	policy, err := devcontainer.ParseFeatureLockfilePolicy(defaults.LockfilePolicy)
	if err != nil {
		return nil, err
	}
	return func() (workspaceplan.WorkspacePlan, error) {
		return buildWorkspacePlan(defaults, policy, opts.ReadOnly, opts.BridgeEnabled, opts.SSHAgent, opts.Dotfiles, opts.TrustWorkspace, opts.AllowHostLifecycle)
	}, nil
}

func commandStreams(io CommandIO) reconcile.CommandStreams {
	return reconcile.CommandStreams{Stdin: io.Stdin, Stdout: io.Stdout, Stderr: io.Stderr, Events: io.Events}
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
