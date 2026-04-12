package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

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
			command, err := a.prepareCommand(cmd, global, jsonOut, workspace, configPath, featureTimeout, lockfilePolicy, nil, &trustWorkspace, nil, appcore.DotfilesOptions{})
			if err != nil {
				return err
			}
			defer command.Close()
			result, err := a.service.Build(cmd.Context(), appcore.BuildRequest{
				Defaults: command.defaults,
				Global:   command.global,
				IO:       command.io,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return command.renderer.PrintJSON(result)
			}
			if command.renderer.TTY() {
				return command.renderer.PrintSummary("Image Ready", []ui.KeyValue{{Key: "Image", Value: result.Image}})
			}
			return command.renderer.PrintText(fmt.Sprintf("Devcontainer image ready: %s", result.Image))
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "auto")
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled Docker mounts, privilege, and build settings")
	addJSONFlag(cmd, &jsonOut)
	return cmd
}
