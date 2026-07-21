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

Status 2026-07-14: Phase 7 and Cloud REST have current capped real-device
evidence. The 2026-07-02 Browser Bluetooth run moved and stopped the device but
predates the reverse-direction fix and lacks endurance evidence. Phase 14's
generated/imported curves pass automated safety checks, but routine-cycle feel
still needs a capped hardware check. The revised Intiface pacer also needs a
matched `motion_trace.v3` hardware run and subjective feel confirmation.

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
- preserve percentage units at the BLE encoder boundary
- invalidate stale command batches and deliver Stop directly from an already-connected browser when the backend is unavailable
- defer native Go Bluetooth until justified by a prototype

Exit evidence:

- Bluetooth ownership decision remains current and a working bridge passes manual checks

Status 2026-07-16: automated browser tests cover percentage encoding, malformed
protobuf rejection, direct Stop during backend loss, command-poll cancellation,
and Stop-before-disconnect teardown. The existing hardware run predates these
changes, the reverse-direction correction, and endurance testing, so R5 remains
open pending the documented capped real-device matrix.

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

Status 2026-07-19: every top-level route, settings subsection, and Phase 14
Browse/Programs/Author/Training view passes rendered checks at 1440x900 and
390x844 with no horizontal overflow, unnamed controls, duplicate IDs, or nested
interactive elements. Route titles, heading progression, mobile navigation,
manual Speed, and speech-provider names have focused coverage. Backend preview
samples, not frontend interpolation, render every playback-preview curve. The
Import tab additionally passes 1280x800 and 390x844 rendered checks with fixed
trim targets and no overflow. Its client-side editor can only produce bounded
motion-content input for the normal validated import endpoint; it cannot start
motion or construct transport payloads.

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
The Go core owns only the backend. The canonical frontend is now React, but its
route lifecycle, asynchronous backend snapshots, optional worker surfaces, and
large integration harness can still recreate a shared-state god-module or hide
failed reads behind plausible empty UI. The unshipped legacy JavaScript is a
reference only and must not become a second implementation.

Mitigation:

- follow `docs/decisions/0004-frontend-strategy.md`: rebuild fresh, minimal-first,
  backend-state-driven; old JS is reference, not base
- apply the size/no-god-module norms to `web/`
- keep route/component state scoped, distinguish loading/error/data explicitly,
  and give independent mutation domains their own admission guards
- keep focused component tests beside the existing app integration harness

Exit evidence:

- canonical React UI built without a ported god-registry; `web/` respects size
  norms; full-route desktop/mobile checks and focused lifecycle tests pass;
  parity review documents remaining UI gaps

Status 2026-07-18: route lifetime, teardown writes, failed-read honesty,
cross-tab chat retry, mutation admission, mobile names, titles, and heading
progression have dedicated tests. All 141 frontend tests and the full rendered
route matrix pass; changed production files remain below the 800-line guideline.

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
- bound compact intent output, make hidden reasoning policy explicit, and
  separate cold load, prompt evaluation, reasoning, visible generation, and
  repair rate before attributing latency to the provider

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
load/chat, curated downloads, and hardware-fit guidance have real-system
evidence.

Source-install mitigation (2026-07-13): `install.ps1` now provisions and verifies
missing Go/Git/CMake/MSVC/Windows SDK/CUDA/Ollama dependencies before a selected
build, while `update.ps1` reuses atomic non-secret choices unless the user opts
to revise them. Windows PowerShell 5.1 plan tests cover managed CUDA and
Ollama-only dependency graphs. This reduces manual setup drift but is not CUDA
load/chat evidence and does not lower R13 yet. A same-process CUDA environment
fix was then verified by building the pinned `b9966` runtime with CUDA 13.3 and
MSVC 19.51 and probing the installed `c749cb0` runner. This supplies CUDA build
evidence, but model load/chat remains unverified and R13 stays High.

Source-update lifecycle hardening (2026-07-13): rebuilds now send Emergency Stop
to a running checkout-owned app, terminate its process tree before replacing
executables, stage Go outputs, clean legacy `*.exe~` backups, and verify that the
new process owns the configured port before opening the browser. This prevents a
hidden bind failure from reopening an older embedded UI. It does not add CUDA
load/chat evidence or lower R13.

Latency-control mitigation (2026-07-13): requests use a reviewed output-token cap
(default 256), explicit automatic/off reasoning maps to provider-native fields,
repair temperature zero is serialized, and warm managed calls skip repeated
health/model-list preflights. A live managed Gemma 4 12B Q4 regression probe then
showed automatic reasoning consuming both 256- and 512-token limits with zero
visible JSON. Reasoning-off and a 128-token managed reasoning budget both
produced valid JSON for the same request. Reasoning now defaults off, the current
pinned managed automatic path receives half the total budget, length finishes are explicit,
and repair retains context while requesting reasoning off. This is one diagnostic case,
not broad fixed-model quality evidence; R13 remains High.

Live-provider follow-up (2026-07-20): the installed managed `llama.cpp b9966`
CUDA runner loaded the imported Gemma 4 11.9B Q4_0 model on an isolated port and
completed a 13-turn state-aware motion matrix with no repairs or malformed
responses. Start/relative/curated speeds stayed within the test's 20–40%
envelope; hold, area clear, and chat-only turns were correct; five repeated
variation requests selected five distinct patterns before an older choice was
eligible. The same final service completed the matrix against Ollama/Granite
4.1 3B, using the bounded repair path where that smaller model needed it. This
supplies real CUDA load/chat and secondary-provider evidence without dispatching
to hardware. Curated downloads and hardware-fit guidance remain open, so R13
stays High.

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

Status 2026-07-19: Chat Autopilot reuses Freestyle's mode lifecycle and emits
only resolved semantic pattern targets through `Engine.Start`/`ApplyTarget`.
Its control moved to Chat for information architecture, but the frontend still
sends only `mode:"autopilot"`; it does not construct motion or transport
payloads. Integration tests assert one continuous wire play across multiple
model-curated boundaries.

Status 2026-07-20: the full producer-to-owner audit still finds one dispatch
path. Buffered frames now merge authored knots with 25 ms probes under a 0.3%
error bound, loop seams preserve velocity when they are not reversals, and
retargets use a bounded C1 path blend. Import hygiene rejects <5% loop spans and
removes only rapid <=2% reversal chatter; slow subtle motion and finite programs
remain intact. Cloud's declared 1% endpoint resolution enables an engine-owned
0.8%-bounded fit that cuts catalog shallow-focus stationary wire time by 74%.
Unused raw Cloud/Bluetooth stroke/add/play HTTP routes were removed, so only the
engine calls mutating transport methods other than emergency Stop. Physical
feel and immediate stroke-envelope changes remain covered by R1/R22 rather than
being declared solved from simulation.

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
- autonomous replies carry an ephemeral speech-request id on their canonical
  chat row, so the controller browser can play new lines without replaying
  initial history; an occupied TTS queue leaves later autonomous lines visible
  but does not deepen the speech backlog
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

Status 2026-07-19: PR #101 extends the same ordering to Chat Autopilot. Tests
cover browser-discoverable speech ids, no initial-history replay, and canceled
announcements staying out of the log. Real-provider and long-session queue
acceptance remain open.

## R16: Handy HSP Firmware v4 / API v3 Scope

Level: Medium

Description:
Dropping HAMP, HDSP, and firmware v3 (ADR 0006) means MagicHandy's Cloud REST
and Browser Bluetooth owners require Handy firmware v4 plus API v3 access and
have no fallback owner. Firmware v3 Handy hardware is unsupported. A missing,
revoked, or incompatible app Application ID also blocks Cloud REST HSP until
fixed, even if the user's connection key is valid. Intiface is a separate
transport-neutral owner for one selected `LinearCmd` actuator and does not
restore legacy Handy protocols.

Mitigation:

- ship and manage the app's own API v3 Application ID if Handy API terms allow;
  treat it as a public client identifier, not a secret, and keep a developer
  override for testing or future revocation
- the connection key stays the user's private credential
- detect and clearly report HSP-unavailable with concrete fix steps (Invariant 8)
- document the Handy-owner firmware v4 / API v3 requirement up front in
  README/setup, separately from Intiface requirements
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
ADR 0007 selects NeuTTS Air as the local, non-Python cloning TTS. The Go worker
adapter owns a first-party persistent runner built against pinned `neutts-rs`
and streams PCM without Python. The source installer builds either a CPU or
CUDA/WGPU runtime and installs verified decoder/backbone assets with managed
llama.cpp. A first-party Rust worker runs a pinned DistillNeuCodec ONNX encoder
and generates validated reference codes from a local WAV without Python. The
older bounded `.pt`/`.npy` normalizer remains an advanced fallback. The pinned
upstream hub client still does not honor `HF_HUB_OFFLINE=1`. Enforced offline
behavior, GPU-memory coexistence with the chat LLM, and subjective speaker
similarity across representative references remain unproven. Controlled
intelligibility now has ASR round-trip evidence.

Mitigation:

- keep the implemented adapter bounded and cancellable, request offline mode,
  require the exact local GGUF cache entry, use a CLI-contract readiness probe
  that does not synthesize, and avoid claiming network sandboxing without a
  network-denied test
- install immutable, checksum-verified inputs through the source installer;
  report missing runner/decoder/codes/transcript states before Start and keep a
  guarded local host-path chooser for custom overrides
- keep reference encoding in a short-lived worker, pin and checksum its ONNX
  graph/external weights, constrain WAV duration/rate/channels, and re-parse and
  range-check generated NPY in Go before publishing it
- keep the model in one worker-owned process, frame every output with bounded
  lengths, cancel by request ID, and terminate the process on unload, worker
  shutdown, or Emergency Stop
- record CPU/CUDA/WGPU acceleration and every required native DLL checksum in
  the managed manifest; surface the selected backend and explain the CUDA
  latency/VRAM versus CPU compatibility tradeoff before installation
- provision eSpeak NG 1.52, quality-probe its IPA during installation, reject
  manifests without that phonemizer identity, and preserve Neuphonic's codec
  lookback/lookahead overlap-add instead of concatenating independent chunks
- keep ElevenLabs as the working non-Python premium path meanwhile
- fall back to F5-TTS (ONNX) or an optional Python worker if the spike fails,
  without blocking the rest of voice

Exit evidence:

- a capped listening run shows acceptable cloning quality and latency with the
  non-Python adapter; the native encoder remains compatible across representative
  WAV formats and source voices

Status 2026-07-15: the spike and Slice 13.6 adapter landed
(`docs/neutts-air-spike.md`, `docs/neutts-worker.md`). Non-Python decode and
streaming are implemented through `neutts-rs`; the core wraps retained PCM at
the playback boundary. The Windows source installer now verifies and builds
`neutts-rs` v0.1.1, converts a verified NeuCodec checkpoint, and installs the
exact Air Q4 cache whenever managed llama.cpp is selected. A pure-Go bounded
normalizer prepared the official Dave `.pt` sample's 372 codes. A pinned
DistillNeuCodec ONNX encoder then generated 373 valid codes directly from the
7.45 second, 44.1 kHz stereo Dave WAV in about 1.3 seconds. The installed
NeuTTS runner accepted those generated codes and produced 106,560 PCM bytes
(2.22 seconds of audio), proving format compatibility. Settings now exposes the
actual WAV-plus-transcript generation flow and keeps pre-encoded paths under
Advanced. Investigation of the installed CPU-only runner found a 127.27-second
wall time, 90.86-second first audio, and 66.72x real-time factor. The pinned
CUDA/WGPU build completed the same engineering path in 2.45 seconds after a
1.90-second model load. The persistent Go-worker path then delivered first
audio in 1.01 seconds and completed in 2.18 seconds on its first request; the
warm request delivered first audio in 0.47 seconds and completed in 1.17
seconds. Cancellation after the first chunk returned `canceled`, and the same
process completed a recovery request with 96,960 PCM bytes before clean exit. A
clean full updater then migrated the installed runtime to schema 3 CUDA/WGPU,
and the relaunched production app autoloaded both voice roles. Two HTTP TTS
requests completed in 2.018 and 0.874 seconds with valid retained WAVs and
same-process reuse; a visible Edge request completed without an autoplay error
after the shell adopted a gesture-unlocked Web Audio sink. Subjective listening,
representative-source quality, CUDA/LLM VRAM coexistence, and network-sandbox
evidence remain open risks. A follow-up quality audit isolated severe slurring
to the experimental pure-Rust phonemizer (wrong IPA and a dropped reference
word) and independent codec chunks. System eSpeak NG 1.52 plus Neuphonic's
overlap-aware stream retained every substantive target word in four random
Parakeet round trips, with two exact sentence transcriptions. Schema 4 forces
older managed runtimes to rebuild; wider listening and speaker-similarity
acceptance still remain open. A consistency follow-up found per-request random
sampling produced 4.60-9.10 second clips for identical text and could terminate
a corpus sentence at 0.14 seconds. Schema 5 selects validated seed 3, records
that choice, uses sample-identical incremental overlap-add, and adds a bounded
memory-only exact-text cache. All four selected-seed corpus clips retained their
target words; a repeated 4.70 second clip replayed byte-identically in 0 ms after
a 1.91 second miss. This removes known same-input variability but does not close
the representative-reference or subjective-listening risk. Advanced Voice
settings retain fixed seed 3 by default, allow another reproducible 32-bit seed
for reference-specific auditioning, and expose per-request randomness only as
an explicit **Varied** mode with cache disabled. The UI states that cache loss;
documentation records that randomization can reintroduce the measured quality
variance. See `docs/neutts-quality-performance.md`.

Status 2026-07-16: a cross-provider reliability audit closed several causes of
silent or intermittent failure without changing the selected voice engines.
The core no longer drops full audio buffers under worker bursts; it rejects
malformed, out-of-order, mixed-format, empty, and oversized terminal output.
Cancellation now stays attached to the exact worker session and a stale NeuTTS
cancel cannot become the next request's response. Worker lifecycle changes are
serialized and hard teardown owns the child process tree. Provider URL,
response-size, staged-audio, reference-WAV, and encoder-output bounds are now
enforced. Retained speech covers the accepted TTS workload and ASR history no
longer evicts clips waiting for ordered playback. Subjective cloning quality,
representative references, GPU/LLM coexistence, and network-denied runtime
evidence remain open under this risk.

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
  open; negative and newer-than-binary versions are clear errors, never an
  index panic or silent downgrade; current schemas are checked for required
  tables, columns, indexes, foreign-key enforcement, and referential integrity
- schema v8 reserves the divergent Rockfire v4-v7 lineage and reconciles its
  core settings/prompt shapes idempotently while preserving uninterpreted LSO
  tables for the explicit migration phase
- one process-owned connection pool shared by all logical domains, with WAL,
  per-connection pragmas, a bounded four-connection pool, one warm idle
  connection, `busy_timeout`, and one serialized writer
- re-measure binary size and idle/active RSS when Phase 11B lands and record in
  `docs/goal-scorecard.md`; the Phase 11B RSS miss is recorded as a waiver, not
  silently relaxed
- preserve the redaction contract: the connection key is never returned by
  reads, diagnostics, or exports; the `.db` file carries the same at-rest
  sensitivity as `settings.json` did
- corrupt-store startup: `quick_check(1)` identifies physical corruption,
  quarantines the exact DB/WAL/SHM files in a private recovery directory, starts
  a fresh current schema, and reports only the backup path; logical schema
  damage still fails clearly rather than discarding data
- schema v10 archives invalid or oversized active settings documents before
  defaults become active, caps recovery history at 20, and never exposes the
  preserved document through public state or diagnostics
- restrict the data directory to `0700` and database sidecars to `0600` on
  POSIX; Windows uses the current user's profile ACL

Exit evidence:

- Phase 11B: settings, memory, and prompt sets round-trip through SQLite with
  tests; the JSON import is covered by fixtures (present, absent, corrupt);
  binary size remains within target; RSS has a recorded waiver; redaction tests
  still pass
- Phase 14: patterns, programs, and reversible feedback round-trip through
  SQLite; synthetic main-v2 and Rockfire-v7 fixtures migrate to v8 without data
  loss; pure-Go build and size budget remain green
- 2026-07-18 persistence audit: physical-corruption quarantine, settings
  recovery history, negative/newer-version handling, current-schema and
  foreign-key validation, transaction-panic rollback, private POSIX modes, and
  one shared production pool have focused regression coverage; the stripped
  binary remains below 30 MB and current idle RSS remains within the SQLite
  waiver

Relates to R8 (user migration) and R11 (goals unmeasured).

## R20: MagicHandy + LSO Merge Integration Risk

Level: Medium

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

- cap source files and request bodies at 8 MiB, browser-inspected sources at
  20,480 actions, selected/backend payloads at 4096 actions, durations at 24
  hours, and stored pattern/program capacities; reject unknown declared schemas,
  malformed metadata, missing positions, and non-finite/out-of-range values in
  both the browser inspection path and backend parser
- distinguish finite programs from repeatable patterns at schema, API, and
  engine levels; only explicit pattern import strips long stationary gaps and
  normalizes relative amplitude
- validate/simplify authoring input server-side and preview with the exact
  backend sampler; merge backend-owned knots into compact rendered previews so
  long-cycle reversals cannot disappear through uniform-sample aliasing; never
  execute raw file payloads or construct transport commands from imported data
- snap trims to source actions and display their exact duration; keep waveform,
  selection shading, pointer mapping, and fixed-size trim targets in one
  coordinate system; keep zoom/pan independent from selected and submitted
  content, provide direct proportional viewport scrolling, release wheel input
  at zoom limits, preserve every selected knot when importing a finite program,
  and reject loop selections above the 255 essential-knot storage bound without
  imposing a false 6.6-second maximum
- route all playback through the shared engine and user speed/stroke envelope;
  controller ownership, Pause, and global Stop remain unchanged
- for generated catalog loops, reject repeated fixed-endpoint micro-strokes and
  repeated same-span runs before the acceleration/reversal fitter; keep
  user-tested timing-preserved promotions visibly `curated` instead of implying
  that generated-motion budgets were applied

Exit evidence:

- malformed/bounds/inversion/gap fixtures pass; imported program completion and
  shared-engine ownership pass at the HTTP layer; a capped real-device sample
  confirms generated and imported content has no unexpected stop, step, or
  reversal behavior

Status 2026-07-19: browser/backend malformed-input tests pass. The Import tab
also passes focused zoomed-coordinate and payload tests plus rendered 1280x800
and 390x844 checks. Both trim targets measure 44 CSS pixels, fitted boundaries
remain inside the frame, vertical wheel input zooms around the cursor, horizontal
wheel input pans, and a proportional pointer/keyboard scrollbar moves the
viewport directly. An isolated backend persisted the trimmed fixture at the
displayed 4:15 duration with all 16 selected knots. Physical-feel evidence
remains open. Three real-world scratch funscripts (3,536-7,394 actions over
20-37 minutes) confirmed that valid 12-20 second loop slices preserve their
duration, while the old 74-point card preview omitted 102-190 stored knot times.
Compact previews now insert saved knots, and impossible dense selections are
blocked before upload by the backend's essential-knot contract.

Status 2026-07-20: live curation identified and retired six built-ins that
combined a fixed return point with 10-20% micro-strokes or repeated 30-40%
spans near the reversal floor. Six replacement source cycles now have a 30%
per-travel floor, at least four amplitude bands, bounded endpoint reuse, and no
long equal-amplitude run before the existing acceleration/reversal fit. Only
those six carry the experimental gate. `Hard and Regular` and `playful jerk`
preserve their accepted user timing under a `curated` tag. Automated hardware
motion was not run for the replacements; their below-40% physical-feel pass
remains open.

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
- absolute-deadline writes do not await the preceding ACK; an eight-command
  bounded ledger correlates responses under transport-owned deadlines, while
  missing/rejected ACKs invalidate the generation and force Stop without retry
- expired segments are discarded instead of replayed in a burst; live lateness
  has a 25% duration-compression bound, and per-wire timing/ACK telemetry is
  included in `motion_trace.v3`
- startup anchors the first neutral point before the shared playback clock
  starts; reverse mapping is fixed at append time while the min/max envelope
  retains the cross-owner immediate-update contract
- selected `DeviceMessageTimingGap` raises the shared sampler cadence and
  selected `StepCount` is exposed as an honest physical resolution limit
- Buttplug ping keepalive stays enabled so Intiface stops devices if
  MagicHandy dies
- live validation drives the same Handy through all three owners as a direct
  like-for-like consistency measurement before the owner is recommended

Exit evidence:

- contract suite green for all owners; Stop/owner-switch/goroutine-lifecycle
  gates extended to Intiface; a real-device session confirms matched feel on
  the same Handy over Cloud REST and Intiface and clean stop behavior on a
  non-Handy Buttplug device if available

Implementation status (updated 2026-07-13): the neutral-frame and shared Stop-preemption
suites plus fake-server handshake, keepalive, selection, underrun, rejection,
Stop, Close, HTTP runtime, and UI tests are implemented. Matched capped Handy
runs over Intiface and Cloud REST passed Start, Pause, phase-preserving Resume,
reverse quick refresh, active and repeated-idle Stop, and close-time Stop where
applicable, without starvation. Automated delayed/missing/rejected ACK,
deadline, coalescing, startup-anchor, timing-capability, concurrent Stop/Close,
and wire-telemetry cases now cover the immediate-mode deficiencies found in the
follow-up review. The risk remains Medium until the revised pacer receives a
matched subjective run; no non-Handy linear device was available for the
conditional run.

Review update 2026-07-20: adaptive buffered owners preserve authored knots,
while an Intiface frame deliberately stays on the selected device timing floor
and is tested not to inject a closer knot. Browser Bluetooth retains 0.1%
firmware point resolution; Cloud's integer API floor is documented rather than
hidden. A matched subjective run of shallow patterns and active envelope
changes is still required, so the risk remains Medium.

Relates to R1 (real-device validation), R14 (one motion path), R16 (device
coverage), and R20 (LSO merge integration).

## R23: Emergency Stop Delivery Gaps

Level: Critical

Description:
The permanent Stop control is mounted outside routes. Active, paused,
repeated-idle, and no-engine requests cancel local work and attempt the selected
owner, with explicit errors when transport delivery fails. An unreachable
backend still cannot forward a Browser Bluetooth command, and no path may infer
physical delivery from local stopped state alone.

Implementation status (2026-07-12): active, paused, idle-engine, and no-engine
paths now attempt the selected transport; unavailable owners preserve local
stopped state while returning an explicit error. Intiface hardware produced
distinct successful active and repeated-idle Stop commands plus a recorded
close-time Stop. Browser-backend loss and current Cloud/Browser hardware retry
evidence remain open, so the risk stays Critical.

Mitigation:

- retain regression coverage that every Stop request attempts the selected
  dispatch owner whenever available, including idle-engine and no-engine states
- preserve the current invariant that local planners and motion state stop even
  when transport delivery fails; surface the failure instead of claiming
  physical delivery
- complete current Cloud REST and Browser Bluetooth hardware checks for retry,
  owner-switch, and failed-delivery reporting; retain backend-loss coverage
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
Browser voice input records WebM/Opus or Ogg, while the managed parakeet.cpp
path accepts WAV input. The original implementation forwarded compressed bytes
unchanged and was incompatible with the default managed microphone path. The UI
now decodes the recording, downmixes and resamples it to 16 kHz mono, and emits
real PCM16 WAV before submission; the managed API rejects non-WAV content.
The original control also acquired and destroyed the microphone for every
utterance, so speech begun during browser device/DSP startup was unrecoverable.
Its first "hands-free" revision merely recorded one fixed interval and stopped,
which did not satisfy the interaction contract.

Mitigation:

- run an end-to-end browser MediaRecorder sample through the pinned managed
  runner before claiming push-to-talk acceptance
- keep browser-side WAV conversion bounded; native audio dependencies must not
  enter the pure-Go core
- keep user-started hands-free capture active until manual stop; use bounded
  browser VAD with pre-roll, calibration, sensitivity/end-of-speech controls,
  and a three-phrase pending queue while the browser remains the
  permission/device owner
- upload raw audio and use a private process-session worker `audio_ref`; never
  log or diagnose captures, remove terminal work immediately, remove the owned
  session on shutdown, and reap stale crashed sessions after the bounded request
  window
- reject unsupported formats with a visible actionable error rather than
  forwarding bytes optimistically
- retain fixture tests for every accepted browser format and the WAV provider
  contract

Exit evidence:

- Chrome/Edge localhost push-to-talk and repeated hands-free phrases produce
  accurate transcripts through the pinned managed Parakeet install, with
  format/error tests and no core CGo dependency

Status 2026-07-15: the deterministic format mismatch, repeated cold-start path,
and fixed-interval pseudo-hands-free behavior are fixed. Hands-free now uses an
AudioWorklet, bounded VAD/pre-roll, sequential phrase submission, persisted
tuning controls, raw HTTP upload, session-scoped `audio_ref` staging, backend
Stop-generation fencing, and lifecycle/boundary regression tests. The engine
also rejects starts admitted before its latest Stop, covering delayed non-chat
motion requests. A production-boundary fixture run started the installed CPU
runner and pinned model, transcribed the official Dave WAV after canonical
16 kHz normalization, stopped the worker, and left no related process running.
A real Chrome/Edge run through the pinned runner/model remains required to close
the risk and quantify first-word accuracy and end-to-end latency.

Relates to R17 (voice dependency and latency risk) and R18 (browser security and
LAN microphone access).

## R25: Browser-Clock Media Sync Alignment

Level: High

Description:
Phase 18's future synchronized funscript player will use the browser video as
its clock while device commands cross transports with different buffering and
wire latency. Poor anchoring, over-eager drift correction, a lost heartbeat, or
a seek race could make motion visibly late, jump phases, or continue after the
player is no longer authoritative.

Mitigation:

- keep media as a semantic client of the one motion engine; it never imports or
  dispatches to a transport directly
- make the video clock authoritative through explicit controller-gated
  play/pause/seek/heartbeat events, one bounded sync session, and generation
  fencing around seeks and replacement
- use the engine's phase-preserving pause and low-jump reanchor behavior; do not
  continuously chase sub-threshold drift
- pause motion after a bounded heartbeat loss, preserve unconditional Stop,
  and expose drift/reanchor reasons in `motion_trace.v3`
- require fake-transport integration tests before device use, followed by a
  capped real-device alignment session across the supported owners

Exit evidence:

- M2 tests cover play, seek, pause, resume, ended, stale events, heartbeat loss,
  Stop, and controller loss with one engine play path and no goroutine leaks;
  M3 records trace-derived drift and subjective alignment on real hardware

Status 2026-07-19: M0 adds only an explicit local catalog, Range streaming,
plain browser playback, and an import preview. It cannot load a script into the
engine or emit sync events, so this risk is not yet activated. M2 and M3 retain
the implementation and hardware gates above.

Relates to R1 (real-device validation), R3 (transport behavior), R9 (UI safety
regression), R14 (one motion path), and R23 (Stop delivery).
