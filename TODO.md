# TODO

## Not Done

- split `internal/runtime.Runner` into a small orchestration layer plus focused services such as planner, image manager, container manager, lifecycle runner, bridge manager, and state store so command flows stop accumulating cross-cutting responsibilities in one type
- make devcontainer resolution side-effect free and move lockfile, feature-state, plan-cache, workspace-state, and bridge-state persistence behind explicit store interfaces so read/inspect paths do not perform hidden writes
- introduce a narrow container-engine/process-execution abstraction that centralizes Docker/Compose/process calls, logging, env policy, and future backend support instead of mixing `docker.Client`, direct `exec.Command`, and bridge-specific process spawning paths
- move security verification policy to the CLI/runtime boundary and make `internal/security` return structured verification results instead of reading env vars and printing warnings directly, so trust behavior is explicit, testable, and consistent across image and feature flows
- isolate bridge support behind a narrower subsystem contract that separates planning, config injection, state persistence, and daemon/runtime behavior so optional macOS-specific behavior stops leaking through core runtime orchestration
- replace hatchctl-owned shell-script assembly for UID/GID reconciliation, dotfiles installation, exec-home discovery, and feature build wiring with typed operations or fixed helper scripts/binaries where possible to reduce quoting, portability, and injection risk
- centralize atomic file writes and recovery semantics for stateful artifacts such as workspace state, resolved plan cache, bridge session/config/status files, and helper shims so partial writes or interruptions cannot leave corrupted control files behind
- fix `TestUpInstallsDotfilesOnceAndReportsStatus` so it does not clone dotfiles from a bind-mounted `file://` repo that trips git `safe.directory` checks in CI; re-enable the test after switching the fixture to a CI-safe transport
- evaluate a hybrid Docker integration that uses the Engine API for inspect/start/exec-heavy paths while keeping Compose behavior compatible where the CLI is still the best fit
- add first-class `config.toml` support with XDG/Linux and macOS-compliant config discovery plus fallback standard locations, and define a clear merge/override hierarchy where broader defaults are overridden by user, workspace, and CLI-nearest config/options; ensure cache/state/artifact outputs also follow platform best practices

## Done

- [x] add first-class dotfiles personalization support with explicit CLI UX and one-time install tracking
- [x] move UID/GID reconciliation out of derived image rebuilds and into a runtime-oriented approach where possible
- [x] redesign bridge transport around a persistent localhost-only control/data channel so browser-open and localhost callback auth flows still work without a detached wide-bind HTTP server or per-connection `docker exec`
- [x] harden single-container metadata parity
- [x] persist merged `devcontainer.metadata` on built images and managed containers
- [x] add Docker-backed integration tests for image labels, merged env, merged users, and lifecycle ordering
- [x] support `forwardPorts` and expose bridge-related runtime wiring through merged config
- [x] improve `read config` output so it can inspect both local config and running container state
- [x] implement `remoteUser` / image-user fallback more precisely from inspected image and container state
- [x] tighten `exec` TTY, stdin, and exit-code behavior to match `devcontainer-cli` more closely
- [x] implement UID/GID update flow parity for existing non-root users
- [x] add stronger state reconciliation for reused, stopped, and recreated containers
- [x] implement mounted bridge helper binary inside the container
- [x] implement full host-side bridge runtime for browser opens and localhost callback forwarding on macOS
- [x] add bridge integration tests covering first start and container reuse
- [x] implement feature consumption support for single-container image and Dockerfile flows
- [x] add feature ordering, metadata persistence, and lifecycle-hook merge behavior
- [x] add tests for feature-installed tools, env, mounts, and lifecycle hooks
- [x] add broader regression tests for config discovery, JSONC parsing, and related file parsing to prevent drift from `devcontainer-cli`
- [x] implement Compose config discovery and parsing
- [x] implement Compose single-service runtime parity
- [x] implement Compose container reuse and ephemeral override-file generation behavior
- [x] add Compose integration tests for image and Dockerfile services
- [x] implement Compose bridge support
- [x] implement Compose UID/GID remap parity
- [x] add config-adjacent feature lockfile parity with digest pinning and reuse behavior
- [x] add direct tarball remote feature support
- [x] add lockfile policy controls such as frozen/update-only workflows
- [x] add deprecated GitHub shorthand remote feature reference support
- [x] improve command UX, progress output, and verbose/debug modes
- [x] add structured JSON output parity where it is useful for automation
- [x] add more real-file regression fixtures for config discovery, compose override arrays/precedence, Dockerfile/Containerfile/context edge cases, and remote feature lockfile reuse behavior
- [x] document supported compatibility surface and known gaps
- [x] document the latest `devcontainer-cli` version/revision this project was synced against so future sync passes can review only newer upstream changes
- [x] decide first public release scope and cut `v0.1.0` only when single-container runtime, bridge, and feature consumption are solid enough for real use
- [x] persist or cache a resolved workspace/runtime plan so `up`, `build`, `exec`, `config`, and lifecycle commands stop re-resolving and re-inspecting the same state repeatedly
- [x] verify container signature with cosign by default (if possible)
- [x] consider more security improvements / secure-by-default things that could be implemented in this project
- [x] harden feature option handling so feature env values are treated as data, not shell code during image builds
- [x] tighten bridge security by writing token-bearing files with owner-only permissions and narrowing listener exposure where possible
- [x] thread `context.Context` through config and feature resolution, use `signal.NotifyContext`, and add sane HTTP client timeouts
- [x] stream remote feature downloads or enforce size limits instead of buffering full artifacts in memory
- [x] replace hand-built Compose override YAML with typed marshaling and preserve mount semantics such as read-only/options
- [x] reduce runtime coupling to global stdio/process spawning by passing runner-level I/O or logging dependencies explicitly
- [x] consolidate repeated resolve/inspect/enrich flows behind a smaller internal prepared-plan helper
- [x] remove leftover compile anchors and cleanup artifacts like `var _ = bridge.Report{}` once no longer needed
- [x] deduplicate map-sorting and other tiny helper utilities where modern stdlib/shared helpers already cover the need
- [x] consider whether Charm CLI tooling could improve usability, visuals, and simplify parts of the command UX
