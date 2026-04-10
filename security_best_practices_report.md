# Security Best Practices Report

## Executive Summary

This review covered the Go CLI trust boundaries around repo-controlled configuration, host capability injection, remote feature verification, local bridge forwarding, filesystem state, and CI/dependency hygiene.

No critical findings were identified. The original high-severity finding around repo-controlled `.hatchctl/config.toml` host-affecting defaults, the medium-severity finding around unsigned loopback OCI feature bypass, and the medium-severity finding around reusable bridge-forwarded localhost ports have all been remediated in the current working tree.

Dependency and secret scanning were clean at audit time: `mise run audit` reported no Go vulnerabilities and `mise run secrets` reported no leaks.

## Critical Findings

No critical findings were identified in this review.

## High Findings

### HBP-001

- Severity: High
- Status: Fixed in current working tree
- Location: `internal/appconfig/config.go:32-69`, `internal/appconfig/config.go:174-183`, `internal/app/defaults.go:83-141`, `internal/plan/workspace.go:122-131`, `internal/reconcile/commands_executor.go:94-109`, `internal/reconcile/lifecycle_executor.go:121-175`
- Evidence:

```go
// internal/appconfig/config.go:60-68,174-176
if path, ok, err := workspaceConfigPath(workspace); err != nil {
    return Config{}, err
} else if ok {
    cfg, err := Load(path)
    if err != nil {
        return Config{}, err
    }
    merged = merge(merged, cfg)
}

func workspaceConfigPath(workspace string) (string, bool, error) {
    path := filepath.Join(workspace, ".hatchctl", "config.toml")
```

```go
// internal/app/defaults.go:116-139
if !req.Dotfiles.Repository.Changed && config.Dotfiles.Repository != "" {
    resolved.Dotfiles.Repository = config.Dotfiles.Repository
}
...
if req.SSHAgent != nil {
    resolved.SSHAgent = req.SSHAgent.Value
    if !req.SSHAgent.Changed && config.SSHAgent != nil {
        resolved.SSHAgent = *config.SSHAgent
    }
}
```

```go
// internal/plan/workspace.go:127-131
Capabilities: capability.Set{
    SSHAgent: capability.SSHAgent{Enabled: req.SSHAgent},
    UIDRemap: capability.UIDRemap{Enabled: workspaceSpec.Merged.UpdateRemoteUserUID == nil || *workspaceSpec.Merged.UpdateRemoteUserUID},
    Dotfiles: capability.Dotfiles{Repository: req.Dotfiles.Repository, InstallCommand: req.Dotfiles.InstallCommand, TargetPath: req.Dotfiles.TargetPath},
    Bridge:   capability.Bridge{Enabled: req.BridgeEnabled},
},
```

```go
// internal/reconcile/commands_executor.go:94-109
if workspacePlan.Capabilities.SSHAgent.Enabled {
    if resolved.Merged, err = injectSSHAgent(resolved.Merged); err != nil {
        return UpResult{}, err
    }
}
...
if workspacePlan.Capabilities.Bridge.Enabled {
    bridgeSession, err = bridgecap.Prepare(resolved.StateDir, helperArch)
    if err == nil {
        resolved.Merged = bridgecap.Inject(bridgeSession, resolved.Merged)
    }
}
```

```go
// internal/reconcile/lifecycle_executor.go:121-123,168-175
if runDotfiles && dotfiles.Enabled() && !capdot.StateMatches(state, dotfiles) {
    if err := e.installDotfiles(ctx, observed, dotfiles, events); err != nil {
        return err
    }
}
...
req, err := e.DockerExecRequest(ctx, observed, true, false, nil, capdot.InstallArgs(cfg.Repository, targetPath, cfg.InstallCommand), ...)
...
return e.engine.Exec(ctx, req)
```

- Impact: A repository can ship `.hatchctl/config.toml` and cause `hatchctl up` to inherit repo-controlled SSH agent passthrough, bridge support, and dotfiles execution without requiring the explicit `--trust-workspace` or `--allow-host-lifecycle` approvals used elsewhere for repo-controlled trust expansion.
- Fix: Treat workspace-local `.hatchctl/config.toml` as untrusted input for host-affecting options. At minimum, ignore or prompt for repo-local `ssh`, `bridge`, and `dotfiles` settings unless the user explicitly opts in. User-level config can remain automatic.
- Mitigation: Until fixed, avoid running `hatchctl up` against untrusted repositories that contain `.hatchctl/config.toml`, especially when an active host `ssh-agent` is available.
- False positive notes: If the intended trust model explicitly considers workspace-local `.hatchctl/config.toml` equivalent to approved local user configuration, document that assumption clearly. The current README and security defaults do not make that boundary explicit.

## Medium Findings

### HBP-002

- Severity: Medium
- Status: Fixed in current working tree
- Location: `internal/reconcile/executor.go:208-214`, `internal/policy/feature_verification.go:13-24`, `README.md:133`, `SECURITY.md:30`
- Evidence:

```go
// internal/reconcile/executor.go:208-214
for _, feature := range resolved.Features {
    allowUnverified := feature.SourceKind == "oci" && (policy.AllowInsecureFeatureVerification() || policy.IsLoopbackOCIReference(feature.Resolved))
    if err := e.imageVerifier.ApplyFeature(feature.Source, feature.Verification, allowUnverified, events); err != nil {
        return err
    }
}
```

```go
// internal/policy/feature_verification.go:13-24
func IsLoopbackOCIReference(ref string) bool {
    ref = strings.TrimSpace(ref)
    if ref == "" {
        return false
    }
    host, _, ok := strings.Cut(ref, "/")
    if !ok {
        return false
    }
    host = strings.ToLower(host)
    return host == "localhost" || strings.HasPrefix(host, "localhost:") || strings.HasPrefix(host, "127.0.0.1:")
}
```

```md
<!-- README.md:133 -->
- unsigned remote OCI features fail by default in non-interactive runs and prompt on TTY; pressing Enter selects `N`

<!-- SECURITY.md:30 -->
- unsigned remote OCI features are rejected by default
```

- Impact: Any OCI feature resolved from `localhost` or `127.0.0.1` bypasses signature verification automatically, which weakens the documented default and allows unsigned feature delivery from a local registry without user approval.
- Fix: Remove the implicit loopback bypass, or gate it behind the same explicit insecure-feature opt-in used elsewhere. If loopback unsigned features must remain supported for local development, document the exception prominently in `README.md` and `SECURITY.md` and make the CLI warn when it is exercised.
- Mitigation: Avoid loopback OCI feature sources unless you control the local registry process and are intentionally testing unsigned content.
- False positive notes: If loopback registries are considered a trusted local-development escape hatch by design, the implementation still needs matching documentation because the current docs state a stricter default than the code enforces.

### HBP-003

- Severity: Medium
- Status: Fixed in current working tree
- Location: `internal/bridge/runtime.go:192-217`, `internal/bridge/runtime.go:272-295`, `internal/bridge/runtime.go:298-338`, `internal/bridge/runtime.go:357-363`
- Evidence:

```go
// internal/bridge/runtime.go:192-205
case "open":
    if s.session.Token != "" && request.Token != s.session.Token {
        _ = writeBridgeResponse(conn, bridgeResponse{Error: "unauthorized"})
        return
    }
    ...
    rewritten, err := s.rewriteLocalURL(request.URL)
```

```go
// internal/bridge/runtime.go:289-295
hostPort, _, err := s.forwardURL(port)
...
rewritten := *parsed
rewritten.Host = net.JoinHostPort(host, strconv.Itoa(hostPort))
return &rewritten, true, nil
```

```go
// internal/bridge/runtime.go:306-329,332-337,357-363
listener, exact, err := listenForwardPort(port)
...
go func() {
    for {
        conn, err := listener.Accept()
        if err != nil {
            return
        }
        go s.handleForwardConn(port, conn)
    }
}()

func (s *bridgeHostService) handleForwardConn(port int, conn net.Conn) {
    defer conn.Close()
    if err := s.connectPort(s.containerID, port, conn, conn); err != nil {
        ...
    }
}

func listenForwardPort(port int) (net.Listener, bool, error) {
    listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
    ...
    listener, err = net.Listen("tcp", "127.0.0.1:0")
```

- Impact: Once the bridge creates a forwarded loopback port, any local process on the host can connect to it; the bridge token protects creating the forward, but not consuming traffic on the forwarded port. On shared hosts this can expose bridged localhost services or OAuth callback traffic to other local users/processes.
- Fix: Add an application-layer authentication step for forwarded connections, or narrow the feature to flows where the redirected URL includes a strong nonce that the receiving side validates. If that is not practical, document the shared-host limitation clearly and prefer exact one-shot callback handling over general port forwarding.
- Mitigation: Treat bridge support as single-user workstation functionality. Avoid enabling it on multi-user systems or remote shared build hosts.
- False positive notes: If the product threat model only covers single-user local workstations, this is reduced to a documented limitation rather than a direct vulnerability. The current docs mention loopback binding, but not the same-host multi-user caveat.

## Low Findings

No low-severity findings were included beyond the items above.

## Positive Controls

- Host-side `initializeCommand` is blocked unless the user explicitly opts in: `internal/policy/lifecycle.go:17-21`
- Repo-controlled Docker privilege expansion is gated behind explicit trust: `internal/policy/workspace.go:17-36`
- Tarball features require `https` or loopback `http`, and registry token realms must stay on the same host with `https` or loopback `http`: `internal/featurefetch/featurefetch.go:650-661`, `internal/featurefetch/featurefetch.go:683-697`
- Bridge state and session artifacts are written with owner-only permissions: `internal/store/fs/bridge.go:33-49`, `internal/store/fs/bridge.go:63-103`, `internal/fileutil/atomic.go:16-44`
- OCI signature verification is implemented and wired into feature resolution: `internal/security/cosign.go:37-67`, `internal/policy/verification.go:79-87`
- CI runs the repository security checks, including `govulncheck` and `gitleaks`: `mise.toml:60-68`, `mise.toml:101-103`, `.github/workflows/ci.yml:41`

## Scan Results

- `mise run audit`: no Go vulnerabilities found at audit time.
- `mise run secrets`: no secrets or credential leaks found at audit time.

## Recommended Next Steps

1. Keep bridge support focused on auth-style localhost callbacks rather than general reusable localhost browsing.
2. Re-run `mise run audit` and `mise run secrets` as part of regular release verification.
