# MagicHandy Risk Register

## Purpose

This register tracks rewrite risks that should survive between phases. A risk stays open until it is explicitly accepted, mitigated, or closed with evidence.

## Risk Levels

- High: can derail the rewrite or produce major user-facing regressions.
- Medium: likely to slow delivery or cause support burden.
- Low: manageable but worth tracking.

## R1: Real-Device Motion Validation Risk

Level: High

Description:
Simulated transport tests cannot fully reproduce Handy cloud REST latency, firmware buffering, HSP playback state, or physical feel. Source-only reasoning produced incorrect motion fixes in the old app.

Mitigation:

- validate motion retargeting on real hardware early
- capture trace exports for failed runs
- convert real traces into fixtures where possible
- keep diagnostics specific enough to distinguish planner behavior from device/API rejection

Exit evidence:

- real-device checklist passes for area focus, speed changes, stroke range changes, reverse changes, same-pattern updates, cross-pattern retargets, and emergency stop

Status 2026-07-01: the Phase 7 checklist passed through the dedicated
`cmd/retarget-validate` runner. The risk stays open until the shipped app path
(UI and chat driving the engine through the selected live dispatch owner)
passes the same checklist on hardware — Phase 9B.

## R2: Two-Codebase Drift

Level: High

Description:
StrokeGPT-ReVibed may continue changing while MagicHandy is being rewritten. Feature parity may drift, and agent time may be split across two architectures.

Mitigation:

- define parity milestones
- avoid porting every legacy behavior immediately
- preserve only important invariants/specs early
- decide when to freeze, continue, backport, or abandon

Exit evidence:

- documented parity/default-app decision

## R3: Motion Retargeting Complexity

Level: High

Description:
Smoothly changing active timed-point streams under variable command latency is the hardest part of the rewrite. If underspecified, MagicHandy can reproduce the same stop/start or hard-reset behavior.

Mitigation:

- maintain `docs/motion-retargeting.md`
- make retarget reasons traceable
- test same-pattern and cross-pattern handoffs
- use real-device validation before broad feature work

Exit evidence:

- real-device retarget tests pass without regular stop/go behavior

## R4: HSP v4 Contract Regression

Level: High

Description:
Known HSP schema and behavior constraints can be forgotten during a ground-up rewrite.

Mitigation:

- maintain `docs/hsp-v4-invariants.md`
- port invariants as executable tests before live transport
- review transport changes against those tests

Exit evidence:

- HSP invariant test suite exists and passes in CI

## R5: Bluetooth Implementation Risk

Level: Medium

Description:
Native Bluetooth on Windows may be costly or unreliable. Browser-owned Web Bluetooth requires an active tab and robust bridge state.

Mitigation:

- default to browser-owned Bluetooth early
- keep no-silent-fallback rule
- document bridge status clearly
- defer native Go Bluetooth until justified by a prototype

Exit evidence:

- Bluetooth ownership decision remains current and a working bridge passes manual checks

## R6: Optional Python Worker Complexity

Level: Medium

Description:
Moving ML dependencies out of the core app avoids core install failures but introduces IPC, process lifecycle, cancellation, and protocol-version complexity.

Mitigation:

- version the worker protocol
- app must run without workers
- implement stub workers before real ML providers
- surface worker status and crash diagnostics

Exit evidence:

- worker protocol tests pass and core app starts without Python workers

## R7: Packaging And Signing Risk

Level: Medium

Description:
Binary release expectations can expand to installers, code signing, auto-update, bundled optional workers, and bundled or downloadable llama.cpp runner variants.

Mitigation:

- start with portable zip
- document signing/auto-update decisions separately
- keep core-only release separate from voice-worker bundles

Exit evidence:

- repeatable GitHub release artifact exists and can run from a clean directory

## R8: User Migration Risk

Level: Medium

Description:
Users may have settings, memories, prompt sets, patterns, programs, and assets in StrokeGPT-ReVibed. A rewrite can lose or misinterpret those files.

Mitigation:

- non-destructive import
- dry-run compatibility report
- unsupported-field report
- representative migration fixtures

Exit evidence:

- migration tests pass and manual import produces a clear report

## R9: UI Regression Risk

Level: Medium

Description:
The current app has many UX learnings around settings organization, quick controls, visualizer mapping, and error visibility. A simpler UI should not lose critical controls or hide diagnostics.

Mitigation:

- preserve major settings mental model
- quick settings must apply immediately
- visualizer reads backend state
- browser tests once UI exists

Exit evidence:

- desktop/mobile visual checks and UI tests pass for settings, quick controls, stop, and diagnostics

## R10: Scope Creep Toward Legacy Parity

Level: High

Description:
Trying to port all legacy modes, pattern authoring, voice providers, and setup flows before the core is proven can stall the rewrite.

Mitigation:

- follow phase order
- keep explicit out-of-scope lists
- require real-device motion milestone before broad feature parity
- prefer small PRs with clear done criteria

Exit evidence:

- Phase 17 parity review recommends default/continue/freeze/backport with clear evidence

## R11: Rewrite Goals Left Unmeasured

Level: High

Description:
Maintainability, lower core memory, and shippable binaries are the stated
reasons for the rewrite, but they are easy to claim and easy to lose. Go does
not deliver them by itself: a god-package, a CGo dependency, or GC-held memory
can each defeat a goal silently. Without targets and enforcement, the rewrite
can complete without achieving its purpose.

Mitigation:

- maintain `docs/goals-and-guardrails.md` with measurable targets
- capture the Python core baseline and Go idle RSS in Phase 1
- enforce CI gates (lint, import boundaries, size norms, `CGO_ENABLED=0`)
- measure RSS at motion/app milestones, not just at idle

Exit evidence:

- recorded baseline plus Go numbers per milestone, and CI enforcing the gates

## R12: Frontend Debt Carryover

Level: Medium

Description:
The Go core owns only the backend. The current frontend is ~13k lines of vanilla
JS with a shared state/element god-registry. Porting it wholesale carries the
maintainability debt across and defeats the goal for half the codebase.

Mitigation:

- follow `docs/decisions/0004-frontend-strategy.md`: rebuild fresh, minimal-first,
  backend-state-driven; old JS is reference, not base
- apply the size/no-god-module norms to `web/`
- defer the heavy authoring UI rather than porting it early

Exit evidence:

- minimal UI built without a ported god-registry; `web/` respects size norms;
  parity review documents remaining UI gaps

## R13: llama.cpp Runner And Model Management Risk

Level: High

Description:
MagicHandy is intentionally making llama.cpp the quality-first Windows/NVIDIA LLM path. That improves control over model choice and runtime behavior, but it also makes runner packaging, CUDA compatibility, model downloads, GGUF metadata, disk usage, licenses, and hardware-fit reporting part of the product. A broken runner or unclear model manager can make the primary chat path harder to use than Ollama.

Mitigation:

- keep the Go core pure-Go and manage llama.cpp as an external `llama-server` process
- pin runner builds and record compatibility metadata
- start with a small curated GGUF catalog instead of an open-ended model zoo
- support importing a local GGUF without forcing a download
- require explicit download confirmation with visible size, license, checksum, and expected hardware fit
- verify downloads before install and move files atomically
- keep Ollama available as the secondary cross-platform provider
- surface runner stderr, health, model-load errors, and hardware-fit warnings in diagnostics

Exit evidence:

- Phase 9 can load and chat with a GGUF model on a supported Windows/NVIDIA setup
- Ollama still works as the secondary provider
- startup/status checks do not download models
- model install/import/load/unload paths are tested and documented

## R14: Per-Source Motion Path Divergence

Level: High

Description:
StrokeGPT-ReVibed handled motion separately for chat, Freestyle, Edge/Milk,
trained patterns, and imported scripts. Protections (velocity caps, depth-jump
splitting, turn smoothing, stop/pause boundaries) added for one path did not
reach the others, which caused recurring mode-specific motion bugs.

Mitigation:

- one shared sampler/sanitizer for all sources (see `docs/motion-retargeting.md`,
  "Shared Sampling And Smoothing Protections")
- new sources produce semantic targets, never a parallel motion path
- import-boundary rules keep `modes`/`chat`/`llm` off `transport`

Exit evidence:

- a test asserts no motion source bypasses the shared path; protections are
  applied once and inherited by every caller

## R15: Chat And Voice Delivery Ordering

Level: Medium

Description:
The old app sometimes spoke a reply the chat panel never displayed, and a
destructively drained global queue let one browser tab consume another's
messages.

Mitigation:

- lockstep chat-emit and TTS-enqueue; per-client cursors over a shared log;
  single-owner audio lease; model-error path kept out of history/TTS/motion
  (see ADR 0003, "Message And Audio Delivery Ordering")

Exit evidence:

- tests cover spoken-equals-shown, multi-client cursor isolation, and the
  model-error path

## R16: Firmware v4 / API v3 Only

Level: Medium

Description:
Dropping HAMP, HDSP, and firmware v3 (ADR 0006) means MagicHandy requires Handy
firmware v4 plus API v3 access and has no fallback transport. Firmware v3
hardware is unsupported. A missing, revoked, or incompatible app Application ID
also blocks Cloud REST HSP until fixed, even if the user's connection key is
valid. This is a deliberate scope cut, but it can surface as "the app does not
move my device" if handled quietly.

Mitigation:

- ship and manage the app's own API v3 Application ID if Handy API terms allow;
  treat it as a public client identifier, not a secret, and keep a developer
  override for testing or future revocation
- the connection key stays the user's private credential
- detect and clearly report HSP-unavailable with concrete fix steps (Invariant 8)
- document the firmware v4 / API v3 requirement up front in README/setup
- keep StrokeGPT-ReVibed available for unsupported setups

Exit evidence:

- connect and HSP-unavailable paths give actionable guidance; the requirement is
  documented before first run; ordinary users do not have to find or paste an
  Application ID unless using the developer override

## R17: NeuTTS Air Cloning And Codec Spike

Level: Medium

Description:
ADR 0007 selects NeuTTS Air as the local, non-Python cloning TTS, but its cloning
quality and a native (non-Python) NeuCodec decoder are unproven for this app. If
it under-delivers on quality or latency, the local non-Python cloning path is at
risk and voice could drift back toward a Python worker.

Mitigation:

- treat NeuTTS Air integration as an explicit early spike in Phase 13 (codec
  decoder + cloning quality/latency), not an assumption
- keep ElevenLabs as the working non-Python premium path meanwhile
- fall back to F5-TTS (ONNX) or an optional Python worker if the spike fails,
  without blocking the rest of voice

Exit evidence:

- a Phase 13 spike shows acceptable NeuTTS Air cloning quality and latency with a
  non-Python decoder, or a documented fallback is chosen

## R18: LAN And Mobile Secure-Context Requirements

Level: Medium

Description:
Web Bluetooth and browser microphone capture only work in secure contexts.
`http://localhost` qualifies, so the default single-machine setup is fine, but
any LAN/mobile use of Bluetooth dispatch or voice input requires HTTPS on a
LAN address. StrokeGPT-ReVibed needed a generated local CA, an Android
certificate-helper endpoint, and exact-IP certificate SANs to make mobile
Chrome work — a large support surface that is easy to promise accidentally by
saying "works on your phone".

Mitigation:

- treat localhost as the supported default; the app binds to 127.0.0.1 unless
  the user opts in to LAN exposure
- decide the LAN/mobile scope explicitly in Phase 13 (voice input) and record
  the HTTPS/certificate decision in Phase 16's exposure decision doc
- if LAN HTTPS is shipped, reuse the StrokeGPT lessons: generated local CA,
  cert-helper flow for Android, exact-IP SANs, and docs that forbid
  port-forwarding
- never describe Bluetooth or voice features as LAN/mobile-capable before the
  secure-context story exists

Exit evidence:

- a recorded decision on LAN/mobile scope, and — if in scope — a working
  documented HTTPS flow verified from a real mobile browser

## R19: Datastore Migration And Budget Risk

Level: Medium

Description:
Moving the three JSON stores (settings, memory, prompt sets) into a single
SQLite datastore (ADR 0008, `modernc.org/sqlite`) introduces a schema, a
migration surface, a one-time JSON→SQLite import, and a new dependency that adds
binary size and RSS. A botched import or migration could lose user data; an
unmeasured dependency could erode the memory and binary budgets that justify the
rewrite; and SQLite's single-writer model can surface `database is locked` if
concurrency is handled naively.

Mitigation:

- pure-Go driver only (`modernc.org/sqlite`), preserving `CGO_ENABLED=0` and
  free cross-builds; never a CGo driver
- non-destructive one-time import: keep the JSON file contents (renamed
  `*.migrated`) rather than deleting them; each legacy domain imports inside a
  SQLite transaction and archives only after commit, with settings import
  reported in load status
- forward-only migrations keyed on `PRAGMA user_version`, run transactionally at
  open; a schema newer than the binary is a clear error, never a silent
  downgrade
- WAL plus `busy_timeout` plus a serialized single writer so the app's own
  concurrency cannot deadlock the store
- re-measure binary size and idle/active RSS when Phase 11B lands and record in
  `docs/goal-scorecard.md`; the Phase 11B RSS miss is recorded as a waiver, not
  silently relaxed
- preserve the redaction contract: the connection key is never returned by
  reads, diagnostics, or exports; the `.db` file carries the same at-rest
  sensitivity as `settings.json` did

Exit evidence:

- Phase 11B: settings, memory, and prompt sets round-trip through SQLite with
  tests; the JSON import is covered by fixtures (present, absent, corrupt);
  binary size remains within target; RSS has a recorded waiver; redaction tests
  still pass

Relates to R8 (user migration) and R11 (goals unmeasured).

## R20: MagicHandy + LSO Merge Integration Risk

Level: High

Description:
Merging LSO's feature set (Intiface/Buttplug transport, motion blocks/queue,
personas, a feature-rich frontend, localization) onto the Go core brings large,
fast-moving surface from a different lineage and different structure/style
preferences. Without shared, enforced standards a merge of this size can erode
the properties that justify the rewrite: a second motion path or a transport
that bypasses the engine (R14), duplicated personalization/content systems that
drift, a heavier browser footprint than the efficiency goal allows, oversized
files or weakened CI gates slipped in to "make it pass," and committed runtime
data or duplicated build artifacts. Two parallel frontends or two motion-content
models shipping at once is the concrete failure mode.

Mitigation:

- one shared floor for every contributor and agent (`AGENTS.md`), enforced by CI
  on every branch before it merges to `main`; gates are strengthened, not
  weakened, as the surface grows
- new transports (e.g., Intiface) implement the `transport` interface only and
  are covered by the motion safety gate; every motion source produces semantic
  targets for the shared engine (no parallel path, R14)
- converge duplicated systems: one canonical frontend, one personalization
  model, one motion-content model — decided deliberately and recorded as ADRs
  (`docs/lso-merge-integration.md`, `docs/lso-merge-alternatives.md`), not
  defaulted-into by merge order
- re-measure RSS, binary size, and browser bundle cost as capability lands, and
  record it in `docs/goal-scorecard.md`; heavy UI features must earn their weight
- repository hygiene: no committed `*.db`/`-wal`/`-shm`, caches, `node_modules`,
  `.scratch/`, or duplicated large binaries; split oversized files rather than
  raising the budget by default

Exit evidence:

- the merged app ships one frontend, one motion path, and one personalization
  model; CI (Go + frontend) is green with no weakened gates; budgets are
  re-measured and recorded; the open merge decisions are settled as ADRs

Relates to R14 (per-source motion divergence), R11 (goals unmeasured), R9 (UI
regression), and R8 (user migration).
