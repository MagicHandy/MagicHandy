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
  reconnect and a fresh resync after browser visibility or server-epoch changes;
- a session-bound Bluetooth gateway independent of remote controller ownership,
  per-connection dispatch identities and loss-triggered shared-engine Stop;
- passive POST acknowledgement handling, preserving validated explicit playback
  activity without letting background reports keep a login alive;
- durable committed chat revisions, coherent read pages, session-scoped bounded
  cursors and cancelable browser recovery without skipped replies or audio replay;
- encoded history byte budgets, resumable resets, explicit long-message previews
  and exact-content downloads that release database connections before writes;
- cancelable chat publication, with autonomous preparation outside the gate and
  persona provenance resolved against one captured conversation;
- independent management IDs, account-scoped login listing/renaming/revocation
  and bounded reconciliation of a lost management response;
- a maintained 209-route role admission inventory, setup/private-account read
  boundaries, operator control permission fixes and cancelable feedback writes;
- capability-specific shared response projections, administrator-only raw
  diagnostics, a minimal public Stop acknowledgement and browser login lifetime
  boundaries for snapshots, notifications and queued quick edits;
- atomic access-change records and bounded runtime audit history, with an
  administrator page/export, explicit loss reporting and trace correlation;
- a latest-run trace writer that preserves immediate reads while removing
  diagnostic storage waits from the public Stop response;
- admission before authentication with reserved controller/gateway capacity,
  bounded storage lookup and honest retryable errors that preserve login cookies;
- bounded overlapping HTTP Stop waiters with per-intent invalidation, prompt
  replies to unfinished uploads, and cancelable media/content writes.

## Request-pressure checkpoint — 2026-09-13

The [request-pressure contract](lan-wan-request-pressure.md) records concurrent
budgets, Stop coalescing, slow-client behavior, measured overhead and remaining
acceptance. Regressions reproduced the unfinished-upload Stop delay, temporary
storage failure clearing login state, and a delayed Stop reusing confirmation
from before a newer run. They now pass. Each overlapping Stop retains its own
invalidation and audit sequence; completed operations never become cached retries.

The 20-spectator TLS/HTTP/2 fixture exercises concurrent state reads, public Stop
and server-side stream revocation. HTTP/1 and HTTP/2 stalled-download fixtures
release request capacity, while a healthy producer gap remains supported. This
advances LAN-06/07/14/19 without closing the full network-fault/load matrix.

Full Go and race suites, vet, zero-issue golangci-lint v2.12.2, the CGO-free
build, TypeScript, all 599 frontend tests in 79 files, 2,150 translation keys
in five locales and the canonical production UI build pass. No hard gate was
changed. Test details and artifact/overhead measurements are in the contract
and [scorecard](goal-scorecard.md).

The current isolated simulator runs at
`http://127.0.0.9:50005/#/settings/access`. Real provider readiness passes and
text-only browser chat returns “The final review build is ready.” in 125 ms,
with a 57 ms first token and LLM motion off. The initial build's SSE check also
verified one provider call without repair or fallback. The served JS SHA-256 matches the
worktree. Its 1,185-byte connection report includes all five request budgets and
Stop waiter status without credentials/session keys. The browser stays an
administrator observer with LAN/WAN settings and Stop visible; no console
warnings/errors were observed. Voice and LLM motion remain off, hardware remains
disconnected, and existing app processes were preserved. Earlier in-app browser
file-save verification remains unresolved as recorded below; HTTP report
verification does not establish a completed browser download.


## Access history checkpoint — 2026-09-13

The [audit contract](lan-wan-audit-history.md) records account/session/grant
changes atomically and runtime outcomes through one bounded writer. The Access
panel fetches on demand, retains one page and provides a one-click JSON export.
Exports exclude credentials, raw request IDs/bodies and application content.
Protected command replay does not create another execution record. Public Stop
has an unauthenticated actor and does no login lookup.

A regression holding the shared SQLite writer exposed existing synchronous
trace persistence after engine Stop. The new trace worker saves the latest
bounded run independently of the response. In-memory trace reads remain
immediate; orderly shutdown flushes with a two-second cancellation budget.
Overlapping Stops now preserve their individual invalidation sequences in
history. No motion sampler or transport dispatch path was added.

Schema migration, rollback, retention, paging, retry deduplication, queue loss,
blocked storage, trace coalescing/redaction/durability and worker teardown have
regression coverage. Browser tests cover download authorization requests,
pagination, late replies, disconnect/collapse, timeouts and incomplete history.
The complete frontend suite passes **599 tests in 79 files**, with **2,145 keys
in five locales**. Full Go/race, vet/lint, CGO-free build and current app evidence
are retained with this checkpoint's validation results. Real WAN/mobile/device
acceptance remains open.

The exact build runs at `http://127.0.0.8:50003/#/settings/access` with isolated
data and simulated motion. Real provider readiness passes, and text-only app
chat returns “The review build is ready.” in **100 ms**, one provider call,
without repair, fallback or motion. The browser stays an observer. The history
shows that claim, command and subsequent watchdog loss/Stop; the retired grant
and expired review login also remain correctly time-limited. No hardware,
public listener, firewall or trust-store settings were changed.

Authenticated export returned 12 events / 3,525 B with attachment metadata;
payload checks exclude the fixture password, chat text and private paths. An
operator export returned 403. The served JS SHA-256 matches the worktree.
Desktop and 390-by-844 viewport review show history controls and a visible Stop;
the narrow document's scroll width remains 390 px. Viewport emulation does not
establish physical mobile acceptance.

Browser download initiation is verified, but complete file saving remains
unverified in the in-app browser: its `Page.downloadWillBegin` event names
`magichandy-access-history.json`, followed by `Page.downloadProgress` with
`state=canceled`, zero received bytes and a 4,431-byte generated JSON file.
The HTTP attachment and browser unit tests pass. Retain this limitation until
an actual browser save is verified; do not equate initiation with completion.

## Observer privacy checkpoint — 2026-09-13

The [response contract](lan-wan-observer-privacy.md) selects shared settings,
motion, voice, media, transport and chat fields without host paths, worker
commands or raw diagnostics. Detailed Labs/trace/transport reads require host
administration. Granted operators retain semantic playback, limits, chat and
feedback; the physical Handy profile remains an administrator setting. Public
Stop acknowledgements contain no previous target and perform no login lookup.

The production UI discards old snapshots, drafts, notifications, streams and
queued quick edits when a login changes or ends. A delayed quick-setting
response cannot flush an older login's next edit. A capability-loading regression
also ensures host settings panels wait for the backend snapshot before mounting.

Full Go tests, the full race suite, vet, zero-issue golangci-lint, the CGO-free
stripped build, TypeScript and five-locale localization checks pass locally.
The full browser suite passed 593 tests in 78 files; the added capability-loading
regression and the final 23-test Settings suite pass after that run. The canonical
UI was rebuilt before the final Go/embedded checks. The maintained matrix covers
all 207 registrations and implicit HEAD; it is still admission coverage, with
behavioral response/UI tests documented separately.

The current source runs at `http://127.0.0.7:50001/#/settings/access` using fresh
isolated accounts/data and simulated motion, with voice and LLM motion off.
The previously used Ollama service was stopped; the installed runtime was
started on loopback with its existing downloaded model. Real provider readiness
and app chat complete, returning “The review build is ready.” in **101 ms**,
one provider call, no repair/fallback and no motion. The fresh-server probe uses
initial heartbeat admission, without takeover or Stop endpoints.

Built-app authenticated HTTP checks verify administrator/operator settings and
state projection, rejection of raw host routes, and shared committed chat text
without model diagnostics. The served `/assets/index-w5vqGE1y.js` matches the
worktree SHA-256 `E951172F9479D874C7777D2F9D8D5F0D656F50E64533C471E8DA5C16AE0F8F2F`.
Browser review verifies the operator Access boundary, own sign-ins, restricted
setup bookmark and shared chat. Desktop and 390-by-844 viewport layouts retain
visible Stop, and the warning/error log is empty. The viewport is restored and
the operator Access tab/process are retained. Viewport emulation is not physical
phone acceptance. The [scorecard](goal-scorecard.md) records artifact costs and
the limits of the response/memory samples.

The full handler/resource/export audit, other deferred browser work, invitation
consent, bounded audit history, stronger WAN enrollment/recovery, controlled
network fault/load/soak and external/mobile/device acceptance remain open.
No numbered checklist acceptance is closed from this checkpoint.

The first Linux CI run rejected a Windows-specific absolute path in the new
settings fixture before its assertions ran. The fixture now uses a temporary
directory and `filepath.Join` for host locations on every OS, preserving the
same private-data sentinels and response assertions. No production path
validation was weakened to accommodate the test.

## Session management and admission checkpoint — 2026-09-13

The [login management contract](lan-wan-session-management.md) adds schema v22,
independent management IDs, account-owned names and revocation, and apply-time
actor validation. Revocation closes active work and retires controller/gateway
authority through the shared Stop lifecycle. The browser reconciles lost action
responses without replay. A fixed-clock regression also prevents the session
cap from evicting the login it just created.

A maintained table covers all **207** registrations and implicit HEAD across
anonymous, observer, granted operator, administrator and revoked-administrator
callers. The audit corrected exact setup-status exposure, aligned private
account-read admission with its handler, and enabled granted controllers to
change LLM motion mode and undo feedback. Feedback transactions now honor
request cancellation. The table establishes admission coverage; full handler,
payload-redaction and UI capability coverage remain separate work.

The full Go and race suites, vet, zero-issue pinned lint, CGO-free build, final
embedded/architecture checks, typechecking, **582 frontend tests in 77 files**,
the **2,074-key/five-locale** audit and production UI build pass. The new tests
cover migration preservation across 300 rows, same-account management,
cross-account denial, canceled/obsolete writer-queue operations, private key
exclusion, HTTP self-revocation acknowledgement and open-stream termination.
The file-size check prompted extraction of account/session/network payload
types into `web/src/api/access-types.ts`; no gate was weakened. Localization
allows only nine exact browser/platform product names to retain their spelling.

The current source runs at **`http://127.0.0.6:49997/#/settings/access`**, using
fresh isolated data and simulated motion. Its real provider readiness check
passes, and app chat returns a nonempty response in **117 ms**, one provider
call, no repair/fallback or motion. Actual HTTP requests rename a synthetic
login and revoke its peer: the peer immediately returns **401**, while other
logins remain valid and the response excludes bearer cookies.

The browser names its own login while remaining an observer, then refreshes
the backend list. Desktop and 390-by-844 viewport review show readable session
rows, wrapping controls and visible Stop. This is desktop browser emulation,
not physical phone acceptance. The viewport is restored, the Access tab and
process remain running, and browser error/warning logs are empty. The served
main asset matches the worktree SHA-256. Earlier app sessions are preserved.
The [scorecard](goal-scorecard.md) records sizes and memory-sample limitations;
raw fixture evidence stays in ignored `.scratch/lan-wan/`.

This advances LAN-03/05/16. All numbered acceptances remain open, including
bounded audit history, invitations, stronger WAN enrollment/recovery, full
handler/UI coverage, load/soak and external/mobile/physical-device evidence.

## History resource checkpoint — 2026-09-13

See the newer [session management checkpoint](#session-management-and-admission-checkpoint--2026-09-13)
for the current review app and validation.

Two regressions reproduced an unbounded 15.7 MB history response from twenty
large synthetic messages and a canceled reader stuck behind chat publication.
The [recovery contract](lan-wan-chat-recovery.md) now defines 256 KiB pages,
stateless snapshot continuation, 16 KiB UTF-8 previews, explicit full-message
downloads and cancelable publication. Newer and late commits remain recoverable;
deletion or pruning between pages invalidates the anchored window. Retry/poll
boundaries retain both continuation and speech suppression.

Regressions cover exact canonical download bytes, UTF-8 boundaries, diagnostics
limits, unchanged stored content, authenticated/mismatched/pending-row access,
database release before socket writes, and failed framing when a message is
removed during its download. The full Go and race suites, vet, zero-issue lint,
CGO-free core build, 575 frontend tests, typechecking, 2,034-key/five-locale audit,
production UI and final embedded/architecture checks pass. No gate was relaxed.

The final source build runs at **`http://127.0.0.5:49995/#/chat`**, with isolated
data and simulated motion. The exact provider readiness probe completes real
generation, and app chat completes in **135 ms**, one provider call, no repair
or fallback and no motion. The retained observer tab shows the long-message
preview/download and the LLM reply, with Stop mounted and Bluetooth disconnected.
One click downloaded all **76,574 bytes**, matching the fixture SHA-256; visual
review corrected the link's default visited color and the final console is
clear. The served main asset hash matches the worktree. Earlier processes were
preserved. Detailed local evidence remains in ignored `.scratch/lan-wan/` and
the [scorecard](goal-scorecard.md) records budgets and measurement limits.

This advances LAN-07/12/13. It does not close full stream/work admission,
permissions/session workflows, WAN authentication/recovery, fault/load/soak,
external HTTPS/proxy, mobile or physical-device acceptance.

## Durable chat recovery checkpoint — 2026-09-13

The [chat recovery contract](lan-wan-chat-recovery.md) separates display order
from committed delivery order. The reproduced cross-login cursor collision is
fixed. A reply committed below the newest display sequence is recovered through
its later committed revision, and the browser no longer acknowledges an
informational head beyond the delivered changes. Rows and recovery metadata
come from one read transaction.

Schema v21 preserves existing conversation content, initializes committed
revisions, retains compatible read markers within the storage cap and remains
safe to reapply over existing revision metadata. The original v11 migration
preservation test caught an over-eager expiry step; that assertion remains, and
expiry now belongs to ordinary reads/advancement rather than migration.

The browser extracts history lifecycle management into a focused hook, merges
stream placeholders with durable rows, handles resets/retention gaps and aborts
obsolete reads. Initial history, reconnect and post-stream reconciliation do
not replay old speech. Read acknowledgements no longer consume controller
sequence numbers or seek command receipts after a lost response. Read-marker
writes honor cancellation and avoid rewriting unchanged positions.

Full Go and race suites, vet and zero-issue pinned lint pass after the migration
compatibility correction. The frontend passes typechecking, localization
(2,031 keys across five locales), and **572 tests in 76 files**. The production
UI, stripped `CGO_ENABLED=0` build and final embedded-asset/import-boundary
checks pass. Sandbox restrictions on existing synthetic Ollama-library fixture
paths required rerunning the Go checks with host filesystem access; their
assertions and the shipped pure-Go boundary remain unchanged.

That checkpoint was reviewed at
`http://127.0.0.3:49991/#/chat`, with fresh isolated data, simulated motion,
voice off and LLM motion off. The configured local Ollama model passes the real
`scripts/check-review-llm.ps1` generation probe. App chat returns “The review
build is ready.” in **113 ms**, one provider call, with no repair/fallback or
motion. The fresh-server probe uses initial heartbeat admission and invokes
neither takeover nor Stop.

Built-app HTTP checks retrieve two committed messages at revision 2. A second
login acknowledges them using the same public browser ID; the first login's
cursor remains zero. Its own acknowledgement then advances the cursor, and a
read after revision 2 returns an empty, consistent tail. Browser sign-in and
reload recover the same two messages without duplicates, in observer mode with
Stop mounted and no console errors/warnings. The served main asset's SHA-256 matches the current canonical
worktree asset for that checkpoint. Existing user/review processes were preserved.

Artifact measurements and memory-sample limits are in the
[scorecard](goal-scorecard.md). Logs and fixture results remain in ignored
`.scratch/lan-wan/`. This does not close response-byte/resource acceptance,
the full route/grant matrix, external HTTPS/proxy deployment, or physical
client/network testing. The full 21-item goal remains active.

## Bluetooth gateway and login activity checkpoint — 2026-09-13

The [gateway contract](lan-wan-bluetooth-gateway.md) documents the boundary.
An exploratory fake-bridge regression reproduced an observer retrieving queued
work by copying the public gateway ID. Normal regression tests now deny copied
identities, preserve the legitimate queue and test status/ACK forgery, handoff,
old generations/epochs and loss stopping the shared fake engine.

The browser retains the gateway through controller handoff, uses the server's
connection metadata, suppresses observer status writes and identifies the
device browser in the panel. Failed/hidden gateway channels attempt local Stop
and release GATT with explicit reconnection. Queued native writes recheck Stop
cancellation after waiting for the writer. Explicit Disconnect still uses the
existing global Stop coordinator; its original regression remains intact.

Automatic POST acknowledgements no longer advance login idle time. Persisted
timestamp tests distinguish heartbeat/bookkeeping from validated playback
intentions and confirm that an expired login cannot be revived.

Full Go and race suites, vet, zero-issue lint, browser typechecking and
localization checks pass. The full browser suite passes **559 tests in 75
files**. Production UI and stripped `CGO_ENABLED=0` core builds pass, followed
by the final embedded-asset/import-boundary checks. A final focused Go run
passes after the lint-only error string capitalization correction.

The gateway checkpoint's review app ran at
`http://127.0.0.2:49989/#/settings/device`, with fresh isolated data,
`-simulate-motion`, voice off and LLM motion off. The separate loopback host
avoids sharing browser login cookies with other review apps on `127.0.0.1`.
Its configured Ollama model passes `scripts/check-review-llm.ps1`. App chat
returns “The review build is ready.” in **112 ms**, with **one provider call**,
no repair/fallback and no motion. The fresh-server probe invokes neither
takeover nor Stop. The retained review browser is signed in as an observer,
with the device connection panel open, Browser Bluetooth selected, no device
connected and Emergency Stop visible. No console errors/warnings were observed;
existing app sessions remain running.

A review-only Go test overlay captures the shared engine's redacted stopped
trace for synthetic gateway expiry, session revocation and reported disconnect.
Each case uses a temporary database, synthetic login and fake motion transport
under the browser-Bluetooth dispatch policy. One sample per case measures
**1.504 ms**, **2.509 ms** and **2.037 ms**, respectively, from fault injection
through an explicit watchdog check to stopped engine and retired gateway.
This excludes the normal watchdog interval, network, GATT and physical device
timing; it is not a stopping-time guarantee or percentile measurement. Each
export contains five rows, including the existing engine Stop. The artifacts
are retained locally as ignored `.scratch/lan-wan/gateway-*-trace.json` and
`gateway-*-measurement.json`. The semantic target mapping, sampler, stroke
limits and transport Stop payload are unchanged. Real-device traces and
physical latency acceptance remain open.

Artifact sizes and memory-sample limits are recorded in the
[scorecard](goal-scorecard.md); verification logs remain in ignored
`.scratch/lan-wan/`. The full 21-item LAN/WAN goal remains in progress.

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

The observation checkpoint's review app ran at
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
media, mode and live-setting routes. Extend the completed route-admission
inventory to handler/resource and UI scope, including payload redaction and an
owner-approved invitation workflow.

The [observer privacy checkpoint](lan-wan-observer-privacy.md) now projects
shared settings/state, runtime status and chat responses without host paths or
diagnostics. Extend that evidence to the exhaustive handler/resource and export
matrix, remaining delayed-work lifetimes and administrator consent workflows.

The remaining durable chat recovery, RTT/stream diagnostics, telemetry budget,
media/voice and transport-location behavior, exhaustive audit attribution and enrollment,
authentication recovery, network fault/load fixtures and real device/browser
matrix remain part of the goal. A simulator or green unit suite cannot establish
real WAN, mobile, certificate enrollment or physical Stop acceptance.

Do not mark the goal complete until every numbered checklist item has current
implementation and acceptance evidence. Deployment on an externally reachable
host also requires actual domain/certificate/routing information; local fixture
tests must not silently change this machine's public exposure.
