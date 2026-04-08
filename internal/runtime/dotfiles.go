package runtime

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
)

const dotfilesInstallHelper = `set -eu
repo=$1
target=$2
install_command=${3:-}

command -v git >/dev/null 2>&1 || { echo 'git not found in container PATH' >&2; exit 127; }
if [ -e "$target/.git" ]; then
  git -C "$target" pull --ff-only >/dev/null 2>&1 || true
elif [ -e "$target" ]; then
  echo "dotfiles target already exists and is not a git checkout: $target" >&2
  exit 1
else
  mkdir -p "$(dirname "$target")"
  git clone --depth 1 "$repo" "$target"
fi
cd "$target"
if [ -n "$install_command" ]; then
  if [ -f "./$install_command" ]; then
    [ -x "./$install_command" ] || chmod +x "./$install_command"
    exec "./$install_command"
  fi
  if [ -f "$install_command" ]; then
    [ -x "$install_command" ] || chmod +x "$install_command"
    exec "$install_command"
  fi
  exec /bin/sh -lc "$install_command"
fi
for candidate in install.sh install bootstrap.sh bootstrap script/bootstrap setup.sh setup script/setup; do
  if [ -e "$candidate" ]; then
    [ -x "$candidate" ] || chmod +x "$candidate"
    exec "./$candidate"
  fi
done
found=0
for file in "$target"/.[!.]* "$target"/..?*; do
  [ -e "$file" ] || continue
  base=$(basename "$file")
  [ "$base" = .git ] && continue
  ln -snf "$file" "$HOME/$base"
  found=1
done
if [ "$found" -eq 0 ]; then
  echo 'No dotfiles install script or top-level dotfiles found.'
fi
`

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
	if strings.Contains(value, "://") || strings.HasPrefix(value, "git@") {
		if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
			parsed, err := url.Parse(value)
			if err != nil {
				return "", err
			}
			if parsed.Host != "" && !strings.HasSuffix(parsed.Path, ".git") {
				parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
				if len(parts) >= 2 {
					value += ".git"
				}
			}
		}
		return value, nil
	}
	parts := strings.Split(value, "/")
	switch len(parts) {
	case 1:
		return "https://github.com/" + parts[0] + "/dotfiles.git", nil
	case 2:
		if parts[0] == "github.com" {
			return guessedDotfilesRepository(parts[0], parts[1], "dotfiles"), nil
		}
		if strings.Contains(parts[0], ".") {
			return guessedDotfilesRepository(parts[0], parts[1], "dotfiles"), nil
		}
		return "https://github.com/" + value + ".git", nil
	case 3:
		if parts[0] == "github.com" {
			return guessedDotfilesRepository(parts[0], parts[1], parts[2]), nil
		}
		if strings.Contains(parts[0], ".") {
			return guessedDotfilesRepository(parts[0], parts[1], parts[2]), nil
		}
	}
	return "", fmt.Errorf("invalid dotfiles repository %q; use user, owner/repo, host/user, host/user/repo, or a git URL", value)
}

func guessedDotfilesRepository(host string, owner string, repo string) string {
	if host == "sr.ht" {
		host = "git.sr.ht"
	}
	return "https://" + host + "/" + owner + "/" + repo + ".git"
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
	targetPath, err := r.resolveDotfilesTargetPath(ctx, containerID, resolved, opts.TargetPath)
	if err != nil {
		return err
	}
	args, err := r.dockerExecArgs(ctx, containerID, resolved, true, false, nil, []string{"/bin/sh", "-s", "--", opts.Repository, targetPath, opts.InstallCommand})
	if err != nil {
		return err
	}
	label := fmt.Sprintf("Installing dotfiles from %s", opts.Repository)
	r.emitPhaseProgress(events, phaseDotfiles, label)
	return r.backend.Run(ctx, runtimeCommand{Kind: runtimeCommandDocker, Phase: phaseDotfiles, Label: label, Args: args, Stdin: strings.NewReader(dotfilesInstallHelper), Stdout: r.stdout, Stderr: r.stderr, Events: events})
}

func (r *Runner) resolveDotfilesTargetPath(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, targetPath string) (string, error) {
	if !strings.HasPrefix(targetPath, "$HOME") {
		return targetPath, nil
	}
	user, err := r.effectiveExecUser(ctx, containerID, resolved)
	if err != nil {
		return "", err
	}
	home, err := r.resolveExecHome(ctx, containerID, user)
	if err != nil {
		return "", err
	}
	if home == "" {
		return targetPath, nil
	}
	if targetPath == "$HOME" {
		return home, nil
	}
	return strings.TrimSuffix(home, "/") + strings.TrimPrefix(targetPath, "$HOME"), nil
}
