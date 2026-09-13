# LAN/WAN implementation log

Goal: **Implement full LAN/WAN support.**

The user selected self-hosted HTTPS, with direct access or a trusted reverse
proxy, as the primary WAN setup. Scope remains the complete
[21-item checklist](lan-wan-control-checklist.md), with the proposed design in
[ADR 0029](decisions/0029-session-bound-network-control.md).

## Current implementation work

The implementation is in progress on `codex/lan-wan-control`:

- account/session-bound tab ownership, backend generations and a random process
  epoch; old-process, pre-Stop and prior-ownership requests cannot reuse control;
- a foreground client heartbeat for protected sessions, distinct from passive
  snapshots and server-sent events;
- a periodic watchdog that invalidates expired control before using shared Stop;
- read-only session inspection, so polling does not renew login idle time;
- active-request cancellation on session expiry/revocation, and on enabling
  protection over previously unprotected streams;
- bounded active-session/request admission with a Stop exemption;
- per-event write deadlines, including correct HTTP/2 reset and response-wrapper
  support so model-generation waits are not mistaken for blocked writes;
- a stop-first protected bootstrap handoff for the creating browser tab;
- schema v20 operator control permissions: explicit administrator consent,
  expiry, replacement and revocation; operators otherwise observe shared data;
- a server-enforced default-administrator policy for host mutations, separate
  from semantic control and self-service account operations;
- explicit local/direct-HTTPS/trusted-proxy modes with a canonical external
  origin independent of the socket address, strict proxy peers/headers and
  forwarded-request isolation from local bootstrap and host dialogs;
- persisted restart-only network settings, host validation, administrator
  password confirmation, local recovery override and certificate reload/expiry
  handling; no automatic listener/firewall/trust-root change;
- localized Access UI, one-click redacted connection reports and control grants;
- protected delivery tickets, sequence ordering, bounded receipts and safe
  recovery of a lost JSON result without repeating the original mutation;
- synchronous ownership cancellation and context-bound settings transactions;
- deferred chat/Lab motion checks that preserve newer control intentions while
  allowing text generation to run outside the immediate-control lane.

## Command-delivery checkpoint — 2026-09-13

The [delivery contract](lan-wan-command-delivery.md) documents the protocol,
receipt states, resource bounds and remaining coverage. Full Go tests, full race
tests, go vet, golangci-lint, the CGO-free stripped build, browser typechecking,
the localization audit, **539 browser tests in 74 files**, the production UI
build and final embedded-asset/import-boundary checks pass.

New regressions exercise an old Start/Resume/mode/media request arriving after
Stop, reordered settings, duplicate delivery, receipt privacy/expiry/capacity,
ticket expiry despite a live lease, Stop during a queued settings write and
interrupted handlers. A blocked provider completes after a newer live-setting
change: the text reply is retained and its obsolete motion is rejected. Voice
bookkeeping does not discard otherwise-current deferred motion. A prepared
settings transaction canceled before commit changes neither disk nor memory.

The exact current binary is running at `http://127.0.0.1:49983` with isolated
data and `-simulate-motion`, voice off and LLM motion off. Authenticated HTTP
checks apply two settings, replay the first and verify that the newer setting
remains active; the first completed result is also retrieved through its receipt.
The original setting is restored. The real review LLM probe passes, and the app
chat path returns “The network review is ready.” in **108 ms**, with **one
provider call**, `repaired=false`, `semantic_fallback=false` and no motion.

The browser is left on Access settings, scrolled to the LAN/WAN configuration,
in observer mode with the simulator idle and Stop visible. Browser takeover was
not completed; the independent authenticated simulator API test above supplies
the takeover/delivery evidence. No public listener, firewall, client trust store
or physical-device configuration was changed. The
[scorecard](goal-scorecard.md) records the current artifact sizes and the limits
of the memory observation. Detailed logs remain in ignored `.scratch/lan-wan/`.

## Foundation checkpoint — 2026-09-12

`go test ./...`, `go test -race ./...`, `go vet ./...`, golangci-lint, the
CGO-free stripped build, browser typechecking, localization audit, 533 browser
tests (73 files) and the production UI build pass. Regression coverage includes
copied tab IDs across accounts/sessions, old generations/process epochs,
heartbeat expiry, open-stream revocation, enabling protection over old streams,
grant expiry/revocation, host-operation denial, spoofed proxy metadata,
DNS identity distinct from bind IP, local recovery and HTTP/2 stream deadlines.

The current review uses isolated data, simulated motion, disabled voice and
LLM motion, and the host's available Ollama model
`huihui_ai/granite4.1-abliterated:3b`. The review LLM script completes a real
generation. An authenticated request through the app chat path returns
“The LAN and WAN review build is ready.” in 140 ms, with one provider call,
`repaired=false`, `semantic_fallback=false`, and no motion application.
Access settings render the new network and permission controls; host validation
and the one-click report download were exercised in the in-app browser.
Visual review corrected missing text-input styling, permission-row wrapping and
the compact controller readout. The final UI build is running at
`http://127.0.0.1:49983/#/settings/access`, signed into a disposable local review
account. Its exact app passes `scripts/check-review-llm.ps1` again after the
review restart. The browser/process are left running; no public listener,
firewall, trust-store or physical-device setting on this machine was changed.

Logs and disposable fixtures are under ignored `.scratch/lan-wan/`. No
numbered checklist item is marked complete from partial evidence. The deployed
nginx example, client trust/renewal, real WAN routing and hardware/mobile
acceptance have not been tested.

## Next work and completion evidence

Complete apply-time race/fault scenarios across all motion,
media, mode and live-setting routes. Expand the permission matrix to an exhaustive
route inventory and owner-approved invitation/session management workflow.

The remaining snapshot/event reconnect ordering, RTT/stream diagnostics, telemetry budget,
media/voice and transport-location behavior, session/audit management,
authentication recovery, network fault/load fixtures and real device/browser
matrix remain part of the goal. A simulator or green unit suite cannot establish
real WAN, mobile, certificate enrollment or physical Stop acceptance.

Do not mark the goal complete until every numbered checklist item has current
implementation and acceptance evidence. Deployment on an externally reachable
host also requires actual domain/certificate/routing information; local fixture
tests must not silently change this machine's public exposure.
