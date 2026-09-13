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
  allowing text generation to run outside the immediate-control lane;
- backend capture revisions for app/motion observations and separate controller
  read revisions, with no-store JSON responses;
- independent controller liveness, ordered motion reconciliation, bounded
  reconnect and a fresh resync after browser visibility or server-epoch changes.

## Observation checkpoint — 2026-09-13

The [observation contract](lan-wan-observations.md) defines capture order,
independent channel timing and reconnect behavior. Full-state collection does
not hold a capture lock during slow diagnostics, and heartbeat renewal does
not wait for that collection. A delayed packet cannot overwrite newer motion;
stream failure retains the newest observation while requiring fresh state.
Visibility return and restart discard obsolete work without automatic takeover
or motion resume. Protected controller packets are validated before enabling
controls, and aborted responses cannot replace cached delivery metadata.

Backend and browser regressions exercise blocked diagnostics, heartbeat renewal
past a lease period while state is blocked, response/event reordering, restart,
duplicate error callbacks, bounded retries, malformed controller metadata and
phone-style background/resume after backend ownership expiry. Durable chat
recovery, real mobile scheduling and telemetry/load measurements remain open.

Full Go and race suites, vet, zero-issue lint, browser typechecking and
localization checks pass. The full browser suite passes **549 tests in 75
files**; all **23** targeted lifecycle/network/delivery tests pass again after
the final backoff adjustment. The production UI and stripped pure-Go core
builds pass, along with the final embedded-asset and architecture checks.

The exact current-source app runs at
`http://127.0.0.1:49987/#/settings/access`, with fresh isolated data,
`-simulate-motion`, voice off and LLM motion off. Its available Ollama model
passes `scripts/check-review-llm.ps1`. App chat returns “The review build is
ready.” in **109 ms**, with **one provider call**, no repair/fallback and no
motion. The fresh-server probe uses initial heartbeat admission and invokes
neither takeover nor Stop. Read-only HTTP checks verify increasing full/motion
capture revisions and `Cache-Control: no-store`.

The browser is signed in and left in observer mode, with the simulator idle,
the LAN/WAN Access panel visible and Emergency Stop mounted. No console errors
or warnings were observed. Existing app sessions remain running. Artifact
sizes and the limits of the memory sample are in the [scorecard](goal-scorecard.md);
logs remain under ignored `.scratch/lan-wan/`.

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

The initial delivery build ran at `http://127.0.0.1:49983` with isolated
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

### CI shutdown correction

The first delivery commit exposed a shutdown timing defect in Linux CI:
`TestQuiesceReleasesMotionEventStream` sometimes received `unexpected EOF`.
An immediate cancellation write deadline could prevent the HTTP/1 response's
final chunk framing. A deterministic local reproducer failed before the fix.
Shutdown now gives socket writes a bounded five-second grace period while
canceling handler work immediately; live-server access revocation still applies
an immediate write deadline. The original test remains intact, both shutdown
tests pass repeated runs, and the full Go/race/vet/lint/CGO-free gates pass again.

The corrected current build is at `http://127.0.0.1:49985/#/settings/access`,
using a fresh isolated simulator database. Its real provider readiness probe
passes, and app chat returns “The review build is ready.” in **106 ms**, with
one provider call, no repair/fallback and no motion. This fresh-server check
used initial heartbeat admission and invoked neither takeover nor Stop.
The review tab is signed in, in observer mode, with the LAN/WAN settings and
Stop visible. Earlier simulator sessions were preserved.

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

The remaining durable chat recovery, RTT/stream diagnostics, telemetry budget,
media/voice and transport-location behavior, session/audit management,
authentication recovery, network fault/load fixtures and real device/browser
matrix remain part of the goal. A simulator or green unit suite cannot establish
real WAN, mobile, certificate enrollment or physical Stop acceptance.

Do not mark the goal complete until every numbered checklist item has current
implementation and acceptance evidence. Deployment on an externally reachable
host also requires actual domain/certificate/routing information; local fixture
tests must not silently change this machine's public exposure.
