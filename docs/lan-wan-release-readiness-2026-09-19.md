# LAN/WAN release readiness — 2026-09-19

The release criterion is secure and reliable operation of the implemented
self-hosted direct HTTPS and trusted reverse-proxy modes. Invitations, QR
enrollment, MFA, automatic firewall management and richer diagnostics remain
roadmap work; they are not blockers for this update.

## Defects corrected

1. **Revoked administrator writes.** An administrator could start a request,
   withhold its body, be disabled by another administrator, then finish that
   request to create a replacement administrator or re-enable itself. The
   review reproduced the durable mutation in all 12 real TLS HTTP/1.1 and
   HTTP/2 attempts. Account writes now validate the acting session, enabled
   account, role and session deadlines inside their write transaction. Control
   permission writes use the same check. Network writes bind current login and
   password proof to their transaction; unauthenticated initial local saves
   recheck the absence of accounts. Account disabling returns the exact retired
   session keys for immediate request/controller/gateway cancellation before
   acknowledgement, including self-disabling with a bounded response.
2. **Old packaged Go runtime.** Inspection of PR269's actual portable executable
   confirmed Go 1.25.0. A concrete applicable advisory was
   [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090), TLS post-handshake CPU
   denial of service before application authentication. `go.mod` now pins the
   supported maintenance release Go 1.26.8; every workflow reads that file and
   source installation requires the same minimum. CI and release quality scan
   source with pinned `govulncheck` v1.8.0, and the Windows artifact verifier
   checks compiler metadata and scans all four exact packaged Go executables.

Fully stripped Windows binaries do not expose the symbols required for precise
binary vulnerability analysis. The official scanner conservatively reports
every vulnerable package in each dependency module in that case. Release builds
now keep the compact symbol table and remove DWARF debug information (`-w`),
allowing scans of the actual shipped code without suppressing findings or
whitelisting advisory IDs. The core remains below its 30 MB budget; see the
[scorecard](goal-scorecard.md). See the scanner's
[documented limitations](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck#hdr-Limitations).

## Regression and acceptance evidence

- Account tests use uncancelled contexts to reject revoked, disabled, expired,
  missing and operator authority for creation, enablement, grants, revocation
  and confirmed configuration. Password-queue tests ensure cancellation timing
  is not the only defense. Valid creation, self-disable, exact session retirement,
  last-administrator retention and peer re-enablement remain covered.
- Permanent real TLS regressions exercise delayed create, self-enable, permanent
  grant and network-save requests over both HTTP/1.1 and HTTP/2. They check
  completed handler retirement and durable rows, including grants that a
  disabled issuer would hide from the public read API.
- A real TLS → reverse proxy → app socket test follows the documented single-hop
  header contract and covers secure host-only login cookies, protected state,
  local-only endpoint denial, foreign-Origin denial, SSE, public Stop, logout,
  and retirement of an already-open stream.
- Existing suites cover certificate identity/expiry/reload, fail-closed listener
  startup, saved configuration/restart, canonical Host and proxy-peer rejection,
  operator permissions, lease loss, command tickets/replay, Stop under pressure,
  HTTP/2 stalled spectators, media transfer cancellation and browser reconnect.

The direct/proxy tests use disposable local sockets, accounts and fake device
transports. No physical device, public listener, firewall rule or trust-store
change is part of this validation. An Internet route and each installed proxy
still require deployment-specific testing. Public Stop intentionally permits
interruption by a reachable peer, and installation accounts share application
content; the existing deployment documentation describes those boundaries.

The release must still pass the complete CI, exact-package Defender scan and
installer lifecycle gates before publication. Local test logs are evidence,
not a substitute for the tagged main-tip artifact checks.
