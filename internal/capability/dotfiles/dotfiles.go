package dotfiles

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/lauritsk/hatchctl/internal/capability"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

const InstallScript = `set -eu
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

type Config = capability.Dotfiles

type Status struct {
	Configured     bool
	Applied        bool
	NeedsInstall   bool
	Repository     string
	InstallCommand string
	TargetPath     string
}

func Normalize(c Config) (Config, error) {
	c.Repository = strings.TrimSpace(c.Repository)
	c.InstallCommand = strings.TrimSpace(c.InstallCommand)
	c.TargetPath = NormalizeTargetPath(strings.TrimSpace(c.TargetPath))
	if c.Repository == "" {
		return Config{}, nil
	}
	repo, err := NormalizeRepository(c.Repository)
	if err != nil {
		return Config{}, err
	}
	c.Repository = repo
	if c.TargetPath == "" {
		c.TargetPath = "$HOME/.dotfiles"
	}
	return c, nil
}

func NormalizeRepository(value string) (string, error) {
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
			return guessedRepository(parts[0], parts[1], "dotfiles"), nil
		}
		if strings.Contains(parts[0], ".") {
			return guessedRepository(parts[0], parts[1], "dotfiles"), nil
		}
		return "https://github.com/" + value + ".git", nil
	case 3:
		if parts[0] == "github.com" {
			return guessedRepository(parts[0], parts[1], parts[2]), nil
		}
		if strings.Contains(parts[0], ".") {
			return guessedRepository(parts[0], parts[1], parts[2]), nil
		}
	}
	return "", fmt.Errorf("invalid dotfiles repository %q; use user, owner/repo, host/user, host/user/repo, or a git URL", value)
}

func NormalizeTargetPath(value string) string {
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

func ResolveTargetPath(home string, targetPath string) string {
	if !strings.HasPrefix(targetPath, "$HOME") || home == "" {
		return targetPath
	}
	if targetPath == "$HOME" {
		return home
	}
	return strings.TrimSuffix(home, "/") + strings.TrimPrefix(targetPath, "$HOME")
}

func StateMatches(state devcontainer.State, cfg Config) bool {
	return state.DotfilesReady && state.DotfilesTransition == nil && state.DotfilesRepo == cfg.Repository && state.DotfilesInstall == cfg.InstallCommand && state.DotfilesTarget == cfg.TargetPath
}

func StatusFor(state devcontainer.State, cfg Config) *Status {
	if !cfg.Enabled() && state.DotfilesRepo == "" && state.DotfilesTarget == "" && !state.DotfilesReady {
		return nil
	}
	return &Status{
		Configured:     cfg.Enabled(),
		Applied:        state.DotfilesReady && state.DotfilesTransition == nil,
		NeedsInstall:   cfg.Enabled() && !StateMatches(state, cfg),
		Repository:     firstNonEmpty(cfg.Repository, state.DotfilesRepo),
		InstallCommand: firstNonEmpty(cfg.InstallCommand, state.DotfilesInstall),
		TargetPath:     firstNonEmpty(cfg.TargetPath, state.DotfilesTarget),
	}
}

func InstallArgs(repo string, targetPath string, installCommand string) []string {
	return []string{"/bin/sh", "-s", "--", repo, targetPath, installCommand}
}

func guessedRepository(host string, owner string, repo string) string {
	if host == "sr.ht" {
		host = "git.sr.ht"
	}
	return "https://" + host + "/" + owner + "/" + repo + ".git"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
