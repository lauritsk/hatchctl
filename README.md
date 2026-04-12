# hatchctl

Run devcontainers from the terminal.

## Overview

`hatchctl` starts a devcontainer-backed workspace, shows the resolved config, and runs commands inside the container without editor tooling.

It is built for people who already use Docker and want a direct CLI for everyday devcontainer tasks like:

- starting a workspace from `devcontainer.json`
- reusing an existing container instead of rebuilding every time
- inspecting merged config and detected runtime state
- running shells, tests, and one-off commands inside the container
- scripting these flows with JSON output

## Install

Install with `mise`:

```sh
mise use github:lauritsk/hatchctl@latest
```

## Requirements

- Docker with a working `docker` CLI on `PATH`
- Docker Compose support through the Docker CLI (`docker compose`)
- a Linux container runtime target for devcontainers
- macOS only if you need browser-open or localhost callback bridge support

`hatchctl` shells out to the Docker CLI.

## Quick Start

Start or reuse the devcontainer for the current workspace:

```sh
hatchctl up
```

Open the default shell inside the container workspace:

```sh
hatchctl exec
```

Run tests inside the container:

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

## Support Matrix

- host OS: macOS and Linux are supported; Windows is not currently supported
- bridge support: browser-open and localhost callback forwarding are macOS only
- container orchestration: single-container image and Dockerfile workflows are supported
- container orchestration: Compose devcontainers are supported for a single service
- automation: JSON output is supported for `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`

## Commands

```sh
hatchctl up
hatchctl up --dotfiles lauritsk/dotfiles
hatchctl up --allow-host-lifecycle
hatchctl up --ssh
hatchctl up --trust-workspace
hatchctl up --feature-timeout 2m
hatchctl build
hatchctl exec
hatchctl exec -- go test ./...
hatchctl config --json
hatchctl run --phase start
hatchctl bridge doctor
```

### Command Guide

- `hatchctl up`: create or reuse the workspace container
- `hatchctl build`: build the devcontainer image without starting the container
- `hatchctl exec`: open the remote user's default shell in the container workspace
- `hatchctl exec -- ...`: run a command inside the container
- `hatchctl config`: show the merged config and detected runtime state
- `hatchctl run --phase ...`: re-run lifecycle steps in an existing container
- `hatchctl bridge doctor`: check whether macOS bridge support is available and healthy

Use `--` with `exec` to separate `hatchctl` flags from the command you want to run in the container.

## Config Files

- user config: platform config dir plus `/hatchctl/config.toml`, usually `~/.config/hatchctl/config.toml` on Linux and `~/Library/Application Support/hatchctl/config.toml` on macOS
- workspace config: `.hatchctl/config.toml`
- workspace values for `workspace`, `state_dir`, `cache_dir`, `bridge`, `ssh`, `dotfiles`, and `verification.trusted_signers` only apply when you also pass `--trust-workspace`
- workspace values for `config`, `feature_timeout`, and `lockfile_policy` apply without extra trust

Dotfiles are configured outside `devcontainer.json`, matching editor tooling. Most users only need `--dotfiles <repo>` or `[dotfiles].repository`. Use `--dotfiles-install-command` and `--dotfiles-target-path` only when the repository needs a custom install step or checkout location. Matching `HATCHCTL_DOTFILES_*` environment variables also work.

Trusted signers are configurable in either config file. When you do not configure signers explicitly, `hatchctl` recommends GitHub Actions keyless signatures from the same GitHub repository for `ghcr.io/<owner>/<repo>` images. Override that with explicit signers when you need a narrower policy:

```toml
[verification]
[[verification.trusted_signers]]
issuer = "https://token.actions.githubusercontent.com"
subject_regexp = "^https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/.+$"
```

Use `--ssh` when you want the container to see the host `ssh-agent` socket. This applies the equivalent of ssh-agent passthrough in `devcontainer.json`. On macOS, hatchctl uses `/run/host-services/ssh-auth.sock` instead of binding the raw launchd socket path. You can persist that in user config, or in workspace config when you also pass `--trust-workspace`.

Remote feature downloads default to a `90s` HTTP timeout. Override that per command with `--feature-timeout`, for example `hatchctl up --feature-timeout 2m`.

Host-side lifecycle commands are gated by default. If a workspace uses `initializeCommand`, rerun with `--allow-host-lifecycle` or set `HATCHCTL_ALLOW_HOST_LIFECYCLE=1` once you trust that repository.

Repo-controlled Docker settings that can expand host access are also gated by default. If a workspace requests custom bind mounts, elevated container privileges, or build paths outside the workspace, review the config first and then rerun with `--trust-workspace` or set `HATCHCTL_TRUST_WORKSPACE=1`.

`--lockfile-policy` controls how remote features are resolved:

- `auto`: use the lockfile when available and refresh it when needed
- `frozen`: fail instead of changing lockfile-backed resolution
- `update`: refresh lockfile-backed resolution

`config` and `bridge doctor` default to `frozen` so inspection commands do not unexpectedly update dependency state.

Use `--bridge` on macOS when the container needs host-side browser open or localhost callback forwarding during auth flows. Forwarded localhost callback ports stay on the original loopback port when available and fall back to a randomized single-use loopback listener only when needed.

## Security Defaults

- `initializeCommand` does not run on the host unless you explicitly opt in with `--allow-host-lifecycle` or `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`
- repo-controlled Docker settings that expand host access do not run unless you explicitly opt in with `--trust-workspace` or `HATCHCTL_TRUST_WORKSPACE=1`
- repo-local `.hatchctl/config.toml` values for `workspace`, `state_dir`, `cache_dir`, `bridge`, `ssh`, `dotfiles`, and `verification.trusted_signers` only apply when you explicitly opt in with `--trust-workspace` or `HATCHCTL_TRUST_WORKSPACE=1`
- direct tarball features must use `https`, except loopback `http` sources used for local development and tests
- unsigned images warn by default and prompt on TTY; pressing Enter selects `N`
- set `HATCHCTL_COSIGN_STRICT=1` to fail closed for unsigned images instead of warning
- unsigned remote OCI features fail by default in non-interactive runs and prompt on TTY; pressing Enter selects `N`
- set `HATCHCTL_ALLOW_INSECURE_FEATURES=1` only when you intentionally want to bypass remote OCI feature verification
- the macOS bridge listener binds to loopback only, and forwarded localhost callback ports stay on the original loopback port when available and otherwise fall back to randomized single-use loopback listeners
- workspace state and cache files are written with owner-only permissions where possible

These defaults are meant to reduce the risk of opening an untrusted repository or consuming an untrusted remote feature source.

## Supported Devcontainer Features

- config discovery for `.devcontainer/devcontainer.json` and `.devcontainer.json`
- JSONC parsing for devcontainer files
- single-container image and Dockerfile workflows
- single-service Compose workflows
- local file-path, OCI, direct tarball, and deprecated GitHub shorthand feature references
- lifecycle execution for `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and `postAttachCommand`; `initializeCommand` is host-side and requires `--allow-host-lifecycle` or `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`
- workspace-scoped state and container reuse
- machine-readable JSON output for `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- dotfiles setup through CLI flags or `HATCHCTL_DOTFILES_*` environment variables

## Development

This repository uses `mise` for local tooling and task orchestration.

Common commands:

- `mise run format`
- `mise run test`
- `mise run test:coverage`
- `mise run build`
- `mise run hatchctl -- help`
- `mise run hatchctl -- up`

## Troubleshooting

See `docs/troubleshooting.md` for common fixes for workspace lock errors, trust-gated config, bridge issues, unsigned image prompts, and release verification failures.

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

Verify a published release with:

```sh
cosign verify-blob hatchctl_checksums.txt \
  --bundle hatchctl_checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```
