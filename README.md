# hatchctl

A terminal-first Development Containers CLI in Go.

## Overview

`hatchctl` is a Go implementation of a Development Containers workflow with compatibility goals informed by `devcontainer-cli`, while keeping a terminal-first command surface.

The project supports running and inspecting devcontainer-based environments across single-container and Compose-based setups, with feature installation, lifecycle execution, and bridge support where implemented.

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

## Upstream Baseline

Behavior and compatibility work in this repository is tracked against `@devcontainers/cli` `v0.85.0-7-g7707502`.

Reference revision:

- `77075028480ba007d4c515564d82ae33ce417a7e`

Known gaps relative to that baseline:

- deprecated GitHub shorthand feature references and fuller lockfile policy controls
- broader compatibility documentation and automation-oriented output parity

## Commands

```sh
hatchctl up
hatchctl build
hatchctl exec -- go test ./...
hatchctl config --json
hatchctl run --phase start
hatchctl bridge doctor
```

## Compatibility Goals

`hatchctl` targets behavioral compatibility with `devcontainer-cli` for supported configuration and runtime flows, while keeping a cleaner terminal-oriented interface.

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

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

Verify a published release with:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```
