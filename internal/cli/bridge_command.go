package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/bridge"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

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
			command, err := a.prepareCommand(cmd, global, jsonOut, workspace, configPath, featureTimeout, lockfilePolicy, nil, nil, nil, appcore.DotfilesOptions{})
			if err != nil {
				return err
			}
			defer command.Close()
			report, err := a.service.BridgeDoctor(cmd.Context(), appcore.BridgeDoctorRequest{
				Defaults: command.defaults,
				Global:   command.global,
				IO:       command.io,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return command.renderer.PrintJSON(report)
			}
			return command.renderer.PrintKeyValues([]ui.KeyValue{
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
