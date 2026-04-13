package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

func (a *App) newConfigCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	var sshAgent bool
	trustWorkspace := appcore.EnvTruthy(appcore.TrustWorkspaceEnvVar)
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
			return a.withPreparedCommand(cmd, global, prepareOptions{jsonOut: jsonOut, workspace: workspace, configPath: configPath, featureTimeout: featureTimeout, lockfilePolicy: lockfilePolicy, trustWorkspace: &trustWorkspace, sshAgent: &sshAgent, dotfiles: dotfiles}, func(command *preparedCommand) error {
				result, err := a.service.ReadConfig(cmd.Context(), appcore.ReadConfigRequest{
					Defaults: command.defaults,
					Global:   command.global,
					IO:       command.io,
				})
				if err != nil {
					return err
				}
				return printJSONOrSummary(command, jsonOut, result, "Configuration", configResultFields(result))
			})
		},
	}
	addWorkspaceFlags(cmd, &workspace, &configPath)
	addResolutionFlags(cmd, &featureTimeout, &lockfilePolicy, "frozen")
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled workspace defaults that expand host access")
	cmd.Flags().BoolVar(&sshAgent, "ssh", false, "show config with host ssh-agent passthrough applied")
	addJSONFlag(cmd, &jsonOut)
	addDotfilesFlags(cmd, &dotfiles)
	return cmd
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
		return "Build File"
	case "compose":
		return "Project Service"
	default:
		return value
	}
}
