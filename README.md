# Hatch Control

`hatchctl` is a Go command-line tool for working with Dev Container based
workspaces from the terminal. It creates or reconnects to a workspace container,
runs commands inside it, shows the resolved Dev Container configuration, and can
rerun Dev Container lifecycle hooks without requiring editor integration.

The project targets terminal-first workflows on macOS and Linux. It shells out to
Docker or Podman instead of talking directly to a container engine API.

## Features

- Start, build, inspect, and enter Dev Container workspaces from one CLI
- Run one-off commands inside the managed container
- Emit JSON output for scripts
- Use Docker by default, with Podman support available
- Keep repository-controlled, host-affecting settings behind explicit trust flags
- Optional macOS bridge support for browser opening and localhost callback
  forwarding

## Requirements

- Go, if building from source
- Docker or Podman available on `PATH`
- Compose support from the selected backend when using compose-based Dev
  Containers
- macOS for bridge support

## Basic Usage

Create or reuse the Dev Container for the current workspace:

```sh
hatchctl up
```

Open a shell inside the managed container:

```sh
hatchctl exec
```

Run a one-off command:

```sh
hatchctl exec -- go test ./...
```

Inspect the resolved config and detected runtime state:

```sh
hatchctl config
```

Run lifecycle hooks again:

```sh
hatchctl run --phase start
```

Use JSON output in scripts:

```sh
hatchctl up --json
hatchctl config --json
hatchctl exec --json -- sh -lc 'go test ./...'
```

Use `--` with `exec` to separate `hatchctl` flags from the command to run inside
the container.

## Commands

- `hatchctl up`: resolve configuration, then create or reconnect to the managed
  Dev Container, building it first when needed
- `hatchctl build`: build the Dev Container image without starting it
- `hatchctl exec`: open a shell or run a command inside the managed container
- `hatchctl config`: show merged config and detected runtime state
- `hatchctl run`: run Dev Container lifecycle phases again in an existing
  container
- `hatchctl bridge doctor`: inspect bridge availability and current bridge
  session state
- `hatchctl version`: print version information

## Configuration

`hatchctl` reads configuration from:

- user config
  - Linux: `~/.config/hatchctl/config.toml`
  - macOS: `~/Library/Application Support/hatchctl/config.toml`
- workspace config: `.hatchctl/config.toml`

Workspace config is intentionally limited until the repository is explicitly
trusted.

Example config:

```toml
backend = "auto"
config = ".devcontainer/devcontainer.json"
feature_timeout = "2m"
lockfile_policy = "auto"

[dotfiles]
repository = "lauritsk/dotfiles"
```

Useful environment variables:

- `HATCHCTL_TRUST_WORKSPACE=1`
- `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`
- `HATCHCTL_COSIGN_STRICT=1`
- `HATCHCTL_ALLOW_INSECURE_FEATURES=1`
- `HATCHCTL_DOTFILES_REPOSITORY`
- `HATCHCTL_DOTFILES_INSTALL_COMMAND`
- `HATCHCTL_DOTFILES_TARGET_PATH`

## Security Model

`hatchctl` assumes `devcontainer.json` and workspace-local config may come from a
repository that has not been trusted yet. Host-affecting behavior is opt-in.

Security defaults include:

- Host-side `initializeCommand` is blocked unless `--allow-host-lifecycle` is
  passed
- Repo-controlled backend settings that expand host access are blocked unless
  `--trust-workspace` is passed
- Unsigned images warn by default; `HATCHCTL_COSIGN_STRICT=1` blocks execution
- Unsigned remote OCI features fail by default in unattended runs
- Direct tarball features must use `https`, except loopback `http` for local use
  and tests
- The macOS bridge uses only loopback addresses
