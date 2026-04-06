# hatchctl

A Go CLI for Development Containers.

## Status

`hatchctl` is in early bootstrap. The repository currently contains the Go CLI skeleton, local developer tooling, CI, and release automation.

## Development

This repository uses `mise` for tool installation and task orchestration.

Common commands:

- `mise run format`
- `mise run check`
- `go run ./cmd/hatchctl`
- `go run ./cmd/hatchctl --version`

The shell entry hook runs `mise run setup`, which currently downloads Go module dependencies.

## Releases

Releases are versioned with Cocogitto and published with GoReleaser.

Typical flow:

1. make sure the release-worthy changes are committed with Conventional Commits
2. run `mise run release:version`
3. push the resulting release commit and `v*` tag
4. GitHub Actions runs `mise run release` for that tag

Note: with Cocogitto defaults, `mise run release:version` only creates a version when the commit history contains a bump-worthy commit such as `feat:` or `fix:`.

## Verifying Releases

Release checksums are signed with keyless Cosign using GitHub Actions OIDC.

Verify a published release with:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/lauritsk/hatchctl/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Then verify the downloaded artifact against `checksums.txt`.
