package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/runtime"
	"github.com/lauritsk/hatchctl/internal/version"
)

type App struct {
	out io.Writer
	err io.Writer

	runner runner
}

type runner interface {
	Up(context.Context, runtime.UpOptions) (runtime.UpResult, error)
	Build(context.Context, runtime.BuildOptions) (runtime.BuildResult, error)
	Exec(context.Context, runtime.ExecOptions) (int, error)
	ReadConfig(context.Context, runtime.ReadConfigOptions) (runtime.ReadConfigResult, error)
	RunLifecycle(context.Context, runtime.RunLifecycleOptions) (runtime.RunLifecycleResult, error)
	BridgeDoctor(context.Context, runtime.BridgeDoctorOptions) (bridge.Report, error)
}

func New(out io.Writer, err io.Writer) *App {
	engine := docker.NewClient("docker")
	return NewWithRunner(out, err, runtime.NewRunner(engine))
}

func NewWithRunner(out io.Writer, err io.Writer, runner runner) *App {
	return &App{out: out, err: err, runner: runner}
}

func (a *App) Run(ctx context.Context, args []string) error {
	root := a.newRootCommand()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func (a *App) newRootCommand() *cobra.Command {
	global := &globalOptions{}
	dotfiles := defaultDotfilesOptions()
	versionFlag := false
	cmd := &cobra.Command{
		Use:           "hatchctl",
		Short:         "Terminal-first Development Containers in Go.",
		Long:          "Terminal-first Development Containers in Go.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if versionFlag {
				_, err := fmt.Fprintln(a.out, version.Version)
				return err
			}
			return cmd.Help()
		},
		Example: strings.Join([]string{
			"hatchctl up",
			"hatchctl up --dotfiles lauritsk/dotfiles",
			"hatchctl --verbose up",
			"hatchctl build --debug",
			"hatchctl up --workspace ../my-project --bridge",
			"hatchctl exec -- go test ./...",
			"hatchctl build --json",
			"hatchctl config --json",
		}, "\n"),
	}
	cmd.SetOut(a.out)
	cmd.SetErr(a.err)
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().BoolVar(&global.Verbose, "verbose", false, "print progress while running")
	cmd.PersistentFlags().BoolVar(&global.Debug, "debug", false, "print detailed execution diagnostics")
	addDotfilesFlags(cmd, &dotfiles)
	cmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "print version information")
	cmd.AddCommand(
		a.newUpCommand(global, &dotfiles),
		a.newBuildCommand(global),
		a.newExecCommand(global),
		a.newConfigCommand(global, &dotfiles),
		a.newRunCommand(global, &dotfiles),
		a.newBridgeCommand(global),
		newVersionCommand(a.out),
	)
	return cmd
}

func (a *App) newUpCommand(global *globalOptions, dotfiles *dotfilesOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var recreate bool
	var bridgeEnabled bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build and start a dev container",
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			result, err := a.runner.Up(cmd.Context(), runtime.UpOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Dotfiles:       dotfiles.runtime(),
				Recreate:       recreate,
				BridgeEnabled:  bridgeEnabled,
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			return renderer.PrintKeyValues(upResultFields(result))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	cmd.Flags().BoolVar(&recreate, "recreate", false, "remove and recreate an existing managed container")
	cmd.Flags().BoolVar(&bridgeEnabled, "bridge", false, "enable macOS auth bridge scaffolding")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func (a *App) newBuildCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the dev container image only",
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			result, err := a.runner.Build(cmd.Context(), runtime.BuildOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			return renderer.PrintText(fmt.Sprintf("Built %s", result.Image))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func (a *App) newExecCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	var remoteEnv []string
	cmd := &cobra.Command{
		Use:                "exec -- COMMAND [ARG...]",
		Short:              "Run a command inside the dev container",
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("exec requires a command")
			}
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			stdout := a.out
			stderr := a.err
			var stdoutBuffer strings.Builder
			var stderrBuffer strings.Builder
			if jsonOut {
				stdout = &stdoutBuffer
				stderr = &stderrBuffer
			}
			code, err := a.runner.Exec(cmd.Context(), runtime.ExecOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
				Args:           args,
				RemoteEnv:      multiValueMap(remoteEnv),
				Stdin:          os.Stdin,
				Stdout:         stdout,
				Stderr:         stderr,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				if err := renderer.PrintJSON(map[string]any{
					"exitCode": code,
					"stdout":   stdoutBuffer.String(),
					"stderr":   stderrBuffer.String(),
					"command":  args,
				}); err != nil {
					return err
				}
			}
			if code != 0 {
				return runtime.ExitError{Code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	cmd.Flags().StringArrayVar(&remoteEnv, "env", nil, "extra remote environment variables in KEY=VALUE form")
	return cmd
}

func (a *App) newConfigCommand(global *globalOptions, dotfiles *dotfilesOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the resolved local configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			result, err := a.runner.ReadConfig(cmd.Context(), runtime.ReadConfigOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Dotfiles:       dotfiles.runtime(),
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			return renderer.PrintKeyValues(configResultFields(result))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "frozen", "lockfile policy: auto, frozen, or update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func (a *App) newRunCommand(global *globalOptions, dotfiles *dotfilesOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var phase string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run lifecycle commands against an existing container",
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			result, err := a.runner.RunLifecycle(cmd.Context(), runtime.RunLifecycleOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Dotfiles:       dotfiles.runtime(),
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
				Phase:          phase,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			return renderer.PrintText(fmt.Sprintf("Ran lifecycle commands for %s (%s).", result.ContainerID, result.Phase))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	cmd.Flags().StringVar(&phase, "phase", "all", "one of: all, create, start, attach")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func (a *App) newBridgeCommand(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Bridge status and diagnostics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.newBridgeDoctorCommand(global), a.newBridgeServeCommand(), a.newBridgeHelperCommand())
	return cmd
}

func (a *App) newBridgeHelperCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "helper",
		Short:  "Bridge helper commands",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newBridgeHelperPassthroughCommand("connect"),
		a.newBridgeHelperPassthroughCommand("open"),
		a.newBridgeHelperPassthroughCommand("serve"),
	)
	return cmd
}

func (a *App) newBridgeHelperPassthroughCommand(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              "Internal bridge helper command",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return bridge.HelperMain(append([]string{name}, args...))
		},
	}
}

func (a *App) newBridgeDoctorCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect bridge status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			policy, err := parseLockfilePolicy(lockfilePolicy)
			if err != nil {
				return err
			}
			report, err := a.runner.BridgeDoctor(cmd.Context(), runtime.BridgeDoctorOptions{
				Workspace:      workspace,
				ConfigPath:     configPath,
				FeatureTimeout: featureTimeout,
				LockfilePolicy: policy,
				Verbose:        global.Verbose || global.Debug,
				Debug:          global.Debug,
				Events:         renderer.Events(),
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(report)
			}
			return renderer.PrintKeyValues([]ui.KeyValue{
				{Key: "Bridge session", Value: report.ID},
				{Key: "Enabled", Value: fmt.Sprintf("%t", report.Enabled)},
				{Key: "State path", Value: report.StatePath},
				{Key: "Helper path", Value: report.HelperPath},
				{Key: "Status", Value: report.Status},
			})
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace folder (defaults to current directory)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to devcontainer.json")
	cmd.Flags().DurationVar(&featureTimeout, "feature-timeout", 90*time.Second, "timeout for remote feature HTTP requests")
	cmd.Flags().StringVar(&lockfilePolicy, "lockfile-policy", "frozen", "lockfile policy: auto, frozen, or update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func (a *App) newBridgeServeCommand() *cobra.Command {
	var stateDir string
	var containerID string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve bridge callbacks for a managed container",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if stateDir == "" || containerID == "" {
				return errors.New("bridge serve requires --state-dir and --container-id")
			}
			return bridge.Serve(cmd.Context(), stateDir, containerID)
		},
	}
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "workspace state directory")
	cmd.Flags().StringVar(&containerID, "container-id", "", "managed container id")
	return cmd
}

func newVersionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(out, version.Version)
			return err
		},
	}
}

type globalOptions struct {
	Verbose bool
	Debug   bool
}

type dotfilesOptions struct {
	Repository     string
	InstallCommand string
	TargetPath     string
}

func defaultDotfilesOptions() dotfilesOptions {
	return dotfilesOptions{
		Repository:     os.Getenv("HATCHCTL_DOTFILES_REPOSITORY"),
		InstallCommand: os.Getenv("HATCHCTL_DOTFILES_INSTALL_COMMAND"),
		TargetPath:     os.Getenv("HATCHCTL_DOTFILES_TARGET_PATH"),
	}
}

func addDotfilesFlags(cmd *cobra.Command, opts *dotfilesOptions) {
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles", opts.Repository, "dotfiles repo (owner/repo, github.com/owner/repo, or git URL); env HATCHCTL_DOTFILES_REPOSITORY")
	cmd.PersistentFlags().StringVar(&opts.Repository, "dotfiles-repository", opts.Repository, "explicit dotfiles repo flag; same as --dotfiles")
	cmd.PersistentFlags().StringVar(&opts.InstallCommand, "dotfiles-install-command", opts.InstallCommand, "dotfiles install script or command; env HATCHCTL_DOTFILES_INSTALL_COMMAND")
	cmd.PersistentFlags().StringVar(&opts.TargetPath, "dotfiles-target-path", opts.TargetPath, "dotfiles checkout path inside the container; env HATCHCTL_DOTFILES_TARGET_PATH")
}

func (o dotfilesOptions) runtime() runtime.DotfilesOptions {
	return runtime.DotfilesOptions{Repository: o.Repository, InstallCommand: o.InstallCommand, TargetPath: o.TargetPath}
}

func (a *App) newRenderer(jsonOut bool) *ui.Renderer {
	return ui.NewRenderer(a.out, a.err, jsonOut)
}

func parseLockfilePolicy(value string) (devcontainer.FeatureLockfilePolicy, error) {
	return devcontainer.ParseFeatureLockfilePolicy(value)
}

func multiValueMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 1 {
			result[parts[0]] = ""
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result
}

func upResultFields(result runtime.UpResult) []ui.KeyValue {
	fields := []ui.KeyValue{
		{Key: "Container", Value: result.ContainerID},
		{Key: "Image", Value: result.Image},
		{Key: "Workspace", Value: result.RemoteWorkspaceFolder},
		{Key: "State", Value: result.StateDir},
	}
	if result.Bridge != nil {
		fields = append(fields, ui.KeyValue{Key: "Bridge", Value: fmt.Sprintf("enabled (%s)", result.Bridge.Status)})
	}
	return fields
}

func configResultFields(result runtime.ReadConfigResult) []ui.KeyValue {
	fields := []ui.KeyValue{
		{Key: "Config", Value: result.ConfigPath},
		{Key: "Workspace", Value: result.WorkspaceFolder},
		{Key: "Workspace mount", Value: result.WorkspaceMount},
		{Key: "Source", Value: result.SourceKind},
		{Key: "Lifecycle", Value: fmt.Sprintf("initialize=%t create=%t start=%t attach=%t", result.HasInitializeCommand, result.HasCreateCommand, result.HasStartCommand, result.HasAttachCommand)},
	}
	if result.ImageUser != "" {
		fields = append(fields, ui.KeyValue{Key: "Image user", Value: result.ImageUser})
	}
	if len(result.ForwardPorts) > 0 {
		fields = append(fields, ui.KeyValue{Key: "Forward ports", Value: strings.Join(result.ForwardPorts, ", ")})
	}
	if result.Bridge != nil {
		fields = append(fields, ui.KeyValue{Key: "Bridge", Value: fmt.Sprintf("enabled=%t mount=%s helper=%s status=%s", result.Bridge.Enabled, result.Bridge.BinPath, result.Bridge.HelperPath, result.Bridge.Status)})
	}
	if result.Dotfiles != nil {
		fields = append(fields, ui.KeyValue{Key: "Dotfiles", Value: fmt.Sprintf("configured=%t applied=%t pending=%t repo=%s target=%s", result.Dotfiles.Configured, result.Dotfiles.Applied, result.Dotfiles.NeedsInstall, result.Dotfiles.Repository, result.Dotfiles.TargetPath)})
	}
	if result.ManagedContainer != nil {
		fields = append(fields, ui.KeyValue{Key: "Managed container", Value: fmt.Sprintf("id=%s status=%s running=%t user=%s metadata=%d", result.ManagedContainer.ID, result.ManagedContainer.Status, result.ManagedContainer.Running, result.ManagedContainer.RemoteUser, result.ManagedContainer.MetadataCount)})
	}
	return fields
}
