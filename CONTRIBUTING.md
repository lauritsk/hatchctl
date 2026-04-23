# Contributing

## Development setup

This repository uses `mise` for local tooling and task orchestration. Assume only `mise` is installed globally.

```bash
mise install
```

`mise install` installs the project tools and hk-managed git hooks. Go commands fetch dependencies as needed. `mise run format`, `mise run lint`, and `mise run fix` all use `hk.pkl` as their source of truth.

## Common tasks

| Goal | Command |
| --- | --- |
| Run hk formatters across the repo | `mise run format` |
| Run the hk pre-commit hook manually | `mise run pre-commit` |
| Run hk lint checks across the repo | `mise run lint` |
| Run hk slow-profile checks across the repo | `mise run lint:slow` |
| Run hk fixers across the repo | `mise run fix` |
| Run the fast test suite | `mise run test` |
| Run tests with coverage | `mise run test:coverage` |
| Run integration tests | `mise run test:integration` |
| Run race-detector tests | `mise run test:race` |
| Build packages | `mise run build` |
| Run the CLI from source | `mise run run -- <args>` |
| Run the full local check suite | `mise run check` |
| Generate an SBOM | `mise run sbom` |

If you touch embedded bridge helper assets, also run `mise run build:bridge-helpers`.

## Commits

Commits must follow Conventional Commits. Cocogitto enforces this locally and in CI.

Create commits through `mise`:

> [!TIP]
> On Git 2.54+, you can also install hk once globally with `hk install --global`. This repo keeps `hk install --mise` in `mise.toml` so `mise install` also wires hooks per clone.


- `mise exec cocogitto -- cog commit <type> "<message>" [scope]`
- add `-B` for breaking changes
- run `mise run check:commits` to validate commit messages locally

Examples:

- `feat: add backend capability detection`
- `fix: preserve localhost redirect handling`
- `docs: clarify release verification`

## Pull requests

Before opening a pull request:

- run `mise run check`
- use a Conventional Commit title for the pull request
- update tests and docs when behavior changes
- keep changes focused and small when possible

If the change is intended to ship in a release, use a bump-worthy commit type such as `feat:` or `fix:`. Commits like `docs:` and `chore:` usually do not produce a version bump with the current Cocogitto defaults.

## Releases

Releases are created from git tags and published through GitHub Actions.

- `mise run release:version` prepares the next release version and tag
- `mise run release:check` validates GoReleaser configuration
- `mise run release:verify` verifies release prerequisites from a clean worktree
- push the resulting commit and `v*` tag to trigger the release workflow
