# Hatch Control

Run Dev Containers from the terminal.

[![CI][badge-ci]][link-actions]
[![GitHub release][badge-release]][link-releases]
![Go 1.26+][badge-go]
![macOS and Linux][badge-platform]

[Install](#install) • [Quick Start](#quick-start) • [Commands](#commands) •
[Configuration](#configuration) • [Security Model](#security-model) •
[Development](#development)

`hatchctl` is a Go CLI for creating, inspecting, and using
Dev Container based workspaces without depending on editor integration. It is
built for terminal-first workflows that start a workspace, open a shell,
show resolved configuration, and run life cycle hooks again from the command line.

> [!NOTE]
> `hatchctl` supports macOS and Linux. The optional bridge for browser opening and
> localhost callback forwarding is macOS-only.

## Why the CLI

- **Terminal-first Dev Containers**: use Dev Containers without opening VS Code
- **Repeatable workflows**: build, start, inspect, and exec through a single CLI
- **Script-friendly output**: use JSON output for automation and CI-style
  tooling
- **Backend flexibility**: supports Docker by default, with `Podman` support
  available
- **Safer defaults**: trust-sensitive repo settings stay gated behind explicit
  trust flags

## Install

Install with `mise`:

```sh
mise use github:lauritsk/hatchctl@latest
```

Install from source:

```sh
go install github.com/lauritsk/hatchctl/cmd/hatchctl@latest
```

Prebuilt binaries for macOS and Linux are published on the
+[GitHub Releases](https://github.com/lauritsk/hatchctl/releases) page.

## Requirements

`hatchctl` shells out to a container backend instead of talking to an engine
API directly.

You will need:

- Docker or `Podman` available on `PATH`
- A Linux container runtime target for Dev Containers
- Compose support from your selected backend when using compose-based
  Dev Containers
- macOS to use bridge support

## Quick Start

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

Run life cycle hooks again:

```sh
hatchctl run --phase start
```

Use JSON output in scripts:

```sh
hatchctl up --json
hatchctl config --json
hatchctl exec --json -- sh -lc 'go test ./...'
```

> [!TIP]
> Use `--` with `exec` to separate `hatchctl` flags from the command you want to
> run inside the container.

## Commands

- `hatchctl up`: resolve configuration, then create or reconnect to the managed
  Dev Container, building it first when needed
- `hatchctl build`: build the Dev Container image without starting it
- `hatchctl exec`: open a shell or run a command inside the managed container
- `hatchctl config`: show merged config and detected runtime state
- `hatchctl run`: run Dev Container life cycle phases again in an existing container
- `hatchctl bridge doctor`: inspect bridge availability and current bridge
  session state
- `hatchctl version`: print version information

Common examples:

```sh
hatchctl --backend auto up
hatchctl up --workspace ../my-project
hatchctl up --dotfiles lauritsk/dotfiles
hatchctl up --ssh
hatchctl up --trust-workspace --allow-host-lifecycle
hatchctl build --json
hatchctl exec --env CI=1 -- sh -lc 'make test'
hatchctl run --phase attach
hatchctl bridge doctor
```

## Configuration

`hatchctl` reads configuration from:

- user config
  - Linux: `~/.config/hatchctl/config.toml`
  - macOS: `~/Library/Application Support/hatchctl/config.toml`
- workspace config: `.hatchctl/config.toml`

Workspace config is intentionally limited until you explicitly trust the
repository.

Example config:

```toml
backend = "auto"
config = ".devcontainer/devcontainer.json"
feature_timeout = "2m"
lockfile_policy = "auto"

[dotfiles]
repository = "lauritsk/dotfiles"

[verification]
[[verification.trusted_signers]]
issuer = "https://token.actions.githubusercontent.com"
subject_regexp = "^https://github.com/lauritsk/hatchctl/.+@refs/tags/.+$"
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

`hatchctl` assumes `devcontainer.json` and workspace-local config may come from
a repository you do not fully trust yet.

> [!IMPORTANT]
> Host-affecting behavior is opt-in. If a workspace wants extra host access,
> `hatchctl` stops and tells you which trust flag to add.

Security defaults include:

- Host-side `initializeCommand` is blocked unless you pass
  `--allow-host-lifecycle`
- Repo-controlled backend settings that expand host access are blocked unless
  you pass `--trust-workspace`
- Unsigned images warn by default; enable `HATCHCTL_COSIGN_STRICT=1` to block
  execution
- Unsigned remote `OCI` features fail by default in unattended runs
- Direct tarball features must use `https`, except loopback `http` for local
  development and tests
- The macOS bridge uses only loopback addresses

See the [security policy](./SECURITY.md) for the project security policy and
reporting contact.

## Development

This repository uses [`mise`](https://mise.jdx.dev/) for tooling and task
orchestration.

Common commands:

```sh
mise run fix
mise run lint
mise run test
mise run test:integration
mise run test:coverage
mise run test:race
mise run build
mise run check
mise run release:verify
mise run run -- up
```

For contributor workflow and release details, see
the [contributor guide](./CONTRIBUTING.md).

## Verifying Releases

Release checksums are signed without keys by Cosign using GitHub Actions `OIDC`.

```sh
cosign verify-blob hatchctl_checksums.txt \
  --bundle hatchctl_checksums.txt.sigstore.json \
  --certificate-identity \
    "https://github.com/lauritsk/hatchctl/"\
    ".github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

[badge-ci]: https://img.shields.io/github/actions/workflow/status/lauritsk/hatchctl/ci.yml?style=flat-square&label=CI
[link-actions]: https://github.com/lauritsk/hatchctl/actions/workflows/ci.yml
[badge-release]: https://img.shields.io/github/v/release/lauritsk/hatchctl?style=flat-square
[link-releases]: https://github.com/lauritsk/hatchctl/releases
[badge-go]: https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go
[badge-platform]: https://img.shields.io/badge/platform-macOS%20%7C%20Linux-555?style=flat-square
