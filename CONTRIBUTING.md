# Contributing

## Development

This repository uses `mise` for local tooling and task orchestration.

Common tasks:

- `mise run format`
- `mise run check`
- `mise run test`
- `mise run test:coverage`
- `mise run test:integration`
- `mise run test:race`
- `mise run check:commits`
- `mise run release:check`
- `mise run release:verify`
- `mise run run -- <args>`

Renovate updates dependencies and GitHub Actions. Regular updates wait at least 7 days before PR creation, then automerge after CI passes. Vulnerability fixes are handled separately and are not delayed. Review `mise.toml` changes carefully when they affect CI, release, or security tooling.

## Commits

Commits must follow Conventional Commits. Cocogitto enforces this in local checks and CI.

Create commits with Cocogitto through `mise`:

- `mise exec cocogitto -- cog commit <type> "<message>" [scope]`
- add `-B` for breaking changes
- run `mise run check:commits` to validate commit messages locally

Examples:

- `feat: add browser bridge support`
- `fix: preserve localhost redirect handling`
- `docs: clarify release verification`

## Pull Requests

Before opening a pull request:

- run `mise run check`
- use a Conventional Commit title for the pull request
- update tests and documentation when behavior changes
- keep changes focused and small when possible

If the change is intended to ship in a release, use a bump-worthy commit type such as `feat:` or `fix:`. Commits like `docs:` and `chore:` usually will not produce a version bump with the current Cocogitto defaults.

## Releases

Releases are created from git tags and published through GitHub Actions.

- `mise run release:version` prepares the next release version and tag
- push the resulting commit and `v*` tag to trigger the release workflow
- `cog.toml` configures Cocogitto to generate `v`-prefixed tags and the repository changelog

`mise run release:version` may legitimately do nothing if the current commit history does not contain a release-worthy Conventional Commit.
