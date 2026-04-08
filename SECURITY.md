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
- direct tarball features must use `https`, except loopback `http` sources for local development and tests
- unsigned remote OCI features are rejected by default
- the macOS bridge listener binds to loopback only
- workspace state and cache artifacts are written with owner-only permissions where practical

Escape hatches:

- `HATCHCTL_ALLOW_HOST_LIFECYCLE=1`: allow trusted repositories to run host-side lifecycle commands
- `HATCHCTL_ALLOW_INSECURE_FEATURES=1`: allow insecure or unsigned remote feature sources when you intentionally accept that risk

These escape hatches are intended for trusted local workflows, migration, and testing. They should not be enabled broadly in shared or untrusted environments.
