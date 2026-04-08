package runtime

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

var dotfilesInstallCandidates = []string{
	"install.sh",
	"install",
	"bootstrap.sh",
	"bootstrap",
	"script/bootstrap",
	"setup.sh",
	"setup",
	"script/setup",
}

type DotfilesOptions struct {
	Repository     string `json:"repository,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
}

type DotfilesStatus struct {
	Configured     bool   `json:"configured"`
	Applied        bool   `json:"applied"`
	NeedsInstall   bool   `json:"needsInstall"`
	Repository     string `json:"repository,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
}

func (o DotfilesOptions) Enabled() bool {
	return strings.TrimSpace(o.Repository) != ""
}

func (o DotfilesOptions) Normalized() (DotfilesOptions, error) {
	o.Repository = strings.TrimSpace(o.Repository)
	o.InstallCommand = strings.TrimSpace(o.InstallCommand)
	o.TargetPath = normalizeDotfilesTargetPath(strings.TrimSpace(o.TargetPath))
	if o.Repository == "" {
		return DotfilesOptions{}, nil
	}
	repo, err := normalizeDotfilesRepository(o.Repository)
	if err != nil {
		return DotfilesOptions{}, err
	}
	o.Repository = repo
	if o.TargetPath == "" {
		o.TargetPath = "$HOME/.dotfiles"
	}
	return o, nil
}

func normalizeDotfilesRepository(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("local dotfiles repository paths are not supported: %q", value)
	}
	if strings.HasPrefix(value, "github.com/") {
		value = "https://" + value
	}
	if strings.Contains(value, "://") || strings.HasPrefix(value, "git@") {
		if strings.HasPrefix(value, "https://github.com/") && !strings.HasSuffix(value, ".git") {
			value += ".git"
		}
		return value, nil
	}
	if strings.Count(value, "/") == 1 {
		return "https://github.com/" + value + ".git", nil
	}
	return "", fmt.Errorf("unsupported dotfiles repository %q; use owner/repo, github.com/owner/repo, or a git URL", value)
}

func normalizeDotfilesTargetPath(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "~/") {
		return "$HOME/" + strings.TrimPrefix(value, "~/")
	}
	if strings.HasPrefix(value, "$HOME/") || value == "$HOME" || strings.HasPrefix(value, "/") {
		return value
	}
	return path.Join("$HOME", value)
}

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func dotfilesTargetAssignment(value string) string {
	if value == "$HOME" {
		return `target="$HOME"`
	}
	if strings.HasPrefix(value, "$HOME/") {
		return `target="$HOME"/` + quoteShell(strings.TrimPrefix(value, "$HOME/"))
	}
	return "target=" + quoteShell(value)
}

func dotfilesStateMatches(state devcontainer.State, opts DotfilesOptions) bool {
	return state.DotfilesReady && state.DotfilesRepo == opts.Repository && state.DotfilesInstall == opts.InstallCommand && state.DotfilesTarget == opts.TargetPath
}

func dotfilesStatus(state devcontainer.State, opts DotfilesOptions) *DotfilesStatus {
	if !opts.Enabled() && state.DotfilesRepo == "" && state.DotfilesTarget == "" && !state.DotfilesReady {
		return nil
	}
	return &DotfilesStatus{
		Configured:     opts.Enabled(),
		Applied:        state.DotfilesReady,
		NeedsInstall:   opts.Enabled() && !dotfilesStateMatches(state, opts),
		Repository:     firstNonEmpty(opts.Repository, state.DotfilesRepo),
		InstallCommand: firstNonEmpty(opts.InstallCommand, state.DotfilesInstall),
		TargetPath:     firstNonEmpty(opts.TargetPath, state.DotfilesTarget),
	}
}

func (r *Runner) installDotfiles(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, opts DotfilesOptions, events ui.Sink) error {
	if !opts.Enabled() {
		return nil
	}
	args, err := r.dockerExecArgs(ctx, containerID, resolved, true, false, nil, []string{"/bin/sh", "-lc", dotfilesInstallScript(opts)})
	if err != nil {
		return err
	}
	label := fmt.Sprintf("Installing dotfiles from %s", opts.Repository)
	r.emitProgress(events, label)
	return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Label: label, Args: args, Stdout: r.stdout, Stderr: r.stderr, Events: events})
}

func dotfilesInstallScript(opts DotfilesOptions) string {
	var script strings.Builder
	script.WriteString("set -eu\n")
	script.WriteString("repo=" + quoteShell(opts.Repository) + "\n")
	script.WriteString(dotfilesTargetAssignment(opts.TargetPath) + "\n")
	script.WriteString("command -v git >/dev/null 2>&1 || { echo 'git not found in container PATH' >&2; exit 127; }\n")
	script.WriteString("if [ -e \"$target/.git\" ]; then\n")
	script.WriteString("  git -C \"$target\" pull --ff-only >/dev/null 2>&1 || true\n")
	script.WriteString("elif [ -e \"$target\" ]; then\n")
	script.WriteString("  echo \"dotfiles target already exists and is not a git checkout: $target\" >&2\n")
	script.WriteString("  exit 1\n")
	script.WriteString("else\n")
	script.WriteString("  mkdir -p \"$(dirname \"$target\")\"\n")
	script.WriteString("  git clone --depth 1 \"$repo\" \"$target\"\n")
	script.WriteString("fi\n")
	script.WriteString("cd \"$target\"\n")
	if opts.InstallCommand != "" {
		script.WriteString("install_command=" + quoteShell(opts.InstallCommand) + "\n")
		script.WriteString("if [ -f \"./$install_command\" ]; then\n")
		script.WriteString("  [ -x \"./$install_command\" ] || chmod +x \"./$install_command\"\n")
		script.WriteString("  exec \"./$install_command\"\n")
		script.WriteString("fi\n")
		script.WriteString("if [ -f \"$install_command\" ]; then\n")
		script.WriteString("  [ -x \"$install_command\" ] || chmod +x \"$install_command\"\n")
		script.WriteString("  exec \"$install_command\"\n")
		script.WriteString("fi\n")
		script.WriteString("exec /bin/sh -lc \"$install_command\"\n")
		return script.String()
	}
	script.WriteString("install_command=\"\"\n")
	script.WriteString("for candidate in")
	for _, candidate := range dotfilesInstallCandidates {
		script.WriteString(" " + quoteShell(candidate))
	}
	script.WriteString("; do\n")
	script.WriteString("  if [ -e \"$candidate\" ]; then\n")
	script.WriteString("    install_command=$candidate\n")
	script.WriteString("    break\n")
	script.WriteString("  fi\n")
	script.WriteString("done\n")
	script.WriteString("if [ -n \"$install_command\" ]; then\n")
	script.WriteString("  [ -x \"$install_command\" ] || chmod +x \"$install_command\"\n")
	script.WriteString("  exec \"./$install_command\"\n")
	script.WriteString("fi\n")
	script.WriteString("found=0\n")
	script.WriteString("for file in \"$target\"/.[!.]* \"$target\"/..?*; do\n")
	script.WriteString("  [ -e \"$file\" ] || continue\n")
	script.WriteString("  base=$(basename \"$file\")\n")
	script.WriteString("  [ \"$base\" = .git ] && continue\n")
	script.WriteString("  ln -snf \"$file\" \"$HOME/$base\"\n")
	script.WriteString("  found=1\n")
	script.WriteString("done\n")
	script.WriteString("if [ \"$found\" -eq 0 ]; then\n")
	script.WriteString("  echo 'No dotfiles install script or top-level dotfiles found.'\n")
	script.WriteString("fi\n")
	return script.String()
}
