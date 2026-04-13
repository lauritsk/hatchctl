# hatchctl

Run devcontainers from the terminal.

[![CI](https://img.shields.io/github/actions/workflow/status/lauritsk/hatchctl/ci.yml?style=flat-square&label=CI)](https://github.com/lauritsk/hatchctl/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/lauritsk/hatchctl?style=flat-square)](https://github.com/lauritsk/hatchctl/releases)
![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)
![macOS and Linux](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-555?style=flat-square)

[Install](#install) • [Quick Start](#quick-start) • [Configuration](#configuration) • [Security Defaults](#security-defaults) • [Development](#development)

`hatchctl` is a Go CLI for creating, inspecting, and using devcontainer-based workspaces without editor integration. It is built around a backend-neutral runtime layer, with Docker as the default backend and Podman as an additional supported backend.

> [!NOTE]
> `hatchctl` supports macOS and Linux. Windows is not currently supported. Browser-open and localhost callback bridge support are macOS-only.

## Why hatchctl

- Start or reuse a devcontainer from the terminal with `hatchctl up`
- Open a shell or run one-off commands with `hatchctl exec`
- Inspect merged config and runtime state with `hatchctl config`
- Re-run lifecycle phases without opening an editor with `hatchctl run`
- Script devcontainer workflows with JSON output
- Keep trust-sensitive behavior gated by explicit opt-in flags

## Install

Install with `mise`:

```sh
mise use github:lauritsk/hatchctl@latest
```

Install from source with Go:

```sh
go install github.com/lauritsk/hatchctl/cmd/hatchctl@latest
```

Prebuilt binaries for macOS and Linux are published on the [GitHub Releases](https://github.com/lauritsk/hatchctl/releases) page.

## Requirements

- A supported container backend on `PATH`
- Docker support through the `docker` CLI, including `docker compose` for project-service devcontainers
- Podman support through the `podman` CLI, including `podman compose` for project-service devcontainers
- A Linux container runtime target for devcontainers
- macOS if you need browser-open or localhost callback bridge support

`hatchctl` shells out through the selected container backend rather than talking to an engine API directly.

## Quick Start

Create or reuse the devcontainer for the current workspace:

```sh
hatchctl up
```

Open the default shell inside the container:

```sh
hatchctl exec
```

Run a command inside the container:

```sh
hatchctl exec -- go test ./...
```

Inspect the merged config and detected runtime state:

```sh
hatchctl config
```

Use JSON output in scripts:

```sh
hatchctl up --json
```

> [!TIP]
> Use `--` with `exec` to separate `hatchctl` flags from the command you want to run in the container.

## Commands

| Command | Purpose |
| --- | --- |
| `hatchctl up` | Create or reuse the managed devcontainer |
| `hatchctl build` | Build the devcontainer image without starting it |
| `hatchctl exec` | Open a shell or run a command inside the container |
| `hatchctl config` | Show merged config and detected runtime state |
| `hatchctl run --phase <phase>` | Re-run lifecycle phases in an existing container |
| `hatchctl bridge doctor` | Check macOS bridge availability and status |

Common examples:

```sh
hatchctl --backend auto up
hatchctl up --dotfiles lauritsk/dotfiles
hatchctl up --ssh
hatchctl up --trust-workspace --allow-host-lifecycle
hatchctl build --json
hatchctl exec --env CI=1 -- sh -lc 'make test'
hatchctl run --phase start
```

## Configuration

`hatchctl` reads configuration from:

- user config: `~/.config/hatchctl/config.toml` on Linux, `~/Library/Application Support/hatchctl/config.toml` on macOS
- workspace config: `.hatchctl/config.toml`

Workspace config is intentionally limited by default. Host-affecting settings such as `workspace`, `state_dir`, `cache_dir`, `bridge`, `ssh`, `dotfiles`, and `verification.trusted_signers` only apply when you explicitly trust the repository with `--trust-workspace` or `HATCHCTL_TRUST_WORKSPACE=1`.

Example:

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
subject_regexp = "^https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/.+$"
```

Useful environment variables:

- `HATCHCTL_TRUST_WORKSPACE=1`
- `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`
- `HATCHCTL_COSIGN_STRICT=1`
- `HATCHCTL_ALLOW_INSECURE_FEATURES=1`
- `HATCHCTL_DOTFILES_REPOSITORY`
- `HATCHCTL_DOTFILES_INSTALL_COMMAND`
- `HATCHCTL_DOTFILES_TARGET_PATH`

## Security Defaults

`hatchctl` assumes `devcontainer.json` and repo-local config may come from repositories you do not fully trust yet.

- Host-side `initializeCommand` is blocked unless you opt in with `--allow-host-lifecycle`
- Repo-controlled container backend settings that expand host access are blocked unless you opt in with `--trust-workspace`
- Unsigned images warn by default; set `HATCHCTL_COSIGN_STRICT=1` to fail closed
- Unsigned remote OCI features fail by default in non-interactive runs
- Direct tarball features must use `https`, except loopback `http` for local development and tests
- The macOS bridge binds to loopback only

See [SECURITY.md](./SECURITY.md) for the full policy and threat model.

## Support Matrix

- Host OS: macOS and Linux
- Bridge support: macOS only
- Container backend support: Docker and Podman
- Devcontainer sources: single-container image and build-file workflows
- Project-service support: single-service Compose devcontainers through Docker backend
- Feature sources: local path, OCI, direct tarball, and deprecated GitHub shorthand
- JSON output: `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`

## Development

This repository uses `mise` for local tooling and task orchestration.

Common commands:

```sh
mise run check
mise run test
mise run test:coverage
mise run test:race
mise run build
mise run hatchctl -- up
```

For contributor workflow, commits, and release process details, see [CONTRIBUTING.md](./CONTRIBUTING.md).

## Troubleshooting

See [docs/troubleshooting.md](./docs/troubleshooting.md) for common fixes around workspace locks, trust-gated config, bridge issues, unsigned images, and release verification failures.

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

```sh
cosign verify-blob hatchctl_checksums.txt \
  --bundle hatchctl_checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```
