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

Updated 2026-07-06. Phases 0 through 11 are merged to `main`. Phase 11B
(SQLite persistence foundation, ADR 0008) is implemented on the current branch.

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
| 9B | App-path device validation, controller ownership | Complete | #15, #16, #17, #22 |
| 10 | Memory, editable prompt sets, settings reset | **Complete** | #24 |
| 11 | Modes as motion clients (Freestyle, chat keepalive) | **Complete** | #26 |
| 11B | SQLite persistence foundation (ADR 0008) | **Complete** | pending |
| 12-17 | Voice, patterns, migration, packaging, parity | Not started | — |

Phase 11 note: Freestyle boundary behavior is proven on the real engine over
the fake transport (one continuous stream across many segment retargets, one
HSP play, zero stops). Real-hardware freestyle validation rides the next
manual device session, alongside the Bluetooth-endurance watch item.

Phase 9B closed with PR #22: the visible Edge Web Bluetooth flow selected the
real device, checked the connection, started motion at 28%, and stopped it via
deterministic chat — full app-path evidence for both dispatch owners lives in
`docs/perf-baseline.md`.

Phase 10 decision (2026-07-02): **chat history stays client-side for now.**
Server-side history is deliberately deferred to Phase 12, where ADR 0003's
shared message log with per-client cursors introduces it as the single
canonical history; building a separate Phase 10 history store would create a
second source of truth that Phase 12 would immediately replace. Parity row 9
tracks it.

### What Exists On Main

- Motion engine with retargeting, latency-aware lead, phase preservation, and
  goroutine-lifecycle safety tests, bound at runtime to the **selected dispatch
  owner** (Cloud REST or Browser Bluetooth) from settings; a fake transport is
  used only for tests and the diagnostics placeholder.
- Cloud REST and Browser Bluetooth transports with diagnostics parity, SSE
  event endpoints, no-fallback behavior, and manual test endpoints.
- Motion-first UI: persistent control bar, single engine-state visualizer,
  always-visible Stop (plus Escape), immediate-apply quick controls,
  diagnostics panel, trace export.
- Streaming LLM chat (managed llama.cpp primary, external llama.cpp, Ollama
  secondary) with a strict JSON contract, one repair pass, malformed-response
  indication, and chat-driven motion through the engine only.
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

1. **Open parity rows**: only server-side chat continuity (Phase 12) remains;
   pause/resume shipped early in the post-Phase-10 shell pass. See
   `docs/ui-design.md`, "Functional Parity Baseline".
2. **Browser Bluetooth endurance** is unproven beyond short sessions; the
   one-hour soak ran on Cloud REST only (scorecard watch list).

### UI Shell Redesign (Sidebar Navigation)

The UI is moving to React now, then from the current status-bar +
single-control-sidebar + settings-window shell to a **permanent left navigation
sidebar that switches pages** (Chat / Preset Modes / Pattern Library /
Settings), with Stop pinned to the sidebar footer on every page. Framework
decision and handoff:
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
   Phase 14; a labeled empty state until then.

Status: steps 1 and 2 have landed together — the UI is now a Vite + React +
TypeScript app (`web/`, built to `web/dist`, embedded by Go; no runtime Node)
implementing the permanent nav rail, status-only bar, pinned Stop, and the
Chat / Preset Modes / Pattern Library / Settings routes with the safety
invariants (Stop outside routes, backend-loss lock, read-only lock) under
Vitest. Autopilot renders as coming-soon until its planner exists (step 3);
Pattern Library is the empty state (step 4). The legacy vanilla UI is retained
under `web/legacy/` for reference until React reaches parity, then removed.

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

Use branch prefix `codex/` when Codex submits and `claude/` when Claude submits.

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
  internal/llm/            provider interface, llama.cpp, Ollama   [exists]
  internal/motion/         targets, plans, sampler, retargeting    [exists]
  internal/transport/      Cloud REST + browser Bluetooth, HSP-only[exists]
  internal/diagnostics/    trace ring, export                      [exists]
  internal/validation/     retarget validation checklist           [exists]
  internal/modes/          freestyle, continuous-chat planners     [exists]
  internal/memory/         long-term memory store                  [exists]
  internal/store/          SQLite datastore, schema, migrations    [exists]
  internal/audio/          voice-output queue, TTS worker client   [planned]
  internal/asr/            voice-input worker client               [planned]
  internal/workers/        external worker lifecycle/protocol      [planned]
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
SQLite datastore (ADR 0008) so the Phase 12 chat log and Phase 14 pattern
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
- Binary size: 17.62 MB plain / 12.10 MB stripped, under the 30 MB stripped
  budget.
- RSS waiver: stripped SQLite build idles at 53.92 MB after `/healthz` and
  54.27 MB after `/api/state`, `/api/settings`, `/api/memory`, and
  `/api/prompt-sets`; this exceeds the original <40 MB idle budget and is
  recorded in `docs/goal-scorecard.md`.

## Done Criteria

- Settings, memory, and prompt sets persist through one SQLite datastore with no
  API/UI contract change; the one-time JSON import is non-destructive and
  tested.
- The `CGO_ENABLED=0` build stays green; binary and RSS budgets are re-measured
  and recorded (or a waiver is recorded).

## Out Of Scope

- the chat message log and per-client cursors (Phase 12, ADR 0003)
- the pattern/program library tables (Phase 14)
- StrokeGPT-ReVibed import (Phase 15) — same schema target, separate phase
- at-rest encryption (the trust model stays a single local operator)

# Phase 12: Voice Worker Boundary

## Suggested `/goal`

`/goal Complete MagicHandy Phase 12: implement the optional voice worker protocol, worker lifecycle management, voice status UI, and stub workers; do not bundle heavy ML models yet.`

## Objective

Add voice architecture without pulling ML dependency instability into the Go
core.

## Scope

Implement:

- versioned worker protocol: request/response envelope, health/status,
  cancellation, timeouts, queue depth, crash reporting
- worker process lifecycle (start/stop/restart) and missing-worker behavior
- stub TTS and ASR workers used by protocol tests
- UI status for missing/unloaded workers

Delivery-ordering rules (ADR 0003, risk R15): chat-emit and TTS-enqueue are
lockstep; per-client cursors over one shared message log; a single-owner audio
lease so two tabs never speak the same clip; model errors never enter history,
TTS, or motion.

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks: app runs without workers; stub worker starts/stops; crash is
visible; cancellation works.

## Done Criteria

- Voice is optional; the protocol is versioned and tested; the core app
  remains usable without Python.

## Out Of Scope

- real providers (Phase 13)
- CUDA setup scripts

# Phase 13: Voice Feature Implementations

## Suggested `/goal`

`/goal Complete MagicHandy Phase 13: implement the non-Python voice providers one per PR — starting with the NeuTTS Air spike, then ElevenLabs cloud TTS, then Parakeet ASR — keeping the core app functional without voice and the voice settings surface minimal.`

## Objective

Add the selected non-Python voice providers incrementally behind the worker
boundary (ADR 0007).

## Scope

One provider per PR/subphase, in this order:

1. **NeuTTS Air spike first** (risk R17): prove the non-Python NeuCodec
   decode path and cloning quality/latency before the full integration; if the
   spike fails, document the fallback (F5-TTS ONNX or optional Python worker)
   and continue with the other providers
2. ElevenLabs cloud TTS (HTTP; expressive + high-fidelity cloning premium)
3. Parakeet-TDT ASR via sherpa-onnx (Go) or achetronic/parakeet
   (OpenAI-compatible server); Whisper optional alternate later

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

Provider-specific tests plus the standard suite. Manual checks with a real
microphone: missing dependency reported clearly, provider loads, cancellation
works, queue depth visible, app survives provider failure, push-to-talk and
hands-free work with the default settings without touching advanced knobs.

## Done Criteria

- NeuTTS Air (or its documented fallback), ElevenLabs, and Parakeet work
  behind the protocol with no Python required.
- Sentence streaming works; spoken text always matches displayed text.
- The default voice settings surface stays small.

## Out Of Scope

- optional Python workers (Chatterbox, CosyVoice) — the protocol door stays open
- always-on voice before push-to-talk reliability is proven

# Phase 14: Pattern Library, Programs, And Authoring

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

Deeper editing (undo/redo history, mirror/repeat/variation transforms,
multi-pattern sequencing) is a stretch goal; sequencing belongs to the Phase
11 arrangement contract rather than a second mechanism.

## Validation

Standard suite plus manual: import, play, draw, simplify without flattening,
preview matches playback, LLM picks only enabled patterns.

## Done Criteria

- Authored content routes through the shared motion engine.
- Drawn patterns are sparse, editable, and interpolated.
- Programs are not confused with short loop patterns.
- The curation contract works with a visible fallback.

## Out Of Scope

- growing a large curated built-in catalog (content work, not architecture)

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

## Validation

Standard suite plus manual import of a real settings file; representative
fixtures cover old and current StrokeGPT-ReVibed formats.

## Done Criteria

- Migration is non-destructive with a clear report and tested fixtures.

## Out Of Scope

- exact behavioral parity for every legacy setting

# Phase 16: Packaging And Release Pipeline

## Suggested `/goal`

`/goal Complete MagicHandy Phase 16: create Windows release packaging, portable zip output, version metadata, file logging, release docs, and decision records for signing, auto-update, and LAN/HTTPS exposure.`

## Objective

Make MagicHandy distributable as a core binary app.

## Scope

Implement:

- Windows binary build, portable zip, embedded assets, default config/data
  directory behavior, version command/endpoint
- release GitHub Actions workflow and release-notes template
- log-to-file by default with a mostly quiet console; print the local URL
  prominently (clickable in terminals that support it)
- keep binding to localhost by default; document that the app is a
  single-operator local controller and must not be port-forwarded
- decision docs: signing, auto-update, worker bundle strategy, and LAN/HTTPS
  exposure (whether MagicHandy ships the HTTPS/cert story or scopes LAN
  access out — see risk R18)
- check the binary-size (<30 MB) and cold-start (<500 ms) budgets from
  `docs/goals-and-guardrails.md`

## Validation

Standard suite plus: unzip release, run from a clean directory, config/data
directories created correctly, no source checkout required.

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
