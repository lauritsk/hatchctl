# hatchctl

Run devcontainers from the terminal.

[![CI](https://img.shields.io/github/actions/workflow/status/lauritsk/hatchctl/ci.yml?style=flat-square&label=CI)](https://github.com/lauritsk/hatchctl/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/lauritsk/hatchctl?style=flat-square)](https://github.com/lauritsk/hatchctl/releases)
![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)
![macOS and Linux](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-555?style=flat-square)

[Install](#install) • [Quick start](#quick-start) • [Commands](#commands) • [Configuration](#configuration) • [Security model](#security-model) • [Development](#development)

`hatchctl` is a Go CLI for creating, inspecting, and using devcontainer-based workspaces without depending on editor integration. It is built for terminal-first workflows: start a workspace, open a shell, inspect resolved config, and re-run lifecycle hooks from the command line.

> [!NOTE]
> `hatchctl` supports macOS and Linux. The optional bridge for browser-open and localhost callback forwarding is macOS-only.

## Why hatchctl

- **Terminal-first devcontainers**: use devcontainers without opening VS Code
- **Repeatable workflows**: build, start, inspect, and exec through a single CLI
- **Script-friendly output**: use JSON output for automation and CI-style tooling
- **Backend flexibility**: supports Docker by default, with Podman support available
- **Safer defaults**: trust-sensitive repo settings stay gated behind explicit opt-in flags

## Install

Install with `mise`:

```sh
mise use github:lauritsk/hatchctl@latest
```

Install from source:

```sh
go install github.com/lauritsk/hatchctl/cmd/hatchctl@latest
```

Prebuilt binaries for macOS and Linux are published on the [GitHub Releases](https://github.com/lauritsk/hatchctl/releases) page.

## Requirements

`hatchctl` shells out to a container backend instead of talking to an engine API directly.

You will need:

- Docker or Podman available on `PATH`
- a Linux container runtime target for devcontainers
- Compose support from your selected backend when using compose-based devcontainers
- macOS if you want bridge support

## Quick start

Create or reuse the devcontainer for the current workspace:

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

Re-run lifecycle hooks:

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
> Use `--` with `exec` to separate `hatchctl` flags from the command you want to run inside the container.

## Commands

| Command | What it does |
| --- | --- |
| `hatchctl up` | Resolve config, build if needed, and create or reconnect to the managed devcontainer |
| `hatchctl build` | Build the devcontainer image without starting it |
| `hatchctl exec` | Open a shell or run a command inside the managed container |
| `hatchctl config` | Show merged config and detected runtime state |
| `hatchctl run` | Re-run devcontainer lifecycle phases in an existing container |
| `hatchctl bridge doctor` | Inspect bridge availability and current bridge session state |
| `hatchctl version` | Print version information |

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

Workspace config is intentionally limited until you explicitly trust the repository.

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

## Security model

`hatchctl` assumes `devcontainer.json` and workspace-local config may come from a repository you do not fully trust yet.

> [!IMPORTANT]
> Host-affecting behavior is opt-in. If a workspace wants extra host access, `hatchctl` stops and tells you which trust flag to add.

Security defaults include:

- host-side `initializeCommand` is blocked unless you pass `--allow-host-lifecycle`
- repo-controlled backend settings that expand host access are blocked unless you pass `--trust-workspace`
- unsigned images warn by default; set `HATCHCTL_COSIGN_STRICT=1` to fail closed
- unsigned remote OCI features fail by default in non-interactive runs
- direct tarball features must use `https`, except loopback `http` for local development and tests
- the macOS bridge binds to loopback only

See [SECURITY.md](./SECURITY.md) for the project security policy and reporting contact.

## Development

This repository uses [`mise`](https://mise.jdx.dev/) for tooling and task orchestration.

Common commands:

```sh
mise run format
mise run lint
mise run test
mise run test:integration
mise run test:coverage
mise run test:race
mise run build
mise run audit
mise run check
mise run release:verify
mise run run -- up
```

For contributor workflow and release details, see [CONTRIBUTING.md](./CONTRIBUTING.md).

## Troubleshooting

See [docs/troubleshooting.md](./docs/troubleshooting.md) for help with:

- workspace locks and busy state
- trust-gated workspace settings
- unsigned images or features
- macOS bridge issues
- release verification failures

## Verifying releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

```sh
cosign verify-blob hatchctl_checksums.txt \
  --bundle hatchctl_checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```