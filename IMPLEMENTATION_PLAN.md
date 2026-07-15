# MagicHandy Go Implementation Plan

## Core Direction

MagicHandy is a Go-first ground-up rewrite of StrokeGPT-ReVibed.

The rewrite is justified by maintainability, cleaner architecture, future binary releases, lower non-ML baseline overhead, simpler long-running concurrency, and fewer Python environment failures in the core install path. Go alone will not fix Handy cloud latency, local LLM memory, CUDA memory, or all motion smoothness bugs. Motion quality must come from a better motion model, transport scheduler, retargeting algorithm, diagnostics, and real-device validation.

The measured evidence still supports the rewrite's lower core overhead: the
StrokeGPT-ReVibed Python core idles at ~525 MB with no model loaded, while the
MagicHandy Go core idled at ~9 MB before SQLite and ~54 MB after the pure-Go
SQLite datastore landed (see `docs/perf-baseline.md`). The SQLite result is a
Phase 11B waiver against the original <40 MB idle budget, not a budget change.

The default voice stack is non-Python: Parakeet (ASR), NeuTTS Air (local cloning TTS), and ElevenLabs (cloud TTS) — see ADR 0007. Python may still be added later behind optional worker boundaries for Chatterbox, CosyVoice, or other ML-heavy features, but it never defines the core app install path.

Local LLM support is quality-first. The primary MagicHandy LLM path is a managed llama.cpp runtime for Windows/NVIDIA systems, using curated GGUF models and explicit model management. Ollama remains supported as the secondary pathway. See `docs/decisions/0005-local-llm-runtime.md` and `docs/model-management.md`.

## Status

Updated 2026-07-14. MagicHandy is a source-runnable alpha, not a packaged or
release-ready application. Phases 0 through 14, 14B, and 14C are merged to
`main`: persisted patterns/programs, Intiface dispatch, the route-independent
connection manager, and the current React shell are implemented. The LLM model
manager (#55) and managed llama.cpp source-build lifecycle (#56) landed ahead
of Phase 16 and anchor its packaging story. The future Windows install binary
remains a thin shell around the app's own first-run setup wizard (decision and
design in `docs/gui-installer.md`). Phase 13 deliberately supports microphone
capture on localhost only; LAN/mobile HTTPS remains a Phase 16 packaging
decision.

Recent maintenance in PRs #63-#67 has landed. It hardens connection and live-limit
controls, bounds LLM output with honest provider-native reasoning control,
separates app-managed Parakeet assets from custom paths, makes source updates
survive merged/deleted feature upstreams, avoids reopening stale UI during
updates, recovers malformed small-model structured responses, and replaces the
Intiface queue-admission loop with deadline-driven asynchronous-ACK pacing.
Source rebuilds Stop and terminate only their checkout-owned app tree before
staged binary replacement, then verify the new server before opening the
browser. Broader LLM quality/latency, the revised Intiface pacer on hardware,
and real managed microphone acceptance are still measurements, not inferred
completion claims.

In this table, **Complete** means the scoped implementation and automated tests
landed. It does not imply that every real-hardware acceptance check, provider
provisioning path, or release gate has passed. Qualifications are shown in the
status column and in "Known Gaps Carried Forward" below.

| Phase | Scope | Status | PRs |
| --- | --- | --- | --- |
| 0 | Planning specs, ADRs, risk register | Complete | direct commits, #5 (voice ADR 0007) |
| 1 | Scaffold, CI, app shell, perf baseline | Complete | #1 |
| 2 | Settings, app state, migrations | Complete | #2 |
| 3 | Transport interface, fake Handy, traces | Complete | #3 |
| 4 | HSP v4 invariant tests, command shaping | Complete | #4 |
| 5 | Real Handy Cloud REST transport | Complete | #6 |
| 5B | Browser Bluetooth dispatch owner | Complete | #8 |
| 6 | Motion engine MVP | Complete | #7 |
| 7 | Retargeting + real-device validation runner | Complete | #9 |
| 8 | Motion UI and live visualizer | Complete | #10 |
| 9 | Local LLM chat driving motion | Complete | #11, #12 |
| 9B | App-path device validation, controller ownership | Implemented; reverse HW recheck open | #15, #16, #17, #22 |
| 10 | Memory, editable prompt sets, settings reset | **Complete** | #24 |
| 11 | Modes as motion clients (Freestyle, chat keepalive) | **Implemented; HW acceptance open** | #26 |
| 11B | SQLite persistence foundation (ADR 0008) | **Implemented; corrupt-DB recovery open** | #32, #33 |
| 12 | Voice worker boundary (protocol, lifecycle, stubs, status UI) | **Complete** | #41 |
| 13.0 | Delivery-ordering foundation (shared chat log, cursors, lockstep TTS, audio lease) | **Complete** | #42 |
| 13.1 | NeuTTS Air spike — non-Python decode proven, RTF ~0.5 CPU (R17) | **Complete** | #43 |
| 13.2 | ElevenLabs cloud TTS worker | **Complete** | #44 |
| 13.3 | Parakeet ASR worker (OpenAI-compatible proxy) | **Complete** | #45 |
| 13.4 | Managed Parakeet runner and interactive installer | **Complete** | #46 |
| 13.5 | Settings compaction: voice input/output split, provider-scoped fields | **Complete** | #49 |
| 13.6 | NeuTTS Air offline stream adapter | **Complete** | #49 |
| 13.7 | Push-to-talk microphone input and Chat voice controls | **Implemented; managed-provider E2E open** | #49 |
| 13.8 | Voice UX hardening: stacked chat layout, control gating, load/feedback loop | **Complete** | #51 |
| 14 | Pattern library, programs, authoring, and LLM curation | **Implemented; HW feel check open** | #52 |
| 14B | Intiface/Buttplug dispatch owner, transport-neutral frame contract (ADR 0010) | **Implemented; pre-async-pacer HW run passed, revised pacer HW run open** | #59, #67 |
| 14C | Floating connection manager, live limits, connection animation | **Implemented; post-#63 rendered QA refresh open** | #60, #63 |
| 16-pre | Model manager, managed llama.cpp, source installer/updater foundations | **Complete** | #55, #56, #61, #62, #64, #65 |
| 9/13 hardening | Small-model structured-output recovery | **Complete** | #66 |
| 15 | Migration importer and compatibility report | Not started | — |
| 16 | Windows packaging, first-run setup, release pipeline | **Foundations landed; release slices not started** | #55, #56, #61, #62, #64, #65 |
| 17 | Final parity/default-app readiness review | Not started | — |

Phase 13.0 note: the ADR 0003 delivery-ordering trio landed as its own PR
before any provider — the SQLite `messages`/`client_cursors` tables (schema
v2) are the canonical chat history (closing parity row 9: history survives
reload and reaches every tab), chat-emit and TTS-enqueue are lockstep (the
enqueued text is byte-identical to the logged reply; error/malformed paths
never reach either), and retained speak audio is served only to the active
controller (the single-owner audio lease), bounded per request and count.

Phase 11 note: Freestyle boundary behavior is proven on the real engine over
the fake transport (one continuous stream across many segment retargets, one
HSP play, zero stops). Real-hardware freestyle validation rides the next
manual device session, alongside the Bluetooth-endurance watch item.

Phase 9B closed with PR #22: the visible Edge Web Bluetooth flow selected the
real device, checked the connection, started motion at 28%, and stopped it via
deterministic chat — full app-path evidence for both dispatch owners lives in
`docs/perf-baseline.md`.

Phase 10 decision (2026-07-02): **chat history stays client-side for now.**
Server-side history was deliberately deferred so ADR 0003's shared message
log with per-client cursors could introduce it as the single canonical
history; building a separate Phase 10 history store would have created a
second source of truth. Resolved by Phase 13.0 (parity row 9 closed).

### What Exists On Main

- Motion engine with retargeting, latency-aware lead, phase preservation, and
  goroutine-lifecycle safety tests, bound at runtime to the **selected dispatch
  owner** (Cloud REST, Browser Bluetooth, or Intiface) from settings; a fake
  transport is used only for tests and the diagnostics placeholder.
- Cloud REST and Browser Bluetooth transports with diagnostics parity, SSE
  event endpoints, no-fallback behavior, and manual test endpoints.
- Motion-first UI: persistent navigation rail, status-only top bar, floating
  connection/limit manager, routed controls, single engine-state visualizer,
  always-visible Stop (plus Escape), diagnostics panel, and trace export.
- Streaming LLM chat (managed llama.cpp primary, external llama.cpp, Ollama
  secondary) with a strict JSON contract, one repair pass, malformed-response
  indication, bounded output, explicit automatic/off reasoning policy, and
  chat-driven motion through the engine only. Provider-native controls carry
  visible latency/quality warnings; no hardware tuning knob is promoted without
  measurement.
- SQLite-backed LLM model manager with managed GGUF copies, standalone GGUF
  import, read-only Ollama library discovery/import, daemon model listing,
  external llama.cpp model listing, SHA-256 verification,
  progress/cancellation, and guarded ID-based selection/removal. Managed mode
  builds pinned llama.cpp `b9966` source into app-owned runtime storage through
  the installer or controller-gated Model UI; no runner/model path settings.
  Curated model downloads remain release work.
- Optional voice workers remain off and never autostart. Source-installed
  Parakeet assets are discovered as one app-managed module with visible
  complete/incomplete state; custom local server/model paths are a separate
  source selection. Saving enablement exposes Start, which succeeds only after
  model readiness.
- SQLite-backed pattern and finite-program library with generated built-ins,
  share-file/funscript import and export, shared-engine playback, backend-sampled
  previews, sparse freehand authoring, and visible reversible preference
  training. The LLM can select only enabled pattern IDs and falls back to the
  deterministic semantic target contract when no library entry applies.
- CI: gofmt, `go vet`, `golangci-lint`, tests, race tests, `CGO_ENABLED=0`
  build, import-boundary tests.

### Known Gaps Carried Forward

These are tracked so they cannot silently become permanent. Goal and budget
status now lives in `docs/goal-scorecard.md`; this list is the open remainder.
(Closed by Phase 9B PRs #15-#17 and the follow-up close-out branch: app-path
Cloud REST hardware validation, owner-switch semantics, controller
enforcement, motion SSE, active RSS and soak measurements, parity rows
1-4/6/8, BLE-session extraction from `web/app.js`, and automated source file
line-budget checks.)

(Also closed since: Browser Bluetooth full app-path validation — PR #22;
editable prompt sets, memory, and reset-to-defaults — Phase 10.)

1. **Open parity rows**: none — the last one (server-side chat continuity,
   row 9) closed with Phase 13.0; pause/resume shipped early in the
   post-Phase-10 shell pass. See `docs/ui-design.md`, "Functional Parity
   Baseline".
2. **Browser Bluetooth endurance** is unproven beyond short sessions; the
   one-hour soak ran on Cloud REST only (scorecard watch list).
3. **Second parity sweep (2026-07-09)**: the Phase 14 motion/library items and
   latency-aware mode dwell floor are now implemented and tested. The remaining
   work is the routine-cycle feel check on real hardware, the Handy 2 scope
   review on R16, and the explicitly deferred diagnostics/UI follow-ups. See
   `docs/legacy-parity-sweep-2026-07.md` for row-level dispositions.
4. **Third legacy sweep (2026-07-11)**: the full StrokeGPT-ReVibed PR history
   (#21–#333) mined for lessons the earlier sweeps missed, dispositioned
   skeptically in `docs/legacy-lessons-sweep-2026-07-11.md`. Near-term items:
   verify mic→parakeet.cpp audio-format compatibility end to end, prune
   abandoned `client_cursors` rows, and stop blocked chat sends from eating
   the draft. The autospeak contract shapes feed the Autopilot slice; the
   motion items (differentiated retarget lead, semantic no-op guard,
   clamp-once speed test) carry explicit skepticism notes — contract shapes
   over copied constants.
5. **Emergency Stop delivery**: active, paused, repeated-idle, and no-engine
   requests now attempt the selected dispatch owner's Stop while preserving
   local stopped state and reporting transport failure honestly. An unreachable
   backend still cannot deliver Browser Bluetooth Stop, and current Cloud REST /
   Browser Bluetooth retry evidence remains open. No document may claim physical
   delivery after communication fails. Tracked as risk R23.
6. **Voice end-to-end acceptance**: hold-to-talk decodes MediaRecorder output,
   while hands-free capture remains active until the user stops it and uses
   browser VAD to submit each phrase as PCM WAV to the managed parakeet.cpp
   path; compressed/fake-WAV payloads fail at the API boundary.
   Provider adapters, source-installer asset discovery, app-managed/custom
   separation, explicit enable/save/Start UI, and guarded Windows host-path
   browsing are implemented. A real Chrome/Edge transcription run remains R24
   exit evidence. The source installer now builds and discovers a pinned NeuTTS
   runner/decoder/backbone with managed llama.cpp. Settings can safely normalize
   official sample-style `.pt` files and compatible one-dimensional int32 `.npy`
   files without Python; arbitrary-WAV encoding and enforced offline operation
   remain R17.
7. **Current-build performance evidence**: the post-SQLite build has current
   idle/API-read measurements, but active motion and the one-hour soak were last
   measured before SQLite. Those rows remain unmeasured for the current build.
8. **Release provisioning**: `install.ps1` now builds every first-party Go voice
   adapter and can provision a clean Windows source machine, including the
   compiler; `update.ps1` preserves or revises those choices. Managed llama.cpp
   and the coupled NeuTTS runtime/assets still build/provision outside the core.
   Phase 16
   must provide checksummed prebuilt runtimes before the GUI setup path can avoid
   installing Git/CMake/Visual Studio rather than merely automating them.

### UI Shell Redesign (Sidebar Navigation)

The UI has moved to React and from the former status-bar +
single-control-sidebar + settings-window shell to a **permanent left navigation
sidebar that switches pages** (Chat / Preset Modes / Pattern Library /
Settings), with Stop pinned to the sidebar footer on every page. Framework
decision and historical handoff:
[docs/decisions/0009-react-frontend.md](docs/decisions/0009-react-frontend.md)
and [docs/react-ui-implementation-handoff.md](docs/react-ui-implementation-handoff.md).
Full shell spec: [docs/ui-navigation-redesign.md](docs/ui-navigation-redesign.md).
It ships in steps that never drop a safety control mid-migration:

1. **React migration scaffold**: Vite + React + TypeScript static build embedded
   by Go; preserve current visible safety behavior before changing the shell.
2. **Shell refactor** (front-end only): nav sidebar + top-level router, Stop to
   the pinned footer, current controls move into the Chat page, settings window
   becomes the Settings page.
3. **Preset Modes + Autopilot**: relocate Freestyle; add an LLM-driven
   **Autopilot** mode in `internal/modes` that changes direction/pattern from
   context through bounded arrangement segments — an engine client, traced,
   Stop/Pause-interruptible, clamped by the quick-settings envelope. Rides the
   Phase 11 mode architecture.
4. **Pattern Library**: the browse/import/player/authoring/curation workspace —
   implemented in Phase 14.

Status: steps 1 and 2 have landed together — the UI is now a Vite + React +
TypeScript app (`web/`, built to `web/dist`, embedded by Go; no runtime Node)
implementing the permanent nav rail, compact status-led top bar, pinned Stop, and the
Chat / Preset Modes / Pattern Library / Settings routes with the safety
invariants (Stop outside routes, backend-loss lock, read-only lock) under
Vitest. Preset Modes is present while Autopilot remains a labeled coming-soon
control until its planner exists (step 3). Pattern Library is merged to `main`
(step 4). The legacy vanilla UI remains under `web/legacy/`
as unshipped reference only; `web/dist` is the single embedded frontend.

## Rewrite Guardrails

- Do not start by porting every feature.
- Build a better motion and transport foundation first.
- Preserve hard-won StrokeGPT-ReVibed HSP constraints as tests.
- Keep semantic motion intent separate from physical transport output.
- Keep modes as clients of the motion engine, not alternate motion engines.
- Make real-device validation a first-class milestone.
- Define parity and kill milestones so the parallel rewrite does not run forever.
- Track the rewrite goals as measurable targets, not claims: memory, binary
  size, and startup are budgeted and checked (see `docs/goals-and-guardrails.md`).
- Keep the core pure-Go (`CGO_ENABLED=0`); native-only needs such as BLE or
  native audio stay behind the browser bridge or a worker, never in the core.
- Enforce maintainability in CI (lint, import boundaries, size norms) so Go
  does not grow its own god-modules. The same norms apply to `web/`.
- Treat the motion goroutine lifecycle as a safety gate (leak and stop-teardown
  tests), because a goroutine that commands the device after stop is unsafe.
- Prefer adjusting the active plan over replacing the active stream for routine
  changes; reserve stream replacement for genuine pattern changes (see
  `docs/motion-retargeting.md`, "Route Policy Learned On Hardware").
- Keep new-feature settings surfaces minimal until defaults are proven in real
  use; expose tuning knobs in diagnostics first, promote them to settings only
  with evidence (the StrokeGPT voice tab grew 12 knobs before defaults were
  validated).

## Goal-Ready Phase Workflow

Each phase below is written so a future `/goal` can complete it end-to-end. A phase should end with:

- code committed and pushed to a scoped branch
- tests passing for the phase
- documentation updated when behavior or architecture changes
- `docs/goal-scorecard.md` updated: affected rows re-scored, budgets
  re-measured when the phase touches size/memory/startup, and a History entry
  appended
- a PR opened unless the phase is explicitly local-only planning
- clear notes about what was intentionally not implemented

Use branch prefix `codex/` when Codex submits and `claude/` when Claude submits;
other contributors and tools use their own clear branch names. Every branch —
whoever or whatever authored it — meets the shared floor in
[AGENTS.md](AGENTS.md) and passes green CI before it merges to `main`.

The project is being combined with LSO (Local Stroke Orchestrator) on this Go
core. Integration is planned in
[docs/lso-merge-integration.md](docs/lso-merge-integration.md), with the open
architectural decisions and their trade-offs in
[docs/lso-merge-alternatives.md](docs/lso-merge-alternatives.md). Merge work
lands on feature branches and reaches `main` by PR under the same standards and
gates as the rest of the project.

When a phase changes visible UI, verify against the live rendered DOM (run the
app headless and inspect/screenshot), not source-only reasoning; source-only
review of UI bugs produced confident wrong fixes in StrokeGPT-ReVibed.

## Target Architecture

```text
MagicHandy/
  cmd/magichandy/          app entrypoint                          [exists]
  cmd/retarget-validate/   Phase 7 real-device validation runner   [exists]
  internal/config/         settings, migrations, defaults          [exists]
  internal/httpapi/        REST and SSE routes                     [exists]
  internal/chat/           chat contract, prompt sets, service     [exists]
  internal/llm/            providers, model inventory/imports      [exists]
  internal/motion/         targets, plans, sampler, retargeting    [exists]
  internal/transport/      Cloud, browser Bluetooth, Intiface owners [exists]
  internal/diagnostics/    trace ring, export                      [exists]
  internal/validation/     retarget validation checklist           [exists]
  internal/modes/          freestyle, continuous-chat planners     [exists]
  internal/patterns/       patterns, programs, import, feedback    [exists]
  internal/memory/         long-term memory store                  [exists]
  internal/store/          SQLite datastore, schema, migrations    [exists]
  internal/voice/          queue, worker protocol, ASR/TTS clients [exists]
  web/                     frontend assets                         [exists]
  docs/
```

# Phase 9B: App-Path Device Validation And Controller Ownership

## Suggested `/goal`

`/goal Complete MagicHandy Phase 9B: define and test dispatch-owner switching semantics, enforce a single active controller with read-only extra clients, push motion state over SSE, validate the full app path (UI and chat driving the engine) on real hardware for both dispatch owners, and record the active-RSS and soak measurements.`

## Objective

Close the gap between "the engine was validated by a dedicated runner" and
"the shipped app is validated." After this phase, the app that users run has
commanded a real device through both dispatch owners, ownership rules are
enforced rather than advisory, and the remaining goal measurements exist.

## Scope

Implement:

- explicit dispatch-owner switching semantics: changing
  `hsp_dispatch_owner` while motion is active stops motion first (traced,
  user-visible), never silently rebinds or dual-commands; tests cover the
  switch-while-running path
- single-active-controller enforcement: one client owns command routes; other
  clients get read-only state plus Stop (design in `docs/ui-design.md`,
  "Connection And Single-Controller"); stale-controller takeover is explicit
- motion state pushed over SSE (reuse the cloud/bluetooth event pattern) with
  the polling loop kept as fallback; the visualizer shows an explicit stale
  state when the stream drops
- BLE session handling is split out of `web/app.js`; `web/app.js` is back under
  the size norms and browser-owned BLE now lives in `web/bluetooth-ui.js`
- automated file-length checks live in `internal/architecture` with a
  grandfathered ceiling for existing oversized files, so size norms no longer
  depend on manual review
- restore the proven StrokeGPT-ReVibed failure-handling behaviors
  (`docs/ui-design.md`, "Functional Parity Baseline"): persistent
  connection-lost banner plus backend-required control lock, a visible cloud
  connection-check action with transport status in the persistent bar,
  commanded-estimate labeling on the visualizer, a documented visible Stop
  shortcut, and chat scrollback stickiness with a jump-to-latest affordance
- add a copyable diagnostics summary (one-click bug-report text) — required
  by this phase's own real-device validation runs
- record Go active-motion RSS and the one-hour sustained-motion soak per
  `docs/goals-and-guardrails.md` (fake transport injection is acceptable for
  the soak; real transport for a shorter active sample)

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual real-device checklist (both Cloud REST and Browser Bluetooth):

- connect through the normal settings UI
- start/stop motion from the control bar
- quick settings apply to active motion immediately
- chat-driven start/adjust/stop moves the device
- kill the backend process: the UI shows the connection-lost banner, locks
  backend-required controls, and recovers when the backend returns
- dispatch-owner switch during active motion stops first and reports why
- second browser tab is read-only but can trigger Stop
- emergency stop works from both tabs
- trace export captures the session

## Done Criteria

- The full app path has moved a real device through both dispatch owners.
- Owner switching and controller ownership are enforced by tests, not advice.
- Motion state reaches the UI by push with a visible stale state.
- The Phase 9B rows of the Functional Parity Baseline
  (`docs/ui-design.md`) are closed: connection-lost banner and control lock,
  visible connection check, estimate labeling, documented Stop shortcut,
  scrollback stickiness, copyable diagnostics.
- `docs/perf-baseline.md` gains active RSS and soak rows.
- Known unresolved motion limitations are re-documented after hardware runs.

## Out Of Scope

- modes, memory, voice
- native Go Bluetooth
- multi-user/remote sessions (post-parity backlog)

# Phase 10: Memory And Prompt Management

## Suggested `/goal`

`/goal Complete MagicHandy Phase 10: implement long-term memory with individual removal and enable/disable, editable prompt sets with protected defaults, the model/prompt/memory settings UI, and tests. Chat continues to work with memory disabled.`

## Objective

Add model personalization and prompt management in a maintainable way. The
current prompt layer is a single hardcoded set (`internal/chat/prompts.go`);
this phase makes prompt sets and memory user-owned data.

## Scope

Implement:

- memory store (add, enable/disable, individual removal, clear all)
- memory injection into the prompt with a visible on/off switch
- prompt sets: create/edit/delete/select, persisted; bundled defaults are
  read-only templates that users copy, not editable in place
- persona/anatomy fields if not already covered by prompt sets
- UI for model/prompt/memory settings as routed views per `docs/ui-design.md`
- explicit reset-to-defaults for settings (parity baseline item; today only
  corrupt-file auto-recovery exists)
- decide chat-history persistence: server-side history that survives reload
  is groundwork for the Phase 12 shared message log (ADR 0003), so at minimum
  record the decision here rather than letting client-only history calcify

Rules carried from StrokeGPT-ReVibed:

- memory is inspectable and resettable; nothing is buried in opaque
  natural-language state the user cannot see or edit
- any model-visible preference the app derives (styles, weights, feedback)
  must appear in the UI immediately when it changes and be reversible
- prompt-set changes never alter the JSON motion contract; the contract is
  code, not prompt text

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks: add/remove/disable memories and verify the prompt excludes
disabled ones; prompt-set selection persists across restart; defaults cannot be
destroyed.

## Done Criteria

- Memory is transparent and manageable; chat works with memory off.
- Prompt sets are editable without modifying code; defaults are protected.

## Out Of Scope

- voice
- memory import from StrokeGPT-ReVibed (Phase 15)

# Phase 11: Modes As Motion Clients

## Suggested `/goal`

`/goal Complete MagicHandy Phase 11: implement Freestyle and continuous-chat motion as clients of the motion engine with a bounded motion-arrangement contract, traceable planner decisions, deterministic style scoring, and no separate motion pathway.`

## Objective

Introduce autonomous motion behavior without recreating the old split motion
architecture or the old failure cluster: regular Freestyle stops,
end-of-sequence stalls, and batch-boundary starvation.

## Scope

Implement:

- Freestyle MVP and normal-chat keep-moving behavior in `internal/modes`
- a bounded **motion arrangement** contract: a planner emits named segments
  (pattern/style, focus region, duration or cycle count, intensity drift) and
  deterministic code compiles them into continuous plans with explicit
  transition rules — the LLM/planner never triggers low-level stream
  replacement per turn (see `docs/motion-retargeting.md`, "Route Policy
  Learned On Hardware")
- planner decision type and planner trace rows (choice, score/weights, sleep
  cadence) so a stopped device is diagnosable as planner-wait vs transport
  failure
- mode start/stop API and UI controls; Stop and settings changes interrupt and
  apply during modes
- engine-level **pause/resume** shipped early (post-Phase-10 shell pass):
  Phase 11 planners must stay interruptible by it — a paused mode resumes its
  plan with preserved phase, and chat/planner keepalives never restart motion
  that the user paused; Stop stays the safety path and is never replaced by
  pause

Rules:

- modes produce semantic targets/plans only; the import boundary keeps them
  off `transport`
- each segment runs long enough to establish a feel (multiple cycles or a
  timed window), not one chat turn
- variation comes from changing targets over time within the safety envelope,
  not from rapid oscillation around one target
- if a saved motion-style preference exists, deterministic planner scoring
  consumes it directly; it is not only prompt bias
- planner cadence must outpace buffer consumption: generation is continuous
  ahead of the lead-time window, and a starvation test proves no stall at
  segment boundaries

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks: Freestyle keeps moving across many segment boundaries without
regular stops; chat mode keeps moving between turns; planner decisions visible
in diagnostics; stop interrupts instantly; settings apply during modes.

## Done Criteria

- Modes use only the motion engine API.
- Continuous mode does not stop between chat turns; Freestyle survives segment
  boundaries without stalls on real hardware.
- Planner decisions are visible in diagnostics.

## Out Of Scope

- Edge/Milk legacy parity unless explicitly requested
- pattern library authoring (Phase 14)
- voice

# Phase 11B: SQLite Persistence Foundation

## Suggested `/goal`

`/goal Complete MagicHandy Phase 11B: introduce a single pure-Go SQLite datastore (modernc.org/sqlite) behind the existing settings, memory, and prompt-set store interfaces, with schema migrations, a non-destructive one-time JSON import, and re-measured budgets.`

## Objective

Replace the three independent JSON stores with one embedded, transactional
SQLite datastore (ADR 0008) so the Phase 13.0 chat log and Phase 14 pattern
library have a store shaped for them, without regressing the pure-Go,
single-binary, low-memory guarantees.

## Scope

Implement:

- `internal/store`: a `modernc.org/sqlite` connection (WAL, `foreign_keys` on,
  bounded cache, `busy_timeout`, serialized single writer) and a forward-only
  migration runner keyed on `PRAGMA user_version`
- move settings, memory, and prompt sets onto DB tables behind their current
  interfaces (`config.Store`, `memory.Store`, `chat.PromptLibrary` keep their
  method signatures and contracts): settings as a versioned document row,
  memory and prompt sets as relational rows
- a non-destructive one-time import: when legacy JSON files exist, each legacy
  store imports its domain in one SQLite transaction and renames the JSON file
  `*.migrated` only after the DB commit; settings import is reported in load
  status
- preserve every existing contract: corrupt-store recovery to safe defaults,
  the redacted settings view (connection key never returned), and "reset
  settings does not touch memory or prompt sets"
- re-measure binary size and idle/active RSS; record in `docs/goal-scorecard.md`

Rules:

- pure-Go only; the `CGO_ENABLED=0` build and depguard `C` denial stay green
- no behavior visible to the API or UI changes beyond load status reporting the
  import; the store swap is substrate-only
- do not normalize settings into columns (ADR 0008): keep the document plus the
  existing migration/redaction machinery

## Validation

```powershell
go test ./...
go test -race ./...
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
```

Fixtures cover import from present, absent, and corrupt JSON stores; a migration
test covers forward migration and the newer-than-binary error; redaction tests
still pass; budgets are re-measured.

## Completion Evidence

- `modernc.org/sqlite v1.53.0` is pinned with Go 1.25-compatible transitive
  packages and validated locally with Go 1.26.4; `CGO_ENABLED=0` build remains
  required.
- Settings, memories, and user prompt sets round-trip through `magichandy.db`;
  legacy `settings.json`, `memories.json`, and `prompt_sets.json` fixtures
  import and archive to `*.migrated`.
- Binary size: 17.92 MB plain / 12.32 MB stripped, under the 30 MB stripped
  budget.
- RSS waiver: stripped SQLite build idles at 54.13 MB after `/healthz` and
  54.36 MB after `/api/state`, `/api/settings`, `/api/memory`, and
  `/api/prompt-sets`; this exceeds the original <40 MB idle budget and is
  recorded in `docs/goal-scorecard.md`.

## Done Criteria

- Settings, memory, and prompt sets persist through one SQLite datastore with no
  API/UI contract change; the one-time JSON import is non-destructive and
  tested.
- The `CGO_ENABLED=0` build stays green; binary and RSS budgets are re-measured
  and recorded (or a waiver is recorded).

## Out Of Scope

- the chat message log and per-client cursors (Phase 13.0, ADR 0003)
- the pattern/program library tables (Phase 14)
- StrokeGPT-ReVibed import (Phase 15) — same schema target, separate phase
- at-rest encryption (the trust model stays a single local operator)

# Phase 12: Voice Worker Boundary

Status: **implemented.** The protocol spec lives in
[docs/voice-worker-protocol.md](docs/voice-worker-protocol.md); the wire
format is `internal/voice/protocol`, lifecycle and the core-owned queue are
`internal/voice`, and the model-free stub is `cmd/voice-stub-worker` +
`internal/voice/stubworker`.

## Suggested `/goal`

`/goal Complete MagicHandy Phase 12: implement the optional voice worker protocol, worker lifecycle management, voice status UI, and stub workers; do not bundle heavy ML models yet.`

## Objective

Add voice architecture without pulling ML dependency instability into the Go
core.

## Scope

Implemented:

- versioned worker protocol (v1): NDJSON request/response envelopes over
  stdio, hello negotiation, health/status with model state and queue depth,
  cancellation by request ID, per-request timeouts, structured error codes,
  no-speech rejection (never an empty transcript into chat)
- worker process lifecycle: settings-driven configure (never autostart),
  start/stop/restart, handshake teardown on version mismatch, crash detection
  with stderr tail, deliberate-stop vs crash distinction, goleak-gated
  goroutine teardown; missing/disabled/unconfigured are visible states
- core-owned serialized request queue (bounded; rejects when full) with a
  tracked recent-request log
- stub TTS and ASR workers (one binary, `-role`) used by protocol tests,
  process-lifecycle tests, and manual checks; supports delay, crash, and
  fail-start injection — no ML models
- voice settings section + worker status UI (dot+text states, provider
  identity, queue depth, last error, start/stop/restart/load/test/cancel)

Delivery-ordering rules (ADR 0003, risk R15) — the parts that need audio
playback and a real provider (lockstep chat-emit/TTS-enqueue, per-client
cursors over one shared message log, the single-owner audio lease) are the
**first work item of Phase 13**, before any provider is wired to chat. Model
errors already stay out of history, TTS, and motion: worker failures terminate
in the voice request log.

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks (all verified live against the stub): app runs without workers;
stub worker starts/stops; startup crash and mid-request crash are visible with
stderr; cancellation interrupts an active request; model load/unload; settings
save reconfigures without autostart.

## Done Criteria

- Voice is optional; the protocol is versioned and tested; the core app
  remains usable without Python. Met: the stub stack is pure Go, workers are
  separate processes, and every voice state is a readout, never a blocker.

## Out Of Scope

- real providers, audio playback, and the delivery-ordering trio above
  (Phase 13)
- CUDA setup scripts

# Phase 13: Voice Feature Implementations

## Suggested `/goal`

`/goal Complete MagicHandy Phase 13: implement the non-Python voice providers one per PR — starting with the NeuTTS Air spike, then ElevenLabs cloud TTS, then Parakeet ASR — keeping the core app functional without voice and the voice settings surface minimal.`

## Objective

Add the selected non-Python voice providers incrementally behind the worker
boundary (ADR 0007).

## Scope

One provider per PR/subphase, in this order:

0. **Delivery-ordering foundation first** (carried from Phase 12, ADR 0003,
   risk R15): the shared chat message log with per-client cursors (the ADR
   0008 `messages`/`client_cursors` tables), lockstep chat-emit/TTS-enqueue,
   and the single-owner audio lease — landed before any provider speaks a
   chat reply, so spoken-equals-shown is guaranteed from the first provider.
   **Done** (schema v2; `speak_replies` setting; lease-gated audio endpoint;
   spoken-equals-shown, cursor-isolation, and model-error tests).
1. **NeuTTS Air spike first** (risk R17): prove the non-Python NeuCodec
   decode path and cloning quality/latency before the full integration; if the
   spike fails, document the fallback (F5-TTS ONNX or optional Python worker)
   and continue with the other providers
2. ElevenLabs cloud TTS (HTTP; expressive + high-fidelity cloning premium)
3. Parakeet-TDT ASR through an external OpenAI-compatible proxy; external
   server choices remain interchangeable, with Whisper an optional alternate
   later
4. **Managed Parakeet runner + installer**: use the existing parakeet.cpp
   `parakeet-server` as a worker-owned, loopback-only process with a local GGUF
   model; add an explicit, checksum-verified installer path. Do not introduce a
   second inference loop, CGo into the core, automatic downloads, or a new
   motion path.

### Slice 13.4: Managed Parakeet Runner And Installer

Status: **complete** in PR #46.

- Select parakeet.cpp over a direct sherpa-onnx binding for the first Windows
  path: its release already contains `parakeet-server`, local GGUF support,
  `/health`, and an OpenAI-compatible transcription endpoint.
- Extend `voice-parakeet-worker` with an external mode and a mutually exclusive
  managed mode (`-server-path`, `-server-model`, optional `-server-port`). The
  worker owns only the process it starts; unload, shutdown, and EOF cancel work
  and stop that child.
- Extend `install.ps1` with an opt-in CPU runner/model download showing size,
  license, and checksum. Install under the local data directory, build the Go
  worker, and leave voice disabled until the user explicitly saves and starts it.
- The app-managed runtime source resolves those canonical assets without path
  inputs and reports complete/incomplete/missing module state. Custom local
  server/model paths remain a separate source. Installation never implies
  enablement or autostart; Save then Start is explicit, and Start includes load.
- Keep the ordinary Voice settings surface minimal. Worker arguments are
  structured one-per-line values so Windows paths with spaces survive without a
  homemade shell parser. Lifecycle actions are state-specific rather than a row
  of disabled controls.
- Validate the external `/v1/models` fallback, parakeet.cpp `/health` readiness,
  managed startup-once behavior, port conflict, unload, EOF cleanup, and the
  valid-WAV test request. A production-boundary run now starts the installed
  CPU runner and pinned model, transcribes a canonical 16 kHz Dave fixture, and
  proves worker/process cleanup. A real managed-Parakeet browser-microphone run
  remains required before closing R24 or claiming microphone-path release
  readiness. The user-started hands-free UI is implemented with bounded VAD and
  manual Stop; its real-microphone segmentation and latency remain release
  evidence rather than a runner-format blocker.

### Slice 13.5: Settings Compaction (Voice Provider Model)

Status: **complete**. Design and implementation notes:
[docs/settings-compaction.md](docs/settings-compaction.md).

- Selection-scoped disclosure across Settings: fields render only when the
  selected provider/mode makes them meaningful; switching selections never
  destroys hidden values; status readouts are never hidden.
- The Voice tab splits into **Speech input (ASR)** and **Speech output
  (TTS)** sections on one page, each with a provider dropdown (ASR: none /
  managed Parakeet / OpenAI-compatible server / custom worker; TTS: none /
  ElevenLabs / NeuTTS Air placeholder / custom worker) and its own worker
  status row; speak-replies renders only when a TTS provider is set.
- Backend-authoritative provider model: `VoiceSettings` gains per-role
  provider discriminators + provider fields (additive, settings version
  unchanged); Go composes worker command/args (templates tested in Go, not
  frontend string-pasting); known worker binaries resolve by documented
  order (override → beside the app executable → data-dir tools) so path
  fields disappear from the common case; legacy path/args settings load as
  `custom` with identical launch behavior.
- Model and Device tabs get the same disclosure rule (llama.cpp vs Ollama
  fields; managed vs external mode; developer app-ID only on override).

### Slice 13.6: NeuTTS Air Stream Adapter

Status: **complete**.

- `voice-neutts-worker` adapts the non-Python `neutts-rs` `stream_pcm`
  runner to ADR 0003 and forwards live signed 16-bit 24 kHz PCM chunks.
  The core wraps retained PCM as WAV only at the playback boundary, preserving
  the final samples without delaying worker-side streaming or cancellation.
- The child requests Hugging Face offline mode. Because the pinned client does
  not enforce that flag, the default Air Q4 path requires an exact local cache
  entry before the worker starts. Worker load probes the pinned runner's CLI
  contract without loading the model or synthesizing audio. The Windows source
  installer now builds and checksum-verifies the runtime assets with managed
  llama.cpp. A network-denied test, persistent model host, prebuilt packaging,
  and arbitrary-WAV reference encoding remain R17/Phase 16 rather than adapter
  claims.
- The current `neutts-rs` 0.1.1 encoder export is a stub despite example text
  suggesting otherwise. Settings provides a controller-gated reference window
  that safely normalizes the official sample-style Torch ZIP `.pt` layout or a
  compatible one-dimensional int32 `.npy`, copies an optional validated WAV for
  preview, and guides exact transcript entry. It never executes pickle and does
  not silently invoke Python or claim to encode an arbitrary WAV.
- Setup and the exact capability boundary are documented in
  [docs/neutts-worker.md](docs/neutts-worker.md).

### Slice 13.7: Browser Voice Input

Status: **complete** for the supported localhost browser path.

- The Chat microphone defaults to user-started hands-free capture that remains
  active until manually stopped. An AudioWorklet supplies mono PCM to a bounded
  VAD segmenter; speech phrases are queued serially for transcription while the
  microphone continues listening. The adjacent disclosure also offers
  hold-to-talk, browser audio-input selection, sensitivity, end-of-speech delay,
  noise suppression, input level, and queue status. These preferences persist
  through the backend settings store.
- Browser recording captures the best available format, then decodes,
  downmixes, resamples, and encodes it into 16 kHz PCM WAV in one output-rate
  pass. The browser uploads raw WAV; the pure-Go HTTP edge validates it and
  hands the worker a private, short-lived `audio_ref` instead of a multi-megabyte
  base64 NDJSON frame. The accepted transcript calls the
  same chat send function as typed text, preserving every motion limit,
  smoothing rule, controller lease, and Stop behavior.
- Individual hands-free phrases and the pending queue are bounded; hold-to-talk
  remains capped at 30 seconds. Transcription has a visible busy/queue state and
  timeout cancels the worker request without stranding later queued phrases.
  Emergency Stop discards capture and pending transcription immediately; a
  backend stop sequence carries the same cancellation signal to other clients,
  invalidates ASR/TTS results and playback, cancels in-flight LLM work, and
  combines request stamps with an engine Stop-admission generation so stale
  voice, typed-chat, manual, library, and mode starts cannot dispatch afterward.
- `http://localhost` is the supported microphone origin. LAN/mobile use is not
  promised until Phase 16 provides an HTTPS and certificate design.
- Background auto-start and capture that continues without a visible user-owned
  session remain out of scope. Real-microphone calibration and first-word
  accuracy remain R24 release evidence.

### Slice 13.8: Voice UX Hardening

Status: **complete**. Source: the 2026-07-10 live UI/UX pass over merged
13.5-13.7 ([docs/ui-ux-review.md](docs/ui-ux-review.md)); fix order and
file/line anchors live there. The slice also replaced the sidebar's active
link treatment: the inset accent bar (clipped into a hook by the corner
radius) became a soft azure fill (`--accent-tint`), with hover staying
neutral graphite.

- Stacked-layout (≤900px) chat fix: the log's `min-height` overflows its
  collapsed shell and paints over the composer; bound the log height at the
  single-column breakpoint so the textarea, mic, and Send stay visible
  (review H1).
- Gate the Chat voice controls on usability, not just provider selection:
  mic hidden without a configured ASR provider and disabled-with-hint when
  the worker is not running; speak-replies quick toggle requires
  `voice.enabled` too. Never autostart stays intact (M1, M2).
- Close the speak-replies loop: auto-send `load` after a user-initiated
  Start of a first-party provider (or on first speak), so replies do not
  fail silently with `model_not_loaded` (M4).
- Make outcomes visible: per-role last-result readout from the request log,
  play the "Send test" clip through the lease-gated audio endpoint, and a
  selection-scoped status-bar voice dot only when voice is enabled and
  unhealthy (M3, M5).
- Polish: worker controls disabled while the section has unsaved changes,
  labeled controller readout at narrow widths, visible 30s recording cap
  (M6, L1, L3).

Each provider must include: setup documentation, load/unload behavior, status
diagnostics, queue/cancellation behavior, sentence-level streaming, and
failure messages that do not crash the core app. Keep TTS off the LLM's GPU
where practical.

UI rules learned from StrokeGPT-ReVibed:

- the routine voice settings surface stays minimal: provider, mode,
  transcript handling, model choice, one calibration/sensitivity path;
  recognition/tuning internals (beam size, VAD thresholds, noise floors) live
  in diagnostics until real-microphone testing proves users need them
- large model downloads are explicit UI actions with visible progress; no
  startup or status check downloads models
- recognized speech routes through the same chat/motion path as typed chat;
  voice never bypasses limits, smoothing, or stop
- hands-free or typed-chat mode-action permissions are off by default and
  per-mode
- physical Stop stays independent of recording/transcription/TTS latency

Secure-context constraint: browser microphone capture (and Web Bluetooth)
require HTTPS on non-localhost origins. Localhost use needs nothing; if
LAN/mobile voice is in scope, it needs a deliberate HTTPS/certificate story
(StrokeGPT required a local CA, an Android cert helper, and exact-IP SANs).
Decide the scope explicitly here and document it; do not accidentally promise
mobile voice (see risk R18).

## Validation

Provider-specific tests plus the standard suite. The managed path still requires
manual checks with a real microphone: missing dependency reported clearly,
provider loads, cancellation works, queue depth is visible, the app survives
  provider failure, continuous segmented hands-free capture and hold-to-talk
  both work with default settings, and sensitivity/end-of-speech controls remain
  usable without exposing provider internals.

## Done Criteria

- NeuTTS Air (or its documented fallback), ElevenLabs, and Parakeet work
  behind the protocol with no Python required.
- Sentence streaming works; spoken text always matches displayed text.
- The default voice settings surface stays small.

## Out Of Scope

- optional Python workers (Chatterbox, CosyVoice) — the protocol door stays open
- background or unattended microphone auto-start without a visible user-owned
  capture session

# Phase 14: Pattern Library, Programs, And Authoring

Status: **complete** — merged 2026-07-11 (#52).

## Suggested `/goal`

`/goal Complete MagicHandy Phase 14: implement the motion pattern library, program/funscript import, pattern playback through the motion engine, an LLM curation contract, and a simplified authoring UI with sane simplification/interpolation.`

## Objective

Bring authored content into the new motion architecture without recreating the
old pattern playback pitfalls, and give the LLM a curation surface instead of
raw motion authoring.

## Scope

Implement:

- built-in pattern format, user pattern registry, program/funscript registry,
  import/export
- playback through the shared motion engine only (risk R14)
- **LLM curation contract**: the model selects `{pattern_id, intensity}` from
  enabled library entries as its primary motion vocabulary once the library
  exists; the deterministic semantic-target path stays as fallback so the
  model is never silenced when nothing matches; disabled patterns are never
  selectable (tested)
- authoring canvas: freehand drawing with simplification and interpolation
  controls, preview from the backend sampler (never a client-side guess)
- import hygiene: strip long inactive gaps from video-synced funscripts when
  used as pattern examples — those gaps are media timing, not motion intent
- feedback (thumbs) that adjusts weights/enablement only visibly and
  reversibly; auto-disable is opt-in
- **training module**: the device auditions enabled patterns (including
  generated ones) and the user rates them; ratings feed the same visible
  weights; user patterns live as individual shareable files with
  import/export (browse-to-file)

Motion-feel requirements carried from StrokeGPT-ReVibed's June 2026
hardware iteration (`docs/legacy-parity-sweep-2026-07.md` §A — these were
paid for on a real device; do not rediscover them):

- patterns are authored on a 0–100 **relative span** and projected into the
  stroke window exactly once at dispatch (double projection collapses
  amplitude into twitches)
- the catalog is **generated parametrically** with wall-clock acceleration
  and reversal-gap budgets enforced by the generator, not hand keyframes
- sampling is **time-parameterized monotone cubic** (PCHIP-style): C1 in
  wall time, no overshoot, zero-velocity reversals
- routine pattern cycles get a **~6.6 s floor** (time-only stretch; burst
  shapes exempt) — shorter cycles stuttered on hardware and the signal was
  invisible in synthetic analysis
- mode planners (Freestyle, Autopilot) hold each applied target for at
  least the recent measured command latency plus padding (**latency-aware
  dwell floor**) so latency spikes never cause replace-thrash

Deeper editing (undo/redo history, mirror/repeat/variation transforms,
multi-pattern sequencing) is a stretch goal; sequencing belongs to the Phase
11 arrangement contract rather than a second mechanism.

## Validation

Standard suite plus manual: import, play, draw, simplify without flattening,
preview matches playback, LLM picks only enabled patterns.

Implementation evidence: unit/API/UI tests cover generated catalogs,
PCHIP sampling, acceleration/reversal budgets, relative projection, funscript
normalization, long-gap stripping, finite-program completion, controller
ownership, disabled-pattern rejection, curation fallback, feedback undo, and
backend previews. The rendered React workspace was exercised at 1280 px and
390 px across Browse, Programs, Author, and Training with no horizontal
overflow; a mobile flex-shrink defect found during that pass was fixed. The
remaining manual evidence is the routine-cycle feel check on the real device,
capped below 40% intensity; synthetic tests cannot establish physical feel.

## Done Criteria

- Authored content routes through the shared motion engine.
- Drawn patterns are sparse, editable, and interpolated.
- Programs are not confused with short loop patterns.
- The curation contract works with a visible fallback.

## Out Of Scope

- growing a large curated built-in catalog (content work, not architecture)

# Phase 14B: Intiface Dispatch Owner

Status: implemented with automated contract, API, lifecycle, and UI coverage.
Matched Handy paths passed before the deadline-driven asynchronous-ACK pacer;
the revised pacer still needs a `motion_trace.v3` matched hardware run and
subjective feel confirmation.
Decision and schema evaluation recorded in
[ADR 0010](docs/decisions/0010-transport-neutral-frames-intiface.md), which
revises ADR 0006's HSP-only dispatch-owner scope and resolves
`docs/lso-merge-alternatives.md` Decision 2 as a first-class owner.

## Suggested `/goal`

`/goal Complete MagicHandy Phase 14B: neutralize the transport frame contract per ADR 0010, then implement the Intiface/Buttplug dispatch owner with the same safety, diagnostics, and consistency obligations as the Handy owners.`

## Objective

Drive Intiface Central-managed devices from the same motion engine, with
output that feels the same as the Handy paths because every owner consumes
the identical timed-point frame under tested, owner-agnostic obligations.

## Schema Verdict (from ADR 0010)

The motion handling schema needs **no structural change**: the engine's
absolute-time, absolute-position (0–100 relative span) timed-point stream is
already the right common denominator for browser Bluetooth, Intiface, and
the Handy v3 API. Two modest modifications ship with this phase:

- rename the `transport` contract's HSP-flavored names to transport-neutral
  ones (`AppendPoints`/`Play`, kinds `points_add`/`points_play`); HSP stays
  the name of the Handy *encoding*, not of the frame
- widen sample/point positions from `int` to `float64` (content and PCHIP
  sampling are already float); owners quantize at encode time — the Handy to
  whole percent, Buttplug to 0..1 floats

## Slices

- **14B.0 — contract neutralization.** The renames and float positions,
  plus the owner-agnostic contract suite: the HSP invariant tests
  generalize so every dispatch owner is tested for exactly-once window
  projection, exactly-once reverse mapping, stop preemption, honest health
  reporting, and no resampling/reshaping of the frame.
- **14B.1 — Buttplug client and owner.** Pure-Go websocket client
  (`github.com/coder/websocket`) speaking Buttplug spec v3 against a
  user-run Intiface Central (default `ws://127.0.0.1:12345`, configurable):
  handshake, ping keepalive (a safety feature — the server stops devices
  when the client dies), device list and single linear-actuator selection,
  the immediate-mode pacer that converts point pairs into absolute-deadline
  `LinearCmd`s with host-side window projection, asynchronous bounded ACK
  correlation, startup anchoring, stale-frame suppression, `StopDeviceCmd` on
  Stop with pacer flush, underrun detection reported as honest playback state,
  and paced-wire diagnostics. Settings gain the `intiface` dispatch owner
  and server address; owner-switch stops the old owner first, like today.
- **14B.2 — validation and docs.** A fake Buttplug server drives the unit
  and lifecycle suites (goleak-gated, Stop/owner-switch gates extended);
  live validation runs the same Handy through all three paths — Cloud REST,
  browser Bluetooth, and Intiface — as a direct like-for-like consistency
  measurement, plus one non-Handy Buttplug device if available. Setup and
  the capability boundary are documented (`docs/intiface.md`).

Phase 14B landed independently. Future LSO integration must reuse this contract
and owner instead of adding a parallel Buttplug implementation (R20).

## Out Of Scope

- vibration/rotation devices (`ScalarCmd`/`RotateCmd` mapping) — a later
  slice once linear output is proven
- multiple simultaneous devices
- running or bundling Intiface Central itself; the user operates it

## Done Criteria

- The owner-agnostic contract suite passes for all three owners.
- Stop, pause/resume, quick-settings refresh, and owner-switch behave
  identically over Intiface (same invariants, same traces).
- The same pattern played over Cloud REST and Intiface on the same Handy is
  indistinguishable in feel at matched latency (manual evidence, like the
  Phase 14 feel check).

## Implementation Evidence

- The neutral `AppendPoints`/`Play` contract and float positions are used by
  the engine and all owners; Handy quantization occurs only at its encoding
  boundaries.
- `TestTransportOwnersPreserveNeutralFrameContract` runs one fractional frame
  through Cloud REST, browser Bluetooth, and Intiface and checks exactly-once
  window/reverse mapping plus unchanged point timing/count.
- `TestTransportOwnersStopPreemptionContract` runs the same Stop-ordering
  invariant against all three owners and rejects motion emitted after Stop.
- The fake Buttplug v3 server covers handshake, ping failure, discovery,
  selection, startup anchoring, delayed/missing/rejected ACKs, stop/close
  preemption, queue/ACK bounds, late/expired coalescing, timing capabilities,
  underrun, and paced-wire diagnostics. HTTP integration covers
  connect/select/dispatch/export/disconnect.
- The React route covers saved-address gating, connect/disconnect, scanning,
  and one linear-actuator selection without constructing transport payloads.
- A 2026-07-12 Intiface Central session discovered `The Handy (FW4+)` and ran
  the shared stroke pattern at 20% through Start, Pause, phase-preserving
  Resume, a live 30–70% reverse refresh, and Stop. Nineteen trace rows had no
  failed result or starvation. Repeated idle Stop produced distinct successful
  commands, and disconnect recorded its close-time Stop. See `docs/intiface.md`.
- A matched Cloud REST run used the same 20% cap, pattern, Pause/Resume, and
  live 30–70% reverse refresh. Its 23 trace rows contained 19 successful
  transport results and no starvation. Pause, active Stop, and repeated-idle
  Stop were distinct successful deliveries at 317, 311, and 310 ms.
- The old live run predates the deadline-driven asynchronous-ACK pacer and only
  measured queue admission. A new `motion_trace.v3` matched run and subjective
  feel confirmation are required. No non-Handy linear device was available, so
  that conditional run remains unperformed.

# Phase 14C: Floating Connection Manager

Status: implemented with provider-state, interaction, responsive, and rendered
coverage.

The shell owns one connection manager on every route. Its compact trigger lives
at the far right of the top bar and opens a floating panel directly below it.
The panel renders only the saved dispatch owner's live actions (Cloud check,
browser Bluetooth, or Intiface connect/discover/select); credentials and
addresses remain in routed Settings except for a compact, write-only Cloud
connection-key row. The Cloud surface also identifies the active bundled or
developer API v3 ID source. Speed and stroke limits moved from Chat into this
manager and still use the semantic immediate-apply API. Reverse direction and
motion style remain in Chat as motion behavior.

The connecting state uses a reference-guided transparent isolation of the
reviewed conductor hand. It renders directly at a fixed square source ratio,
without the approximate SVG clip and luminance mask that distorted its shape.
The scaled composition keeps the hand, three intense-blue vector arcs, and the
poster's tall capsule, shorter domed body, LED, and square marker inside one
frame. The arcs occupy the lower half and stagger toward the device while
connecting; connected shows the complete signal. Disconnected shows no signal
and a red square; only a failed connection attempt adds a briefly shaking red X.
The square turns green when connected. `docs/connection-artwork.md` preserves
the generation, construction, state, and refactor details.
Reduced-motion users get static state feedback. The non-modal disclosure
restores focus on close, leaves Escape to Stop, and clears the reserved mobile
Stop/footer region.

The shared motion visualizer now uses a compact vertical Handy 2-inspired body
and sleeve in both top-bar and Chat forms. It displays the configured stroke
envelope and moves from the backend's commanded position estimate; the detailed
form also exposes state, target speed, and active target without becoming a
control.

Validation: the current source has 52 React test cases plus typecheck/build. The
recorded 1280×800 and 390×844 rendered checks cover the initial Phase 14C state;
PR #63 changed visible controls and the visualizer afterward, so a current-build
rendered QA refresh remains open.

# Phase 15: Migration From StrokeGPT-ReVibed

## Suggested `/goal`

`/goal Complete MagicHandy Phase 15: implement import tools for StrokeGPT-ReVibed settings, memories, prompt sets, motion patterns, and programs, with dry-run mode, a compatibility report, and tests.`

## Objective

Let users migrate without manually copying files or guessing what carried over.

## Scope

Import for `my_settings.json`, memories, prompt sets, motion patterns,
programs/funscripts, and selected safe assets. Include dry-run mode, a
compatibility report, an unsupported-field report, and non-destructive
behavior. Secrets (connection key, API keys) are imported into the redacted
settings store, never echoed in the report.

Personalization carried from the legacy notes
(`docs/legacy-parity-sweep-2026-07.md` §G):

- legacy personas map onto prompt sets; the GLaDOS persona ships as an
  importable example, not a bundled default
- an optional user identity/interest selector (self-ID, interests, custom
  entries) feeds the prompt/memory system — off by default, stored locally
  like all personal data, editable and removable like any memory

## Validation

Standard suite plus manual import of a real settings file; representative
fixtures cover old and current StrokeGPT-ReVibed formats.

## Done Criteria

- Migration is non-destructive with a clear report and tested fixtures.

## Out Of Scope

- exact behavioral parity for every legacy setting

# Phase 16: Packaging And Release Pipeline

## Suggested `/goal`

`/goal Complete MagicHandy Phase 16: create Windows release packaging — portable zip plus a setup binary that launches the in-app first-run wizard — with version metadata, file logging, release docs, and decision records for signing, auto-update, and LAN/HTTPS exposure.`

## Objective

Make MagicHandy distributable as a core binary app that a non-developer can
install and configure end to end without a source toolchain: prebuilt llama.cpp
runtime choice, model downloads, voice provisioning, and StrokeGPT-ReVibed
porting through a GUI. Source builds remain an advanced/developer fallback.

Delivered ahead of this phase (#55, #56, #61, #62, #64, #65): the
model-manager foundation now
owns schema v9 inventory, managed GGUF storage, standalone/Ollama import,
ID-based selection, and the Model UI. The app also owns a pinned source-build
lifecycle for llama.cpp on Windows/amd64, including CPU/CUDA choice, build
status, cancellation, manifest validation, and installer opt-out for existing
Ollama users. The source installer can bootstrap WinGet/Go/Git/CMake/MSVC/CUDA,
build all first-party workers, persist non-secret choices, and reuse them from a
fast-forward-only updater. Phase 16 still owns curated checksum-pinned model
downloads, hardware-fit recommendations, and release packaging that avoids
installing a source toolchain for non-developers.

**GUI installer decision** (ADR 0011; evaluation in
[docs/gui-installer.md](docs/gui-installer.md)): the heavily interactive
setup surface is the app itself — a first-run onboarding wizard (`#/setup`)
in the embedded React UI orchestrating the existing build/import/provision
APIs — delivered by a thin Inno Setup binary that handles only install
directory, shortcuts, the uninstall entry (program files only; the data
directory survives), and launching the app into setup. Native installer
frameworks and dedicated Electron/Tauri installer apps were evaluated and
rejected for the interactive surface; the portable zip stays as the second
artifact.

## Scope

Implement, as slices:

- **16.0 — release plumbing**: Windows binary build, portable zip, embedded
  assets, default config/data directory behavior, version command/endpoint,
  release GitHub Actions workflow and release-notes template; publish
  checksum-pinned CPU and CUDA llama.cpp runtime bundles from the same pinned
  revision, with manifests and license notices consumable by the app
- **16.1 — Windows setup binary**: Inno Setup script compiled in CI
  (build-time-only dependency), Start Menu/desktop shortcuts, Add/Remove
  Programs uninstall that leaves the data directory, silent-install flags,
  over-install upgrades, finish page launching first-run setup
- **16.2 — first-run onboarding wizard** (`#/setup`, re-runnable from
  Settings): welcome/consent → device → LLM runtime (checksummed prebuilt
  CPU/CUDA download by default, advanced managed source build, skip-for-Ollama
  with store import, or external URL)
  → LLM model (import or curated download) → optional voice provisioning
  (Parakeet runner+model and source-built NeuTTS assets moved from `install.ps1` into
  checksummed, size/license-visible, progress-reporting API endpoints;
  ElevenLabs key entry) → finish. Every step skippable; every step is the
  existing settings/API surface, never a second implementation
- **16.3 — StrokeGPT-ReVibed porting step**: the wizard surfaces the Phase
  15 importer — install-location detection, dry-run preview with the
  compatibility report, per-category opt-in, non-destructive import
  (depends on the Phase 15 importer API)
- log-to-file by default with a mostly quiet console; print the local URL
  prominently (clickable in terminals that support it)
- keep binding to localhost by default; document that the app is a
  single-operator local controller and must not be port-forwarded
- decision docs: signing, auto-update, worker bundle strategy, WebView2
  app-window shell (presentation only), and LAN/HTTPS exposure (whether
  MagicHandy ships the HTTPS/cert story or scopes LAN access out — see
  risk R18)
- check the binary-size (<30 MB) and cold-start (<500 ms) budgets from
  `docs/goals-and-guardrails.md` (the setup binary is a separate artifact
  with its own small overhead; the core binary budget is unchanged)

## Validation

Standard suite plus: unzip release, run from a clean directory, config/data
directories created correctly, install and select a prebuilt managed llama.cpp
runtime on a machine without Go/Git/CMake/Visual Studio, and confirm no source
checkout is required.

## Done Criteria

- A user can download and run the core app without Python.
- Release artifact includes license and README; budgets are measured.
- Optional voice worker setup is documented separately.

## Out Of Scope

- production code signing unless credentials/process already exist
- auto-update implementation unless explicitly approved

# Phase 17: Parity Review And Default-App Decision

## Suggested `/goal`

`/goal Complete MagicHandy Phase 17: compare MagicHandy against StrokeGPT-ReVibed, document remaining gaps, run real-device and packaging checks, and recommend whether MagicHandy becomes the default app.`

## Objective

Make an explicit product decision instead of allowing the rewrite to drift
indefinitely.

## Scope

Review motion reliability, transport diagnostics, chat, modes, voice status,
settings coverage, migration coverage, packaging quality, and known gaps.
Walk the Functional Parity Baseline in `docs/ui-design.md` row by row; any
still-open row is either closed, explicitly accepted, or becomes an issue.
Also decide on deferred product questions: device profiles (Handy 1 vs
Handy 2 speed behavior; Handy 2 Pro overclock only with documented limits),
and which post-parity backlog items are worth scheduling.

Produce `docs/parity-review.md`, `docs/default-app-readiness.md`, GitHub
issues for remaining gaps, and a recommendation: default, continue parallel,
freeze, or backport/abandon.

## Done Criteria

- Gaps are explicit; the recommendation is concrete.
- If ready, criteria for freezing StrokeGPT-ReVibed are documented.

## Out Of Scope

- fixing every gap discovered during review

# Cross-Phase Testing Requirements

Every implementation phase runs:

```powershell
go test ./...
go test -race ./...
```

plus `go vet`, `golangci-lint`, and a `CGO_ENABLED=0` build (see
`docs/goals-and-guardrails.md`).

When motion or transport behavior is touched, also run the goroutine-leak and
emergency-stop-teardown tests (the motion safety gate), and update
trace/golden tests.

When frontend is touched, verify against the live rendered DOM (headless run
plus inspection/screenshot), and run browser/UI tests once they exist.

When real-device behavior is touched, capture: exact scenario, transport mode,
firmware/API mode, command latency summary, trace export, observed behavior,
and what was intentionally not changed.

# Parity And Kill Milestones

## Motion Core Milestone

MagicHandy motion becomes eligible to replace StrokeGPT-ReVibed motion when:

- manual motion works on real hardware **through the shipped app path**
  (Phase 9B), not only the validation runner
- area focus works while already moving
- settings changes apply immediately while moving
- no regular stop/go behavior occurs during retargets
- trace diagnostics explain failures

## App Default Milestone

MagicHandy becomes eligible as the recommended app when:

- chat plus motion works end-to-end
- settings import works
- emergency stop is reliable
- basic diagnostics are present
- packaging produces a usable Windows binary

## Freeze Decision

Once MagicHandy reaches the app default milestone, decide whether to freeze
StrokeGPT-ReVibed except for critical fixes, continue both temporarily, or
backport the motion core idea and abandon the full rewrite.

# Post-Parity Backlog

Ideas preserved from StrokeGPT-ReVibed planning that are deliberately not
phases. They wait until after the Phase 17 decision:

- **Story mode**: scripted/model-guided scene sequences using the same
  arrangement contract and curated library; needs voice and sequencing first.
- **Internet-exposed remote control / multi-user sessions**: changes the
  threat model entirely (auth, roles, per-user sessions, global stop
  authority); the current product is one trusted local operator.
- **Device profiles and Handy 2 Pro overclock**: only with documented limits,
  warnings, and a fallback path.
- **Native Go Bluetooth**: only if a prototype shows clear wins over the
  browser bridge.
- **Bounded speed-test button**: move the device at a chosen speed over the
  safe range for a short fixed duration from the settings sliders.
- **Live device-position polling** for a confirmed-position visualizer layer,
  if the API exposes it without excessive traffic.
- **Optional Python voice workers** (Chatterbox, CosyVoice) behind the same
  worker protocol.

# Implementation Rule

Do not start by porting every feature. Start with a better motion and
transport foundation, preserve the hard-won HSP constraints as tests, validate
on real hardware early — through the app users actually run — then make chat
and modes call into the new core.
