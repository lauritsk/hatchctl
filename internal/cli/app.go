package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/bridge"
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

type preparedCommand struct {
	renderer *ui.Renderer
	defaults appcore.CommandDefaults
	global   appcore.GlobalOptions
	io       appcore.CommandIO
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
	cmd.PersistentFlags().StringVar(&global.Backend, "backend", "", "container backend to use (for example: auto, docker, or podman)")
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
