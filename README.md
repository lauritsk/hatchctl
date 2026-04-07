# hatchctl

A terminal-first Development Containers CLI in Go.

## Overview

`hatchctl` is a Go implementation of a Development Containers workflow with compatibility goals informed by `devcontainer-cli`, while keeping a terminal-first command surface.

The project supports running and inspecting devcontainer-based environments across single-container and Compose-based setups, with feature installation, lifecycle execution, and bridge support where implemented.

## v0.1.0 Scope

The first public release is intended to cover the workflows that are already implemented and tested enough for early adopters to use in real projects:

- single-container image and Dockerfile devcontainer flows
- single-service Compose devcontainer flows
- local, OCI, direct tarball, and deprecated GitHub shorthand feature references
- lifecycle execution, managed-container reuse, and config inspection
- machine-readable JSON output for automation-oriented commands
- macOS bridge support for browser-open forwarding, including the helper binary used inside managed containers

The first release does not aim to be a full drop-in replacement for every `devcontainer-cli` workflow. It is a usable compatibility-focused baseline for supported flows, with additional parity work expected after `v0.1.0`.

## Install

Published binaries will be attached to GitHub Releases.

After downloading the archive for your platform:

```sh
tar -xzf hatchctl_<version>_<os>_<arch>.tar.gz
install ./hatchctl /usr/local/bin/hatchctl
```

If you use the macOS bridge flow, keep the shipped `hatchctl-bridge-helper-linux-<arch>` artifact alongside the release assets. `hatchctl` mounts that helper into managed Linux containers when bridge support is enabled.

## Requirements

- Docker with a working `docker` CLI on `PATH`
- Docker Compose support through the Docker CLI (`docker compose`)
- a Linux container runtime target for devcontainers
- macOS only for the current browser-open bridge support

`hatchctl` shells out to the Docker CLI today, so the local Docker CLI behavior is part of the supported surface.

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
- `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- machine-readable JSON output for `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- lifecycle execution for `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and `postAttachCommand`
- workspace-scoped state and managed container reuse
- mounted bridge helpers plus host bridge runtime for browser-open forwarding on macOS

## Support Matrix

- host OS: macOS is supported, including bridge support
- host OS: Linux is expected to work for non-bridge flows
- host OS: Windows is not currently supported as a first-release target
- container orchestration: single-container devcontainers are supported
- container orchestration: Compose devcontainers are supported for a single service
- automation: human-readable terminal output is supported
- automation: JSON output for selected commands is supported
- bridge support: browser-open forwarding is macOS only
- bridge support: localhost callback forwarding is implemented as part of the macOS bridge feature set

## Upstream Baseline

Behavior and compatibility work in this repository is tracked against `@devcontainers/cli` `v0.85.0-7-g7707502`.

Reference revision:

- `77075028480ba007d4c515564d82ae33ce417a7e`

Known gaps relative to that baseline:

- broader compatibility coverage outside the documented supported surface, especially workflows beyond single-container and single-service Compose usage
- host-platform support outside the documented macOS/Linux expectations

## Commands

```sh
hatchctl up
hatchctl up --feature-timeout 2m
hatchctl build
hatchctl exec -- go test ./...
hatchctl config --json
hatchctl run --phase start
hatchctl bridge doctor
```

Remote feature downloads default to a `90s` HTTP timeout. Override that per command with `--feature-timeout`, for example `hatchctl up --feature-timeout 2m`.

## Compatibility Goals

`hatchctl` targets behavioral compatibility with `devcontainer-cli` for supported configuration and runtime flows, while keeping a cleaner terminal-oriented interface.

## Public Release Notes

Before adopting `hatchctl` broadly, verify it against your own devcontainer configuration if you depend on behavior outside the documented support matrix. The project is being published as an early, usable `v0.1.0`, not as a claim of full `devcontainer-cli` parity.

## Development

This repository uses `mise` for tool installation and task orchestration.

Common commands:

- `mise run format`
- `mise run test`
- `mise run build`
- `go run ./cmd/hatchctl help`
- `go run ./cmd/hatchctl up`

## Releases

Releases are versioned with Cocogitto and published with GoReleaser.

Typical flow:

1. make sure the release-worthy changes are committed with Conventional Commits
2. run `mise run release:version`
3. push the resulting release commit and `v*` tag
4. GitHub Actions runs `mise run release` for that tag

Before cutting `v0.1.0` or any later release, smoke-test the generated artifacts rather than relying only on `go run` from a development checkout.

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

Verify a published release with:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```
