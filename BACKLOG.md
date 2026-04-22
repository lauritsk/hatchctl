# Backlog

## Simplify architecture

1. Move config parsing, metadata merging, mount parsing, and workspace-spec resolution into `internal/devcontainer`; leave `internal/spec` as temporary compatibility shim until callers move.
2. Fold `internal/plan` into workspace/devcontainer flow so command setup has one source of truth.
3. Collapse `internal/app` and `internal/appconfig` into one workspace/defaults package.
4. Reshape `internal/reconcile` around one session-driven runtime pipeline instead of many cross-file helper hops.
5. Merge backend factory/runtime descriptors so Docker and Podman setup live in one place.
