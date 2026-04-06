# hatchctl

A terminal-first Development Containers CLI in Go.

## Status

`hatchctl` now contains the first real rewrite slice for a `devcontainer-cli` replacement.

Implemented now:

- config discovery for `.devcontainer/devcontainer.json` and `.devcontainer.json`
- JSONC parsing for devcontainer files
- single-container image and Dockerfile flows
- `up`, `build`, `exec`, `config`, `run`, and `bridge doctor`
- lifecycle execution for `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and `postAttachCommand`
- workspace-scoped state and managed container reuse
- bridge scaffolding for the future mounted helper approach

Deferred to the next slices:

- full metadata merge parity
- feature consumption parity
- Compose support
- full bridge proxy/runtime implementation
- richer UI and verbosity modes

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

The rewrite is targeting behavioral compatibility with `devcontainer-cli` for the files it supports, while adopting a cleaner terminal-first command surface.

Current scope:

- single-container runtime workflows first
- Compose second
- features and authoring workflows after runtime parity is stable

`../cli` is the reference implementation during the rewrite.

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
