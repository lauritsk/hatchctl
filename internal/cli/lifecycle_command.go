package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
)

func (a *App) newRunCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var phase string
	allowHostLifecycle := appcore.EnvTruthy(appcore.AllowHostLifecycleEnvVar)
	trustWorkspace := appcore.EnvTruthy(appcore.TrustWorkspaceEnvVar)
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
			command, err := a.prepareCommand(cmd, global, jsonOut, workspace, configPath, featureTimeout, lockfilePolicy, nil, &trustWorkspace, nil, dotfiles)
			if err != nil {
				return err
			}
			defer command.Close()
			result, err := a.service.RunLifecycle(cmd.Context(), appcore.RunLifecycleRequest{
				Defaults:           command.defaults,
				AllowHostLifecycle: allowHostLifecycle,
				Phase:              phase,
				Global:             command.global,
				IO:                 command.io,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return command.renderer.PrintJSON(result)
			}
			return command.renderer.PrintText(fmt.Sprintf("Lifecycle phase %q completed for container %s.", result.Phase, result.ContainerID))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().StringVar(&phase, "phase", "all", "lifecycle phase to run: all, create, start, or attach")
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled workspace defaults that expand host access")
	cmd.Flags().BoolVar(&allowHostLifecycle, "allow-host-lifecycle", allowHostLifecycle, "trust and run host-side lifecycle commands such as initializeCommand")
	addJSONFlag(cmd, &jsonOut)
	addDotfilesFlags(cmd, &dotfiles)
	return cmd
}
