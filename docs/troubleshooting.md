# Troubleshooting

## Workspace Is Busy

`hatchctl` writes a coordination lease under the workspace state directory to prevent concurrent mutations.

- wait for the active command to finish if another `hatchctl up`, `build`, or `run` is still in progress
- if the reported PID is stale, inspect the state directory and remove the lock only after confirming no active `hatchctl` process still owns it
- use `hatchctl config --json` to confirm the resolved `stateDir` when you are debugging the wrong workspace

## Trust-Gated Workspace Settings

Some `devcontainer.json` settings are blocked until you explicitly trust the repository.

- rerun with `--trust-workspace` when the repo requests host-affecting Docker settings such as extra binds, privilege, or build contexts outside the workspace
- rerun with `--allow-host-lifecycle` when the repo uses host-side `initializeCommand`
- prefer user-level config for personal defaults; workspace-local `.hatchctl/config.toml` host-affecting settings only apply after `--trust-workspace`

## Unsigned Images Or Features

Image and remote OCI feature verification fail closed in non-interactive runs.

- configure trusted signers in `.hatchctl/config.toml` when you expect a signed remote image or feature source
- check the exact image or feature reference in the error output before adding trust rules
- set `HATCHCTL_ALLOW_INSECURE_FEATURES=1` only for intentional local testing or migration cases

## macOS Bridge Issues

Bridge support is only active on macOS.

- run `hatchctl bridge doctor` first to confirm that the bridge helper can start and that host prerequisites are available
- if browser-open forwarding fails, rerun with `--debug` and inspect the bridge status file under the workspace `stateDir`
- if a previous bridge process is wedged, stop the old process before retrying so `hatchctl up --bridge` can create a fresh session

## Release Verification Failures

`mise run release:verify` checks that generated artifacts stay clean from a detached worktree.

- run `go mod tidy` and `go generate ./...` locally to reproduce the exact failure
- inspect changes to `go.mod`, `go.sum`, and generated files before tagging a release
- review `mise.toml`, `.goreleaser.yaml`, and embedded bridge assets together when release-only failures appear after toolchain updates
