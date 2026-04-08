# hatchctl

A Development Containers CLI in Go.

## Overview

`hatchctl` runs and inspects devcontainer-based environments across single-container and Compose-based setups, with feature installation, lifecycle execution, and bridge support where available.

## Supported Workflows

- single-container image and Dockerfile devcontainer flows
- single-service Compose devcontainer flows
- local, OCI, direct tarball, and deprecated GitHub shorthand feature references
- lifecycle execution, managed-container reuse, and config inspection
- machine-readable JSON output for automation-oriented commands
- macOS bridge support for browser-open forwarding

## Install

After downloading a release archive for your platform:

```sh
tar -xzf hatchctl_<version>_<os>_<arch>.tar.gz
install ./hatchctl /usr/local/bin/hatchctl
```

## Requirements

- Docker with a working `docker` CLI on `PATH`
- Docker Compose support through the Docker CLI (`docker compose`)
- a Linux container runtime target for devcontainers
- macOS only for the current browser-open bridge support

`hatchctl` shells out to the Docker CLI.

## Capabilities

- config discovery for `.devcontainer/devcontainer.json` and `.devcontainer.json`
- JSONC parsing for devcontainer files
- single-container image and Dockerfile flows
- local file-path feature consumption for single-container image and Dockerfile flows
- OCI feature consumption for single-container image and Dockerfile flows
- direct tarball feature consumption for single-container image and Dockerfile flows
- minimal Compose config discovery, build, up, exec, and reuse for a single service
- ephemeral Compose override-file generation for mounts, env, labels, and command behavior
- Compose feature consumption for image-based and Dockerfile-based single-service flows
- Compose bridge support and Compose UID/GID remap parity for single-service flows
- config-adjacent feature lockfiles with digest and integrity reuse
- first-class dotfiles personalization through CLI flags or `HATCHCTL_DOTFILES_*` env vars
- `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- machine-readable JSON output for `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- lifecycle execution for `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and `postAttachCommand`
- workspace-scoped state and managed container reuse
- browser-open forwarding on macOS bridge sessions

## Support Matrix

- host OS: macOS is supported, including bridge support
- host OS: Linux is supported for non-bridge flows
- host OS: Windows is not currently supported
- container orchestration: single-container devcontainers are supported
- container orchestration: Compose devcontainers are supported for a single service
- automation: human-readable terminal output is supported
- automation: JSON output for selected commands is supported
- bridge support: browser-open forwarding is macOS only
- bridge support: localhost callback forwarding is implemented as part of the macOS bridge feature set

## Commands

```sh
hatchctl up
hatchctl up --dotfiles lauritsk/dotfiles
hatchctl up --feature-timeout 2m
hatchctl build
hatchctl exec -- go test ./...
hatchctl config --json
hatchctl run --phase start
hatchctl bridge doctor
```

Dotfiles are configured outside `devcontainer.json`, matching how editor tooling treats them. Most users only need `--dotfiles <repo>`. `--dotfiles-repository`, `--dotfiles-install-command`, and `--dotfiles-target-path` are available when you need more control, and each has a matching `HATCHCTL_DOTFILES_*` environment variable.

Remote feature downloads default to a `90s` HTTP timeout. Override that per command with `--feature-timeout`, for example `hatchctl up --feature-timeout 2m`.

## Development

This repository uses `mise` for local tooling and task orchestration.

Common commands:

- `mise run format`
- `mise run test`
- `mise run build`
- `mise run hatchctl -- help`
- `mise run hatchctl -- up`

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

Verify a published release with:

```sh
cosign verify-blob hatchctl_checksums.txt \
  --bundle hatchctl_checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```
