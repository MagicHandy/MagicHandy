# Goal Scorecard

## Purpose

The rewrite has three stated goals — maintainability, lower core memory, and
shippable binary releases — plus a safety gate and a real-device milestone.
`docs/goals-and-guardrails.md` defines the targets; this scorecard tracks
whether the project is actually meeting them, in one place, with evidence.
Evidence lives in `docs/perf-baseline.md`, `docs/risk-register.md`, and the
Functional Parity Baseline in `docs/ui-design.md`; this file only summarizes
and links.

Update rule: every phase-completion PR updates the affected rows and appends a
dated History entry. A budget miss is recorded here in the same PR that
misses it — never silently relaxed (see `docs/goals-and-guardrails.md`).
The Phase 17 parity review audits this file row by row.

Scoring key:

- **Met** — target satisfied with recorded evidence.
- **At Risk** — trending toward a miss or sitting at the boundary.
- **Violated** — currently out of budget; needs a fix or a recorded waiver.
- **Unmeasured** — required evidence not yet captured.
- **Pending** — owned by a future phase; not yet expected.

## Snapshot — 2026-07-19, funscript import and timeline repair

### Goal 1: Maintainability

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| CI gates | gofmt, vet, golangci-lint (staticcheck, funlen, gocyclo, depguard), test, race, `CGO_ENABLED=0` build on every PR | **Met** | `.github/workflows/test.yml`; `.golangci.yml` (funlen 100/60, gocyclo 20). Windows PowerShell 5.1 now additionally gates installer syntax, state hygiene, plans, launcher quoting, and updater Git safety. |
| Import boundaries | chat/llm/modes never touch transport; nothing depends on httpapi; no CGo | **Met** | depguard rules + `internal/architecture` boundary tests |
| Size norms — Go core | no core file over ~600-800 lines | **At Risk** | Advisory findings: `internal/config/settings.go` 1,013 lines, `internal/httpapi/voice.go` 1,268, `internal/httpapi/voice_test.go` 1,134, `internal/transport/intiface.go` 1,194, and `internal/transport/intiface_test.go` 1,372. All remain below the 1,500-line emergency ceiling; split when responsibilities can be separated without weakening lifecycle ownership. |
| Size norms — web | same norms for `web/` | **At Risk** | Advisory findings: `web/src/App.test.tsx` 1,210 lines, `web/src/styles/components.css` 1,346, and retired reference-only `web/legacy/app.css` 846. Continuous capture stays isolated from ChatPanel; `web/dist` remains the single shipped build. |
| Size norms — installer scripts | focused modules; review exceptions | **At Risk** | `scripts/installer/InstallerSupport.psm1` is 2,479 physical lines. It is outside the Go/web architecture size test and remains a manually reviewed guideline exception; the next installer slice should separate state/core build, package/bootstrap, managed LLM, and voice-runtime helpers without duplicating updater state or safety teardown. |
| Size-norm enforcement | norms surface as findings, not manual review | **Met** | `internal/architecture.TestSourceFileLineBudgets` reports advisory findings above 800 lines and enforces the 1,500-line emergency ceiling for `cmd`, `internal`, and `web`; PowerShell remains manually reviewed. |
| God-object avoidance | no single struct owning unrelated state | **Met** | Packages match the target architecture; library persistence/import/feedback live in `internal/patterns`, while the engine owns playback and completion. |
| Phase discipline | scoped PRs, tests, docs per phase | **Met** | Phases through 14C and the ahead-of-phase model/runtime/installer work are merged by PR with code, tests, migrations, risk updates, and budget measurements. Post-#63 rendered UI evidence still needs a refresh and is tracked explicitly. |

### Goal 2: Core Memory

All numbers exclude Ollama/llama.cpp/CUDA/TTS/ASR per the measurement rules.
Full rows in `docs/perf-baseline.md`.

| Item | Target | Status | Evidence |
| --- | --- | --- | --- |
| Python baseline | measured before claims | **Met** | StrokeGPT-ReVibed core idle 524.75-524.81 MB (2026-07-01, commit `6c56985`) |
| Go core idle RSS | < 40 MB | **Violated (waived)** | A conservative persistence-audit sample held 53.89 MiB after `/healthz` and 54.36 MiB after all six DB-backed reads. Three repeated exact-final launches later held only 13.16-13.24 MiB idle, but private bytes remained 47.27-47.58 MiB; Windows residency is therefore not stable enough to close the existing SQLite waiver. Re-evaluate with controlled CI telemetry if the conservative sample climbs past ~60 MiB. |
| Go core active RSS | < 80 MB | **Unmeasured** | Model-manager reads settle at 53.40 MiB, but that is not the required active-motion + transport + SSE + chat scenario. Earlier real-device samples (16.75-16.76 MB Cloud REST; 17.52-17.53 MB Browser Bluetooth) predate SQLite and remain historical baselines only. |
| Sustained soak | 1 h RSS within +20% of active baseline | **Unmeasured** | The 2026-07-02 run measured 18.41-20.16 MB over 56 warmed samples (+9.53%), but it predates SQLite. Re-run the full scenario on the current build. |

Risk R11 (goals unmeasured) is substantially closed for memory, with the Phase
11B SQLite idle-RSS waiver now explicit.

### Goal 3: Binary Releases

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| Pure-Go core | `CGO_ENABLED=0` build always works | **Met** | CI gate; depguard denies `C` |
| Binary size | < 30 MB | **Met** | Current timeline-controls tree: 20,633,600 bytes plain and 14,497,280 bytes stripped with `-ldflags "-s -w"`; still well below 30 MB. |
| Cold start to serving UI | < 500 ms | **At Risk** | 679 / 282 / 287 ms over 3 runs with a copied production-style SQLite configuration pointing at the installed managed NeuTTS runtime. The client-side PowerShell probe pre-creates its HTTP client but still includes process-spawn and request overhead; startup no longer hashes roughly 1.1 GiB before listening, but the cold first run still misses the target. Add server-side timestamps in Phase 16 before judging. |
| Release pipeline | portable zip, versioning, release workflow | **Pending** | Phase 16 |

### Safety Gate: Motion Goroutine Lifecycle

| Item | Status | Evidence |
| --- | --- | --- |
| goleak in motion and transport `TestMain` | **Met** | `internal/motion/goleak_test.go`, `internal/transport/goleak_test.go` |
| Stop-teardown coverage | **At Risk** | Active, paused, idle-engine, no-engine, concurrency, owner-switch, and server shutdown attempts are covered. Intiface hardware confirmed distinct active/repeated-idle commands and close-time Stop. Backend-loss delivery for browser-owned Bluetooth remains inherently unavailable and current Cloud/Browser retry evidence is still open. |
| Race tests in CI | **Met** | `go test -race` gate (CI runs it with CGO on Ubuntu) |

### Real-Device Milestone (Motion Core)

| Item | Status | Evidence / Notes |
| --- | --- | --- |
| Engine retarget checklist on hardware | **Met** | Phase 7 via `cmd/retarget-validate` |
| Full app path — Cloud REST | **Met** | A 2026-07-12 isolated Phase 14B app build at 20% passed the connection check, preflight Stop, Start, Pause/Resume, live reverse refresh, active Stop, and repeated-idle Stop. Its 19 transport results all succeeded without starvation. This predates PR #63's visible connection/limit refinements, whose rendered QA refresh remains open (`docs/perf-baseline.md`, "Phase 14B Intiface Hardware Evidence"). |
| Full app path — Browser Bluetooth | **At Risk** | The 2026-07-02 visible Edge Web Bluetooth run moved and stopped the real device, but it predates the reverse-direction fix and was a short session. Revalidate reverse, unconditional Stop, and endurance on hardware. |
| Full app path — Intiface | **At Risk** | The 2026-07-12 Handy workflow passed safety and lifecycle checks, but it predates the deadline-driven asynchronous-ACK pacer and measured queue admission rather than wire timing. Repeat the matched run with `motion_trace.v3` and record subjective feel (`docs/intiface.md`). |
| Controller ownership + owner-switch semantics | **Met** | Phase 9B controller lease, read-only clients, stop-first owner switch, motion SSE (`docs/controller-dispatch-semantics.md`, PR #16) |

### Functional Parity (UI/UX vs StrokeGPT-ReVibed)

Tracked row by row in `docs/ui-design.md`, "Functional Parity Baseline".
Summary: original regression rows 1-9 are closed. Phase 14 also restores the
reference app's functional pattern browser, finite program/funscript player,
freehand authoring, and visible/reversible training feedback while keeping one
backend-authoritative preview and motion path.

## Watch List

Ranked by threat to the stated goals:

1. **Emergency Stop delivery gaps.** Active, paused, repeated-idle, and
   no-engine paths attempt the selected owner and report failed delivery while
   preserving local teardown. An already-connected Browser Bluetooth owner now
   also invalidates fetched work and writes Stop directly during backend loss;
   current Cloud/Browser retry and teardown hardware evidence remains open.
2. **Cold start at the boundary.** Two warmed managed-NeuTTS-configured runs
   were below the target, but the 679 ms cold run was not. Client probe overhead
   and host caching are not separated; treat 500 ms as unconfirmed until Phase
   16 measures it server-side.
3. **Browser Bluetooth endurance.** The full short UI/chat path now passes, but
   Web Bluetooth still depends on an active Edge tab, user-driven pairing, and
   browser GATT stability. Do not treat the short run as a one-hour BLE soak.
4. **Feature growth vs binary/memory/browser budgets.** The current embedded
   browser payload is 861,761 raw / 555,195 gzip bytes because the isolated
   connection artwork contributes 437,417 gzip bytes. HTML/CSS/JS is 417,525 raw
   / 117,778 gzip bytes, and the stripped binary is 14,497,280 bytes. These
   remain within budget, but future bitmap additions must not normalize this
   one-time fidelity cost.
5. **GPU voice/LLM coexistence.** Persistent CUDA NeuTTS fixes interactive
   latency but keeps a second llama.cpp context resident. It passed isolated
   synthesis on a 16 GiB RTX 5070 Ti; representative simultaneous managed-LLM
   load and lower-VRAM acceptance remain R17 evidence.

## History

- **2026-07-19** - Funscript import hardening and timeline repair: the Import
  tab now has compact keyboard-operable zoom/pan/fit controls, viewport-aware
  downsampling, fixed-size draggable action-snapped trim handles, precise
  subsecond/hour readouts, and a persistent selection-length value. Waveform,
  selection, and pointer mapping use one coordinate system; vertical wheel input
  zooms around the cursor, horizontal or Shift-wheel input pans, and a
  proportional pointer/keyboard scrollbar moves the viewport directly. Outward
  wheel input is released at zoom limits. Zoom state cannot alter trim state or
  submitted actions. Browser and backend validation now reject unknown schemas,
  malformed metadata, missing/out-of-range actions, oversized files, and
  mismatched names instead of silently repairing them; sources up to 20,480
  actions remain inspectable when trimmed to the 4,096-action backend limit.
  Finite program imports preserve all selected knots. All 156 frontend tests,
  typecheck/build, and `go test ./...` pass. Relative to the merged Import-tab
  baseline, HTML/CSS/JS grew 8,755 raw / 2,461 gzip bytes to 417,525 / 117,778;
  the complete embedded payload is 861,761 / 555,195 using the established
  per-file level-9 method and unchanged artwork. Plain/stripped pure-Go binaries
  are 20,633,600 / 14,497,280 bytes. No transport path changed; real-device feel
  for preserved imported knots remains R21 exit evidence. The local race build
  remains unavailable because this host has no `gcc`; CI retains the race gate.

- **2026-07-18** - Frontend route, state, and accessibility audit: settings
  drafts survive subsection navigation; quick controls flush pending teardown
  writes; chat history failures are retryable and cross-tab tail reads retry on
  the next backend poll; settings, memory, prompt, model, and voice failures no
  longer masquerade as valid empty/disabled state; and rapid persistence/mode
  mutations are serialized before rerender. Mobile navigation, manual Speed,
  and ASR/TTS provider controls have distinct accessible names; route titles
  and library heading levels are explicit. All top-level routes, five settings
  subsections, and four library views passed 1440x900 and 390x844 rendered DOM
  checks with zero horizontal overflow, duplicate IDs, unnamed controls, or
  nested interactive elements. Typecheck/build and all 141 frontend tests pass.
  Relative to checked-in `main`, HTML/CSS/JS grew 6,940 raw / 1,462 gzip bytes
  to 395,332 / 111,650; the complete embedded payload is 839,568 / 549,067
  using per-file gzip level 9 with a zero timestamp. Hardware behavior is
  unchanged. Go tests, vet, lint, and the pure-Go build pass; the local race
  build remains unavailable because this host has no `gcc`, while CI retains
  the race gate.

- **2026-07-18** - SQLite persistence reliability audit: production now owns one
  bounded database pool instead of six independently churned pools, and every
  logical store shares one transaction lock. Schema v10 preserves malformed or
  oversized settings in bounded history; physical corruption quarantines exact
  DB/WAL/SHM files before a fresh schema is created, while logical schema damage
  fails non-destructively. Settings migrations are durably rewritten and legacy
  reads/app writes share a 256 KiB bound. Version bounds, current-schema/
  foreign-key checks, panic rollback, POSIX modes, redacted recovery status,
  and shared lifecycle ownership have focused tests. Plain/stripped binaries
  are 20,602,368 / 14,466,560 bytes after the installer and library merges. A
  conservative RSS sample was 53.89 MiB
  idle and 54.36 MiB after all six DB-backed reads; three repeated final-binary
  launches held 13.16-13.24 MiB idle but 47.27-47.58 MiB private bytes, so the
  existing SQLite waiver remains. The local race build is
  unavailable because this host has no C compiler; CI retains that gate.

- **2026-07-18** - Installer/update reliability audit: persisted choices now
  use a closed, strongly typed schema with cross-field checks; updater-relative
  state paths resolve once before script delegation; dependency PATH refresh
  preserves session tools; all Go executables stage and promote as one
  rollback-capable Windows/pure-Go set; pinned Parakeet runner contents are
  verified before activation; and generated launchers have a guarded removal
  path. Windows PowerShell 5.1 tests cover malformed state, hostile caller
  directory and Go environment, failed later-worker builds, tampered pinned
  files, launcher ownership, and relative-path delegation. A clean-machine
  dependency bootstrap and Phase 16 release artifacts remain acceptance work;
  the 2,479-line support module remains an explicit maintainability risk to
  split in the next installer slice.

- **2026-07-18** - Pattern-library frontend reliability pass: failed catalog
  reads now show Retry instead of a false empty state; conflicting mutations are
  deduplicated by semantic key while independent work remains visible; unsaved
  authoring survives tab changes; stale previews cannot overwrite newer edits;
  imports avoid a redundant catalog fetch; and canvas drawing commits React
  state once per gesture. Roving tab focus, failed-weight rollback, stable knot
  focus, and defensive progress/curve clamping have focused coverage. The
  frontend suite is 121 tests and typecheck/build pass. Relative to the
  checked-in `main` bundle, HTML/CSS/JS grew 4,736 raw / 1,328 gzip bytes to
  388,392 / 109,947; the complete embedded payload is 832,628 / 547,344 using
  per-file gzip level 9. Hardware motion behavior is unchanged.

- **2026-07-16** - Frontend reliability pass: Browser Bluetooth now preserves
  semantic percentage units, invalidates stale command batches, and delivers a
  direct Stop while an existing GATT session outlives the backend. TTS audio is
  retrieved concurrently but played in order; capture Stop epochs, quick-setting
  writes, settings reset/save, completion-driven polling, and chat SSE framing
  have focused regression coverage. All 109 frontend tests, typecheck, build,
  Go tests, vet, lint, and the pure-Go build pass. Desktop 1440x900 and mobile
  390x844 checks found no horizontal overflow on chat, voice/model settings, or
  the connection manager. HTML/CSS/JS grew 7,734 raw / 2,325 gzip bytes to
  383,656 / 108,721; the complete embedded payload is 827,892 / 546,148. Plain
  and stripped binaries are 20,498,944 / 14,386,176 bytes. The local race build
  remains unavailable because this host has no C compiler; CI retains that gate.
- **2026-07-15** - NeuTTS sampling controls: the validated fixed seed 3 remains
  the default, while one collapsed Advanced section offers another reproducible
  unsigned 32-bit seed, a New seed command, or explicit per-request Varied mode.
  Varied is labeled as repeat-cache-off and documented as capable of restoring
  the measured quality variance; it is not presented as an enhancement. Missing
  settings default additively without a schema bump, and old API clients preserve
  saved values. Plain/stripped core binaries are 20,272,128 / 14,220,288 bytes.
  Embedded UI is 820,158 raw / 543,823 gzip bytes; HTML/CSS/JS is 375,922 raw /
  106,396 gzip bytes, a 2,038 raw / 535 gzip increase.

- **2026-07-15** - NeuTTS consistency and repeat latency: pinned `neutts-rs`
  selected a new random seed for every request; 12 identical warm requests
  varied from 4.60-9.10 s of audio. A mixed corpus rejected one seed that
  produced 0.14 s/silence and selected deterministic seed 3, which retained all
  target words. Incremental overlap-add produced SHA-256-identical corpus WAVs
  while removing repeated full-history mixing. An 8-entry/8 MiB memory-only PCM
  LRU reduced a repeated 4.70 s clip from 1.91 s synthesis to a 0 ms identical
  replay. Browser completion polling is 250 ms instead of 1000 ms. Schema-5
  manifests force managed runtimes onto this behavior; representative listening
  and live incremental browser PCM remain open R17 work. Plain/stripped core
  binaries are 20,263,936 / 14,212,608 bytes. Embedded UI remains 818,120 raw
  bytes and is 543,288 gzip bytes; HTML/CSS/JS is 373,884 raw / 105,861 gzip
  bytes, a one-byte gzip increase. A clean full-feature schema-4-to-5 update
  completed in 10 minutes 56 seconds and preserved all saved feature choices.
  In the relaunched production app, one uncached request completed in 2.799
  seconds and its exact repeat in 34 ms; both returned the same 277,484-byte WAV
  and SHA-256, with the shared queue returning to zero.

- **2026-07-15** - NeuTTS intelligibility correction: direct reconstruction of
  the official Dave codes transcribed correctly, isolating the defect from the
  reference encoder and codec. The pinned pure-Rust phonemizer mispronounced
  common words and dropped one reference word; isolated 25-token codec decodes
  also created discontinuities. The runner now invokes eSpeak NG 1.52 and uses
  Neuphonic's lookback/lookahead overlap-add stream. Four random controlled
  clips reached first audio in 1.06-2.05 s and synthesis completion in
  2.06-3.89 s; managed Parakeet recovered every substantive target word and
  exactly transcribed two clips. Clip duration was 3.10-6.08 s and overlaps
  synthesis during streaming playback, so synthesis timing is not presented as
  end-to-end audible completion. Schema-4 manifests force older runtimes to
  rebuild onto the verified phonemizer path. A clean full-feature schema-3-to-4
  update completed in 11 minutes, left no partial directories, verified the
  activated runner hash, relaunched both voice workers, and completed a
  141,120-byte browser request with an empty terminal queue.

- **2026-07-15** - Persistent accelerated NeuTTS and voice startup: source
  inspection found the installed runner was CPU-only (`n_gpu_layers=0` plus CPU
  codec) and started a fresh model process per request. The old path measured
  127.27 s wall time and 90.86 s to first audio. The pinned CUDA/WGPU build
  loaded in 1.90 s; through the new persistent framed worker, first request
  TTFA/total were 1.01/2.18 s and warm request TTFA/total were 0.47/1.17 s.
  Cancellation and same-process recovery passed. A clean updater run migrated
  the installed runtime to schema 3 CUDA/WGPU in 11 minutes 40 seconds; its
  2,154,884,823-byte (2.007 GiB) voice tree records five CUDA DLL hashes. A
  follow-up update reused it and rebuilt/relaunched in 11.2 seconds. Enabled ASR
  and chat-speech roles autoloaded to `running` / model `ready`; production HTTP
  requests completed in 2.018 and 0.874 seconds with same-process reuse. A
  visible Edge request produced 59,520 bytes, cleared the shared queue, and
  completed without browser warnings after the player moved to a gesture-
  unlocked Web Audio context. Plain/stripped core binaries are 20,262,912 /
  14,211,584 bytes. Embedded UI is 818,120 raw / 543,287 gzip bytes; HTML/CSS/JS
  is 373,884 raw / 105,860 gzip bytes. Against the preceding build, the playback
  fix adds 512 bytes to each binary and 548 raw / 312 gzip UI bytes.

- **2026-07-15** - NeuTTS playback and native reference generation: a
  shell-owned browser player now follows backend TTS requests through the
  five-minute worker deadline, while Settings renders ASR/TTS requests once in
  a shared voice queue. A short-lived Rust/ONNX worker generates NeuCodec
  reference codes from WAV without Python. Its executable, DirectML runtime,
  ONNX graph, and external weights total 558,141,816 bytes (532.3 MiB) of
  optional installed assets; the 7.45 s Dave reference encoded in about
  1.0-1.3 s and observed approximately 1.3 GiB peak worker working set. The
  installed NeuTTS runner accepted its 373 codes and emitted 106,560 PCM bytes.
  Plain/stripped core binaries are 20,249,600 / 14,202,368 bytes; embedded UI is
  817,572 raw / 542,975 gzip bytes. Against the preceding entry this is a
  23,040 / 18,432-byte core increase and a 2,763 raw / 404 gzip UI increase.
  The optional worker/model process remains excluded from core RSS by the
  scorecard measurement rules.

- **2026-07-15** — Startup, continuous voice, and NeuTTS hardening: optional
  voice staging is lazy, state polling is serialized and abortable, the static
  shell and React startup/error states remain responsive, and app startup no
  longer rehashes the managed NeuTTS runtime. User-started hands-free capture
  now segments and serially transcribes phrases until manually stopped, with
  persisted microphone, sensitivity, end-of-speech, and noise-suppression
  controls. A bounded pure-Go parser prepares compatible Torch ZIP/NPY reference
  codes without executing pickle; the focused dialog requires an audio preview
  and exact transcript before applying app-managed paths. The installed runner
  passed its CLI probe in about 10 ms; a real Dave synthesis took 122.576 s,
  produced its first audio at 87.98 s, and yielded 101,760 PCM bytes after the
  known diagnostic was removed. The installed managed-Parakeet CPU module also
  completed an API transcription of the official 7.45 s Dave sample after it
  was normalized to the browser's canonical 16 kHz WAV contract; worker stop
  left no app, adapter, or model-server process running. Plain/stripped binaries
  are 20,226,560 / 14,183,936 bytes; embedded UI is 814,809 raw / 542,571 gzip
  bytes (105,144 gzip excluding unchanged artwork). This is a 308,224 /
  221,184-byte binary increase and a 25,044 raw / 7,073 gzip UI increase. RSS
  and browser-microphone
  segmentation/latency were not remeasured; desktop/mobile rendered checks were
  console-clean.

- **2026-07-14** — Browser voice startup/latency hardening: the Chat microphone
  now keeps a visibly releasable warm stream, supports bounded click-on
  hands-free and hold modes plus input selection, performs filtered browser WAV
  conversion without the old JavaScript copy, uploads raw audio, and hands ASR a
  private session-scoped `audio_ref`. Emergency Stop now invalidates voice and
  in-flight chat generations before motion dispatch. Plain/stripped binaries
  are 19,918,336 / 13,962,752 bytes; embedded UI is 789,765 raw / 535,498 gzip
  bytes (98,101 gzip excluding unchanged artwork). This is a 65,024 / 48,640-byte
  binary increase and a 10,224 raw / 2,793 gzip UI increase from the preceding
  managed-NeuTTS measurement, all within budget; RSS and real-microphone latency
  were not remeasured.

- **2026-07-14** — Managed NeuTTS source installation: selecting managed
  llama.cpp now also provisions LLVM/libclang and pinned Rust 1.94.0, builds
  `neutts-rs` v0.1.1 with its CPU llama.cpp binding, converts a verified
  NeuCodec checkpoint, and atomically installs the verified Air Q4 cache.
  Skipping managed llama.cpp explicitly skips NeuTTS. Installer and app checks
  pin revisions and rehash runtime/model bytes; reference codes remain user
  supplied. Plain/stripped binaries are 19,853,312 / 13,914,112 bytes; embedded
  UI is 779,541 raw / 532,705 gzip bytes (95,308 gzip excluding unchanged
  artwork). This is a 15,872 / 11,776-byte Go binary increase and a 227-byte
  gzip UI increase from the preceding voice audit, all within budget; RSS and a
  full external NeuTTS build were not measured on this host.

- **2026-07-14** — Voice installation/runtime audit: browser microphone data is
  converted to 16 kHz PCM WAV before managed Parakeet submission; NeuTTS now
  preflights adapter, runner, decoder, exact backbone cache, reference codes,
  transcript, and a bounded synthesis before reporting ready; and local path
  fields use a controller-gated Windows host picker. Source-install completion
  now distinguishes built adapters from configured external runtimes. Plain /
  stripped binaries are 19,837,440 / 13,902,336 bytes; embedded UI is 779,484
  raw / 532,478 gzip bytes (95,081 gzip excluding unchanged artwork). This is a
  1,169-byte gzip UI increase and remains within budget; RSS was not remeasured.

- **2026-07-14** — Audited implementation progress after PRs #63-#67. Updated
  phase status, Stop behavior, transport scope, voice acceptance, source-size
  evidence, and the distinction between pre-asynchronous-ACK hardware evidence
  and the current Intiface validation gap. No budget target was changed.

- **2026-07-13** — Intiface pacing no longer waits for each Buttplug ACK before
  the next absolute deadline. A bounded asynchronous ledger, response deadlines,
  stale-frame suppression, startup anchoring, append-time reverse mapping,
  device timing capabilities, generation-safe Stop/Close barriers, and
  `motion_trace.v3` wire telemetry close the static smoothness deficiencies.
  Plain/stripped binaries are 19,793,920 / 13,870,080 bytes; embedded UI is
  777,057 raw / 531,309 gzip bytes (93,912 gzip excluding unchanged artwork).
  The revised path still needs a matched live Handy feel/timing run.

- **2026-07-13** — A live managed Gemma 4 12B Q4 reproduction confirmed that
  automatic reasoning consumed the complete 256-token JSON budget and returned
  no visible content; 512 tokens failed the same way. Reasoning-off and a
  128-token managed reasoning budget both produced valid JSON for the exact
  request. Reasoning now defaults off, the current pinned managed automatic path
  is bounded, provider truncation is explicit, repair retains original context
  and requests reasoning off, and parser-valid examples end with an STGPT-style immutable guard.
  Plain/stripped binaries are 19,710,464 / 13,807,616 bytes; embedded UI is
  776,443 raw / 531,099 gzip bytes (93,702 gzip excluding unchanged artwork).

- **2026-07-13** — Source rebuilds no longer replace an executable while its old
  process still owns the HTTP port. The updater sends Emergency Stop, tears down
  only the checkout-owned process tree, stages Go outputs before replacement,
  removes legacy `*.exe~` backups, and waits for the rebuilt process to own the
  port and answer `/api/state` before opening the browser. Temporary-app tests
  cover quoted data paths, Stop/teardown, foreign and multiple-instance refusal,
  and backup cleanup; staging/readiness paths retain syntax and source coverage.
  Core/UI bytes are unchanged, so the immediately preceding measurements remain
  current.

- **2026-07-13** — Model settings now bound compact LLM output (default 256),
  expose provider-native automatic/off reasoning with latency/quality/support
  warnings, serialize zero-temperature repair, and skip redundant warm managed
  readiness probes. Cloud firmware/API requirements render as a notice rather
  than a disabled-looking field. Voice settings distinguish the detected
  MagicHandy Parakeet module from custom local paths and explain Enable > Save >
  Start; Start now means model-ready. The source updater safely handles live and
  merged/deleted feature upstreams with ancestry-checked fast-forwards. Plain /
  stripped binaries are 19,704,320 / 13,802,496 bytes; embedded UI is 776,296
  raw / 531,060 gzip bytes (93,663 gzip excluding unchanged artwork). LLM runtime
  latency remains unmeasured; these figures are size evidence only.

- **2026-07-13** — Chat's heading now aligns with its wide workspace and the
  position visualizer uses a compact vertical Handy body/sleeve form. Speed and
  Stroke use dual-thumb controls with track-sized pointer input, native
  keyboard/AT semantics, independent backend patches, and strict Stroke bound
  separation. Cloud REST remains a stateless backend-authoritative connection
  check rather than presenting a frontend-only session. The initial connection
  phase is neutral until the first snapshot arrives. Plain/stripped binaries are
  19,682,304 / 13,786,624 bytes; embedded UI is 771,643 raw / 530,031 gzip bytes
  (92,634 gzip excluding the unchanged connection artwork).

- **2026-07-13** — source installation can now begin on 64-bit Windows without
  preinstalled Go, Git, CMake, MSVC, CUDA, or Ollama. Missing selected packages
  are consented, installed through WinGet (with Microsoft's repair path), and
  verified in-process. The installer builds the core plus all three first-party
  voice adapters and atomically stores only non-secret choices. `update.ps1`
  displays those choices, asks whether to revise them, refuses dirty trees, and
  fast-forwards before rebuilding. Both entry points add operation branding and
  honest ready/plan-only completion art. Windows PowerShell 5.1 tests cover
  state hygiene, dependency graphs, launcher quoting, clean fast-forward, and
  dirty-tree refusal. A clean pinned CPU llama.cpp build completed in 70.8 s and
  reported `c749cb0`; broad Go/frontend gates passed. Plain/stripped binaries
  are 19,677,696 / 13,782,016 bytes; UI bytes and the 53.47 MiB idle sample are
  retained because only the explicit-build helper changed at runtime.

- **2026-07-12** — Phase 14C adds the route-independent connection manager with
  provider-scoped live actions and immediate speed/stroke limits. Its trigger
  now lives in the top bar; a 444,236-byte transparent, reference-guided hand
  isolation replaces the distorting SVG luminance mask. The final target
  recreates the reference's tall capsule, domed body, LED, and square marker;
  three intense-blue arcs appear only for connecting/connected states. The
  square is red while disconnected and green when connected; only a failed
  attempt shows a briefly shaking red X. The shared position estimate is now a
  Handy rail/carriage visualizer instead of an abstract track. Cloud REST adds
  a scoped write-only connection key control and visible API v3 ID source,
  while empty developer overrides fall back to the bundled StrokeGPT-ReVibed
  ID. Plain/stripped binaries are
  19,675,648 / 13,779,968 bytes, idle RSS is 53.47 MiB, and the full embedded
  browser payload is 529,003 bytes gzip (91,576 excluding the artwork).

- **2026-07-12** — Phase 14B live safety close-out on `The Handy (FW4+)` through
  Intiface Central: a 20% stroke passed Pause/Resume and an immediate reverse
  window refresh with 19 successful trace rows and no starvation. Active and
  repeated-idle Stop produced distinct successful commands; disconnect recorded
  its close-time Stop. The same change makes idle/no-engine Stop attempt the
  selected owner and report unreachable transports honestly. Final plain and
  stripped binaries measure 19,205,632 / 13,309,952 bytes; idle RSS is 53.20
  MiB; embedded UI is 86,893 bytes gzip. A matched Cloud run also passed with
  19 successful results and no starvation; subjective feel remains open, and
  no non-Handy device was available.

- **2026-07-11** — Phase 14B implementation: the transport contract now uses
  neutral point/play names and float positions, with Handy quantization only at
  encode time. A pure-Go Buttplug v3 owner adds persistent Intiface Central
  sessions, keepalive, discovery, one linear-actuator selection, scheduled
  `LinearCmd`, queue/underrun health, and stop-first teardown. Fake-server,
  shared owner-contract, HTTP, lifecycle, and React tests are green. Plain and
  stripped binaries initially measured 19,197,440 / 13,303,808 bytes; idle RSS
  was 52.88 MiB; embedded HTML/CSS/JS was 86,864 bytes gzip. Final measurements
  after unconditional Stop hardening are recorded in the newer row above.

- **2026-07-11** — Managed llama.cpp source build and model-selection parity:
  the app and installer share a pinned `b9966` / `c749cb0` builder, validate an
  app-owned runtime manifest, support CPU/CUDA/auto plus cancellation, and
  resolve managed selections by SQLite model ID. The installer explains direct
  runner-control benefits and supports `-SkipLlamaBuild` for existing Ollama
  users who want to avoid duplicate runtime/model storage. A clean CPU build
  completed in 54.2 seconds and installed 18,432,916 bytes; rerun was
  idempotent. The embedded UI passed 1280×800 and 390×844 checks with no
  horizontal overflow or console warnings; a real 16-model Ollama daemon
  accepted a saved model selection and reported ready without a llama.cpp
  build or model load. Budget evidence: 18,822,656 bytes plain / 13,031,936
  stripped; 52.73 MiB idle / 53.40 MiB after model-manager reads; UI 85,718
  bytes gzip.

- **2026-07-11** — LLM model-manager foundation: schema v9 adds managed-model
  metadata; explicit GGUF and configurable Ollama-library imports copy into a
  private store with SHA-256 verification, cancellation, deduplication, and
  selected-model removal protection. The provider list no longer depends on a
  valid selected Ollama model. The rendered Model screen was checked at 1280px
  and 390px widths; a real Windows Ollama library and daemon each reported the
  same 16 models without starting a model copy.

- **2026-07-11** — Phase 14 complete on the review branch: generated built-in
  patterns, user patterns, finite programs, MagicHandy share files and bounded
  funscript import now persist in schema v8 and play only through the shared
  motion engine. The LLM receives enabled IDs/weights as a curation catalog;
  disabled IDs are rejected and an all-disabled library keeps the deterministic
  fallback. Authoring uses reversal-preserving simplification and backend PCHIP
  previews; training feedback is visible, exact-undo, and auto-disable remains
  opt-in. The divergent GitHub `Rockfire` lineage was audited rather than
  merged: six runtime DB files, duplicate UI/datastore trees, stale bundles,
  and its direct manual-queue transport path were excluded; schema v8 preserves
  its core rows and uninterpreted LSO tables for Phase 15. Rendered 1280 px and
  390 px checks covered all library tabs and fixed one mobile clipping defect.
  Budget evidence: binary 18,464,256 bytes plain / 12,766,208 stripped; RSS
  52.49 MiB idle / 52.99 MiB after library reads; UI 80,533 bytes gzip
  (+8,174, +11.3%). The capped real-device routine-cycle feel check remains.

- **2026-07-06** — Phase 11B complete on the current branch: settings,
  memories, and user prompt sets now round-trip through one pure-Go SQLite
  datastore (`magichandy.db`, `modernc.org/sqlite v1.53.0`) with forward
  `PRAGMA user_version` migrations, WAL/busy-timeout pragmas, serialized write
  transactions, and legacy JSON import fixtures. Legacy `settings.json`,
  `memories.json`, and `prompt_sets.json` are archived as `*.migrated` after
  import. Redaction still holds: the imported Handy connection key remains in
  the private settings snapshot and does not appear in public settings.
  Binary re-measured: 17.92 MB plain / 12.32 MB stripped, under the <30 MB
  stripped budget. RSS waiver: stripped build idles at 54.13 MB after
  `/healthz` and 54.36 MB after DB-backed API reads, exceeding the original
  <40 MB idle target; this is accepted for Phase 11B as the cost of pure-Go
  SQLite, not a silent target change.
- **2026-07-06** — Decision recorded (ADR 0008): persistence moves to a single
  pure-Go SQLite datastore (`modernc.org/sqlite`, `CGO_ENABLED=0`) in Phase
  11B, replacing the three JSON stores (settings, memory, prompt sets).
  Planning only — no code or measurement yet. Binary/RSS impact is Watch-List
  item 3 and must be re-measured when 11B lands (current headroom: 7.70 MB
  stripped against the 30 MB budget; idle 8.96 MB against 40 MB). The redaction
  contract and "reset keeps memory and prompt sets" are preserved by the ADR.
- **2026-07-05** — Motion-safety review fixes (external review pass). Three
  confirmed defects fixed with regression tests: (1) reverse direction
  double-inverted — the engine pre-reversed HSP points and the Cloud/Bluetooth
  transports reversed again from the same setting, so `reverse=true` was a
  silent no-op on the shipped path; the engine now emits semantic positions and
  the transport boundary owns reverse (Invariant 3). **Consequence for prior
  rows:** the Cloud REST / Browser Bluetooth "full app path validated" runs did
  not actually exercise working reverse direction; re-verify reverse on the next
  hardware session. (2) A concurrent Stop/Pause during Start's transport setup
  could call a nil cancel func and panic; the loop cancel is now installed
  atomically with `running=true`. (3) The recovery stop reused the just-cancelled
  loop context, so the safety stop could be dropped on a real transport; it now
  sends on a detached context. (A self-deadlocking `waitForLoop` in
  `stopForRecovery` was proposed on a separate open branch; a regression test
  here guards against it.)
- **2026-07-03** — Phase 11 complete: `internal/modes` implements Freestyle
  and chat keepalive as motion-engine clients behind a bounded
  motion-arrangement contract (1-8 segments, 4-120s each, optional focus and
  one mid-segment drift). Deterministic style scoring (gentle/balanced/
  intense, a persisted quick setting) with seeded, fully-traced planner
  decisions (`planner` rows: seed, score table, segment). The no-stall gate
  passes on the real engine over the fake transport: many segment boundaries,
  exactly one HSP play, zero stops. Keepalive restarts only after transport
  recovery — never after user stop or pause (tested). Import boundaries hold
  (modes never import transport). Binary re-measured: 10.84 MB plain /
  7.70 MB stripped (+0.10 MB).
- **2026-07-02** — Phase 10 complete: user-managed long-term memory
  (`internal/memory`, `/api/memory`, immediate-apply UI with individual and
  global switches), editable prompt sets with protected built-ins
  (`internal/chat` library, `/api/prompt-sets`), the code-owned chat contract
  (`ComposeSystem` appends the motion JSON contract so prompt edits cannot
  weaken it), and the settings factory reset (parity row 7 closed). Chat
  verified to work with memory disabled at the service and API levels.
  Binary size re-measured: 10.74 MB plain / 7.62 MB stripped (+0.12 MB).
- **2026-07-02** - Patched Browser Bluetooth app-path validation passed in the
  user's running Edge profile with the real `OHD_hw0_29b3243120f4` device:
  visible Check connection used bridge readiness and did not queue `hsp/state`;
  visible Start motion at 28% traced `stroke_window`, `hsp_add`, and
  `hsp_play` as `browser_ack`; deterministic chat `stop` returned
  `Stopping motion.` and traced Stop as `browser_ack`. A repeat visible
  Start/Stop captured Browser Bluetooth active RSS at 17.52-17.53 MB across
  three samples. The earlier `hsp/state`/`hsp_play` failures are retained in
  `docs/perf-baseline.md` as debugging history.
- **2026-07-02** — Live Browser Bluetooth follow-up with the device online:
  Edge selected `OHD_hw0_29b3243120f4`, the bridge became ready, and a
  non-moving Stop command ACKed in 102 ms. The run found and fixed three
  app-path defects before motion could complete: Browser Bluetooth now maps
  semantic motion stream IDs to numeric BLE stream IDs, the command long-poll
  recovers after backend restarts, and Bluetooth command consumers use
  per-tab IDs so stale tabs cannot steal commands. The follow-up also split
  the Browser Bluetooth Go transport out of the bridge file, removing the last
  file-size override. Full Bluetooth motion/chat remains unmeasured because the
  live GATT link then disconnected or reported `hsp/state` timeout before a
  capped start could run.
- **2026-07-02** — Phase 9B close-out follow-up extracted browser-owned BLE
  session handling from `web/app.js` into `web/bluetooth-ui.js`, brought web
  files back under the size norm, and added `TestSourceFileLineBudgets` so file
  growth is enforced automatically. Browser Bluetooth app-path validation
  remains blocked on Windows/Chromium seeing the `OHD`/Handy BLE advertisement.
- **2026-07-02** — Initial scorecard @ `f5441ba`. Memory goal fully measured
  and met (idle 8.96 MB, active 16.76 MB, soak +9.53%, Python baseline
  525 MB). Binary 10.59 MB / cold start 411-522 ms measured ad hoc. Cloud
  REST app path validated on hardware; Bluetooth later refined from "manual
  gesture needed" to "BLE visibility needed" after Edge/Windows saw no
  selectable `OHD`/Handy advertisement. Size norms violated by `web/app.js`
  (1120); no automated size enforcement yet.
