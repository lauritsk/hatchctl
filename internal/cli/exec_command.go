package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	appcore "github.com/lauritsk/hatchctl/internal/app"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

func (a *App) newExecCommand(global *globalOptions) *cobra.Command {
	var workspace string
	var configPath string
	var lockfilePolicy string
	var featureTimeout time.Duration
	var jsonOut bool
	var remoteEnv []string
	var sshAgent bool
	trustWorkspace := appcore.EnvTruthy(appcore.TrustWorkspaceEnvVar)
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
			command, err := a.prepareCommand(cmd, global, jsonOut, workspace, configPath, featureTimeout, lockfilePolicy, nil, &trustWorkspace, &sshAgent, appcore.DotfilesOptions{})
			if err != nil {
				return err
			}
			defer command.Close()
			stdout, stderr := execWriters(command.renderer, false)
			var stdoutBuffer strings.Builder
			var stderrBuffer strings.Builder
			if jsonOut {
				stdout = &stdoutBuffer
				stderr = &stderrBuffer
			} else if shouldUseRawExecStreams(os.Stdin, os.Stdout) {
				stdout, stderr = execWriters(command.renderer, true)
			}
			execIO := command.io
			execIO.Stdin = os.Stdin
			execIO.Stdout = stdout
			execIO.Stderr = stderr
			code, err := a.service.Exec(cmd.Context(), appcore.ExecRequest{
				Defaults:  command.defaults,
				Args:      args,
				RemoteEnv: multiValueMap(remoteEnv),
				Global:    command.global,
				IO:        execIO,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				if err := command.renderer.PrintJSON(map[string]any{
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
	cmd.Flags().BoolVar(&trustWorkspace, "trust-workspace", trustWorkspace, "trust repo-controlled workspace defaults that expand host access")
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
