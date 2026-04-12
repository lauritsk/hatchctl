# Security Policy

## Reporting a Vulnerability

Do not open a public GitHub issue for security-sensitive reports.

Report vulnerabilities by emailing `security@solisec.org` with:

- a description of the issue
- affected versions or commits
- reproduction steps or a proof of concept
- any suggested mitigation

You can expect an initial response within 5 business days.

## Supported Versions

Security fixes are provided for the latest released version.

Before the first stable release, fixes may be made on `main` without backporting to older tags.

## Current Security Model

`hatchctl` runs developer-controlled containers and reads repository-controlled `devcontainer.json` files, so the trust boundary is important.

Current defaults:

- host-side `initializeCommand` is blocked unless the user explicitly opts in with `--allow-host-lifecycle` or `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`
- repo-local `.hatchctl/config.toml` values for `workspace`, `state_dir`, `cache_dir`, `bridge`, `ssh`, `dotfiles`, and `verification.trusted_signers` are ignored unless the user explicitly opts in with `--trust-workspace` or `HATCHCTL_TRUST_WORKSPACE=1`
- direct tarball features must use `https`, except loopback `http` sources for local development and tests
- unsigned images warn by default; set `HATCHCTL_COSIGN_STRICT=1` to fail closed instead
- unsigned remote OCI features are rejected by default
- the macOS bridge listener binds to loopback only, and forwarded localhost callback ports stay on the original loopback port when available and otherwise fall back to randomized single-use loopback listeners
- workspace state and cache artifacts are written with owner-only permissions where practical

Environment overrides:

- `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`: allow trusted repositories to run host-side lifecycle commands
- `HATCHCTL_ALLOW_INSECURE_FEATURES=1`: allow insecure or unsigned remote feature sources when you intentionally accept that risk
- `HATCHCTL_COSIGN_STRICT=1`: require signed images instead of warning

These overrides are intended for trusted local workflows, migration, and testing. Do not enable insecure settings broadly in shared or untrusted environments.
