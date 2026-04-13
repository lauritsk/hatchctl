package cli

import (
	"strconv"
	"time"

	"github.com/spf13/cobra"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

func (a *App) newRenderer(jsonOut bool) *ui.Renderer {
	return ui.NewRenderer(a.out, a.err, jsonOut)
}

func (a *App) resolveDefaults(cmd *cobra.Command, workspace string, configPath string, featureTimeout time.Duration, lockfilePolicy string, bridgeEnabled *bool, trustWorkspace *bool, sshAgent *bool, dotfiles appcore.DotfilesOptions) (appcore.CommandDefaults, error) {
	if globalBackend := cmd.Flag("backend"); globalBackend != nil && globalBackend.Value.String() != "" {
		_ = cmd.Flags().Set("backend", globalBackend.Value.String())
	}
	return appcore.ResolveDefaults(resolveDefaultsRequest(cmd, workspace, configPath, featureTimeout, lockfilePolicy, bridgeEnabled, trustWorkspace, sshAgent, dotfiles))
}

func (a *App) newCommandIO(renderer *ui.Renderer) appcore.CommandIO {
	return appcore.CommandIO{Events: renderer.Events(), Stdout: renderer.Stdout(), Stderr: renderer.Stderr()}
}

func (a *App) prepareCommand(cmd *cobra.Command, global *globalOptions, jsonOut bool, workspace string, configPath string, featureTimeout time.Duration, lockfilePolicy string, bridgeEnabled *bool, trustWorkspace *bool, sshAgent *bool, dotfiles appcore.DotfilesOptions) (*preparedCommand, error) {
	renderer := a.newRenderer(jsonOut)
	defaults, err := a.resolveDefaults(cmd, workspace, configPath, featureTimeout, lockfilePolicy, bridgeEnabled, trustWorkspace, sshAgent, dotfiles)
	if err != nil {
		renderer.Close()
		return nil, err
	}
	return &preparedCommand{renderer: renderer, defaults: defaults, global: global.app(), io: a.newCommandIO(renderer)}, nil
}

func (c *preparedCommand) Close() {
	if c == nil || c.renderer == nil {
		return
	}
	c.renderer.Close()
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}
