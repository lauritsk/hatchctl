package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

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
			command, err := a.prepareCommand(cmd, global, jsonOut, workspace, configPath, featureTimeout, lockfilePolicy, &bridgeEnabled, &trustWorkspace, &sshAgent, dotfiles)
			if err != nil {
				return err
			}
			defer command.Close()
			result, err := a.service.Up(cmd.Context(), appcore.UpRequest{
				Defaults:           command.defaults,
				AllowHostLifecycle: allowHostLifecycle,
				Recreate:           recreate,
				Global:             command.global,
				IO:                 command.io,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return command.renderer.PrintJSON(result)
			}
			if command.renderer.TTY() {
				if err := command.renderer.PrintSummary("Devcontainer Ready", upResultFields(result)); err != nil {
					return err
				}
				return command.renderer.PrintCommandList("Next", upSuggestedCommands(command.defaults.Workspace, command.defaults.ConfigPath, command.defaults.FeatureTimeout, command.defaults.LockfilePolicy, command.defaults.SSHAgent))
			}
			if err := command.renderer.PrintKeyValues(upResultFields(result)); err != nil {
				return err
			}
			return command.renderer.PrintText("\nNext:\n  " + strings.Join(upSuggestedCommands(command.defaults.Workspace, command.defaults.ConfigPath, command.defaults.FeatureTimeout, command.defaults.LockfilePolicy, command.defaults.SSHAgent), "\n  "))
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
