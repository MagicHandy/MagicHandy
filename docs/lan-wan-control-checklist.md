# Robust LAN and WAN control checklist

Prepared 2026-09-12 against `v0.1.0-alpha.45` / `bc410b582bf84c4f0b25a55317b3947a3d795210`.
This is an implementation backlog from source review, not a claim of completed
network, penetration, mobile, or physical-device testing. Unchecked items remain
open. The first three items are the recommended first implementation pass.

The released baseline remains [ADR 0017](decisions/0017-authenticated-lan-https.md):
loopback by default, or authenticated HTTPS on one exact private/link-local IP.
Public binds, forwarding, tunnels and reverse-proxy hosting remain unsupported.
WAN deployment requires a new reviewed exposure decision. The active
implementation now targets self-hosted direct HTTPS or a trusted reverse proxy
under [ADR 0029](decisions/0029-session-bound-network-control.md), as selected by
the user. See the [implementation/evidence log](lan-wan-implementation.md) and
[deployment contract](self-hosted-https.md). All numbered acceptance items remain
open; implemented components alone do not close real-network/device evidence.

## Existing foundation to preserve

- [x] Explicit-address LAN startup requires TLS, a currently valid matching
  certificate and an enabled account; unsafe address combinations fail closed.
- [x] Backend accounts use bounded Argon2id hashing, opaque cookie sessions,
  expiry/revocation and login throttling; login and account administration have UI.
- [x] Explicit controller takeover invokes Stop before completing the handoff.
- [x] Emergency Stop remains mounted and callable when authentication expires.
- [x] Motion uses the shared engine and transport boundary; frontend state comes
  from backend snapshots and a motion SSE stream.

These are implemented foundations, not completed LAN/WAN acceptance claims.
Account roles currently protect access to one shared installation; accounts do
not provide separate chat, settings, library or media tenants.

## Findings that determine priority

| Source observation | Consequence to address | Items |
| --- | --- | --- |
| `controllerRuntime` stores a browser-supplied client ID without an account/session binding; snapshots include the active client ID. | A copied ID must not substitute for authorized ownership or bypass takeover. | LAN-01 |
| The 15-second lease expires only when touched; expiry clears ownership without invoking Stop, and the next touch can claim it. | Loss of a remote controller needs an explicit watchdog and safe handoff policy. | LAN-02 |
| `handleMotionEvents` touches the lease before every server emission, every 125 ms. | Outbound telemetry is not proof that the controlling UI is responsive or consenting. | LAN-02 |
| Authentication resolves at HTTP request admission; the long-lived motion loop does not recheck it. Logout/revocation handlers do not close that loop. | Existing streams and admitted work need bounded revocation, independent of a cooperative browser. | LAN-03 |
| `writeSSE` writes and flushes synchronously without a per-write deadline; the HTTP server has no global write timeout. | A stalled receiver needs bounded resource use without imposing a short total lifetime on healthy streams. | LAN-07 |
| The browser polls full state every two seconds and receives separate unversioned motion events; its local revision counter cannot establish backend ordering. | Reconnect needs authoritative ordering, freshness and measured bandwidth costs. | LAN-12, LAN-13 |

Source entry points: [controller](../internal/httpapi/controller.go),
[motion events](../internal/httpapi/motion.go),
[authentication](../internal/httpapi/auth.go),
[SSE writer](../internal/httpapi/chat.go),
[HTTP/TLS startup](../cmd/magichandy/server_security.go), and
[browser state](../web/src/state/app-state.tsx).

## P0 — authority and failure behavior

- [ ] **LAN-01 — Bind controller ownership to the authenticated session.** Keep
  account identity, login session, browser tab and control grant distinct. Add a
  server-issued ownership generation checked when commands are applied, not
  just when requests enter. A client ID remains an identifier, not a credential.
  **Acceptance:** another account, another session of the same account, or an
  old controller cannot gain authority by copying an ID or replaying a prior
  generation. Legitimate takeover still passes through Stop.

- [ ] **LAN-02 — Add explicit client heartbeats and a disconnect watchdog.**
  Separate read-only observation from claiming or renewing control. Expiry must
  trigger the chosen shared-engine safety action even if no new HTTP request
  arrives. Define the policy for remote-controlled motion versus any deliberately
  enabled unattended local mode; default remote loss to stopping and explicit
  re-arming. Document per-transport detection and physical-stop bounds, including
  already buffered device motion.
  **Acceptance:** cable loss, blackholed traffic, a frozen tab, phone lock and
  process crash expire ownership predictably. A surviving spectator cannot keep
  the controller alive. Reconnect cannot restart motion implicitly.

- [ ] **LAN-03 — Propagate expiry and revocation to active work.** Logout,
  password reset, account disabling and future grant revocation must cancel the
  affected session's streams and pending commands and release its ownership.
  Bound revalidation latency for motion SSE, chat SSE, audio streams and slow
  downloads; distinguish activity that renews login idle time from background
  polling. Enforce this on the server, consistent with
  [OWASP session lifecycle guidance](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
  **Acceptance:** an already-open stream stops disclosing protected data after
  revocation, and an admitted delayed command cannot apply afterward. Stop
  remains usable on the expired browser.
  **In progress:** [gateway and idle handling](lan-wan-bluetooth-gateway.md)
  separates automatic POST acknowledgements from login activity and includes
  the device gateway in bounded session revalidation.

- [ ] **LAN-04 — Reject stale, duplicate and reordered commands.** Extend the
  existing Stop-generation protections consistently across start/resume,
  retarget, mode, media and live-setting writes. Define request IDs, bounded
  deduplication, command expiry and monotonic ordering scoped to ownership.
  Reconcile unknown outcomes before retrying non-idempotent actions.
  **Acceptance:** delay a Start until after Stop/takeover, reorder slider updates,
  and drop a response after application. No old command restarts motion, no
  duplicate executes twice, and the newest accepted setting wins.
  **In progress:** the [2026-09-13 delivery checkpoint](lan-wan-command-delivery.md)
  implements tickets, ordering, bounded receipts, deferred-motion checks and
  response-loss recovery; exhaustive route and network-fault acceptance remains.

- [ ] **LAN-05 — Define and enforce remote permissions and consent.** Inventory
  every API capability: observe, control, configure devices, edit files/media,
  install modules, change worker commands, manage models and administer accounts.
  Add a real viewer role if needed; being a second tab is not that role. Specify
  owner-approved, scoped, expiring invitations/grants before a linked control
  profile affects hardware. Explicitly choose shared-data visibility versus
  per-user isolation. Keep host-native dialogs local and privileged host
  operations restricted. Follow the server-side authorization principles in
  [OWASP authorization guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html).
  **Acceptance:** a route-level role/grant matrix passes negative tests as well
  as UI tests. Selecting a linked profile never silently grants authority or
  reveals private history, media or host configuration.
  **In progress:** the [login management and admission checkpoint](lan-wan-session-management.md)
  covers all 207 current registrations and implicit HEAD at role admission.
  Handler/resource, payload-redaction, consent and full UI coverage remain open.

- [ ] **LAN-06 — Preserve Stop under congestion and partial failure.** Reserve
  bounded capacity for Stop despite login floods, slow streams, LLM generation,
  media transfers and storage contention. Keep local hardware-side stopping
  available when the network is partitioned. Distinguish request sent, backend
  cancellation and transport-confirmed stop in the UI. Decide how the existing
  unauthenticated Stop exception is contained at a future WAN boundary without
  locking out an expired participant or changing the local safety invariant.
  **Acceptance:** measure receipt-to-cancellation and physical confirmation
  separately under load. A disconnected UI reports an unconfirmed Stop honestly;
  it cannot guarantee delivery across a severed network.

- [ ] **LAN-07 — Bound streaming and request resources.** Add cancelable
  per-write deadlines, bounded queues, slow-client eviction and per-session/IP
  connection and work limits. Coalesce replaceable telemetry; preserve durable
  chat events. Audit body/upload/decompression limits and worker/file endpoints
  alongside the existing header/read timeouts. Do not apply one short global
  write timeout to all long-running speech/chat streams.
  **Acceptance:** slow readers and repeated reconnects do not grow goroutines,
  memory or queues indefinitely, delay Stop, or starve a healthy client.

- [ ] **LAN-08 — Decide the supported WAN architecture and threat model.**
  Evaluate an authenticated private overlay as the initial WAN target; compare
  a managed relay or explicit reverse-proxy deployment separately. Define the
  device host, transport gateway, TLS termination, account authority, remote
  participant and trust boundaries. Cover IPv4, IPv6, CGNAT, NAT traversal and
  overlay address ranges rather than assuming every VPN address passes today's
  private-IP validator. Do not enable router forwarding automatically.
  **Acceptance:** an ADR identifies supported topologies, attack/consent cases,
  operational ownership and Stop semantics, with the prerequisites for each
  topology. The diagram retains one motion engine and one dispatch owner.

## P1 — dependable LAN operation and remote user experience

- [ ] **LAN-09 — Add guided LAN setup and recovery.** Offer an interface/address
  choice, a verified access URL or QR code without credentials, certificate
  checks and narrowly scoped Windows firewall configuration. Handle DHCP
  changes, multiple NICs, VPN adapters, port conflicts and IPv6 deliberately.
  Retain a local recovery path if remote setup fails.
  **Acceptance:** a second machine reaches the authenticated app; restart and
  an address change produce a working endpoint or an actionable error without
  falling back to an unprotected listener.

- [ ] **LAN-10 — Complete certificate and client-trust lifecycle.** Define
  issuance/import, private-key permissions, supported DNS/IP SANs, renewal,
  replacement and expiry warnings. Plan trust enrollment/removal for each
  supported client OS; avoid silent trust-root installation and verification
  bypasses. Secure-context eligibility is a separate prerequisite from API
  availability, as described by [MDN](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts).
  **Acceptance:** normal operation, renewal, wrong SAN, missing intermediate,
  not-yet-valid/expired certificate and client trust removal are exercised on
  real devices. A failed replacement leaves a recoverable secure configuration.

- [ ] **LAN-11 — Add a connection diagnostics screen and download.** Show
  client-to-app reachability, TLS/auth status, active control lease, last state
  age, RTT/jitter, stream health and app-to-device status as separate facts.
  Reuse the bounded/redacted failure-report approach for one-click support
  downloads; omit tokens, keys, chat, audio and private paths by default.
  **Acceptance:** users can distinguish firewall/DNS/TLS/login/stream/device
  failures from a model-generation delay, and safely review a useful report.

- [ ] **LAN-12 — Make reconnect and stale state deterministic.** Add a server
  boot/session epoch and monotonic snapshot/event revisions, a full snapshot
  resync, and cursor recovery for durable chat. Define bounded backoff/jitter and
  visibility-resume behavior; retain one polling request and one event stream.
  SSE's automatic reconnect is transport recovery, not renewed authorization
  or command acknowledgement ([MDN SSE](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)).
  **Acceptance:** old events, overlapping HTTP responses, a server restart and
  a backgrounded phone cannot overwrite fresh state or restore obsolete control.
  Motion-changing controls require fresh ownership/state; Stop stays reachable.
  **In progress:** the [observation checkpoint](lan-wan-observations.md) adds
  capture revisions, independent controller liveness, bounded stream recovery
  and visibility resync. [Committed chat recovery](lan-wan-chat-recovery.md)
  adds coherent read pages, login-scoped bounded cursors, late-commit recovery,
  retention-gap reporting and cancelable browser resync. History now uses 256 KiB
  pages, resumable resets and explicit long-message previews/downloads. The full
  network-fault matrix and real-network/mobile acceptance remain open.

- [ ] **LAN-13 — Reduce telemetry and polling cost with measurements.** Measure
  the existing two-second full-state polling and eight-Hz per-client motion
  serialization before optimizing. Consider compact snapshots, shared encoded
  telemetry, change revisions, adaptive spectator rates and hidden-tab policy.
  Remove redundant flushing and avoid repeated database work where measured.
  **Acceptance:** record bytes/client/minute, CPU, allocations, RSS and p95/p99
  state age for one controller and multiple spectators, without weakening
  heartbeat, revocation or Stop behavior. Update the goal scorecard.

- [ ] **LAN-14 — Validate remote video and voice workflows.** Exercise Range
  requests, seeking, bandwidth contention, audio buffering, microphone upload,
  permissions and reconnect. Keep inference endpoints/private worker ports on
  the host behind the app API. Define authorized remote file selection without
  exposing the server's native path picker or arbitrary filesystem access.
  **Acceptance:** a throttled media transfer cannot block control; speech cancels
  promptly, unsupported capture/playback is explained, and authorized media
  continues or pauses predictably after connection loss.

- [ ] **LAN-15 — Make transport location and capabilities explicit.** Identify
  whether device access lives on the app host, a browser BLE bridge, or an
  Intiface host. A remote browser must not silently move the hardware connection
  or create another motion owner. Detect required APIs on the actual browser.
  **Acceptance:** the UI explains supported control/voice/Bluetooth combinations;
  gateway loss and phone sleep follow the defined stop policy and never create
  competing transport sessions.
  **In progress:** [authenticated browser gateway](lan-wan-bluetooth-gateway.md)
  binds dispatch to its session and connection generation, keeps it distinct
  from remote control, labels its browser location and tests loss/handoff with
  fake transport and browser fixtures. Real platform/GATT acceptance remains.

- [ ] **LAN-16 — Add session management and a bounded audit trail.** Let users
  inspect/revoke their sessions and let administrators inspect grant/ownership
  transitions. Record actor, grant, command/result, takeover, Stop, login failure
  and revocation with retention limits and redaction. Keep credentials and raw
  intimate content out of audit records and exports.
  **Acceptance:** a reported remote action can be attributed and correlated with
  a trace; users can end access from a lost device and see when it took effect.
  **In progress:** [account-owned session inspection, names and revocation](lan-wan-session-management.md)
  are implemented, including active-work retirement. Bounded audit history,
  grant/ownership attribution and the complete acceptance scenario remain open.

## P2 — WAN implementation and release acceptance

- [ ] **LAN-17 — Implement only the selected WAN deployment.** After LAN-08,
  implement its explicit origin, address, routing and TLS policy. If proxies are
  supported, trust forwarded headers only from configured peers, preserve
  Secure cookies and exact external origins, disable stream buffering, and
  prevent forwarded traffic from reaching loopback-only bootstrap/dialog APIs.
  Bound handshake/reconnect failures and test NAT changes and tunnel restarts.
  **Acceptance:** supported deployments pass end to end; spoofed forwarding
  headers and access to backend/worker ports cannot bypass authorization.

- [ ] **LAN-18 — Complete authentication hardening and recovery for WAN.**
  Choose MFA/passkeys or equivalent stronger access policy, secure enrollment,
  revocable invitations, credential-guessing/lockout protections and a local
  owner-recovery process. Test recovery without deleting private app data or
  silently turning authentication off. Add step-up authentication for sensitive
  account or remote-access changes where the selected threat model requires it.
  **Acceptance:** lost credentials, a lost second factor, a stolen session and
  a compromised invite have documented, tested containment and recovery paths.

- [ ] **LAN-19 — Build repeatable network fault and load tests.** Cover request
  delay/reordering/duplicate delivery, response loss, blackholed connections,
  server sleep/restart and reconnect storms. Start with RTT profiles of 20,
  100, 300 and 800 ms; 0/1/5% loss; jitter; 5/30/120-second outages; and one
  controller plus 1/5/20 spectators. These are proposed test profiles, not
  measured product support limits.
  **Acceptance:** invariants pass at every profile; record command/Stop p95/p99,
  disconnect-to-stop bounds, stale-state duration, bytes, CPU, RSS and goroutines.
  Select supported operating limits from the measurements and run an overnight
  soak within that envelope. Keep all safety, race and import-boundary gates.

- [ ] **LAN-20 — Record real device/browser acceptance.** Use at least two
  physical clients and each claimed browser/OS, including supported Android and
  iOS cases. Exercise trust setup, login, observer mode, takeover, expiry,
  microphone/audio, BLE where actually supported, phone lock/backgrounding,
  Wi-Fi changes and supported cellular/WAN transitions.
  **Acceptance:** retain versions, topology, transport, latency, trace and visual
  evidence, including unsupported combinations. Physical motion testing requires
  its own authorized session; a simulator alone does not close this item.

- [ ] **LAN-21 — Complete security review and operational release gates.**
  Review CSRF, DNS/Host rebinding, privilege escalation, ID/session replay,
  grant revocation, local-only endpoint bypass, request floods and secret-safe
  exports. Test secure settings/certificate persistence across app upgrades,
  rollback, reboot and firewall changes. Publish deployment and recovery steps,
  supported topology/browser limits and measured failure behavior.
  **Acceptance:** blockers are resolved or explicitly scoped out; update ADRs,
  Phase 20, R18/R29, UI docs and the scorecard before claiming robust WAN support.

## Suggested implementation sequence

1. LAN-01, LAN-02 and LAN-03: authority binding, heartbeat/watchdog and revocation.
2. LAN-04 through LAN-07: command delivery, permissions, Stop capacity and streams.
3. LAN-08 through LAN-16: make the topology decision and deliver a supported LAN
   setup, certificate lifecycle, reconnect, diagnostics and resource budget.
4. LAN-17 and LAN-18: implement and harden the approved WAN option.
5. LAN-19 through LAN-21: build test fixtures alongside the above work, then close
   measured network, real-device and security acceptance before release claims.

Every completed item should link its implementation PR and acceptance evidence.
Work on the first two safety passes does not depend on choosing a WAN vendor.
