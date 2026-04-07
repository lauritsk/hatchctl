package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/runtime"
	"github.com/lauritsk/hatchctl/internal/version"
)

type App struct {
	out io.Writer
	err io.Writer

	runner *runtime.Runner
}

func New(out io.Writer, err io.Writer) *App {
	engine := docker.NewClient("docker")
	return &App{
		out:    out,
		err:    err,
		runner: runtime.NewRunner(engine),
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	switch args[0] {
	case "help", "--help", "-h":
		a.printHelp()
		return nil
	case "version", "--version", "-v":
		_, err := fmt.Fprintln(a.out, version.Version)
		return err
	case "up":
		return a.runUp(ctx, args[1:])
	case "build":
		return a.runBuild(ctx, args[1:])
	case "exec":
		return a.runExec(ctx, args[1:])
	case "config":
		return a.runConfig(ctx, args[1:])
	case "run":
		return a.runUserCommands(ctx, args[1:])
	case "bridge":
		return a.runBridge(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], helpText())
	}
}

func (a *App) runUp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	recreate := fs.Bool("recreate", false, "remove and recreate an existing managed container")
	bridgeEnabled := fs.Bool("bridge", false, "enable macOS auth bridge scaffolding")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	verbose := fs.Bool("verbose", false, "print the runtime plan before executing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}

	result, err := a.runner.Up(ctx, runtime.UpOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
		Recreate:       *recreate,
		BridgeEnabled:  *bridgeEnabled,
		Verbose:        *verbose,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.out, result)
	}
	return a.printUpResult(result)
}

func (a *App) runBuild(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	verbose := fs.Bool("verbose", false, "print the runtime plan before executing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}

	result, err := a.runner.Build(ctx, runtime.BuildOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
		Verbose:        *verbose,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.out, result)
	}
	_, err = fmt.Fprintf(a.out, "Built %s\n", result.Image)
	return err
}

func (a *App) runExec(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	remoteEnv := multiValue{}
	fs.Var(&remoteEnv, "env", "extra remote environment variables in KEY=VALUE form")
	if err := fs.Parse(args); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}
	cmd := fs.Args()
	if len(cmd) == 0 {
		return errors.New("exec requires a command")
	}

	stdout := a.out
	stderr := a.err
	var stdoutBuffer strings.Builder
	var stderrBuffer strings.Builder
	if *jsonOut {
		stdout = &stdoutBuffer
		stderr = &stderrBuffer
	}
	code, err := a.runner.Exec(ctx, runtime.ExecOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
		Args:           cmd,
		RemoteEnv:      remoteEnv.Map(),
		Stdin:          os.Stdin,
		Stdout:         stdout,
		Stderr:         stderr,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		if err := writeJSON(a.out, map[string]any{
			"exitCode": code,
			"stdout":   stdoutBuffer.String(),
			"stderr":   stderrBuffer.String(),
			"command":  cmd,
		}); err != nil {
			return err
		}
	}
	if code != 0 {
		return runtime.ExitError{Code: code}
	}
	return nil
}

func (a *App) runConfig(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "frozen", "lockfile policy: auto, frozen, or update")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}

	result, err := a.runner.ReadConfig(ctx, runtime.ReadConfigOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.out, result)
	}
	_, err = fmt.Fprintf(a.out, "Config: %s\nWorkspace: %s\nWorkspace mount: %s\nSource: %s\nLifecycle: initialize=%t create=%t start=%t attach=%t\n",
		result.ConfigPath,
		result.WorkspaceFolder,
		result.WorkspaceMount,
		result.SourceKind,
		result.HasInitializeCommand,
		result.HasCreateCommand,
		result.HasStartCommand,
		result.HasAttachCommand,
	)
	if err != nil {
		return err
	}
	if result.ImageUser != "" {
		if _, err := fmt.Fprintf(a.out, "Image user: %s\n", result.ImageUser); err != nil {
			return err
		}
	}
	if len(result.ForwardPorts) > 0 {
		if _, err := fmt.Fprintf(a.out, "Forward ports: %s\n", strings.Join(result.ForwardPorts, ", ")); err != nil {
			return err
		}
	}
	if result.Bridge != nil {
		_, err = fmt.Fprintf(a.out, "Bridge: enabled=%t mount=%s helper=%s status=%s\n",
			result.Bridge.Enabled,
			result.Bridge.BinPath,
			result.Bridge.HelperPath,
			result.Bridge.Status,
		)
		if err != nil {
			return err
		}
	}
	if result.ManagedContainer != nil {
		_, err = fmt.Fprintf(a.out, "Managed container: id=%s status=%s running=%t user=%s metadata=%d\n",
			result.ManagedContainer.ID,
			result.ManagedContainer.Status,
			result.ManagedContainer.Running,
			result.ManagedContainer.RemoteUser,
			result.ManagedContainer.MetadataCount,
		)
	}
	return err
}

func (a *App) runUserCommands(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "auto", "lockfile policy: auto, frozen, or update")
	phase := fs.String("phase", "all", "one of: all, create, start, attach")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}

	result, err := a.runner.RunLifecycle(ctx, runtime.RunLifecycleOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
		Phase:          *phase,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.out, result)
	}
	_, err = fmt.Fprintf(a.out, "Ran lifecycle commands for %s (%s).\n", result.ContainerID, result.Phase)
	return err
}

func (a *App) runBridge(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(a.out, "Usage: hatchctl bridge doctor [--workspace PATH] [--config PATH] [--json]")
		return err
	}
	if args[0] != "doctor" {
		if args[0] == "serve" {
			return a.runBridgeServe(ctx, args[1:])
		}
		return fmt.Errorf("unknown bridge command %q", args[0])
	}

	fs := flag.NewFlagSet("bridge doctor", flag.ContinueOnError)
	fs.SetOutput(a.err)
	workspace := fs.String("workspace", "", "workspace folder (defaults to current directory)")
	configPath := fs.String("config", "", "path to devcontainer.json")
	lockfilePolicy := fs.String("lockfile-policy", "frozen", "lockfile policy: auto, frozen, or update")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	policy, err := parseLockfilePolicy(*lockfilePolicy)
	if err != nil {
		return err
	}

	report, err := a.runner.BridgeDoctor(ctx, runtime.BridgeDoctorOptions{
		Workspace:      *workspace,
		ConfigPath:     *configPath,
		LockfilePolicy: policy,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.out, report)
	}
	_, err = fmt.Fprintf(a.out, "Bridge session: %s\nEnabled: %t\nState path: %s\nHelper path: %s\nStatus: %s\n",
		report.ID,
		report.Enabled,
		report.StatePath,
		report.HelperPath,
		report.Status,
	)
	return err
}

func (a *App) runBridgeServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bridge serve", flag.ContinueOnError)
	fs.SetOutput(a.err)
	stateDir := fs.String("state-dir", "", "workspace state directory")
	containerID := fs.String("container-id", "", "managed container id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stateDir == "" || *containerID == "" {
		return errors.New("bridge serve requires --state-dir and --container-id")
	}
	return bridge.Serve(ctx, *stateDir, *containerID)
}

func (a *App) printHelp() {
	_, _ = io.WriteString(a.out, helpText())
}

func helpText() string {
	return strings.TrimSpace(`hatchctl

Terminal-first Development Containers in Go.

Commands:
  hatchctl up       Build and start a dev container
  hatchctl build    Build the dev container image only
  hatchctl exec     Run a command inside the dev container
  hatchctl config   Inspect the resolved local configuration
  hatchctl run      Run lifecycle commands against an existing container
  hatchctl bridge   Bridge status and diagnostics
  hatchctl version  Print version information

Examples:
  hatchctl up
  hatchctl up --workspace ../my-project --bridge
  hatchctl exec -- go test ./...
  hatchctl build --json
  hatchctl config --json
`) + "\n"
}

func (a *App) printUpResult(result runtime.UpResult) error {
	lines := []string{
		fmt.Sprintf("Container: %s", result.ContainerID),
		fmt.Sprintf("Image: %s", result.Image),
		fmt.Sprintf("Workspace: %s", result.RemoteWorkspaceFolder),
		fmt.Sprintf("State: %s", result.StateDir),
	}
	if result.Bridge != nil {
		lines = append(lines, fmt.Sprintf("Bridge: enabled (%s)", result.Bridge.Status))
	}
	_, err := fmt.Fprintln(a.out, strings.Join(lines, "\n"))
	return err
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func parseLockfilePolicy(value string) (devcontainer.FeatureLockfilePolicy, error) {
	return devcontainer.ParseFeatureLockfilePolicy(value)
}

type multiValue []string

func (m *multiValue) String() string {
	return strings.Join(*m, ",")
}

func (m *multiValue) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func (m multiValue) Map() map[string]string {
	result := make(map[string]string, len(m))
	for _, item := range m {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 1 {
			result[parts[0]] = ""
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result
}

var _ = bridge.Report{}
