package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/version"
)

type App struct {
	out io.Writer
	err io.Writer

	service service
}

type service interface {
	Up(context.Context, appcore.UpRequest) (appcore.UpResult, error)
	Build(context.Context, appcore.BuildRequest) (appcore.BuildResult, error)
	Exec(context.Context, appcore.ExecRequest) (int, error)
	ReadConfig(context.Context, appcore.ReadConfigRequest) (appcore.ReadConfigResult, error)
	RunLifecycle(context.Context, appcore.RunLifecycleRequest) (appcore.RunLifecycleResult, error)
	BridgeDoctor(context.Context, appcore.BridgeDoctorRequest) (bridge.Report, error)
}

func New(out io.Writer, err io.Writer) *App {
	return &App{out: out, err: err, service: appcore.NewDefault()}
}

func NewWithService(out io.Writer, err io.Writer, service service) *App {
	return &App{out: out, err: err, service: service}
}

func (a *App) Run(ctx context.Context, args []string) error {
	root := a.newRootCommand()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func (a *App) newRootCommand() *cobra.Command {
	global := &globalOptions{}
	versionFlag := false
	cmd := &cobra.Command{
		Use:           "hatchctl",
		Short:         "Run devcontainers from the terminal",
		Long:          "Create, inspect, and use devcontainer-based workspaces directly from the terminal.",
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
	cmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "print version information")
	cmd.AddCommand(
		a.newUpCommand(global),
		a.newBuildCommand(global),
		a.newExecCommand(global),
		a.newConfigCommand(global),
		a.newRunCommand(global),
		a.newBridgeCommand(global),
		newVersionCommand(a.out),
	)
	return cmd
}

func (a *App) newUpCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var recreate bool
	var bridgeEnabled bool
	var sshAgent bool
	trustWorkspace := appcore.EnvTruthy(appcore.TrustWorkspaceEnvVar)
	allowHostLifecycle := appcore.EnvTruthy(appcore.AllowHostLifecycleEnvVar)
	var jsonOut bool
	dotfiles := appcore.DefaultDotfilesOptions()
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Create or reuse a devcontainer for this workspace",
		Long: strings.Join([]string{
			"Create or reuse the managed devcontainer for a workspace.",
			"",
			"Use this as the normal entry point when you want hatchctl to resolve the config, build the image if needed, and start or reconnect to the container.",
			"If the workspace asks for host-side lifecycle commands or elevated Docker settings, hatchctl stops and tells you which trust flag to add.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl up",
			"hatchctl up --workspace ../my-project",
			"hatchctl up --dotfiles lauritsk/dotfiles",
			"hatchctl up --trust-workspace --allow-host-lifecycle",
			"hatchctl up --json",
		}, "\n"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, &bridgeEnabled, &trustWorkspace, &sshAgent, dotfiles))
			if err != nil {
				return err
			}
			result, err := a.service.Up(cmd.Context(), appcore.UpRequest{
				Defaults:           defaults,
				AllowHostLifecycle: allowHostLifecycle,
				Recreate:           recreate,
				Global:             appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Events: renderer.Events(),
					Stdout: renderer.Stdout(),
					Stderr: renderer.Stderr(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			if renderer.TTY() {
				if err := renderer.PrintSummary("Devcontainer Ready", upResultFields(result)); err != nil {
					return err
				}
				return renderer.PrintCommandList("Next", upSuggestedCommands(defaults.Workspace, defaults.ConfigPath, defaults.FeatureTimeout, defaults.LockfilePolicy, defaults.SSHAgent))
			}
			if err := renderer.PrintKeyValues(upResultFields(result)); err != nil {
				return err
			}
			return renderer.PrintText("\nNext:\n  " + strings.Join(upSuggestedCommands(defaults.Workspace, defaults.ConfigPath, defaults.FeatureTimeout, defaults.LockfilePolicy, defaults.SSHAgent), "\n  "))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().BoolVar(&recreate, "recreate", false, "remove and recreate an existing managed container")
	cmd.Flags().BoolVar(&bridgeEnabled, "bridge", false, "enable macOS browser-open and localhost callback forwarding")
	cmd.Flags().BoolVar(&sshAgent, "ssh", false, "mount the host ssh-agent socket into the container")
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled Docker mounts, privilege, and build settings")
	cmd.Flags().BoolVar(&allowHostLifecycle, "allow-host-lifecycle", allowHostLifecycle, "trust and run host-side lifecycle commands such as initializeCommand")
	addJSONFlag(cmd, &jsonOut)
	addDotfilesFlags(cmd, &dotfiles)
	return cmd
}

func (a *App) newBuildCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	trustWorkspace := appcore.EnvTruthy(appcore.TrustWorkspaceEnvVar)
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the devcontainer image without starting it",
		Long: strings.Join([]string{
			"Build the resolved devcontainer image without starting a container.",
			"",
			"Use this when you want to validate image changes, warm the cache in CI, or inspect build failures separately from container startup.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl build",
			"hatchctl build --workspace ../my-project",
			"hatchctl build --lockfile-policy update",
			"hatchctl build --json",
		}, "\n"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, nil, &trustWorkspace, nil, appcore.DotfilesOptions{}))
			if err != nil {
				return err
			}
			result, err := a.service.Build(cmd.Context(), appcore.BuildRequest{
				Defaults: defaults,
				Global:   appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Events: renderer.Events(),
					Stdout: renderer.Stdout(),
					Stderr: renderer.Stderr(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			if renderer.TTY() {
				return renderer.PrintSummary("Image Ready", []ui.KeyValue{{Key: "Image", Value: result.Image}})
			}
			return renderer.PrintText(fmt.Sprintf("Devcontainer image ready: %s", result.Image))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled Docker mounts, privilege, and build settings")
	addJSONFlag(cmd, &jsonOut)
	return cmd
}

func (a *App) newExecCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	var remoteEnv []string
	var sshAgent bool
	cmd := &cobra.Command{
		Use:   "exec [-- COMMAND [ARG...]]",
		Short: "Open a shell or run a command inside the devcontainer",
		Long: strings.Join([]string{
			"Open the remote user's default shell in the managed devcontainer, or run a command with `--`.",
			"",
			"Examples:",
			"  hatchctl exec",
			"  hatchctl exec -- pwd",
			"  hatchctl exec -- go test ./...",
			"  hatchctl exec --env CI=1 -- sh -lc 'make test'",
			"",
			"Use `--` to separate hatchctl flags from the command you want to run in the container.",
			"`--json` requires an explicit command so hatchctl can return the exit code and captured output.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl exec",
			"hatchctl exec -- pwd",
			"hatchctl exec -- go test ./...",
			"hatchctl exec --env CI=1 -- sh -lc 'make test'",
			"hatchctl exec --json -- sh -lc 'go test ./...'",
		}, "\n"),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut && len(args) == 0 {
				return errors.New("missing command for exec --json; use 'hatchctl exec --json -- <command>'")
			}
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, nil, nil, &sshAgent, appcore.DotfilesOptions{}))
			if err != nil {
				return err
			}
			stdout, stderr := execWriters(renderer, false)
			var stdoutBuffer strings.Builder
			var stderrBuffer strings.Builder
			if jsonOut {
				stdout = &stdoutBuffer
				stderr = &stderrBuffer
			} else if shouldUseRawExecStreams(os.Stdin, os.Stdout) {
				stdout, stderr = execWriters(renderer, true)
			}
			code, err := a.service.Exec(cmd.Context(), appcore.ExecRequest{
				Defaults:  defaults,
				Args:      args,
				RemoteEnv: multiValueMap(remoteEnv),
				Global:    appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Stdin:  os.Stdin,
					Stdout: stdout,
					Stderr: stderr,
					Events: renderer.Events(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				if err := renderer.PrintJSON(map[string]any{
					"exitCode": code,
					"stdout":   stdoutBuffer.String(),
					"stderr":   stderrBuffer.String(),
					"args":     args,
				}); err != nil {
					return err
				}
			}
			if code != 0 {
				return appcore.ExitError{Code: code}
			}
			return nil
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().BoolVar(&sshAgent, "ssh", false, "require host ssh-agent passthrough for the managed container")
	addJSONFlag(cmd, &jsonOut)
	cmd.Flags().StringArrayVar(&remoteEnv, "env", nil, "set container environment variables as KEY=VALUE; repeat as needed")
	return cmd
}

func shouldUseRawExecStreams(stdin *os.File, stdout *os.File) bool {
	return isTerminalFile(stdin) && isTerminalFile(stdout)
}

func execWriters(renderer *ui.Renderer, interactive bool) (io.Writer, io.Writer) {
	if interactive {
		return os.Stdout, os.Stderr
	}
	return renderer.Stdout(), renderer.Stderr()
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (a *App) newConfigCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	var sshAgent bool
	dotfiles := appcore.DefaultDotfilesOptions()
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the merged config and detected runtime state",
		Long: strings.Join([]string{
			"Inspect the resolved devcontainer config and the runtime state hatchctl detected for this workspace.",
			"",
			"This command is for troubleshooting and scripting. It defaults to `--lockfile-policy frozen` so inspection does not update feature resolution as a side effect.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl config",
			"hatchctl config --workspace ../my-project",
			"hatchctl config --json",
		}, "\n"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, nil, nil, &sshAgent, dotfiles))
			if err != nil {
				return err
			}
			result, err := a.service.ReadConfig(cmd.Context(), appcore.ReadConfigRequest{
				Defaults: defaults,
				Global:   appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Events: renderer.Events(),
					Stdout: renderer.Stdout(),
					Stderr: renderer.Stderr(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			if renderer.TTY() {
				return renderer.PrintSummary("Configuration", configResultFields(result))
			}
			return renderer.PrintKeyValues(configResultFields(result))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "frozen")
	cmd.Flags().BoolVar(&sshAgent, "ssh", false, "show config with host ssh-agent passthrough applied")
	addJSONFlag(cmd, &jsonOut)
	addDotfilesFlags(cmd, &dotfiles)
	return cmd
}

func (a *App) newRunCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var phase string
	allowHostLifecycle := appcore.EnvTruthy(appcore.AllowHostLifecycleEnvVar)
	var jsonOut bool
	dotfiles := appcore.DefaultDotfilesOptions()
	cmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"lifecycle"},
		Short:   "Re-run lifecycle steps in an existing container",
		Long: strings.Join([]string{
			"Re-run devcontainer lifecycle phases in an existing managed container.",
			"",
			"Use this when you need to repeat create, start, or attach hooks. For opening a shell or running ad hoc commands, use `hatchctl exec` instead.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl run",
			"hatchctl lifecycle --phase attach",
			"hatchctl run --phase start --allow-host-lifecycle",
			"hatchctl run --json --phase create",
		}, "\n"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, nil, nil, nil, dotfiles))
			if err != nil {
				return err
			}
			result, err := a.service.RunLifecycle(cmd.Context(), appcore.RunLifecycleRequest{
				Defaults:           defaults,
				AllowHostLifecycle: allowHostLifecycle,
				Phase:              phase,
				Global:             appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Events: renderer.Events(),
					Stdout: renderer.Stdout(),
					Stderr: renderer.Stderr(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(result)
			}
			return renderer.PrintText(fmt.Sprintf("Lifecycle phase %q completed for container %s.", result.Phase, result.ContainerID))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().StringVar(&phase, "phase", "all", "lifecycle phase to run: all, create, start, or attach")
	cmd.Flags().BoolVar(&allowHostLifecycle, "allow-host-lifecycle", allowHostLifecycle, "trust and run host-side lifecycle commands such as initializeCommand")
	addJSONFlag(cmd, &jsonOut)
	addDotfilesFlags(cmd, &dotfiles)
	return cmd
}

func (a *App) newBridgeCommand(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "macOS bridge commands and diagnostics",
		Long:  "Inspect and manage the macOS bridge used for browser-open and localhost callback forwarding.",
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
		Short: "Check macOS bridge availability and status",
		Long: strings.Join([]string{
			"Inspect bridge availability, helper paths, and the current session state for this workspace.",
			"",
			"This command defaults to `--lockfile-policy frozen` so diagnostics do not update feature resolution as a side effect.",
		}, "\n"),
		Example: strings.Join([]string{
			"hatchctl bridge doctor",
			"hatchctl bridge doctor --workspace ../my-project",
			"hatchctl bridge doctor --json",
		}, "\n"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			renderer := a.newRenderer(jsonOut)
			defer renderer.Close()
			defaults, err := appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, nil, nil, nil, appcore.DotfilesOptions{}))
			if err != nil {
				return err
			}
			report, err := a.service.BridgeDoctor(cmd.Context(), appcore.BridgeDoctorRequest{
				Defaults: defaults,
				Global:   appcore.GlobalOptions{Verbose: global.Verbose, Debug: global.Debug},
				IO: appcore.CommandIO{
					Events: renderer.Events(),
					Stdout: renderer.Stdout(),
					Stderr: renderer.Stderr(),
				},
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return renderer.PrintJSON(report)
			}
			return renderer.PrintKeyValues([]ui.KeyValue{
				{Key: "Bridge session", Value: report.ID},
				{Key: "Bridge enabled", Value: fmt.Sprintf("%t", report.Enabled)},
				{Key: "Current status", Value: report.Status},
				{Key: "State path", Value: report.StatePath},
				{Key: "Helper path", Value: report.HelperPath},
			})
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "frozen")
	addJSONFlag(cmd, &jsonOut)
	return cmd
}

func (a *App) newBridgeServeCommand() *cobra.Command {
	var stateDir string
	var containerID string
	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Serve bridge callbacks for a managed container",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if stateDir == "" || containerID == "" {
				return errors.New("missing required flags: --state-dir and --container-id")
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

func (a *App) newRenderer(jsonOut bool) *ui.Renderer {
	return ui.NewRenderer(a.out, a.err, jsonOut)
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

func upResultFields(result appcore.UpResult) []ui.KeyValue {
	fields := []ui.KeyValue{
		{Key: "Container", Value: result.ContainerID},
		{Key: "Image", Value: result.Image},
		{Key: "Workspace", Value: result.RemoteWorkspaceFolder},
	}
	if result.Bridge != nil {
		fields = append(fields, ui.KeyValue{Key: "Bridge", Value: fmt.Sprintf("enabled (%s)", result.Bridge.Status)})
	}
	return fields
}

func upSuggestedCommands(workspace string, configPath string, featureTimeout time.Duration, policy string, sshAgent bool) []string {
	base := []string{"hatchctl", "exec"}
	if workspace != "" {
		base = append(base, "--workspace", shellQuote(workspace))
	}
	if configPath != "" {
		base = append(base, "--config", shellQuote(configPath))
	}
	if featureTimeout != 90*time.Second {
		base = append(base, "--feature-timeout", featureTimeout.String())
	}
	if policy != "" && policy != string(devcontainer.FeatureLockfilePolicyAuto) {
		base = append(base, "--lockfile-policy", policy)
	}
	if sshAgent {
		base = append(base, "--ssh")
	}
	execPrefix := strings.Join(base, " ") + " --"
	return []string{
		strings.Join(base, " "),
		execPrefix + " pwd",
		execPrefix + " go test ./...",
	}
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}

func configResultFields(result appcore.ReadConfigResult) []ui.KeyValue {
	fields := []ui.KeyValue{
		{Key: "Config file", Value: result.ConfigPath},
		{Key: "Workspace folder", Value: result.WorkspaceFolder},
		{Key: "Workspace mount", Value: result.WorkspaceMount},
		{Key: "Devcontainer type", Value: formatSourceKind(result.SourceKind)},
		{Key: "State directory", Value: result.StateDir},
		{Key: "Lifecycle hooks", Value: fmt.Sprintf("initialize=%s create=%s start=%s attach=%s", yesNo(result.HasInitializeCommand), yesNo(result.HasCreateCommand), yesNo(result.HasStartCommand), yesNo(result.HasAttachCommand))},
	}
	if result.CacheDir != "" {
		fields = append(fields, ui.KeyValue{Key: "Cache directory", Value: result.CacheDir})
	}
	if result.ImageUser != "" {
		fields = append(fields, ui.KeyValue{Key: "Image user", Value: result.ImageUser})
	}
	if len(result.ForwardPorts) > 0 {
		fields = append(fields, ui.KeyValue{Key: "Forwarded ports", Value: strings.Join(result.ForwardPorts, ", ")})
	}
	if result.Bridge != nil {
		fields = append(fields, ui.KeyValue{Key: "Bridge support", Value: fmt.Sprintf("enabled=%t status=%s helper=%s mount=%s", result.Bridge.Enabled, result.Bridge.Status, result.Bridge.HelperPath, result.Bridge.BinPath)})
	}
	if result.Dotfiles != nil {
		fields = append(fields, ui.KeyValue{Key: "Dotfiles", Value: fmt.Sprintf("configured=%t applied=%t needs-install=%t repo=%s target=%s", result.Dotfiles.Configured, result.Dotfiles.Applied, result.Dotfiles.NeedsInstall, result.Dotfiles.Repository, result.Dotfiles.TargetPath)})
	}
	if result.ManagedContainer != nil {
		fields = append(fields, ui.KeyValue{Key: "Managed container", Value: fmt.Sprintf("id=%s status=%s running=%t remote-user=%s metadata=%d", result.ManagedContainer.ID, result.ManagedContainer.Status, result.ManagedContainer.Running, result.ManagedContainer.RemoteUser, result.ManagedContainer.MetadataCount)})
	}
	return fields
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatSourceKind(value string) string {
	switch value {
	case "image":
		return "Image"
	case "dockerfile":
		return "Dockerfile"
	case "compose":
		return "Compose"
	default:
		return value
	}
}
