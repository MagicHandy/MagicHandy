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

Status 2026-07-11: Phase 7 and both Phase 9B shipped app paths (Cloud REST and
Browser Bluetooth) passed capped real-device checks. Phase 14 adds generated
and imported curves; automated sampling/safety checks pass, but its 6.6 s
routine-cycle feel threshold remains an explicit capped hardware check. The
risk therefore remains open for new motion-content behavior. A Phase 14 Edge
attempt selected Browser Bluetooth and capped speed at 35%, but the chooser saw
no compatible advertisement; no motion command was sent.

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

Status 2026-07-11: schema v8 safely opens the divergent Rockfire v7 database,
preserves its settings/prompt data, and leaves its LSO-only tables untouched.
That is database-lineage compatibility, not the Phase 15 user importer; dry-run
mapping from StrokeGPT-ReVibed and LSO content remains open.

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

Status 2026-07-11: Phase 14's Browse, Programs, Author, and Training tabs passed
rendered checks at 1280 px and 390 px. The pass found and fixed a flex-shrink
bug that made mobile training preferences unreachable. Backend preview samples,
not frontend interpolation, render every library curve.

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
- keep model metadata in SQLite and model bytes in one private managed store;
  guard removal of the selected file
- treat Ollama's library as read-only: bounded manifest parsing, explicit copy,
  manifest SHA-256 verification, and clear rejection of split/auxiliary layers
- require explicit download confirmation with visible size, license, checksum, and expected hardware fit
- verify downloads before install and move files atomically
- keep Ollama available as the secondary cross-platform provider
- surface runner stderr, health, model-load errors, and hardware-fit warnings in diagnostics

Exit evidence:

- Phase 9 can load and chat with a GGUF model on a supported Windows/NVIDIA setup
- Ollama still works as the secondary provider
- startup/status checks do not download models
- model install/import/load/unload paths are tested and documented

Current evidence (2026-07-11): schema v9 inventory, standalone GGUF import,
Ollama daemon listing, configurable filesystem scan/import, atomic verified
copies, cancellation, deduplication, and selected-model removal protection are
implemented and fixture-tested. A live Windows library with 16 manifests scans
as 16 compatible models without starting a copy. Managed llama.cpp now pins
`b9966` / `c749cb0`, builds from source through an embedded controller-gated
helper, validates an app-owned manifest, resolves models by inventory ID, and
starts the runner offline with its UI disabled. A fresh Windows CPU build was
verified end to end (54.2 s, 18,432,916 installed bytes), as were idempotent
reuse and the Ollama-without-managed-runtime path. R13 remains High until CUDA
build/load/chat, curated downloads, and hardware-fit guidance have real-system
evidence.

Source-install mitigation (2026-07-13): `install.ps1` now provisions and verifies
missing Go/Git/CMake/MSVC/Windows SDK/CUDA/Ollama dependencies before a selected
build, while `update.ps1` reuses atomic non-secret choices unless the user opts
to revise them. Windows PowerShell 5.1 plan tests cover managed CUDA and
Ollama-only dependency graphs. This reduces manual setup drift but is not CUDA
build/load/chat evidence and does not lower R13 yet.

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

Status 2026-07-11: Phase 14 pattern and finite-program playback both construct
semantic `MotionTarget` content and enter the existing engine. API tests assert
engine ownership and disabled-pattern rejection; finite completion performs an
engine-owned Stop. Import-boundary tests still keep `patterns`, `chat`, and
`modes` away from transport internals. The audited Rockfire `manualqueue`
transport owner was deliberately not merged.

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
- Phase 12 landed the substrate: versioned worker protocol with cancellation
  and queue-depth reporting, a core-owned serialized bounded queue, no-speech
  rejection (never an empty transcript into chat), and worker errors that
  terminate in the voice request log — never in history, TTS, or motion.
  The ordering trio itself (shared log + cursors, lockstep emit/enqueue,
  audio lease) is the first Phase 13 work item, before any real provider is
  wired to chat, because there is no audio playback to order against yet.

Exit evidence:

- tests cover spoken-equals-shown, multi-client cursor isolation, and the
  model-error path — **all three landed with the Phase 13 delivery-ordering
  foundation**: `TestSpokenReplyAlwaysMatchesDisplayedReply` (the enqueued
  TTS text is byte-identical to the logged reply, and only the controller
  can fetch the clip), `TestChatCursorsAreIsolatedAndMonotonicOverHTTP`, and
  `TestModelErrorsNeverEnterHistoryOrTTS`. The risk stays listed until a
  real provider (not the stub) has exercised the same path end to end.

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
- before Phase 16 packaging claims device support: review current Handy API
  docs for Handy 2 / Handy 2 Pro deltas (including the documented overclock
  mode) and expose per-device max-speed limits only from documentation —
  never guessed values (legacy notes item; see
  `docs/legacy-parity-sweep-2026-07.md` §D)
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

Status 2026-07-09: the spike ran (`docs/neutts-air-spike.md`). Non-Python
decode is proven twice over (official ONNX NeuCodec decoder; independent
pure-Rust FSQ+Vocos+ISTFT decoder in the `neutts` crate). Measured on this
project's Windows dev machine, CPU-only, Q4 GGUF via llama.cpp: RTF
0.51-0.62 (faster than real time), ~2 s first-sentence latency, one-time
14 s reference encode. Cloned-output WAVs await subjective listening, and
the worker integration (Rust-crate host preferred; Go+onnxruntime as the
alternate) is the follow-on 13.1 PR — the risk drops from "unproven tech"
to ordinary integration work.

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

Status 2026-07-09: Phase 13 records **localhost-only** microphone support.
MagicHandy does not claim LAN/mobile voice input. HTTPS, local CA, exact-IP
SANs, and Android certificate support remain a Phase 16 exposure decision.

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
- schema v8 reserves the divergent Rockfire v4-v7 lineage and reconciles its
  core settings/prompt shapes idempotently while preserving uninterpreted LSO
  tables for the explicit migration phase
- WAL plus `busy_timeout` plus a serialized single writer so the app's own
  concurrency cannot deadlock the store
- re-measure binary size and idle/active RSS when Phase 11B lands and record in
  `docs/goal-scorecard.md`; the Phase 11B RSS miss is recorded as a waiver, not
  silently relaxed
- preserve the redaction contract: the connection key is never returned by
  reads, diagnostics, or exports; the `.db` file carries the same at-rest
  sensitivity as `settings.json` did
- corrupt-store startup: a corrupt legacy JSON file still recovers to defaults,
  but a corrupt `magichandy.db` currently fails at open rather than recovering
  (the JSON stores never failed startup). Restoring never-fail startup — back up
  the bad DB, start fresh, and report it in load status — is a tracked follow-up

Exit evidence:

- Phase 11B: settings, memory, and prompt sets round-trip through SQLite with
  tests; the JSON import is covered by fixtures (present, absent, corrupt);
  binary size remains within target; RSS has a recorded waiver; redaction tests
  still pass
- Phase 14: patterns, programs, and reversible feedback round-trip through
  SQLite; synthetic main-v2 and Rockfire-v7 fixtures migrate to v8 without data
  loss; pure-Go build and size budget remain green

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

## R21: Imported Motion Content Risk

Level: High

Description:
Pattern share files and third-party funscripts are untrusted inputs that can be
huge, malformed, nearly stationary, unexpectedly long, or physically harsh.
Treating media-timed scripts as repeatable patterns can also preserve long
inactive gaps or normalize an unusable span into misleading motion.

Mitigation:

- cap request bodies at 8 MiB, action counts, durations at 24 hours, and stored
  pattern/program capacities; reject non-finite/out-of-range positions
- distinguish finite programs from repeatable patterns at schema, API, and
  engine levels; only explicit pattern import strips long stationary gaps and
  normalizes relative amplitude
- validate/simplify authoring input server-side and preview with the exact
  backend sampler; never execute raw file payloads or construct transport
  commands from imported data
- route all playback through the shared engine and user speed/stroke envelope;
  controller ownership, Pause, and global Stop remain unchanged

Exit evidence:

- malformed/bounds/inversion/gap fixtures pass; imported program completion and
  shared-engine ownership pass at the HTTP layer; a capped real-device sample
  confirms generated and imported content has no unexpected stop, step, or
  reversal behavior

Relates to R1 (real-device validation), R8 (migration), and R14 (one motion
path).

## R22: Third Dispatch Owner (Intiface) Surface Risk

Level: Medium

Description:
ADR 0010 adds Intiface/Buttplug as a third dispatch owner. Unlike the two
Handy owners, it is immediate-mode: a host-side pacer schedules every command
in wall time, with no device-side buffer, starving report, or stroke-window
projection. New failure modes (timer drift, underrun, missed stop-preemption,
double or missed window projection) could make the same motion feel different
per owner or, worse, weaken stop behavior on one path. Buttplug-side devices
also vary widely in actuator limits the Handy owners never see.

Mitigation:

- one owner-agnostic contract suite (Phase 14B.0): exactly-once window
  projection, exactly-once reverse mapping, stop preemption, honest health
  reporting, and no resampling — run against every owner including a fake
  Buttplug server
- motion-feel shaping (PCHIP, acceleration/reversal budgets, cycle and dwell
  floors) stays engine/generator-side so owners cannot diverge by design
- the pacer detects its own underrun and reports honest playback state; the
  stop-and-report rule (ADR 0006) applies — never a silent fallback
- Buttplug ping keepalive stays enabled so Intiface stops devices if
  MagicHandy dies
- live validation drives the same Handy through all three owners as a direct
  like-for-like consistency measurement before the owner is recommended

Exit evidence:

- contract suite green for all owners; Stop/owner-switch/goroutine-lifecycle
  gates extended to Intiface; a real-device session confirms matched feel on
  the same Handy over Cloud REST and Intiface and clean stop behavior on a
  non-Handy Buttplug device if available

Implementation status (2026-07-12): the neutral-frame and shared Stop-preemption
suites plus fake-server handshake, keepalive, selection, underrun, rejection,
Stop, Close, HTTP runtime, and UI tests are implemented. Matched capped Handy
runs over Intiface and Cloud REST passed Start, Pause, phase-preserving Resume,
reverse quick refresh, active and repeated-idle Stop, and close-time Stop where
applicable, without starvation. The risk remains Medium until subjective feel
is confirmed; no non-Handy linear device was available for the conditional run.

Relates to R1 (real-device validation), R14 (one motion path), R16 (device
coverage), and R20 (LSO merge integration).

## R23: Emergency Stop Delivery Gaps

Level: Critical

Description:
The permanent Stop control is mounted outside routes and active engine Stop
cancels local work before attempting transport Stop. However, an idle engine can
return without retrying transport Stop, the no-engine HTTP path returns success
without creating the selected transport solely to stop it, and an unreachable
backend cannot forward a Browser Bluetooth command. These gaps can make the UI
report a locally stopped engine without proving that the physical device
received a Stop.

Implementation status (2026-07-12): active, paused, idle-engine, and no-engine
paths now attempt the selected transport; unavailable owners preserve local
stopped state while returning an explicit error. Intiface hardware produced
distinct successful active and repeated-idle Stop commands plus a recorded
close-time Stop. Browser-backend loss and current Cloud/Browser hardware retry
evidence remain open, so the risk stays Critical.

Mitigation:

- make every Stop request attempt the selected dispatch owner's Stop whenever
  that owner is available, including idle-engine and no-engine states
- preserve the current invariant that local planners and motion state stop even
  when transport delivery fails; surface the failure instead of claiming
  physical delivery
- add idle/no-engine/read-only/backend-loss coverage for Cloud REST and Browser
  Bluetooth, plus real-device checks for retry and owner-switch cases
- keep Stop mounted outside routes and controller ownership gates

Exit evidence:

- automated tests prove unconditional delivery attempts and local teardown for
  active, paused, idle, no-engine, read-only, owner-switch, and transport-error
  paths; capped hardware checks record Cloud REST and Browser Bluetooth results

Relates to R1 (real-device validation), R3 (transport behavior), and R9 (UI
safety regression).

## R24: Browser Microphone And Managed ASR Format Mismatch

Level: High

Description:
Browser push-to-talk records WebM/Opus or Ogg and sends that payload unchanged
through the core. The managed parakeet.cpp path is documented and tested with
WAV input, and the worker changes multipart metadata without decoding or
transcoding. The UI and adapters can therefore be implementation-complete while
the default managed microphone path remains incompatible on real browsers.

Mitigation:

- run an end-to-end browser MediaRecorder sample through the pinned managed
  runner before claiming push-to-talk acceptance
- either negotiate a format the runner decodes or add bounded decoding at the
  worker boundary; native audio dependencies must not enter the pure-Go core
- reject unsupported formats with a visible actionable error rather than
  forwarding bytes optimistically
- retain fixture tests for every accepted browser format and the WAV provider
  contract

Exit evidence:

- Chrome/Edge localhost push-to-talk produces an accurate transcript through
  the pinned managed Parakeet install, with format/error tests and no core CGo
  dependency

Relates to R17 (voice dependency and latency risk) and R18 (browser security and
LAN microphone access).
