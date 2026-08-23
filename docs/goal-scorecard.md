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

## Snapshot — 2026-08-22, compiled-output Creative fidelity and reconciled clocks

### Goal 1: Maintainability

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| CI gates | gofmt, vet, golangci-lint (staticcheck, funlen, gocyclo, depguard), test, race, `CGO_ENABLED=0` build on every PR | **Met** | `.github/workflows/test.yml`; `.golangci.yml` (funlen 100/60, gocyclo 20). Windows PowerShell 5.1 additionally gates installer syntax, localized catalog parity, state hygiene, plans, launcher quoting, and updater Git safety. Frontend tests gate catalog/placeholder/encoding parity, typed and static rendered strings, literal toasts/confirms, and adjacent-fragment hazards. |
| Import boundaries | chat/llm/media/modes/persona never touch transport; persona never owns motion; nothing depends on httpapi; no CGo | **Met** | depguard rules + `internal/architecture` boundary tests |
| Size norms — Go core | no core file over ~600-800 lines | **At Risk** | Current advisory findings include `internal/config/settings.go` 1,434 lines, `internal/config/settings_test.go` 1,476, `internal/httpapi/chat.go` 1,395, `internal/httpapi/voice.go` 1,377, `internal/chat/service.go` 1,260, `internal/modes/manager.go` 1,036, `internal/motion/content.go` 1,087, `internal/motion/engine.go` 1,009, `internal/motion/dynamic.go` 849, `internal/transport/intiface.go` 1,209, and `internal/transport/intiface_test.go` 1,377. Dynamic HTTP compilation was extracted into focused `dynamic_motion.go`, and correction/coverage policy remains in `dynamic_intent.go`; every enforced path remains below the 1,500-line emergency ceiling. |
| Size norms — web | same norms for `web/` | **At Risk** | Current advisory findings include `web/src/api/types.ts` 1,308 lines, `web/src/App.test.tsx` 1,487, `web/src/components/SyncedVideoPlayer.tsx` 1,144, `web/src/styles/components.css` 1,459, `web/src/styles/shell.css` 1,089, and retired reference-only `web/legacy/app.css` 846. Setup and update UI use the canonical app and lazy locale chunks; `web/dist` remains the single shipped build. `App.test.tsx` is close to the 1,500-line emergency ceiling and must be split before more broad shell coverage lands there. |
| Size norms — installer scripts | focused modules; review exceptions | **At Risk** | `scripts/installer/InstallerSupport.psm1` is 2,374 physical lines and remains outside the Go/web architecture size test as a manually reviewed guideline exception. Optional Python/PyTorch speech setup lives in dedicated install/update scripts; the shared module owns bootstrap/state/process helpers used by source and GUI provisioning. |
| Size-norm enforcement | norms surface as findings, not manual review | **Met** | `internal/architecture.TestSourceFileLineBudgets` reports advisory findings above 800 lines and enforces the 1,500-line emergency ceiling for `cmd`, `internal`, and `web`; PowerShell remains manually reviewed. |
| God-object avoidance | no single struct owning unrelated state | **Met** | Packages match the target architecture; pattern persistence/import/feedback live in `internal/patterns`, the explicit video catalog lives in `internal/media`, and the engine remains the sole owner of motion playback. |
| Phase discipline | scoped PRs, tests, docs per phase | **Met** | Phase 18 M1-M2 reuse one bounded exact-name funscript document for the canvas and one shared-engine finite media target, keep host paths out of the API, and leave real-device timing claims to the explicit M3 acceptance gate. |

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
| Binary size | < 30 MB | **Met** | Current local Go 1.26.4 alpha.34 candidate: 24,167,936 bytes plain and 17,394,176 bytes release-style stripped with `CGO_ENABLED=0` and `-trimpath`; the packaged core remains well below 30 MB. Tag CI uses the `go.mod` 1.25 toolchain and remains authoritative for published artifacts. |
| Cold start to serving UI | < 500 ms | **Met** | Five fresh isolated-data launches of the current stripped binary listened in 67.9-94.0 ms and completed `/healthz` in 68.7-119.5 ms total, including process spawn and loopback request. Managed preload is asynchronous; these fixtures had no installed model or voice worker. |
| Release pipeline | setup exe, portable zip, versioning, release workflow | **Met** | `v0.1.0-alpha.34` uses `ReviewedUnsignedPublic`: the tag workflow Defender-scans the exact public directory, verifies setup/ZIP manifests and two-entry checksums, exercises custom and Program Files lifecycle, and publishes three explicit assets. The policy is limited to alpha.8 through alpha.11 and alpha.13 through alpha.34 with Microsoft case `15c1e36d-fb35-4c5d-85de-83707169818a`; withdrawn alpha.12 remains rejected. Pull requests remain short-lived `UnsignedCI`, and `SignedPublic` remains the long-term publisher-identity gate. |

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
| Full app path — Cloud REST | **Met** | A 2026-07-12 isolated Phase 14B app build at 20% passed the connection check, preflight Stop, Start, Pause/Resume, live reverse refresh, active Stop, and repeated-idle Stop. Its 19 transport results all succeeded without starvation. The latest installed-session trace added 439 points/85 legs, ordinary 330–341 ms append acknowledgements, one network error, and a successful 330 ms user Stop; it exposed a long 31–35% Creative span plateau and abrupt endpoint shape. That trace predates alpha.27's correction, so matched post-change feel remains open (`docs/motion-calibration.md`). |
| Full app path — Browser Bluetooth | **At Risk** | The 2026-07-02 visible Edge Web Bluetooth run moved and stopped the real device, but it predates the reverse-direction fix and was a short session. Revalidate reverse, unconditional Stop, and endurance on hardware. |
| Full app path — Intiface | **At Risk** | The 2026-07-12 Handy workflow passed safety and lifecycle checks, but it predates the deadline-driven asynchronous-ACK pacer and measured queue admission rather than wire timing. Repeat the matched run with `motion_trace.v3` and record subjective feel (`docs/intiface.md`). |
| Controller ownership + owner-switch semantics | **Met** | Phase 9B controller lease, read-only clients, stop-first owner switch, motion SSE, and explicit stop-first takeover with a globally locked handoff (`docs/controller-dispatch-semantics.md`) |

### Functional Parity (UI/UX vs StrokeGPT-ReVibed)

Tracked row by row in `docs/ui-design.md`, "Functional Parity Baseline".
Summary: original regression rows 1-9 are closed. Phase 14 also restores the
reference app's functional pattern browser, finite program/funscript player,
freehand authoring, and visible/reversible training feedback while keeping one
backend-authoritative preview and motion path. Interactive LLM motion now also
reads authoritative current state, preserves steady/pacing-only continuity,
supports named area focus, and bounds explicit pattern variation.
The user-facing Creative mode adds bounded model-authored center/span or named-
anchor geometry without a second motion path and is now the default; Pattern
Library and Off remain explicit persisted alternatives. Supported-provider and
capped real-device A/B acceptance remains open. Its
outer geometry can now carry an independent model-selected floor/profile, so
stroke length changes across a long deterministic phrase while center/rhythm
variation and pace remain separate. Wander/contrast now explore that envelope
within short perceptual windows. Creative true reversals use monotone C2
quintic easing instead of a C1 acceleration corner, and compact 2–4-section
phrases add long-horizon route, stroke-length, and timing diversity without a
second motion path. The whole-percent Creative fitter preserves time-at-
position as well as position-at-time so device quantization cannot silently
linearize a short eased stroke.
The installed managed 12B Gemma model passed 25/25 broader first-response
Creative decisions and 9/9 repetitions of the reproduced position-correction
sequence without an engine or transport. The alpha.29 phrase-age matrix kept
3/3 short-age holds, changed 3/3 long-age/six-hold cases, and completed a
12-decision run with one hold, 11 semantic changes, 11 range envelopes, four
horizons, and no repair/fallback. Alpha.30 then passed three repeated exact-
Autopilot five-decision sessions: 15/15 responses used three sections and every
mapped trajectory compiled with least-varied 12-second stroke CV/range of at
least 9.5%/15.9 points. A max-rate, 180-second phrase-age probe still obeyed an
explicit hold. The retained 42–72 Creative fixture now raises compiled mean
travel from 103.8 to 167.5%/s while its peak follows the selected calibration,
instead of alpha.29's flat 109.8%/s at every setting. The active catalog contains 81 built-ins: 25
non-experimental and 56 experimental, after retiring the six canonical entries
disabled in the installed alpha.28 profile. The shared 1–100 loop-speed scale is calibrated through the selected Original /
Handy 2 Standard / Handy 2 Pro published travel and normal speed envelope, with
a separate exact-curve runtime envelope.
Opted-in chat voice now also receives bounded persona/anatomy context, strict
per-session model mood, and three canonical recent assistant lines while
utility remains byte-identical and all motion gates remain unchanged.
Direct embodied chat wording grants bounded current-turn permission, but the
model now owns the action/no-action choice; deterministic code no longer
synthesizes an omitted start.

## Watch List

Ranked by threat to the stated goals:

1. **Emergency Stop delivery gaps.** Active, paused, repeated-idle, and
   no-engine paths attempt the selected owner and report failed delivery while
   preserving local teardown. An already-connected Browser Bluetooth owner now
   also invalidates fetched work and writes Stop directly during backend loss;
   current Cloud/Browser retry and teardown hardware evidence remains open.
2. **Cold start at the boundary.** The current three-launch schema-v2 sample is
   below budget with asynchronous managed-model preload. Host caching was not
   separated, so Phase 16 still owns controlled release telemetry.
3. **Browser Bluetooth endurance.** The full short UI/chat path now passes, but
   Web Bluetooth still depends on an active Edge tab, user-driven pairing, and
   browser GATT stability. Do not treat the short run as a one-hour BLE soak.
4. **Feature growth vs binary/memory/browser budgets.** The complete embedded
   browser payload is 1,696,068 raw / 803,095 level-9 gzip bytes. Lazy loading
   limits the English startup path to 810,662 raw / 214,068 gzip bytes; all
   HTML/CSS/JS is 1,251,832 raw / 365,698 gzip bytes. Alpha.30's reconciled
   countdown adds 875 raw / 280 gzip bytes to each code-bearing measure; the
   compiled-motion changes are backend-only. Creative C2 phrases,
   section-aware status, and the eight-level Motion change rate control add 71
   raw bytes and 227 gzip bytes to the checked-in alpha.27 bundle;
   the English startup path is 146 raw bytes smaller and 142 gzip bytes larger.
   Creative envelope status
   adds 933 raw / 417 gzip bytes overall and 452 raw / 149 gzip bytes to the
   English startup path against alpha.25. Motion calibration,
   the merged Handy model control, and aligned visible set-point controls add
   6,502 raw / 1,427 gzip bytes overall and 6,307 raw / 1,342 gzip bytes to
   startup against checked-in `main`. Selectable Dynamic motion,
   mode-specific status, and the Chat control-sidebar list add 3,718 raw / 1,199
   gzip bytes overall and 2,696 raw / 780 gzip bytes to the English startup
   path against branch base `0d0a8348`; the artwork is unchanged. The localized Ollama
   compatibility state and stable repair-draft presentation add 448 raw / 212
   gzip bytes overall and 245 raw / 121 gzip bytes to the English startup path.
   The updater's
   channel-neutral status wording adds 86 raw bytes and removes 2 gzip bytes
   overall; the English startup path adds 24 raw and removes 2 gzip bytes. The managed llama.cpp
   loading-state UI adds 465 raw / 75 gzip bytes to each measure. The Phase 16
   setup and update UI add 84,371 raw / 25,014 gzip bytes overall and 43,171 raw
   / 10,356 gzip bytes to the English startup path against the preceding
   snapshot. LLM loading controls,
   phase diagnostics, speech policy, and duplicate-process recovery add 21,116
   raw / 6,163 gzip bytes overall and 9,696 raw / 2,180 gzip bytes to startup
   against the checked-in predecessor. The local-TTS transition
   removes 14,798 raw / 5,341 gzip bytes overall and 4,402 raw / 1,764 gzip bytes
   from startup relative to its checked-in predecessor. Independent Autopilot clocks,
   preferences, localization, and playback acknowledgement add 12,158 raw /
   3,315 gzip bytes against their preceding checked-in bundle; browser-session
   notification persistence adds another 1,424 raw / 481 gzip bytes. The
   synchronized-video transport, embedded auto-hide overlay, vertical volume
   control, and explicit seek lifecycle add 18,411 raw / 4,693 gzip bytes total
   (14,247 / 3,434 on the
   English startup path) against
   their checked-in predecessor; the bitmap is unchanged. These remain within
   budget, but future locales, personas, and bitmap additions must keep startup
   and total payload growth explicit.
5. **GPU voice/LLM coexistence.** Faster Qwen3-TTS and Chatterbox are isolated
   optional servers, but each can still compete with a managed LLM for VRAM.
   Chat can now interrupt queued/active speech, and Faster Qwen stops an
   abandoned stream after the current generator yield. Live simultaneous-LLM
   cancellation acceptance remains R17 evidence; Chatterbox CPU is the
   documented fallback.

## History

- **2026-08-22** - Prepared alpha.34 after the installed alpha.33 Cloud trace
  showed 4,824 points over 403.5 seconds but only 1.46% of commanded time below
  position 10 and 0.45% above 90. Its first 15 Creative phrases clustered at
  center 48–52 / span 40–45 until explicit chat requested the base. Motion-only
  system authority, bounded compiled-band history, intent-first planning,
  rate-scaled sampling, aggregate perceptual admission, and one
  non-prescriptive high-rate retry now let the active Gemma move from localized
  to authored 45/68/82/92-span phrases without a numeric broad target or forced
  schedule. Wander approaches its selected outer span more often through the
  smooth correlated envelope. Two live Pauses had also restarted Autopilot
  about 2.2 seconds into their Stop round-trips; a pre-I/O mode latch now covers
  the exact idle/unpublished-paused gap and preserves the stream until explicit
  Resume. The final ten-turn hands-off run, five-turn explicit three-section
  run plus long-age hold, and three repeated Hei speech-novelty runs passed
  transport-free on the installed 12B Gemma. A fresh isolated review app then
  passed real provider generation and one no-repair/no-fallback text-only app
  chat; fake transport stayed idle with zero commands and an empty trace.
  Full local release gates pass except the unavailable Windows race toolchain;
  Linux CI is authoritative. A capped device feel confirmation remains tracked
  separately. Candidate binaries are 24,167,936 / 17,394,176 bytes; the
  frontend is unchanged from alpha.33. No post-change physical motion was
  commanded.

- **2026-08-22** - Prepared alpha.33 after a read-only inspection of the
  installed alpha.32 Autopilot failure. The 97-row trace had no drops and
  recorded 15 valid Creative targets followed by 15 startup Stops, 15
  position-read failures, and 15 failed starts. The stationary Original Handy
  reported 111.33 mm against a 5.00-102.83 mm full-stroke response, placing it
  at 108.7% of inferred calibrated travel and beyond the old 3% recovery
  allowance. Startup now admits at most a 10% initial excursion to the shared
  speed-bounded lead-in while retaining the raw distance and strict stationary
  1% arrival gate. Larger geometry ends only the failed mode generation instead
  of retrying, and transient failures keep their backoff. Exact-geometry,
  boundary, circuit, stale-generation, transient, and lifecycle regressions
  pass with the full Go, vet, zero-issue lint, zero-CGo, PowerShell 5.1
  installer, localization/typecheck/build, and 408-test frontend gates. A fresh
  isolated alpha.33 app passed real llama.cpp generation and one no-repair,
  no-fallback text-only app chat in 544 ms; fake transport stayed idle at zero
  commands with motion unavailable and Autopilot off. Candidate binaries are
  24,116,224 / 17,374,720 bytes, and the frontend is byte-identical to alpha.32.
  No post-change physical motion was commanded.

- **2026-08-22** - Prepared alpha.32 after reproducing the Settings-backed
  default persona card opening `#/settings/model`. The card now deep-links to
  `#/settings/prompts`, where the default prompt set, reply register, persona
  description, anatomy context, prompt editor, and memory manager are actually
  owned. The focused 25-test persona suite and full 408-test frontend suite,
  localization, typecheck, and production build pass. A fresh isolated rendered
  build confirmed the card href, clicked through to the Prompts & memory h2,
  and found the Persona description control. The same build used fake transport
  with zero commands, passed real llama.cpp generation readiness, and completed
  a text-only chat turn with “The chat is working correctly.” Candidate binaries
  remain 24,137,728 / 17,370,112 bytes; the corrected two-character route adds
  2 raw / 1 level-9 gzip byte to the complete, code-only, and English-startup
  browser measures. No physical motion was commanded.

- **2026-08-22** - Prepared alpha.31 after inspecting the installed Hei session.
  Its initial Creative phrase survived nine autonomous decisions for roughly
  two minutes at motion-change rate 8, and subsequent updates stayed inside one
  narrow range character. Autopilot now treats its enabled state as continuing
  bounded authorization, renders rate as a whole-phrase continuity bias, and
  distinguishes meaningful outer-band/section contrast from nearby scalar
  churn while preserving explicit holds. The retained perceptual baseline
  recognizes smooth cumulative change without requiring abrupt jumps. Speech
  prompts use recent lines to avoid recycling sentence shape, action, image, or
  salient nouns, and managed llama.cpp runs at low priority so CUDA prefill
  yields to Stop, the app shell, and backend-owned countdown presentation. A
  redundant one-section model response is losslessly promoted to one ordinary
  phrase instead of spending a second inference on repair. The active Gemma
  model passed the max-rate material/range-change scorecard repeatedly, the
  explicit-hold and four-line Hei speech-novelty cases, the full 9/9 interactive
  matrix, and five repeated one-section first-pass regressions. Full Go tests,
  vet, zero-issue lint, 408 frontend tests, localization, typecheck/build, and
  pure-Go builds pass; local race remains unavailable without a C compiler and
  CI is authoritative. A final isolated app completed a real chat turn in one
  976 ms generation on the exact model with fake transport and zero commands.
  Candidate size is 24,137,728 / 17,370,112 binary bytes; production browser
  output is unchanged. No post-change physical motion was commanded.

- **2026-08-22** - Prepared alpha.30 after reproducing two independent Creative
  regressions in compiled output: selected speeds 42–72 collapsed to the same
  109.8%/s mean travel, and effective stroke lengths remained too regular in
  short windows. Dynamic plans now fit each interval locally to the selected
  Handy calibration while retaining exact quintic velocity, acceleration,
  jerk, and reversal-gap checks. The retained fixture rises monotonically from
  103.8 to 167.5%/s mean travel and follows its requested peaks. Correlated
  `Breathe` drift, deterministic replacement seeds, and compact compiled-feel
  context add stroke diversity without random jitter or a forced-change timer.
  Three repeated exact-Autopilot Gemma sessions produced valid three-section
  phrases in 15/15 decisions; the least-varied compiled 12-second window still
  measured 9.5% stroke CV and 15.9 points of range, while a max-rate 180-second
  explicit hold remained a hold. Countdown presentation now reconciles to
  absolute backend deadlines, and session buildup is proven to exclude paused
  time. Full Go tests, vet, zero-issue lint, 407 frontend tests, localization,
  typecheck/build, the PowerShell 5.1 installer suite, and plain/stripped
  `CGO_ENABLED=0` builds pass locally. The isolated current-source app also
  completed one real Gemma chat turn in one call with `motion.action: none`, no
  repair/fallback, and no device credential. Candidate size is 24,109,568 /
  17,368,064 binary bytes; browser totals are 1,696,068 raw / 803,095 gzip
  overall and 810,662 / 214,068 for English startup. No post-change physical
  motion was commanded; matched hardware feel remains open.

- **2026-08-22** - Prepared alpha.29 after diagnosing the installed level-8
  Creative session. The scheduler made decisions about every 12 seconds inside
  its 8–16 second window, but six explicit model holds preserved one semantic
  phrase for roughly 92 seconds. Autopilot now exposes/traces phrase age,
  reconsiderations, hold streak, and current horizon; speed, horizon, and
  backend phrase seeds do not reset semantic freshness. The full envelope and
  section contract is available, but no threshold or section requirement forces
  change. The active Gemma model kept 3/3 short-age holds, changed 3/3 otherwise
  identical long-age/six-hold cases, and completed a 12-decision run with one
  hold, 11 semantic changes, 11 range envelopes, four horizons, and no repair or
  fallback. All probes were transport-free.
  Creative is now the fresh/pre-selector default while explicit Pattern Library
  and Off choices persist. Read-only catalog inspection found 87 built-ins, 81
  enabled and six disabled, with no disabled user content. Those six are retired;
  reconciliation deletes canonical built-in rows only, yielding 81 active
  patterns (25 non-experimental, 56 experimental). Full Go tests, vet, lint,
  405 frontend tests, localization, typecheck/build, the catalog designer, the
  PowerShell 5.1 installer suite, and plain/stripped `CGO_ENABLED=0` builds pass.
  Local Windows race remains unavailable because no C compiler is installed;
  mandatory Ubuntu CI remains authoritative. The candidate measures 24,067,584
  / 17,321,472 binary bytes. Browser totals remain byte-identical at 1,695,193
  raw / 802,815 gzip overall and 809,787 / 213,788 for English startup. No
  post-change device motion was commanded; matched physical feel remains open.

- **2026-08-22** - Prepared alpha.28 with an eighth Motion change rate stop.
  Levels 1–7 retain their exact windows and level 8 tightens only the upper
  bound to 8–16 seconds; the old fewer/more explanation is removed. The
  backend validator, scheduler window, Creative decision context, visible
  set-point scale, five locales, migrations, and tests all use the same 1–8
  contract. The session-buildup safety explanation now uses compact helper
  typography beside a standard accessible reset icon, reducing that action row
  from five lines to three at the shipped sidebar width. Review handoff now
  requires an available, loaded configured LLM and
  a real provider generation; LLM-related changes additionally require one
  text-only MagicHandy turn without repair, fallback, or motion. Full Go tests,
  vet, lint, 404 frontend tests, localization, typecheck/build, the PowerShell
  5.1 installer suite, and plain/stripped `CGO_ENABLED=0` builds pass. Local
  Windows race remains unavailable because no C compiler is installed; the
  mandatory Ubuntu CI gate remains authoritative. The candidate measures
  24,028,672 / 17,311,744 binary bytes and 1,695,193 raw / 802,815 level-9 gzip
  browser bytes. No device was connected or commanded.

- **2026-08-22** - Replaced Creative's C1 whole-leg reversal profile with a
  monotone C2 quintic-Hermite curve that shares velocity and acceleration at
  every knot. Exact polynomial acceleration and Creative-only perceptual jerk
  extrema lengthen abrupt plans through the existing runtime fitter; stored
  catalog and imported curves are unchanged. Added compact model-authored
  phrases of 2–4 complete semantic sections, expanded into one loop-closed
  engine curve with at least 60 seconds of maximum-reference travel,
  deterministic occurrence novelty, and a bounded current-seed refresh on
  replacement. Automated acceptance checks C2 knot/seam continuity,
  monotonicity, all Handy profiles, at least six whole-percent reversal lengths
  with coefficient of variation above 0.15, and Creative-only inverse-timing
  preservation through Cloud's 1% quantization. Pace-only copied fields can no
  longer collapse a section phrase; failed position replies must change
  geometry, and explicit pace requests must change speed in the requested
  direction. The local Ollama model passed the focused four-case section matrix
  without an engine or transport. Motion changes is now a backend-owned 1–7
  scale (90–240s through 8–24s, default level 4); legacy presets/custom windows
  migrate, and Creative Autopilot clamps against the effective scale window.
  Full Go tests, vet, lint, 404 frontend tests, localization, typecheck/build,
  the PowerShell 5.1 installer suite, and plain/stripped `CGO_ENABLED=0` builds
  pass. Local Windows race remains unavailable because no C compiler is
  installed; the mandatory Ubuntu CI gate remains authoritative.
  The candidate measures 24,013,312 bytes plain / 17,311,744 stripped, and the
  canonical browser build measures 1,695,258 raw / 802,914 level-9 gzip bytes.
  No post-change hardware command was issued, so matched physical-feel and Stop
  evidence remains open.

- **2026-08-20** - Separated Creative's remaining physical-feel defects into
  local stroke-length regularity and endpoint velocity shape. Wander and
  contrast now select correlated tight/middle/broad range levels at roughly
  two-cycle cadence; tests require every circular six-cycle window to explore
  at least 18% of the available band and prevent four-cycle plateaus below 8%.
  Center and grouped leg timing add independent seeded texture with a
  perceptual square-root response. Creative true reversals now use whole-leg
  shape-preserving PCHIP easing; stored patterns retain their shorter
  acceleration-budgeted guides, and both pass through the same exact runtime
  limiter, engine, sampler, transport, and Stop path. A buffered 1%-resolution
  regression retains gradual braking at the wire boundary. The latest
  installed trace contained 439 points/85 legs and exposed the old 31–35% span
  plateau across more than 30 reversals; no post-fix device command was issued.
  Position-correction validation now checks effective tip/base/full coverage,
  retains preceding context for terse follow-ups, and prevents a geometry-only
  request from rewriting unrelated axes. The installed managed model passed
  9/9 reproduced correction decisions and the broader 25/25 matrix without an
  engine or transport. Full Go tests, vet, lint, 402 frontend tests,
  localization, typecheck/build, PowerShell 5.1 installer tests, and plain/
  stripped `CGO_ENABLED=0` builds pass. Local Windows race remains unavailable
  because no C compiler is installed and remains a mandatory Linux CI gate.
  The binary is 23,933,952 / 17,241,088 bytes. No frontend source changed, and
  the canonical rebuilt browser payload remains 1,695,122 raw / 802,588 gzip
  bytes. Alpha.27 extends the version-bound reviewed unsigned policy without
  changing installer behavior or payload composition.

- **2026-08-20** - Added explicit Creative stroke-length envelopes without a
  second motion or transport path. The model selects outer geometry, a bounded
  span floor, and steady/breathe/wander/contrast; center/rhythm variation,
  speed, and decision horizon remain independent. Variable profiles compile to
  deterministic loop-closed phrases lasting at least about 30 seconds at the
  fastest supported normal Handy profile. Adaptive long-curve phase search and
  forward handoff scoring preserve position/direction continuity when profiles
  change. Parser/state validation prevents a claimed variable profile from
  silently normalizing steady. The installed managed 12B Gemma model passed
  16/16 repeated envelope cases and the broader 9/9 Creative matrix on the
  first response, with zero repairs/fallbacks and no engine or transport. Full
  Go tests, vet, lint, 402 frontend tests, localization, typecheck/build, and
  `CGO_ENABLED=0` plain/stripped builds pass. The PowerShell 5.1 installer suite
  and a complete local setup/portable/checksum build plus
  `ReviewedUnsignedPublic` payload verification also pass; exact clean-runner
  lifecycle, race (local Windows has no `gcc`), and Defender scanning remain CI
  gates. The binary is
  23,894,528 / 17,212,416 bytes, and the
  embedded browser payload is 1,695,122 raw / 802,588 gzip bytes (+933 / +417
  against alpha.25); English startup is 809,933 / 213,646 (+452 / +149). No
  hardware connection or motion command was issued, so physical feel and the
  Ollama schema matrix remain open.

- **2026-08-20** - Reproduced the installed alpha.24 exit as a Creative-plan
  panic: full-span geometry at 96% variation rounded one endpoint to
  `100.00000000000001`; strict validation rejected it, the plan constructor
  discarded that error, and sampling indexed an empty curve. Alpha.25 clamps
  the final projection, retains compilation errors, rejects them across every
  engine admission/retarget path before transport work, and makes empty-curve
  sampling stationary rather than process-fatal. All 101 variation values now
  compile and sample across all three Handy profiles. Full Go tests, vet, lint,
  402 frontend tests, typecheck/build, the Windows PowerShell 5.1 installer
  suite, and plain/stripped `CGO_ENABLED=0` builds pass. The binary changed from
  23,847,936 / 17,176,064 bytes to 23,859,712 / 17,184,256 bytes, remaining
  below budget. No hardware connection or motion command was issued.

- **2026-08-20** - Recalibrated shared loop and optional media-limit speed
  semantics after qualitative hardware feedback found Dynamic robotic and 73%
  materially slow. The former 100%=180% travel/s reference made 73% only
  131.4%/s. The new affine 1–100 control uses a Connection-menu Original /
  Handy 2 Standard / Handy 2 Pro calibration based on the published 110/125 mm
  travel and 32–400/450 mm/s normal envelopes; 73% on Original now requests
  272.4%/s (299.6 mm/s). Catalog authoring retains its 3000%/s² and 450 ms quality gates while
  runtime playback evaluates an exact rendered-curve 7500%/s² and 100 ms
  envelope. Reversal ramps are bounded in played time, fractional phase removes
  authored-millisecond plateaus, and Dynamic variation combines a route-sized
  eight-second-or-longer harmonic wander with bounded timing breathing. Chat's
  ordered cadence/style controls are visible set-point sliders; categorical
  motion/authority modes are segmented radio choices, with queued saves
  preserving the last rapid edit. Handy publishes no per-model acceleration
  limits, so the shared runtime ceiling remains conservative and the Pro's
  optional 800 mm/s overclock is excluded. The full Go suite, vet, lint, 402
  frontend tests, typecheck/build, and plain/stripped `CGO_ENABLED=0` builds
  pass. Local `go test -race` remains unavailable because `gcc` is absent; CI
  retains that mandatory gate. Browser and binary measurements are recorded in
  the current snapshot. The initiating report had no transport/latency/trace evidence and no
  post-change hardware command was issued, so R1 and Dynamic default acceptance
  remain open. See `docs/motion-calibration.md`.

- **2026-08-20** - Added selectable Dynamic / Pattern Library / Off LLM motion
  modes. Dynamic emits bounded semantic geometry and slow loop-closed variation
  into the existing engine; tests cover strict prompt separation, parser bounds,
  model-owned no-op decisions, interior-anchor pass-through velocity, reversal
  dwell, velocity-aware retargeting, one shared transport Play, Autopilot
  mapping, persistence, immediate Chat-sidebar selection, and status rendering.
  Follow-up review removed the selector from global navigation and made buildup
  duration authoritative: model planning turns can react to elapsed progress but
  can no longer accelerate or rewind the bar.
  Pattern remains the default until live-provider and capped hardware A/B
  acceptance. Full Go and 398-test frontend suites, vet, lint, localization,
  typecheck, production build, and pure-Go build pass. The local Windows race
  run remains unavailable because the installed LLVM toolchain lacks a MinGW
  UCRT sysroot; the Ubuntu CI race gate remains authoritative.

- **2026-08-20** - Ran the first retained transport-free Dynamic live matrix
  through packaged b9966 CUDA llama.cpp and the installed 26B Gemma GGUF. All
  nine start/hold/update/Stop cases passed without repair after bounded anchor
  precedence normalized the model's redundant anchor-plus-window output. Moving
  the volatile motion snapshot near the final output guard increased reusable
  prompt prefix coverage to roughly 74-97%; after the initial turn, first deltas
  arrived in 109-286 ms and complete responses in 366 ms-1.161 s. GPU graph
  warm-up also contributed to the end-to-end gain. The harness instantiated no
  engine or transport and issued no device command. Real-device Dynamic A/B
  acceptance remains open.

- **2026-08-20** - Moved managed llama.cpp's first-generation GPU setup into
  asynchronous startup preload. A bounded 16-token non-thinking warmup uses
  autonomous scheduler priority, so interactive Chat cancels it rather than
  waiting behind it. On packaged b9966 CUDA with the installed 26B Gemma and a
  32K context, the pre-change first UI turn was 37,372 ms to first token /
  37,526 ms total; after preload warmup the first fresh turn was 4,961 /
  5,087 ms and the next was 183 / 309 ms. The HTTP UI remained available during
  the 73,747 ms background load-plus-warmup. Both isolated turns selected no
  motion and the review data contained no connection key.

- **2026-08-14** - Closed the normalized-pattern dwell regression found in a
  retained Cloud trace. `Hard and Regular` had no HSP timestamp gap, but its
  short 100 -> 74 accent inherited nearly a full-stroke duration and remained
  below perceptible velocity for 950 ms at 40%; `Deep-Partial Sequence` exposed
  the same defect for 2.86 seconds, and a cyclic check found 360 ms split across
  the `Tease` seam. Velocity-balanced leg timing reduces all three planner
  intervals to 50 ms or less without changing turning positions, total travel,
  controller ownership, or the transport contract. A
  default-catalog 40% continuity gate and a separate 1%-resolution Cloud frame
  regression test cover the planner and emitted path. The observed Cloud run
  used 35-40%, 322-379 ms request latency, continuous accepted coverage until a
  later network error, and a successful explicit Stop; post-fix physical feel
  remains open.

- **2026-08-13** - Separated pattern identity from playback pace. Before this
  correction, built-in authored rates ranged from 57.6 to 436.4% travel/s and
  the complete 1–100 control changed any selected loop by less than 2x, so the
  chosen pattern dominated the requested speed. The shared planner now measures
  total loop travel and targets `180 * speed_percent / 100` semantic
  percentage-points/s, with curve-specific acceleration and reversal floors.
  This was the 2026-08-13 intermediate calibration and was superseded by the
  2026-08-20 minimum-to-maximum reference curve above.
  Tests cover equal requested rate across unlike patterns, proportional
  20/40/80 pacing, focus compression/expansion, soft anchors, loop-seam
  reversals, retained knots, and every built-in at 100%.
  Programs use direct clock scaling; video remains media-clock-locked.
  The 87 built-ins now carry geometry/relative-rhythm metadata: 31 are visible
  to the model by default and 56 are experimental. Stable database IDs remain
  for persistence, while deterministic opaque handles prevent historical
  pace-biased IDs from entering prompts. Untouched legacy names migrate to the
  new labels without overwriting user renames. The model contract and Pattern
  Library UI use `speed_percent`; old `intensity` input remains compatibility-only
  and is normalized immediately.
  Installed Gemma/llama.cpp live evaluation returned strict JSON on 10/10 Chat
  turns with no schema/range violations, then completed 12/12 full-catalog
  Autopilot decisions with zero errors, nine distinct speeds (65–85), and six
  resolved patterns. Neither run entered the engine or transport.
  Full Go tests, vet, zero-issue lint, localization, typecheck, production build,
  all 395 frontend tests, catalog tooling, and plain/stripped zero-CGo builds
  pass. Local race remains unavailable because MSYS2 has no compiler; CI retains
  the mandatory race gate. The core is 23,768,576 / 17,088,000 bytes
  plain/stripped. Rebuilding the canonical UI changes the checked-in payload by
  -16 raw / +1 gzip byte to 1,684,034 / 799,562. No hardware command was issued.

- **2026-08-03** - Rejected GGUFs that a text-only managed llama.cpp runner
  cannot load even when Ollama stores them as one model layer. Import and
  inventory parse at most 64 MiB of GGUF metadata, reject split shards and
  embedded audio, vision, or projector components, cache unchanged inventory
  results, and expose existing incompatible copies as `unsupported` instead of
  `ready`. The parser accepted the installed text-only 12B Gemma and rejected
  the reported fused Gemma 4 in 40 ms total. During malformed-output repair,
  Chat now keeps an already-visible first-pass reply stable until the final
  backend message instead of visibly starting the reply again. Plain/stripped
  `CGO_ENABLED=0` binaries are 23,712,768 / 17,049,600 bytes. The complete
  browser payload is 1,677,300 raw / 797,770 level-9 gzip; HTML/CSS/JS is
  1,233,064 / 360,373 and English startup is 796,342 / 210,540 (+448 / +212
  overall and +245 / +121 at startup). Alpha.12's portable-only Release was
  withdrawn without moving its tag; alpha.13 restores the explicit reviewed
  setup/ZIP/checksum path and normal over-install guidance. A local alpha.13
  setup/ZIP/checksum build passed `ReviewedUnsignedPublic` provenance, PE,
  unsigned-status, manifest, and outer-hash verification; Microsoft Defender
  scanned that exact directory with no threats.
  Full Go tests, vet, zero-issue lint, 387 frontend tests, localization,
  typecheck/build, installer policy tests, and plain/stripped zero-CGo builds
  pass. No hardware connection or motion command was used.

- **2026-08-03** - Restored update discovery for prerelease builds and corrected
  clean-machine Faster Qwen model finalization. The backend now scans a bounded
  GitHub release list, ignores drafts and invalid or mismatched tags, and chooses
  the highest release allowed by stable/alpha/beta/RC progression instead of
  calling the stable-only `/releases/latest` endpoint. A current core built with
  alpha.9 metadata queried the live public API and returned alpha.10 as
  available. Faster Qwen now materializes ordinary app-owned model files while
  retaining completed files and resumable local metadata; complete alpha.10 snapshots remain
  compatible. Focused Go tests and the full PowerShell installer suite pass. The
  local Go 1.26.4 candidate core is 23,660,032 bytes plain / 17,031,168
  release-style stripped. The complete
  browser payload is 1,676,852 raw / 797,558 gzip; HTML/CSS/JS is 1,232,616 /
  360,161 and English startup is 796,097 / 210,419. No hardware connection or
  motion command was used.

- **2026-08-03** - Corrected managed-runtime readiness after fresh packaged
  installs. Official and former source-built llama.cpp paths use the same
  pinned `b9966` / `c749cb0` source and launch flags; the packaged runner was
  healthy but exposed its structured HTTP 503 `Loading model` state while a
  cold GGUF was still loading. The provider now distinguishes that bounded
  state from other 503 failures, managed readiness allows 90 seconds, and the
  Model screen polls instead of reporting a dead health endpoint. Current
  provider code reached the installed verified CUDA runner and received a
  valid Gemma response in 468 ms. The full current-app chat SSE path then
  returned `PACKAGED LLAMA READY` in 867 ms with a 714 ms first token and no
  motion selection. Fresh Faster Qwen setup now validates the
  exact Python/server/adapter/model contract and clears stale worker paths,
  while installer probes and runtime launches share an app-owned Numba cache.
  Managed Python servers use kill-on-close process-tree containment so stopping
  the worker also stops launcher descendants. A current-branch CUDA run became
  ready in under 30 seconds, completed the app queue with a finalized 99,884-byte
  WAV, then left zero Qwen Python processes and no port 8991 listener after
  Stop. Full Go tests, vet, zero-issue lint, 383 frontend tests, typecheck/build,
  zero-CGo builds, and the installer suite pass. Local race remains unavailable
  because MSYS2 has no compiler package; CI retains the mandatory race gate.
  The complete browser payload is 1,676,766 raw / 797,560 gzip; HTML/CSS/JS is
  1,232,530 / 360,163 and English startup is 796,073 / 210,421 (+465 / +75 for
  all three). No hardware connection or motion command was used.

- **2026-08-03** - Microsoft completed false-positive submission
  `15c1e36d-fb35-4c5d-85de-83707169818a` with final determination
  `Not malware` and removed the alpha.6 Defender detection. Alpha.8 restores
  setup publication through a new `ReviewedUnsignedPublic` policy bound to that
  case. The tag workflow builds one public setup/ZIP/checksum set and exercises
  the exact setup through custom and Program Files install, over-install,
  retention, purge, uninstall, and clean reinstall before publishing three
  explicit paths. Pull-request artifacts remain CI-only, and trusted signing
  remains open R28 exit evidence. No app, motion, transport, model, or voice
  behavior changed.

- **2026-08-02** - Hardened the CI-only Inno setup after a controlled
  same-payload comparison identified a 32-bit bootstrap around the amd64 app and
  a solid `lzma2/ultra64` overlay as avoidable matches for alpha.6's structural
  heuristic tags. The replacement uses a native x64 loader and non-solid
  `zip/9`, and release acceptance now reads every EXE's PE machine field. The
  approximately 17.9 MB local candidate passed the isolated install lifecycle
  and a current Defender custom scan with no threats. It remains unsigned and
  cannot enter a public release without valid timestamped Authenticode.

- **2026-08-02** - Withdrew the `v0.1.0-alpha.6` GitHub Release after Microsoft
  Defender classified its unsigned Inno Setup executable as
  `Behavior:Win32/DefenseEvasion.A!ml` and blocked launch with ShellExecuteEx
  error 225. The reported file matched the published SHA-256 and passed the
  tagged release gates; that provenance did not justify bypassing a severe
  endpoint-security result. The exact file was submitted to Microsoft under
  case `15c1e36d-fb35-4c5d-85de-83707169818a`. ADR 0014 now blocks unsigned
  setup files from public releases, isolates them as seven-day CI lifecycle
  evidence, publishes an independently verified portable-only artifact set,
  and reserves setup publication for valid timestamped Authenticode on every
  executable. No core, device, motion, model, or voice behavior changed.

- **2026-08-02** - Corrected the remaining managed-Python launcher boundary for
  `v0.1.0-alpha.6`. Alpha.5 recovered the patch-specific interpreter after
  Windows rejected uv's optional minor-version junction, but `uv venv` then
  wrote that missing minor alias into `pyvenv.cfg` and installed a 46 KiB uv
  trampoline. The first launch therefore failed with `uv trampoline failed to
  spawn Python child process`. Setup now runs the validated interpreter's
  standard-library `venv --without-pip`, which writes the exact patch path and
  standard 256 KiB Windows launcher; uv remains the package installer. Retry
  probes now safely replace alpha.5's broken environment. Native dependency
  commands also treat exit status as authoritative even when a successful tool
  writes diagnostics to stderr. Real Python 3.11.15 and 3.10.20 acceptance
  produced 262,144-byte and 260,608-byte standard launchers, exact patch homes,
  clean uv package checks, and successful reinstall reuse. Current dependency
  dry runs resolve 97 Faster Qwen packages, 125 base Chatterbox packages, and
  the three pinned Chatterbox engine repairs without changing either
  environment. No frontend, core, hardware, or motion behavior changed.

- **2026-08-02** - Corrected the managed-Python boundary for
  `v0.1.0-alpha.5` after a fresh Windows profile rejected uv's internal
  minor-version junction with `ERROR_UNTRUSTED_MOUNT_POINT`. Python and uv's
  cache now live inside the selected TTS provider, with global executable links
  and registry registration disabled. Setup locates and version-probes the
  patch-specific interpreter directly, so a junction-only failure after a
  complete extraction can continue while incomplete extraction remains fatal.
  uv credential lock files are app-owned too, avoiding a second restricted
  `%APPDATA%\uv` boundary, and Faster Qwen now probes the NVIDIA driver before
  large downloads. Recovery fixtures cover the nonzero uv exit, unrelated-error
  refusal, interrupted venv replacement, exact interpreter handoff,
  process-scoped ownership settings, driver probing, and compatible venv reuse.
  Real isolated Python 3.11 and 3.10 installs, venv creation, and current pinned
  dependency resolution also passed. No frontend bundle, core behavior,
  hardware connection, or motion changed.

- **2026-08-02** - Audited every executable and native boundary used by managed
  voice setup for `v0.1.0-alpha.4`. The installer now probes `uv.exe`, bypasses
  an unusable WinGet portable alias through the real package directory, probes
  Git and exposes it to `uv` child processes, and packages validated dependency
  constraints with the Windows payload. Clean Python 3.11 and 3.10 resolutions
  covered 97 Faster Qwen packages and 125 Chatterbox packages respectively.
  The exact pinned Qwen, Chatterbox server, and Chatterbox engine sources were
  reviewed for external executables and native builds: managed WAV paths need
  neither FFmpeg nor native SoX, and Chatterbox's one source distribution is a
  pure-Python antlr runtime. Chatterbox now mirrors its upstream ONNX/protobuf
  repair, constrains engine-sensitive NumPy/safetensors versions, and verifies
  its actual Turbo/audio/native imports. Faster Qwen retains `pip check`; both
  providers verify the Hugging Face launcher and selected PyTorch backend before
  model download. No frontend bundle, core memory path, hardware connection, or
  motion behavior changed.

- **2026-08-02** - Corrected the managed TTS fresh-install and retry path for
  `v0.1.0-alpha.3`. A new source clone is staged atomically and its expected
  no-checkout deleted-file status is no longer mistaken for a user edit. Older
  empty alpha.2 worktrees recover in place, while populated dirty worktrees,
  untracked files, and wrong remotes remain protected. Local Git fixtures cover
  fresh clone, exact alpha.2 residue, clean completion, and tracked-edit
  preservation. The setup Model Library now fits the routed viewport, keeps its
  header and action footer fixed, scrolls one bounded body, and separates model
  selection, GGUF import, and Ollama import into labeled tool regions. A live
  scan of 16 local Ollama models stayed within a 300 px candidate list with no
  document-level overflow. This UI pass adds 1,719 raw / 256 level-9 gzip bytes:
  the complete browser payload is 1,676,301 / 797,485, HTML/CSS/JS is 1,232,065 /
  360,088, and the English startup path is 795,608 / 210,346. Installer and 382
  frontend tests, typecheck, build, all non-race Go tests, vet, lint, and the
  zero-CGo build pass. Local race execution remains unavailable because MSYS2
  has no MinGW compiler package; CI and the tag workflow retain the race gate.
  No hardware connection or motion command was used.

- **2026-08-02** - Replaced managed llama.cpp source compilation with official
  `b9966` Windows CPU and CUDA 12.4 archives pinned by exact size and SHA-256.
  The installer resumes partial HTTPS downloads, rejects unsafe ZIP paths,
  stages atomically, records provenance and the upstream MIT license, probes
  commit `c749cb0`, and requires CUDA device detection before activation. It
  accepts valid legacy source-built manifests but no longer provisions Git,
  CMake, MSYS2, Visual Studio, or CUDA Toolkit for this path. Real-network CPU
  and CUDA installs passed; the CUDA runner detected an RTX 5070 Ti. No model
  was loaded and no hardware motion was issued.

- **2026-08-02** - Added the unsigned Windows x64 distribution path and
  read-only update discovery. One clean-source-enforcing builder produces a
  manifest-verified portable ZIP and Inno setup EXE from the same pure-Go core,
  workers, GPL/source notices, and optional provisioning helpers. The PR
  workflow has read-only permissions; a separate tag workflow publishes only
  an exact current `main` commit after repeating quality and install-lifecycle
  gates. In-app
  setup now owns normal device, LLM, model, ASR, and TTS choices while the plain
  source installer builds the core and opens that wizard. Versioned builds use
  a bounded backend client to query the canonical latest stable GitHub Release,
  cache for six hours, throttle failed automatic retries for 15 minutes,
  revalidate with ETags, compare semantic versions, and produce a deduplicated
  opt-out notification without downloading or executing anything. Local
  first-alpha acceptance verifies every manifest entry and both outer hashes,
  custom and Program Files installs, destination and shortcut choices, ARP
  metadata, active-process over-install, Japanese settings retention, graceful
  shutdown, explicit data retention, bounded default-data purge, and clean
  reinstall. The candidate artifacts are approximately 15.6 MB portable and
  9.3 MB setup. The core measures 23,602,688 / 16,984,576 bytes
  plain/stripped; the embedded UI is 1,646,533
  raw / 788,728 level-9 gzip bytes. Full Go and 376-test frontend suites, vet,
  lint (zero issues), zero-CGo build, PowerShell integration/recovery tests,
  production frontend build, and production-only npm audit pass. The local
  race run remains unavailable because this MSYS2 installation has `gcc-libs`
  but no GCC/Clang compiler package; CI and the tag workflow retain the race
  gate. No hardware command was issued.

- **2026-08-01** - Fixed managed Faster Qwen stopping after ordinary seed or
  tone saves. Those controls now travel with each speech request, preserving
  the resident model and reference cache; actual provider, model, device, port,
  or reference changes reconfigure and asynchronously restore persisted
  auto-launch roles. Health checks invalidate stale readiness when an owned
  Python server exits, and explicit Start retries the child launch and model
  load. A one-frame full streaming warm-up reduced local RTX 5070 Ti cold
  readiness from 14.82 to 13.06 seconds while preserving about 0.40 seconds to
  first audio. A live settings save completed in 7 ms without changing the
  worker start timestamp; the next full-path request returned 3.6 seconds of
  valid audio in 1.52 seconds and passed a Parakeet intelligibility check.
  A managed-child failure now renders as Not ready with a bounded diagnostic
  and the existing Load model recovery action instead of a green Running
  readout. The complete embedded browser payload is 1,562,162 raw / 763,714
  level-9 gzip bytes (+126 / -649).
  Plain/stripped `CGO_ENABLED=0` binaries are 23,296,000 / 16,720,896 bytes
  (+10,240 / +8,704), still below budget. No hardware command was issued.

- **2026-08-01** - Removed four independent sources of local-model tail
  latency without changing prompts, sampling, parsing, or motion semantics.
  Managed llama.cpp now preloads asynchronously by default, with an explicit
  on-demand memory-saving option. One coordinator serializes generation,
  prioritizes Chat over queued Autopilot work, and cancels an in-flight
  autonomous inference before admitting an interactive turn. A new speech
  policy either interrupts pending TTS (default) or lets it finish, and Faster
  Qwen now bounds abandoned audio buffering and closes its model generator.
  Windows managed runners use kill-on-close Job Object containment. An exact
  executable-path duplicate blocks another launch and opens a confirmation
  dialog; the controller-gated termination endpoint revalidates the PID's path
  immediately before stopping it. Eight isolated complete Chat-route turns
  against the installed Gemma/llama.cpp endpoint had one cache-fill turn at
  1,836 ms total / 1,537 ms first token, followed by seven turns at 379-614 ms
  total / 164-441 ms first token (416 / 175 ms medians), all with one provider
  call and no repair. Current browser payload is 1,562,036 raw / 764,363 gzip
  bytes (+21,116 / +6,163); plain/stripped binaries are 23,285,760 /
  16,712,192 bytes. Three fresh stripped launches reached health in
  92.9-136.7 ms. Go, frontend, localization, installer, Python syntax, lint,
  vet, and zero-CGo gates pass; local race remains unavailable without GCC and
  the CI race gate remains authoritative. No hardware command was issued.

- **2026-08-01** - Curated the imported funscript clips against the same speed
  envelope the designed catalog already answers to, after a hardware report that
  nearly all of them stuttered. Measured, 95 of 171 (56%) held a contiguous span
  under 30%/s longer than 200ms, against 0 of 28 for the designed set. The
  failures tracked the intensity band the earlier relabelling had assigned,
  monotonically: 100% of Gentle contained a dead stroke against 23% of Intense,
  because that pass had labelled by *mean* stroke speed, which averages motion
  together with stillness -- a clip that was mostly stopped scored a low mean and
  came out labelled in the band that reads as most inviting. Excision now removes
  stillness without touching geometry (strokes too small to feel are merged,
  strokes slower than the floor are shortened to it, bounded by the reversal gap
  and the acceleration budget), and whatever still fails is dropped. Chord speed
  proved an insufficient proxy -- monotone-Hermite smoothing drives velocity
  toward zero at every turning point, so 62 of 153 clips that passed on chords
  still stalled once rendered -- so the decision is made in Go against the real
  curve and the runtime's own `exceedsCatalogSafetyBudgets`.
  A first pass kept 90 and was still wrong, reported from hardware as
  micro-stalling: it gated at 30%/s, which is "has it stopped" rather than the
  45%/s speed floor, so a clip crawling at 35%/s passed -- one spent 76% of its
  cycle there. Two further corrections came out of that. The gate now uses the
  floor itself, and it measures the **longest contiguous dip** rather than total
  time below it: total time does not discriminate at all (median 8% for designed
  patterns against 7% for imports) because every reversal dips, while the longest
  dip separates them completely, 45-50ms for every designed pattern against 420ms
  for the reported clip and 3,490ms for the worst. And what is authored is not
  what loads -- `NormalizePatternDefinition` resampled one clip from 9,109ms and
  32 clean points to 10,470ms and 21 fractional ones, a 1.15x stretch that turned
  a 42%/s floor into 36.6%/s -- so validation reads `BuiltinPatternDefinitions()`
  rather than its own arithmetic. 59 of 171 survive; the catalog is 87 patterns
  and the composed prompt falls to **17,589 B (~4,397 tokens)**, against ~15,949
  before this line of work began.
  `docs/pattern-quality.md` is the plain-language guideline for both failure
  modes, the geometry that constrains them, and which script to reach for. Three
  tests that pinned curated ids or a fixed built-in count now resolve them, so a
  future re-curation cannot fail unrelated tests. gofmt, go vet, golangci-lint
  (0 issues), full Go tests and the CGO_ENABLED=0 build pass.

- **2026-08-01** - Restored model ownership of embodied chat motion and closed
  two follow-on defects in the bulk pattern import. Read-only inspection of the
  active session showed `Fuck me` produced a model-authored start, while `Suck
  me` and `kiss it` produced no motion and never entered the engine. The prompt
  and positive permission matcher now understand those embodied requests, but
  the stopped-state omitted-motion fallback was removed: the model chooses
  `start`, `target`, or no action, while negation/meta-conversation, capability,
  state, speed/range, engine, Stop-epoch, and transport gates remain
  deterministic. Ordinary chat retains nonzero-temperature/top-p sampling, so
  the choice can vary without introducing a backend coin flip. Two isolated
  live runs against the installed Gemma/llama.cpp model selected starts for all
  three inputs without an engine or transport.
  The live evaluator accepts the managed runner's backend-reported fallback
  port instead of assuming 8080. Separately, all 171 Rockfire files remain
  active through a synchronized, test-enforced v3 manifest and ordinary catalog
  controls: 165 unsafe source curves are resampled, low-prominence reversals are
  removed, and 170 are visibly experimental while `Easy Drive 4` remains
  normally labeled. No `curated-*` filename receives an exact-timing exemption.
  The relabel tool is repository-relative, Python generation is offline-only,
  and the Go bulk import requires explicit experimental acknowledgment.
  Plain/stripped zero-CGo binaries are 23,496,192 / 16,949,248 bytes
  (+545,792 / +534,528 from the quarantine tree). A fresh isolated launch
  listened in 107.0 ms and returned `/healthz` in 139.1 ms. The Pattern Library
  again exposes `experimental` as a visible/filterable review status instead of
  hiding it with implementation-only tags; the canonical web
  build is 1,535,379 raw / 756,133 gzip bytes (-15 / -11). Full Go tests, vet,
  golangci-lint, all 357 frontend tests,
  typecheck, production build, script checks, and live Gemma evaluation pass.
  Local race execution remains unavailable because `gcc` is absent; CI retains
  the mandatory race gate. No hardware command was issued.
- **2026-08-01** - Made managed llama.cpp context allocation explicit and
  quarantined a pattern-catalog safety regression. Settings > Model now offers
  backend-reviewed 16,384/32,768/65,536/131,072-token values only for the
  app-owned runner, persists 32,768 by default, invalidates a stale managed
  provider when changed, and passes the value as `--ctx-size`; external
  llama.cpp and Ollama keep ownership of their context. The UI and docs state
  the RAM/VRAM and prompt-fit tradeoff without claiming the unmeasured larger
  values are faster. A read-only `motion_trace.v3` investigation of a reported
  35% speed-limit failure found the semantic target correctly clamped at 35%,
  while selected `curated-fast-drive-20` contained 40 ms reversals that a broad
  imported-curve test exemption had allowed into the enabled catalog. All 171
  bulk generated clips are removed from active built-ins and prior builtin rows
  are purged on reconciliation; their source remains available for review, and
  user-owned imports are preserved. Exact timing exemptions now require either
  `hard-and-regular` or `playful-jerk` plus the `curated` tag. Accepted startup
  calibration excursions retain the raw observed position for the skip test, so
  clamping cannot bypass verified acquisition. No hardware command was issued;
  capped post-fix Cloud validation remains open under R1. The canonical browser
  build is 1,535,394 raw / 756,144 gzip bytes (+2,749 / +874), with the startup
  path +1,246 / +284 bytes. Full Go tests, vet, golangci-lint, the zero-CGo
  build, live-tag compilation, all 357 frontend tests, typecheck, localization,
  and production build pass. Local race testing remains unavailable because the
  Windows host has no C compiler; CI retains the mandatory race gate.
- **2026-08-01** - Recovered a real-device startup failure and removed a managed
  LLM allocation mismatch. The Handy reported slide 0-100% as absolute
  5.00-102.83 while parked at 4.00, placing it 1.02% below its own calibrated
  minimum and just outside the shared 1% tolerance. Calibration sanity now has
  a separate 3% bound while commanded lead-in arrival remains at 1%. A Cloud
  REST check started Stroke at 5%, remained running 3.153 seconds later, then
  stopped successfully. The `motion_trace.v3` export contained 18 rows with no
  drops; relevant Cloud calls were 302-317 ms and preflight Stop through Play
  spanned 1.95 seconds. Sampling, sanitization, arrival verification, dispatch,
  and Stop semantics were intentionally unchanged. Separately, two dead-parent
  llama-server processes left by force-killed app instances were removed before
  an isolated same-model benchmark. The model-default 262k/four-slot server took
  19.512 seconds for a 6,842-token prompt and 47-token response; a 32k/single-slot
  server took 2.204 seconds in the same single-request probe. Managed mode now
  pins that bounded launch shape, while GPU offload, Flash Attention, batching,
  and prompt caching stay automatic; repeated and simultaneous-TTS trials remain
  open.
- **2026-07-31** - Cut chat latency and made personas actually change the voice.
  A report that replies had slowed traced to the curated funscript import: it
  added 171 built-ins, all seeded enabled, taking the composed system prompt from
  roughly 1,738 tokens to **15,949**, of which the pattern catalog was **93.8%**.
  Neither suspected cause contributed - the replacement pattern names average
  12.9 B, and the persona sections are absent entirely when no lore is set. The
  imported labels were also unusable for selection: every part of a source script
  shared one name, description and tag set ("... Part 1/7" through "Part 7/7")
  while the parts differ in measured intensity by 2.04x at the median and up to
  12.45x, so the model saw seven indistinguishable options that feel nothing
  alike. Span is 100% of range at every percentile, so the relabelling carries
  pace and rhythm rather than depth: five intensity bands measured across the
  whole catalog (Gentle/Easy/Steady/Fast/Intense, 10-800 %/s) crossed with a
  rhythm word from stroke-speed variance, ordered by intensity within each band.
  That removed 26,385 B of catalog payload. The catalog encoding then changed from
  an array of JSON objects to one delimited line per pattern, because five
  repeated keys cost about 62 B per entry against roughly 65 B of actual label --
  at two hundred patterns the punctuation cost as much as the content. Weight is
  emitted only once feedback has moved it off the default, since the preference
  rule cannot apply while every entry is equal. Two remaining duplicates went
  after that: the fifteen velocity-authored descriptions were carrying padding
  around their shape and their "when to reach for it" signal (1,689 -> 992 B
  authored), and only the leading two tags per pattern now reach the model, since
  tags were the largest single item left at 5.8 KB and the tail of each list
  earns none of it -- an imported clip carries its source's role rather than
  anything about its motion, repeated identically across every part of a script,
  so it cannot separate the entries the model must choose between. The library
  keeps the full tag list, so UI filtering is unchanged. Catalog 32,144 ->
  **16,118 B**; live composed prompt **40,706 -> 24,680 B (~6,170 tokens)**,
  against ~15,949 tokens before any of this work, verified through
  `/api/diagnostics/prompt-composition`. Escaping that `json.Marshal` used to
  provide is now this package's own job, so `promptTableField` collapses
  whitespace and strips the delimiter and
  `TestCatalogRowsCannotBeForgedByPatternLabels` asserts a hostile pattern name
  cannot forge a row. A weight-ranked cap on catalog size was prototyped and
  backed out: with every weight at the default it pruned by name, dropping
  hand-designed patterns in favour of bulk imports.
  Personas changed on three prompt seams. The description arrived as a bare
  labelled fact under a profile that said to use it "only for identity and reply
  wording", so a character sheet read as trivia; it now states what it is and asks
  the model to play it, with the injection guard kept as a separate, unweakened
  sentence. Lore now says it is background fact rather than a manner to imitate,
  naming the description as the source of manner. Repetition of one pet name
  traced to the anti-repetition rule listing sentence structure, key nouns and
  sensation focus - a term of address is none of those, so nothing discouraged it
  while three recent lines containing it read as an established habit; terms of
  address are now named. All five prompt locales match, and the editor states that
  the model reads the description every reply and that lore is separate. gofmt,
  vet, golangci-lint (0 issues), full Go tests, typecheck, 354 frontend tests, and
  the 1,330-key x 5-locale audit pass.

- **2026-07-31** - Stabilized managed Faster Qwen output after a real 2 MiB
  retained-audio rejection and highly variable speech. The core now retains up
  to 8 MiB per playable clip and nine clips (72 MiB worst case), while the
  managed server defaults to fixed seed `1337`, performs one short hidden
  streaming warm-up, and applies a text-proportional generation ceiling.
  Fixed-seed first and second visible requests produced identical 334,124-byte
  WAVs after a 12.3-second cold model start. Before the ceiling, a four-word
  varied-seed request ran to 6,858,284 bytes (about 143 seconds); two equivalent
  final varied requests completed in 1.07/1.24 seconds as 103,724/134,444-byte
  WAVs (2.16/2.80 seconds). The reported reference is valid 48 kHz mono PCM but
  lasts 19.702 seconds with 6.668 seconds of detected silence across 15 gaps;
  the GUI and docs now recommend a clean exact-transcript 3-to-10-second
  excerpt. Go vet, changed-package tests, Python compilation, all 352 frontend
  tests, localization/typecheck/build, and pure-Go app/worker builds pass. The
  full local Go run passes outside four sandbox-blocked Ollama symlink tests;
  local race remains unavailable without `gcc`, and CI is authoritative for
  both. Plain/stripped binaries are 22,945,792 / 16,410,112 bytes (+9,728 /
  +6,144). English startup is 722,734 / 192,425 raw/gzip (+2,266 / +385); all
  HTML/CSS/JS and complete output are 1,083,326 / 314,668 and 1,527,562 /
  752,095 (+4,996 / +559 and +4,996 / +589).

- **2026-07-31** - Made Faster Qwen reference setup GUI-owned. The command-line
  installer no longer prompts for or rejects an empty reference WAV/transcript,
  its completion output directs users to Settings > Voice, and module-state v2
  excludes those app settings while remaining compatible with v1. Backend
  status distinguishes installed runtime files from reference readiness, and a
  same-provider module update cannot erase GUI-saved reference values. Full Go
  tests, vet, lint (zero issues), the pure-Go build, all 351 frontend tests,
  localization/typecheck/build, and the Windows installer integration suite
  pass. Local race execution remains unavailable because `gcc` is absent; the
  Ubuntu CI gate remains authoritative. Plain/stripped binaries are
  22,936,064 / 16,403,968 bytes (-512 / unchanged). English startup is
  720,468 / 192,040 raw/gzip (+216 / +46); all HTML/CSS/JS and complete output
  are 1,078,330 / 314,109 and 1,522,566 / 751,506 (+1,174 / +363).

- **2026-07-31** - Integrated managed Faster Qwen3-TTS and Chatterbox choices
  into the localized source-installer decision tree. A PowerShell-only
  bootstrap now repairs WinGet and installs Git before cloning, while selected
  TTS setup provisions uv, managed Python 3.11 (Faster Qwen) or 3.10
  (Chatterbox), PyTorch, and the model without a preinstalled compiler.
  Installer-state schema 3 persists the module, CPU/CUDA, and auto-launch
  choices; ordinary updates validate and reuse installed multi-gigabyte assets.
  The Windows installer integration suite passes. These script and
  documentation changes do not alter the embedded browser payload or Go
  binary.

- **2026-07-31** - Retired NeuTTS after repeated quality acceptance failures
  and replaced it with one bounded OpenAI-compatible TTS adapter, scripted
  Faster Qwen3-TTS and Chatterbox modules, and a generic external-server
  provider. Settings schema v2 disables the removed provider safely, preserves
  private bearer credentials, and exposes explicit auto-launch ownership.
  Managed Chatterbox readiness requires `/api/model-info` to report
  `loaded=true`; worker tests cover cancellation, response bounds, WAV repair,
  credential redaction, and owned-child teardown. Its installer selects the
  pinned CUDA 12.1 or 12.8 dependency set from NVIDIA compute capability.
  Go tests, lint, installer
  integration, frontend typecheck/tests/build, and `CGO_ENABLED=0` builds pass;
  the local Windows host lacks the C compiler required for `go test -race`, so
  the mandatory Ubuntu CI gate remains authoritative. Browser and binary
  measurements are recorded in the current snapshot.

- **2026-07-30** - Replaced paired-video native controls and the 400 ms scrub
  inference window with an explicit app-owned transport and seek lifecycle.
  Play now holds the exact media anchor until the backend reports active
  script motion; scrubbing freezes immediately, issues one Stop, commits one
  timestamp, and re-arms once only when playback was intended at scrub start.
  Script filter and rate changes use the same freeze/stop/re-anchor path. The
  video speed cap now limits each authored segment delta instead of chasing a
  stale absolute target, preserving reversals and never increasing a segment's
  speed; peak-rounding diagnostics report the emitted apex reduction. Full Go
  tests, vet, lint (zero issues), the `CGO_ENABLED=0` build, all 355 frontend
  tests, typecheck, localization audit, and the production build pass. The
  transport is embedded over a transparent bottom fade and auto-hides only
  during active playback; pause, arm, seek, errors, and keyboard focus keep it
  visible. Hovering or focusing mute reveals a compact vertical volume slider.
  A current isolated browser build was checked with the paired
  `Kishiri106 By Mouth` media: a rejected arm kept the video at `00:00`, the
  transport had no horizontal overflow at 390 px or 320 px, and the console
  remained clean. Windows race execution still cannot start because no C
  compiler is installed; the mandatory Ubuntu CI race job remains
  authoritative. No real-device motion was authorized for this pass, so R25
  remains High. The complete embedded output is 1,536,190 raw / 756,484
  level-9 gzip bytes; the English startup path is 724,654 / 193,758.

- **2026-07-30** - Chat composition now follows conventional messaging
  behavior: Enter sends, Shift+Enter preserves multiline input, and an Enter
  key event during IME composition is ignored. A focused component regression
  test covers all three paths. Below 560 px, the stacked conversation no longer
  inherits the 520 px tablet minimum; its scrollback is bounded to 170-240 px.
  At the checked 320 x 700 viewport, empty scrollback falls from 315 px to
  210 px, the conversation is 329 px high, and horizontal overflow remains zero.

- **2026-07-30** - Retired the 15 built-ins the user had disabled by hand and
  replaced them with 15 velocity-authored ones. Measuring the disabled set found
  two failure modes: five stalled, spending a contiguous span under 30%/s
  (`Cascade` 2.46 s of a 6.6 s loop, `Descending Ladder` 2.04 s, `Deep, Medium,
  Short` 1.68 s, `Pendulum` 1.04 s, `Surge` 0.68 s) against 0.15 s worst for any
  retained pattern; the other ten never held a pace, averaging a 33%/s slowest
  stroke against 62%/s retained. The cause was that `Positions` and
  `TravelMillis` were independent lists, so stroke velocity was never designed --
  `Cascade` put a 14-unit and an 84-unit stroke on nearly the same duration.
  Replacements derive travel time from an intended stroke velocity and reach the
  cycle floor by repeating their phrase instead of letting `mustFitCatalog`
  stretch every timestamp, which had slowed `Descending Ladder` 1.22x. Cycles run
  6.7-12.4 s and no two replacements are near-duplicates by shape. The former
  screening rule rewarded reach *variety*, which measurement contradicts -- the
  disabled set was the more varied one -- so it is replaced by a speed envelope
  taken from the retained patterns, admitting all 13 and rejecting 12 of the 15
  retired (`Sway`, `Rolling`, and `Double Tap` sit inside the retained range).
  `TestAdaptiveCatalogFramesReduceSubtleStairSteps...` was restated: its
  "adaptive beats fixed by 10%" margin came from the stalling patterns
  manufacturing the fixed-frame baseline, not from the framer. Live catalog worst
  stall is now 172 ms, down from 2,475 ms. Retirement reuses the existing
  `seedBuiltins` delete, so feedback rows cascade away. gofmt, go vet,
  golangci-lint (0 issues), and full Go tests pass.

- **2026-07-30** - Replaced passive read-only controller status with an
  explicit **Take control** action. Handoff marks every browser read-only,
  invokes global Emergency Stop on a detached bounded context, and transfers
  ownership only after the Stop attempt completes; concurrent takeover is
  rejected and an unconfirmed physical Stop remains a visible warning. Browser
  controller IDs are now tab-scoped instead of shared through `localStorage`;
  reloads retain their ID while new and duplicated tabs replace copied session
  state. Backend concurrency/failure tests, eight focused frontend tests, all
  337 frontend tests, the 1,318-key localization audit, full Go tests/vet,
  TypeScript, production build, and golangci-lint pass. English startup is
  706,223 / 189,486 raw/gzip (+3,224 / +729); all HTML/CSS/JS is
  1,066,034 / 312,220 (+6,629 / +1,778); complete embedded output is
  1,510,270 / 749,617. Pure-Go binaries are
  23,008,256 plain / 16,441,344 stripped bytes. Windows race testing cannot
  start without `gcc`; the unchanged Ubuntu CI race gate remains authoritative.

- **2026-07-30** - Added versioned local persona portability. The leading
  persona utility tile now has equal New and Import halves, while the editor
  exports the persisted persona and blocks export during unsaved/in-flight text
  or lore edits. Bounded `.mhpersona` ZIPs carry a strict JSON manifest,
  optional validated JPEG, lore, and a selected behavior profile; imports mint
  fresh persona/lore/profile IDs, trust local built-in profiles by ID, and
  exclude sessions, timestamps, settings, memories, anatomy, capability gates,
  and motion limits. Unknown paths/fields/schema versions, asset mismatches,
  oversized compressed or decompressed entries, malformed portraits, and
  non-controller imports are rejected. Full `go test ./...`, vet, lint (zero
  issues), all 328 frontend tests, the 1,311-key localization audit, typecheck,
  and production build pass. Windows cannot run `go test -race` without a C
  compiler; the unchanged Ubuntu race gate remains authoritative. English
  startup is 702,999 / 188,757 raw/gzip; all HTML/CSS/JS is 1,059,405 /
  310,442; complete embedded output is 1,503,641 / 747,839. Pure-Go binaries
  are 22,987,776 plain / 16,424,448 stripped bytes.

- **2026-07-30** - Separated notification event consumption from visible
  history. A cleared backend completion can no longer be recreated by the next
  `/api/state` snapshot, and current-session history, read state, and a bounded
  consumed-source ledger survive reloads in `sessionStorage` without adding
  database state. New scan/job completion timestamps still produce new events;
  malformed or unavailable browser storage falls back to in-memory behavior.
  Full `go test ./...`, vet, lint (zero issues), focused
  clear/remount/read regressions, all 324 frontend tests, the 1,307-key
  localization audit, typecheck, and the production build pass. English
  startup is 699,807 / 187,885 raw/gzip; all HTML/CSS/JS is 1,055,388 /
  309,352; complete embedded output is 1,499,624 / 746,749.

- **2026-07-30** - Replaced Autopilot's coupled segment/speech loop with
  independent backend-owned clocks. Motion planning runs ahead of its deadline
  while the current pattern continues; a model hold only reschedules and never
  retargets the engine. Motion and speech use separate strict JSON contracts,
  categorical `soon` / `normal` / `later` timing inside user bounds, and
  independent random streams so speech preferences cannot perturb pattern
  selection. Interactive chat cancels and blocks stale autonomous inference.
  Speech waits for TTS capacity and, when audio is enabled, starts its next
  interval after browser playback acknowledgement with a bounded fallback.
  The Chat sidebar exposes separate motion and speech presets, a hard-off
  speech choice, custom bounds, adaptive timing, and speech-motion authority;
  its 1280x720 review keeps the complete motion status visible and removes an
  empty voice-section divider. Full `go test ./...`, vet, lint (zero issues),
  322 frontend tests, the 1,307-key localization audit, typecheck, production
  build, and `CGO_ENABLED=0` plain/stripped builds pass. Local race execution
  remains unavailable because `gcc` is absent; CI retains the mandatory race
  gate. Plain/stripped binaries are 22,873,600 / 16,339,968 bytes. English
  startup is 698,383 / 187,404 raw/gzip; all HTML/CSS/JS is 1,053,964 /
  308,871; complete embedded output is 1,498,200 / 746,268. No hardware motion
  was issued.

- **2026-07-29** - Hardened Phase 19 after an independent implementation
  review. Current-schema validation now requires every v16/v17 persona column,
  index, and lore cascade; clearing to the Settings-backed persona persists as
  an explicit new-chat choice; datastore failures no longer silently change
  reply identity. Persona selection and editing serialize with chat generation
  and Autopilot lifecycle, while committed persona/lore mutations no longer
  depend on a second response-assembly read. Portrait validation decodes the
  complete JPEG, erasure failures remain retryable, and startup reconciles
  missing, interrupted, and orphaned files. Relevant lore supports Han, kana,
  and Hangul phrases while retaining Latin whole-word matching. The browser
  consumes server-owned lore modes and keyword limits, counts Unicode code
  points like Go, and rejects incomplete persona contracts instead of inventing
  enums or bounds. Non-loopback HTTP binds now fail until authenticated HTTPS
  LAN support exists, and persona joins the enforced motion/transport import
  boundary. Full `go test ./...`, vet, lint (zero issues), 321 frontend tests,
  localization audit, typecheck, production build, and a `CGO_ENABLED=0` build
  pass. Local race compilation remains unavailable because `gcc` is absent; CI
  retains the mandatory race gate. Plain/stripped binaries are 22,768,640 /
  16,255,488 bytes. English startup is 691,182 / 185,815 raw/gzip;
  HTML/CSS/JS is 1,041,806 / 305,556; complete embedded output is 1,486,042 /
  742,953 (+821 / +237 against the checked-in bundle). The Pattern Library
  navigation icon was also replaced with a complete stacked-gallery symbol. No
  hardware motion was issued.

- **2026-07-29** - The engine snapshot now separates a clock-sampled
  `current_sample` from `last_sample`, which remains the accepted buffer tail.
  Motion SSE publishes at the engine's 125ms sampling cadence; the shared Handy
  visualizer projects the current semantic point through the active stroke
  window and reverse direction, then uses a matching 130ms linear transition.
  Pause freezes the current estimate, and deterministic engine tests guard both
  live advancement and buffer-tail separation. Reverse direction moved from
  Chat to a right-aligned row in the connection manager; two horizontal range
  rows keep that control block at 107px, below the former limits-only layout.
  The notification panel is 360px wide with 30px commands, a separated
  management group, and the standard trash icon. The persona editor desktop cap
  is 960px while the mobile window still leaves 10px above global Stop. At
  1280x720 and 390x844, all reviewed surfaces had zero horizontal overflow.
  `CGO_ENABLED=0` binaries are 22,710,784 / 16,212,480 bytes plain/stripped;
  startup UI is 689,355 / 185,173 raw/gzip and complete embedded output is
  1,484,215 / 742,311.


- **2026-07-29** - Replaced the Personas editor drawer with one centered modal
  window over the unchanged, dimmed grid. The window locks page scroll, moves
  focus to Close, traps Tab while retaining the global emergency Stop in the
  focus cycle, restores focus on close, and keeps its header and action row
  fixed around one bounded scroll body. Editor sections use dividers instead of
  nested cards. The Personas heading and grid now share the 1,180px wide
  workspace container, and grid tracks explicitly start-align, fixing the
  Firefox-only 100px tile offset. Its desktop width now reaches 1,180px rather
  than leaving the paired form fields constrained to 760px, and it centers in
  the routed workspace so that width cannot overlap the rail Stop control. The
  design doc and SVG layout sketch now describe the shipped overlay. A
  390x844 Firefox pass also caught and fixed
  the global Stop bar covering the modal actions; the dialog now reserves the
  mobile footer while the backdrop continues to cover the viewport. All 315
  frontend tests, the 1,279-key localization audit, typecheck, production
  build, `go test ./...`,
  `go vet ./...`, and lint (zero issues) pass. The English startup payload is
  686,751 raw / 184,757 gzip bytes (+872 / +181); all HTML/CSS/JS and complete
  embedded output are 1,037,375 / 304,498 and 1,481,611 / 741,895
  (+872 / +181 each). Plain/stripped binaries are 22,704,640 / 16,206,848
  bytes (+1,024 / +1,536), both within budget. No hardware motion was issued.
- **2026-07-29** - Stabilized the persona editor and aligned its chat selector.
  Lore loading now follows the persona ID instead of parent callback identity,
  preventing app-state polling from clearing and rebuilding the lore section.
  The drawer keeps its header and action bar fixed around one bounded scroll
  body; the page reserves desktop space for the drawer and uses consistent
  portrait tracks. Chat now presents the persona selector as a box-shaped
  dropdown aligned with the session tabs. The generic MagicHandy profile is now
  listed in both surfaces as a backend-provided view of global model settings;
  selecting it leaves the session persona ID empty and does not create a second
  editable database row. Regression tests cover settings synchronization,
  callback-identity rerenders, one lore request, and persistent content. All 314
  frontend tests, 1,279-key localization validation, typecheck, production
  build, `go test ./...`, `go vet ./...`, and lint (zero issues) pass. An
  isolated 1280x986 browser run held the drawer at the same 806px scroll offset
  across repeated backend polls; logs showed one lore request after opening
  instead of the prior repeated requests. No hardware motion was issued.
  Against the preceding build, the English startup payload is 685,879 raw /
  184,576 gzip bytes (+3,329 / +674); all HTML/CSS/JS and complete embedded
  output are 1,036,503 / 304,317 (+3,660 / +736) and 1,480,739 / 741,714
  (+3,660 / +736). Plain/stripped binaries are 22,703,616 / 16,205,312 bytes
  (+8,192 / +5,632), both within budget.
- **2026-07-29** - Completed the bounded persona-lore slice and corrected the
  preceding persona foundation after review. Persona prompt sets now select the
  actual interactive and Autopilot prompt, default focus applies only to an
  unscoped start when area focus is enabled, and assistant provenance drives
  portraits, speaker names, and mid-session persona dividers. Schema v17 stores
  at most 8 enabled-or-disabled lore entries, 500 characters each and 2,000 in
  aggregate, with Off, Relevant only, and Full prompt modes; relevant matching
  uses normalized whole-word or phrase boundaries and unknown persisted modes
  fail closed. Lore is JSON-quoted as data before the code-owned response and
  motion contracts. A backend-exact diagnostics inspector reports prompt
  sections, character counts, model, prompt set, persona, and included lore,
  while an opt-in `liveeval` harness measures 0/500/1,000/2,000-character model
  budgets without reaching the motion engine or a transport. The v16 migration
  now preserves an incompatible legacy Rockfire `personas` table instead of
  failing or overwriting it. Full `go test ./...`, vet, lint (zero issues), the
  live-evaluation compile check, localization audit, all 312 frontend tests,
  typecheck, production build, and plain/stripped `CGO_ENABLED=0` builds pass.
  Local race execution remains unavailable because MinGW `gcc` is absent; CI
  retains the mandatory Ubuntu race gate. The configured llama.cpp model was
  not loaded, so model-specific lore baselines remain explicitly unmeasured.
  Isolated fake-transport browser checks covered the persona grid/editor and
  prompt inspector at 1280x720, plus 390x844 overflow metrics, with no console
  errors or horizontal overflow. No hardware motion was issued. Against the
  preceding build, the English startup payload is 682,550 raw / 183,902 gzip
  bytes (+35,796 / +8,060); all HTML/CSS/JS and complete embedded output are
  1,032,843 / 303,581 (+57,633 / +16,788) and 1,477,079 / 740,978
  (+57,633 / +16,788). Plain/stripped binaries are 22,695,424 / 16,199,680
  bytes (+359,936 / +276,480), both within the 30 MB budget.
- **2026-07-29** - Added 21 opt-in appearance palettes from the surviving
  scratch mockups while preserving Steel Azure as the default. The backend
  owns the closed theme catalog, validates and persists selection in SQLite,
  publishes supported choices with the settings snapshot, preserves selection
  across updates from older clients, and falls back safely when a retired value
  is loaded. Settings presents concise names and color swatches in a compact,
  accessible radio list; all palettes change neutral and interactive tokens
  only. Canonical green, amber, and red safety semantics remain unchanged, and
  the emergency Stop button has no shadow or glow in any theme. A static
  contrast audit passed WCAG AA text pairs and 3:1 interactive boundaries for
  every palette. The localization audit covers 1,197 keys in all five locales.
  All 291 frontend tests, typecheck, production build, `go test ./...`, vet,
  lint, and the `CGO_ENABLED=0` build pass. An isolated fake-transport browser
  run covered Steel Azure, Deep Violet, Warm Titanium, and High Contrast at
  1280x720 plus the one-column picker and fixed Stop clearance at 390x844.
  Saving changed the backend snapshot and survived reload; all views remained
  free of horizontal overflow and console errors, and computed Stop shadow was
  `none`. The first desktop pass exposed an incomplete tinted half-row, so the
  final picker uses container-aware one/two/four-column tracks and balances its
  two-item final row. No hardware motion was issued. Against the preceding
  2026-07-29 build, the English startup payload is 646,754 raw / 175,842 gzip
  bytes (+13,264 / +3,877); all HTML/CSS/JS is 975,210 / 286,793
  (+14,307 / +4,121), and complete embedded output is 1,419,446 / 724,190
  (+14,307 / +4,121). Plain/stripped binaries are 22,335,488 / 15,923,200
  bytes (+19,456 / +19,456), both within the 30 MB budget.
- **2026-07-29** - Recovered the interrupted post-PR #146 media and settings
  follow-up. Converting the open video now follows only the new output from the
  matching successful job, ignores older same-name rows and late stale job
  snapshots, and returns to the catalog with an explicit fallback instead of
  flashing or stranding the user on "Video unavailable." Device and Model
  settings use one top-level card layer; Model permissions are no longer a
  framed fieldset nested inside the Local LLM card. The AAC target is adjustable
  from 96 through 576 kbps in 16 kbps steps, with existing AAC copied and the UI
  identifying the value as a target that FFmpeg may clamp for the source format.
  The localization audit covers 1,194 keys in all five locales. All 286 frontend
  tests, typecheck, production build, `go test ./...`, vet, lint, and
  plain/stripped `CGO_ENABLED=0` builds pass. Local race execution remains
  unavailable because MinGW `gcc` is absent; CI retains the mandatory Ubuntu
  race gate. Responsive browser capture was attempted against an isolated
  fake-transport server but the Windows browser-automation sandbox failed during
  ACL setup before it could connect; DOM hierarchy regressions and the
  production render path are covered, but no new visual viewport claim is made.
  No hardware motion was issued. Against the 2026-07-28 measurement, the English
  startup payload is 633,490 raw / 171,965 gzip bytes (+3,573 / +824);
  all HTML/CSS/JS is 960,903 / 282,672 (+6,959 / +2,098), and complete embedded
  output is 1,405,139 / 720,069 (+6,959 / +2,098). Plain/stripped binaries are
  22,316,032 / 15,903,744 bytes (+8,704 / +8,192), both within the 30 MB budget.
- **2026-07-28** - Consolidated media library management and shell feedback.
  Settings > Media now keeps locations, startup behavior, missing-entry cleanup,
  thumbnail generation, and compatibility conversion in one scan-options group.
  Manual and explicitly opted-in startup scans use the same bounded scanner,
  cancellation state, structured summary, and follow-up path. Cleanup defaults
  on but deletes only catalog rows and owned thumbnails after a root is read
  completely; source media and unseen entries under unavailable or partial
  roots are always preserved. A shell notification center adapts Stash's
  visible task model into backend-derived Activity, current Attention, and
  bounded session Recent sections, with toasts retained in the same history and
  only one top-bar disclosure open at a time. Desktop 1440x900 and mobile
  390x844 rendered checks found no overflow, clipping, fixed-Stop overlap, or
  blank state card; testing used the fake transport and issued no hardware
  motion. All 281 frontend tests, the 1,190-key localization audit, typecheck,
  production build, `go test ./...`, vet, lint, and plain/stripped
  `CGO_ENABLED=0` builds pass. Local race execution remains unavailable because
  MinGW `gcc` is absent; CI retains the mandatory Ubuntu race gate. Against the
  prior checked-in build, the English startup payload is 629,917 raw / 171,141
  gzip bytes (+19,019 / +4,066); all HTML/CSS/JS is 953,944 / 280,574
  (+35,446 / +8,636), and complete embedded output is 1,398,180 / 717,971.
  Plain/stripped binaries are 22,307,328 / 15,895,552 bytes (+48,128 /
  +46,592), both within the 30 MB budget.

- **2026-07-26** - Multilingual UI, installer, and chat prompts. English,
  Spanish, Brazilian Portuguese, Simplified Chinese, and Japanese now share
  1,028-key browser catalogs with strict key, placeholder, encoding, static
  string, toast/confirmation, and sentence-fragment audits. UI and built-in chat
  reply languages persist independently; non-English catalogs lazy-load, the
  localized installer changes language before its remaining decision tree,
  updates preserve both choices, and `change-language.ps1` provides a native-name
  recovery flow. Localized code-owned prompts preserve English JSON keys/enums,
  final output guards, capability limits, repair language, and custom-prompt
  ownership. The final local Gemma 4 12B llama.cpp matrix passed 12/12 chat-only,
  motion-intent, and malformed-repair cases across the four non-English locales
  in 15.78 seconds without constructing a motion engine or transport. All 264
  frontend tests, localization audit, typecheck/build, the PowerShell 5.1
  installer suite, `go test ./...`, vet, lint, and plain/stripped
  `CGO_ENABLED=0` builds pass. Local race execution remains unavailable because
  MinGW `gcc` is absent; CI retains the mandatory Ubuntu race gate. The English
  startup bundle is 574,771 raw / 157,870 gzip bytes; all HTML/CSS/JS is 831,834
  / 246,210 and complete embedded output is 1,276,070 / 683,607. Plain/stripped
  binaries are 21,971,968 / 15,614,976 bytes.

- **2026-07-26** - Video initiation and Firefox seek-status pass. Paired media
  now mounts immediately so browser preloading can begin while its bounded script is fetched;
  controls remain withheld until validation. The timeline response prepares one
  server-side document for promotion on first arm, deep seeks binary-search past
  old actions, and Cloud's independent physical slider/stroke reads overlap
  under the existing Stop admission. Seek feedback names the target timestamp
  while stopping, resynchronizing, and resuming, then clears only on `playing`.
  Focused coverage reproduces Firefox's pause-before-seek event order. All 253
  frontend tests, typecheck/build, the full Go suite, vet, lint, and plain plus
  stripped `CGO_ENABLED=0` builds pass. The local race suite remains unavailable:
  installed Clang targets MSVC and rejects Go's MinGW flags, while MinGW GCC is
  absent; CI retains the mandatory race gate. Against the checked-in bundle,
  Node 22 zlib level-9 measurement is +1,035 raw / +346 gzip bytes: 510,053 /
  141,425 HTML/CSS/JS and 954,289 / 578,822 complete. Plain/stripped binaries
  are 21,610,496 / 15,260,160 bytes. Physical alignment was not re-measured; the
  M3 hardware gate remains open.

- **2026-07-23** - Chat voice continuity parity: non-utility interactive chat
  adds bounded, quoted persona and user-anatomy settings, the reviewed strict
  17-value model mood persisted in existing per-message diagnostics, and three
  canonical recent assistant lines for anti-repetition. Mood is visible from
  backend active-session state and has no motion representation. Current-turn
  authorization strips model motion from chat-only requests and rejects
  negation/conversation wording. Chat Stop invalidates overlapping generations,
  publishes its sequence before transport latency, and has deterministic
  phrases/replies for every built-in language. Schema v13 keeps generated
  assistant rows outside history, mood, and cap pruning until the Stop barrier
  commits them. Prompt-only settings no longer refresh active motion, and
  utility prompt composition remains byte-identical. A follow-up review pass
  closed the issues its own audits raised: physical Emergency Stop no longer
  queues behind settings or voice-worker lifecycle locks (the shared chat
  delivery mutex is gone; speech submission is ordered by the Stop epoch and
  cancels itself if it raced one, and voice submission no longer holds the
  manager lock across a worker spawn), a Stop that lands after the reply
  commits reports only canceled motion/speech instead of contradicting
  canonical history, non-Emergency stop failures use neutral wording, and the
  current-turn gate no longer treats safety/permission questions ("is it safe
  to start moving?") or contracted negatives ("I didn't want you to start
  moving") as authorization to start the device. The rebuilt four-file browser
  payload is 943,204 bytes raw / 576,236 gzip; HTML/CSS/JS is 498,968 raw /
  138,809 gzip. `CGO_ENABLED=0` binaries measure 21,502,976 bytes plain and
  15,173,632 bytes stripped, both within budget.

- **2026-07-25** - Floating playback panel. The paired-script calibration moved
  to where it is used: a compact overlay on the Videos workspace (288x321 px)
  holding a per-video sync offset and three script filters, reachable without
  leaving the picture. The offset is now per video added to a setup-wide value,
  stored in schema v14 on `media_videos`, and applied by shifting the sync
  runtime's anchor rather than the slice point — so adjusting it during playback
  moves the video without stopping the device. Filters are smoothing (reusing
  the existing reversal stabilizer) and peak rounding (a bounded quadratic
  fillet that moves a hand-authored triangle toward a sine); both re-arm the run
  because they change accepted points, both report their measured effect, and
  the zero value is authored-exact. Peak rounding is pinned by tests that it
  never raises peak velocity, that its reduction is bounded and grows with the
  window, that each corner is capped by its own leg, and that a script too dense
  for the fillets plays unrounded rather than thinned. Payload is 953,185 raw /
  577,833 gzip; HTML/CSS/JS 508,949 / 140,812; binaries 21,577,728 plain and
  15,235,584 stripped. Filter feel is not verified on hardware.

- **2026-07-24** - Paired-script timing pass. A report that motion ran slightly
  ahead of the video resolved into three causes: the HSP play timestamp was
  captured before a network round trip that refreshes the Handy clock offset and
  returned stale (worst at the first play of a session and at every 5-minute TTL
  expiry, which explains the intermittency); that timestamp described a
  different instant than the origin the engine clock is built on; and resuming
  aligned the video a moment before the seek and decoder restart it then paid
  for, leaving a one-directional offset inside the tolerated band. A simulated
  two-leg network measures the timestamp/origin distance at 361 ms before and
  1 ms after. Settings > Media gains a bounded script offset for authoring bias,
  display latency, and device lag, which code cannot remove. A defect found in
  live verification and not by the unit tests is pinned separately: the public
  settings projection built MediaSettings field by field, so the new value saved
  and every read returned zero. Payload is 945,025 raw / 575,933 gzip;
  HTML/CSS/JS 500,789 / 138,912; binaries 21,530,112 plain and 15,197,184
  stripped. Hardware timing is not verified here.

- **2026-07-28** - Retired the manual pattern Focus range. The authored-span
  correction already expands a focused loop pattern into its requested named
  area automatically; the separate persisted range was a niche outer envelope,
  not part of that fix, and its placement beside global connection limits
  implied that video obeyed it when video deliberately does not. The slider,
  quick API fields, and persisted settings fields are removed; legacy values
  are ignored so they cannot survive invisibly. Named area focus, the automatic
  20-point minimum, and clock-locked media exclusion remain unchanged. All 276
  frontend tests, localization audit, typecheck/build, the full Go suite, and
  vet pass. The English startup payload is 610,898 raw / 167,075 gzip bytes;
  all HTML/CSS/JS is 918,498 / 271,938 and complete embedded output is
  1,362,734 / 709,335. Plain/stripped `CGO_ENABLED=0` binaries are 22,259,200 /
  15,848,960 bytes.

- **2026-07-24** - Motion focus and reversal follow-up. Two reports shared one
  cause: the reversal ramp was a fixed 75 ms of authored curve time, so it did
  not shrink when playback slowed or a focus window compressed the stroke, and a
  focused pattern shrank twice because the window scaled the whole semantic
  range rather than the pattern's own span. The ramp is now an acceleration
  budget, a confined loop pattern fills its window, focus windows below 20
  points are refused rather than delivered as a hold, and the user has a live
  **Focus** range that chat zones subdivide instead of escaping. Measured over
  the catalog at 20% speed with whole-percent output, rounded stationary time
  fell from 11,910 ms to 1,048 ms with fewer wire points, at a cost of 0.05
  percentage points of peak wire error. The rebuilt browser payload is 943,580
  bytes raw / 575,567 gzip; HTML/CSS/JS is 499,344 raw / 138,546 gzip.
  `CGO_ENABLED=0` binaries measure 21,522,944 bytes plain and 15,190,528
  stripped, both within budget. Hardware feel is not verified here.

- **2026-07-22** - Video and pattern continuity follow-up: the retained failing
  trace proved parsing and slicing were source-exact, but exposed two later
  defects. Cloud accepted only about 2.1 seconds ahead, and media sampling then
  contracted every position around 50% by the configured maximum. At 30%,
  `81 -> 41 -> 66` became about `59.3 -> 47.3 -> 54.8`, turning subtle authored
  strokes into near-resolution movement with a perceived dwell at reversals.
  Clock-locked Cloud media now maintains a batched 10-second lead under the
  100-point owner cap, emits exact authored knots without synthetic one-second
  chunk points, and preserves authored video timing and position by default.
  Settings > Media adds an opt-in causal speed cap; changing its effective value
  Stops and re-arms media without stopping unrelated patterns. Startup remains
  independently speed-bounded. A capped Cloud run kept sixteen 750 ms
  heartbeats `following` with 1 ms calibrated drift and 327-364 ms successful
  append latency; a 01:16 to 01:30 seek re-armed once and stayed healthy.
  Pattern/program speed remains timeline-based, and a capped pattern trace had
  no duplicate positions or explicit holds. The full Go suite, 228 frontend
  tests, typecheck, build, vet, lint, and `CGO_ENABLED=0` build pass. Embedded UI
  is 937,161 raw / 574,432 gzip bytes; stripped binary is 15,059,968 bytes.
  Subjective continuity and matched Browser Bluetooth/Intiface evidence remain
  the acceptance gate.

- **2026-07-22** - Position-aware Cloud startup: the first capped continuity
  run exposed an unbounded first action because HSP's `t=0` point was treated
  as if its later segment timing also constrained acquisition from an unknown
  physical position. Start/Resume now Stop stale HSP, read slider/stroke state,
  widen rather than narrow around the current position, play a speed-bounded
  engine-owned HSP lead-in, verify arrival, then apply the requested window and
  begin the main/media clock. Live firmware then proved its relative position
  field is scoped to the active stroke window; an absolute-coordinate repeat
  measured 29.33 mm, led to the 24.57 mm target, verified 24.67 mm at zero
  speed inside the final 20-80% window, and only then played the main stream.
  The full capped validation ended with a successful Stop. Automated coverage
  pins a 90%-to-20% move at a 20% cap to 3.5 seconds and proves Emergency Stop
  prevents main playback; subjective confirmation of first-action feel remains
  open.

- **2026-07-22** - Cloud startup arrival sequencing: a failed video trace showed
  the physical slider still catching up after the scheduled lead-in duration.
  The engine had issued Stop before observing arrival, freezing motion short of
  the target and making repeated starts inch forward. Startup now leaves the
  final lead-in target active for up to six cancelable physical reads, Stops
  only after stationary in-window arrival, and verifies once more after Stop.
  Automated coverage pins delayed convergence, bounded fail-stop, post-Stop
  drift rejection, and Emergency Stop during polling. A capped 10% Cloud run
  then moved from 92.00 mm (88.93%) to the 5.00 mm endpoint, observed and
  post-Stop verified 4.67 mm at zero reported speed, started the main stream,
  and ended with Stop HTTP 200. All 128 concurrent `/api/state` probes
  succeeded in 1-5 ms, ruling out a startup-held core-state lock.

- **2026-07-22** - Paired-video timeline clarity follow-up: corrected extrema
  bucketing that attached both ends of every sparse segment to the later
  endpoint's screen column, creating false vertical bars. The 88 px
  multi-color plot is now a 60 px high-contrast azure position trace with
  same-pixel dense extrema, sparse guides, a smoothed three-pixel activity
  rail, and an outlined playhead. Rendered fake-transport QA at 1280x720 and
  390x844 covered a 3,536-action script, measured the timeline at 60 px on both
  viewports, found no horizontal overflow, and produced a clean browser
  console. All 223 frontend tests, typecheck/build, `go test ./...`,
  `go vet ./...`, `golangci-lint`, and the `CGO_ENABLED=0` build pass.
  HTML/CSS/JS is
  490,439 raw / 136,606 gzip bytes; complete embedded output is 934,675 /
  574,033 (+799 / +270 from M1-M2). No dependency or runtime-memory budget
  moved.

- **2026-07-22** - Phase 18 M1-M2 paired-video playback: exact-basename
  funscripts now load through one jailed 16 MiB / 100,000-action parser, render
  in a hideable intensity timeline below the native video, and run as finite
  linear targets through the shared motion engine. Play holds the video until
  admission; pause, seek, rate change, media stall/error, end, close, drift,
  timeout, and Emergency Stop use explicit Stop/re-arm semantics. Per-player
  session/sequence fences reject reordered close and arm requests. Rendered
  fake-transport QA at 1280x720 and 390x844 exercised a 3,536-action script,
  play/pause/live seek/end, timeline hide/show, responsive layout, and a clean
  browser console; no real-device alignment claim is made before M3. All 222
  frontend tests, typecheck/build, `go test ./...`, `go vet ./...`, pinned
  `golangci-lint` v2.12.2, and the `CGO_ENABLED=0` build pass. The local race
  build remains unavailable because `gcc` is absent; CI retains the mandatory
  Ubuntu race gate. HTML/CSS/JS is 489,640 raw / 136,336 gzip bytes; complete
  embedded output is 933,876 / 573,763 (+17,789 / +5,311 from `main`).
  Plain/stripped binaries are 21,275,648 / 14,995,456 bytes.

- **2026-07-22** - Browser-style Chat tab compaction: session tabs now prefer
  236px, share available width evenly, and stop at 112px desktop / 104px narrow
  before horizontal overflow. The native scrollbar is hidden; wheel input,
  keyboard focus, active-session changes, and strip resizes reveal overflowed
  tabs while New Chat retains its protected hit area. Rendered 1600x900,
  1280x720, and live-resized 390x844 checks measured 236 / 193 / 104px tab
  widths, kept the active tab fully visible, and found no page overflow or New
  Chat overlap. All 209 frontend tests, typecheck/build, `go test ./...`,
  `go vet ./...`, `golangci-lint`, and plain/stripped `CGO_ENABLED=0` builds
  pass. HTML/CSS/JS is 471,851 raw / 131,025 gzip bytes; complete embedded
  output is 916,087 / 568,452 (+783 / +296). Plain/stripped binaries are
  21,155,328 / 14,898,176 bytes. The local race build remains unavailable
  because `gcc` is absent; CI retains the mandatory gate.

- **2026-07-22** - Chat tab visual correction: the protected New Chat slot now
  uses a 32 px borderless-at-rest icon aligned to the tab-label baseline. Active
  tabs use one symmetric top-corner surface and bottom accent without the former
  three-sided outline. Focus moves to the complete compound tab shape instead
  of stopping between its label and menu segments. Rendered 1280x720 and
  overflowing 390x844 checks confirmed that New Chat remains the center-point
  hit owner with no overlap. HTML/CSS/JS is 471,068 raw / 130,729 gzip bytes;
  complete embedded output is 915,304 / 568,156 (+524 / +90). Plain/stripped
  `CGO_ENABLED=0` binaries are 21,154,816 / 14,897,664 bytes.

- **2026-07-21** - Cloud buffer continuity and Chat tab hit targets: a 194-row
  live trace showed ordinary HSP adds acknowledged with only 80-125 ms left in
  the preceding buffer and three responses after its tail, including -540 ms
  on a 958 ms request. Cloud now declares a 1.5-second accepted-buffer floor,
  the engine reaches it before Play, lead selection reserves the dispatch tick,
  and checks use the actual emitted tail. API v3 HTTP 200 `error` envelopes now
  fail without advancing the HSP index. The current device check returned
  `DeviceNotConnected`, so no automated motion was attempted and the capped
  physical rerun remains open. Chat gives New Chat a separate 46 px trailing
  slot and omits actionless menu triggers; 1280x720 and overflowing 390x844
  checks found no blocked or overlapping hit area. All 208 frontend tests,
  typecheck/build, `go test ./...`, `go vet ./...`, `golangci-lint`, and plain
  plus stripped `CGO_ENABLED=0` builds pass. HTML/CSS/JS is 470,544 raw /
  130,639 gzip bytes; complete embedded output is 914,780 / 568,066 (+330 /
  +73). Plain/stripped binaries are 21,154,304 / 14,897,152 bytes. The local
  race build remains unavailable because `gcc` is absent; CI retains the gate.

- **2026-07-21** - Managed LLM endpoint and Chat workspace hardening: managed
  llama.cpp keeps its preferred port when free and selects another loopback
  port immediately before process start when occupied, so it cannot mistake an
  unrelated local service for its runner. Early child exits now return bounded
  stderr without waiting for the full load timeout. With NVIDIA Broadcast still
  listening on 8080, the installed CUDA Gemma model loaded on the selected
  fallback in 3.82 seconds and completed a structured request in 0.38 seconds.
  The live Cloud connection check reported HSP available/stopped without a
  motion command. Chat now attaches its compact title to the 43px session strip
  and places New Chat directly after the rightmost tab; 1280x720 and 390x844
  rendered checks found no clipping or fixed-control overlap. All 207 frontend
  tests, typecheck/build, `go test ./...`, `go vet ./...`, `golangci-lint`, and
  plain/stripped `CGO_ENABLED=0` builds pass. HTML/CSS/JS is 470,214 raw /
  130,566 gzip bytes; complete embedded output is 914,450 / 567,993 (+402 /
  +84, measured like-for-like against the checked-in bundle). Plain/stripped
  binaries are 21,143,040 / 14,890,496 bytes. The local
  race build remains unavailable without `gcc`; CI retains that mandatory gate.

- **2026-07-21** - Motion boundary follow-up: live API v3 state returned numeric
  `play_state`, which the string-only readiness parser rejected, and Cloud's
  engine clock included all setup/prebuffer latency. Numeric states now map to
  named health states; established stopped/not-initialized playback forces
  recovery Stop; and buffered owners align to the accepted Play midpoint. A
  capped 20% clock run reduced engine/device skew from about 1.4-1.5 s to
  120-160 ms including a state-read round trip. A 30%-maximum retarget checklist
  completed 15 commands with zero failures, 307-355 ms Cloud latency, 668-1,141
  ms retarget lead, and confirmed Emergency Stop. Correct clocking reduced
  prebuffer output from 13 batches / 125 points to 10 / 90. Intiface now feeds
  selected `StepCount`, scaled through the active stroke window, into the shared
  bounded sampler. Cloud enforces its 100-point add limit, and Handy reverse
  mapping mirrors native quantized steps exactly. No dependency or browser
  payload changed; full gate results are recorded by the PR.

- **2026-07-20** - Motion pathway and subtle-jitter audit: all production
  sources now converge on the shared engine; unused raw Cloud/Bluetooth
  stroke/add/play HTTP routes were removed. Across the 29-pattern catalog over
  two cycles at 10% focus, fixed 125 ms Cloud-rounded sampling produced 3,385
  points, 915 duplicate edges, and 113,001 ms of stationary segments.
  Authored-knot plus 25 ms adaptive sampling reduced that to 2,294 / 528 /
  71,232 ms. A Cloud-resolution-aware engine pass reduced it again to 1,420 /
  151 / 29,717 ms (74% less stationary time than the old grid), with 0.843%
  worst measured wire error versus a 3.125% missed `Hard and Regular` peak on
  the old grid. Semantic output remains bounded to 0.35%. Loop seams now retain
  cyclic velocity unless they truly reverse,
  retargets use a 750 ms C1 blend whose final frame removes generated <=2%
  chatter, Browser Bluetooth retains 0.1% native point resolution, and loop
  import rejects <5% source spans plus rapid <=2% chatter.
  Slow subtle reversals and stored finite programs remain source-exact. Focused
  Go, frontend, typecheck, and lint gates pass. The rebuilt embedded UI is
  914,048 raw / 567,909 gzip bytes total (+475 / +172), with HTML/CSS/JS at
  469,812 / 130,482 bytes. Matched below-40% hardware feel remains open.

- **2026-07-20** - Hardware-feedback pattern curation and rename:
  live inspection found 6 disabled rows among 29 patterns. `Deep Bookends`,
  `Lower Midrange Mix`, `Midrange with Full Finish`, `Mid-to-Top Switch`,
  `One Deep, Three Shallow`, and `Top-Anchored Depths` shared fixed-endpoint
  micro-strokes or repeated same-span runs that produced limited variation and
  jittery motion. They are explicitly retired and replaced by six complete
  source cycles from distinct source fingerprints. Tests require each new
  experimental replacement to keep at least 30% travel, four amplitude bands,
  bounded endpoint reuse, and no long equal-amplitude run before the existing
  acceleration/reversal fit. All retained patterns lose the experimental tag.
  Exact `Hard and Regular` and `playful jerk` timing is promoted under a
  `curated` tag; seed reconciliation preserves names, enablement, and weights
  while removing only exact duplicates. Inline rename now works for every
  pattern while IDs and built-in curves stay immutable. Fresh-data API QA found
  29 total / 6 experimental / 2 curated / 0 retired rows. Desktop 1280x800 and
  mobile 390x844 rendered checks found no body overflow; the mobile rename form
  remained within the viewport. All 205 frontend tests, typecheck/build,
  `go test ./...`, `go vet ./...`, `golangci-lint`, and plain/stripped
  `CGO_ENABLED=0` builds pass. HTML/CSS/JS is 469,337 raw / 130,310 gzip bytes;
  complete embedded output is 913,573 / 567,737. Plain/stripped binaries are
  21,082,624 / 14,827,520 bytes. Local race testing remains unavailable because
  `gcc` is absent from `PATH`; CI retains that gate. No automated hardware
  motion was run, and physical feel for the six replacements remains open.

- **2026-07-20** - Motion-semantic sampled loops and attached Chat status:
  899 user-provided funscripts across three local collections were reduced to
  reversal extrema and screened for complete, closed, meaningfully ranged
  phrases with bounded reversal spacing. Twelve transformed loops expand the
  experimental catalog from 12 to 24. Names, descriptions, tags, and selection
  use only curve behavior; source filenames, paths, and payloads are not retained
  or committed. Existing databases receive the additions through the idempotent
  built-in seed with no schema migration and keep user enablement and weights.
  Engine snapshots now publish the backend-resolved active pattern name. Chat's
  detailed visualizer is a compact bottom-attached status band showing state,
  commanded estimate, active pattern, range, speed, and source; stopped state
  suppresses retained active metadata. All 204 frontend tests, typecheck/build,
  `go test ./...`, `go vet ./...`, `golangci-lint`, and plain/stripped
  `CGO_ENABLED=0` builds pass. Rendered 1280x720 and 390x844 checks found no
  horizontal overflow, clipping, or collision with fixed Stop controls; a fresh
  database exposed all 27 built-ins and the isolated sampled preview matched its
  motion-semantic label. HTML/CSS/JS is 467,421 raw / 129,676 gzip bytes;
  complete embedded output is 911,657 / 567,073. Plain/stripped binaries are
  21,063,680 / 14,832,128 bytes. Local race testing remains unavailable because
  `gcc` is absent from `PATH`; CI retains that gate. No hardware motion was run.

- **2026-07-20** - Retained chat sessions and full-space Chat workspace:
  schema v12 migrates the former global log into backend-owned sessions with
  session-scoped messages, cursors, and redacted response diagnostics. Saved
  tabs persist, while the backend enforces at most one active unsaved working
  tab and rejects unresolved save/discard transitions. Startup settings choose
  previous versus new chat and whether the current unsaved tab survives a
  restart; starting clean disables draft retention, while clean shutdown and
  the next crash-recovery startup both enforce the policy. Saved tabs are
  always retained. Chat now fills the routed workspace
  beside its control sidebar, exposes stable keyboard-operable tabs with a
  visible and right-click menu, confirms every New action, and binds streaming
  and Autopilot output to the selected session. Assistant avatars expose the
  useful diagnostic provenance from StrokeGPT-ReVibed without persisting
  prompts, request bodies, memories, or credentials. All 202 frontend tests,
  typecheck/build, `go test ./...`, `go vet ./...`, `golangci-lint`, and
  plain/stripped `CGO_ENABLED=0` builds pass. Desktop 1440x900 and mobile
  390x844 rendered checks found no horizontal overflow or browser warnings;
  save/new/discard, settings persistence, and diagnostics focus visibility
  passed against a fresh fake-transport app. HTML/CSS/JS is 466,075 raw /
  129,414 gzip bytes; complete embedded output is 910,311 / 566,811.
  Plain/stripped binaries are 21,052,416 / 14,820,864 bytes. The local race
  build remains unavailable without a C compiler; CI retains that gate. No
  hardware motion was run.

- **2026-07-20** - State-aware interactive LLM motion and expanded built-ins:
  each chat turn now receives the authoritative engine state, user speed bands,
  current area/content, and a four-transition runtime trace tail. Steady,
  ordinary, and pacing-only requests preserve unspecified state; explicit
  variation rejects semantic no-ops and recent-pattern loops, with one repair
  and a deterministic fresh-pattern fallback that preserves separately
  requested speed/area changes. Idle `target` cannot start motion. Four
  persisted model permissions hide disabled methods server-side; they reuse the
  existing settings JSON document, so no SQLite schema migration is needed.
  The built-in catalog now has three established plus twelve complete-cycle,
  opt-in experimental patterns. Managed llama.cpp b9966/CUDA with the installed
  Gemma 4 11.9B Q4_0 model completed all 13 live cases first-pass; the Ollama
  Granite 4.1 3B matrix completed with bounded repairs. Both used a 20–40%
  envelope and no transport dispatch. The model screen collapses a large Ollama
  inventory and keeps its compact permission grid in the primary viewport.
  All 194 frontend tests, typecheck/build, `go test ./...`, `go vet ./...`,
  `golangci-lint`, and plain/stripped `CGO_ENABLED=0` builds pass. Desktop and
  390x844 checks found no overflow or browser warnings. HTML/CSS/JS is 447,616
  raw / 125,034 gzip bytes; complete embedded output is 891,852 / 562,461.
  Plain/stripped binaries are 20,953,600 / 14,744,576 bytes. The local race
  build remains unavailable without `gcc`; CI retains that gate. No hardware
  motion was run.

- **2026-07-20** - Video workspace and handling review: Videos is now a
  first-class wide workspace and sidebar destination instead of a Pattern
  Library tab. Pattern browsing, authoring, import, and training keep their own
  route, while leaving Videos unmounts the native player. The catalog preserves
  loaded rows across refresh/scan failures, distinguishes reload from an
  explicit cancellable filesystem scan, retries transient scan-status reads,
  exposes missing locations without making them unreachable to assistive
  technology, and searches both names and saved locations. Playback now avoids
  noisy duration writes, offers a real media reload, clears recovered errors,
  and serves stable MIME types for every accepted extension. All 191 frontend
  tests, typecheck/build, `go test ./...`, `go vet ./...`, `golangci-lint`, and
  plain/stripped `CGO_ENABLED=0` builds pass. Desktop 1440x900 and mobile
  390x844 rendered checks found no horizontal overflow or browser warnings;
  the eight-second sample reached ready state 4, and route teardown removed the
  media element. HTML/CSS/JS is 445,192 raw / 124,549 gzip bytes; complete
  embedded output is 889,428 / 561,976. Plain/stripped binaries are 20,872,192
  / 14,680,064 bytes. The local race build remains unavailable without `gcc`;
  CI retains that gate. No hardware motion was run.

- **2026-07-19** - Phase 18 M0 media foundation: schema v11 adds a nullable,
  indexed video catalog fed only by saved absolute locations and explicit
  depth/file-bounded scans. Opaque IDs, rooted file handles, component-level
  symlink rejection, file-identity validation, and `http.ServeContent` provide
  jailed constant-memory Range streaming. The
  Videos grid/search and native player share one motion-free component with the
  optional funscript import preview; exact-basename script presence is metadata
  only. Hidden library tabs unmount their player so playback cannot continue
  invisibly. Startup reconciles catalog rows to saved locations, and the Videos
  tab remains available when the independent pattern catalog fails. Modal
  layering and focus preserve global Emergency Stop. The Autopilot regression was an
  ownership/provenance defect, not a second schema: Chat retains the Autopilot
  sidebar control, Diagnostics retains Manual Motion, both key active state to
  `target.source`, and the API rejects manual retargets of autonomous or idle
  engines. All 186 frontend tests, typecheck/build, `go test ./...`, `go vet
  ./...`, `golangci-lint`, and plain/stripped `CGO_ENABLED=0` builds pass.
  Desktop 1440x900 and mobile 390x844 rendered checks had no horizontal
  overflow; a 640x360 eight-second sample reached browser ready state 4. Final
  HTML/CSS/JS is 441,374 raw / 123,497 gzip bytes; complete embedded output is
  885,610 / 560,924. Plain/stripped binaries are 20,867,072 / 14,654,976 bytes.
  The local race build remains unavailable without `gcc`; CI retains that gate.
  No hardware motion was run. Resumed manual acceptance scanned two roots with
  four encountered files into three videos and one exact-basename pair without
  issues. A full 2 GiB sparse stream returned 200 and 2,147,483,648 bytes in
  0.829 s; peak server RSS rose 1,495,040 bytes (63,774,720 to 65,269,760), and
  a 648-byte tail Range returned 206. M0's multi-root and constant-memory checks
  are closed before M1.

- **2026-07-19** - Manual-test ownership and control placement: Chat's compact
  control sidebar now owns Autopilot, while the explicitly badged Manual Motion
  surface lives under Settings > Diagnostics. Manual test state reads the
  backend target's `manual_ui` provenance instead of treating every running
  engine as a test; starting a test stops the active run and drains its
  autonomous owner before claiming the shared engine. No database migration is
  needed.
  All 169 frontend tests, typecheck/build, `go test ./...`, `go vet ./...`,
  `golangci-lint`, and plain plus stripped `CGO_ENABLED=0` builds pass. Desktop
  1440x900 and mobile 390x844 rendered checks had no horizontal overflow or
  browser warnings and reproduced the Autopilot-to-diagnostics handoff. The
  final HTML/CSS/JS payload is 420,401 raw / 118,635 gzip bytes; the complete
  embedded payload is 864,637 / 556,052. Plain/stripped binaries are 20,685,312
  / 14,537,216 bytes. No real-device motion was run and transports are unchanged.

- **2026-07-19** - Chat Autopilot review and Chat integration: the assistant
  session now lives directly above the canonical conversation instead of
  competing with deterministic Freestyle in Preset Modes. The model receives a
  bounded canonical history tail; custom library definitions survive hold and
  drift; Stop/mode cancellation invalidates raced announcements; and successful
  autonomous lines enter the shared log before optional browser-playable TTS.
  Busy TTS remains text-only rather than deepening the speech queue. The Chat
  strip exposes Start/Stop, Pause/Resume, and honest model/hold/planner-fallback
  provenance without duplicating assistant text. All 166 frontend tests,
  typecheck/build, `go test ./...`, `go vet ./...`, `golangci-lint`, and plain
  plus stripped `CGO_ENABLED=0` builds pass. Relative to the merged funscript
  baseline, HTML/CSS/JS grew 2,336 raw / 572 gzip bytes to 420,436 / 118,597;
  the complete embedded payload is 864,672 / 556,014 using per-file gzip level
  9 with a zero timestamp and unchanged artwork. Plain/stripped binaries are
  20,682,240 / 14,535,168 bytes. Live-model, long-session, and real-device
  Autopilot acceptance remain open; no transport implementation changed.

- **2026-07-19** - Funscript import hardening and timeline repair: the Import
  tab now has compact keyboard-operable zoom/pan/fit controls, viewport-aware
  downsampling, fixed-size draggable action-snapped trim handles, precise
  subsecond/hour readouts, and a persistent selection-length value. Waveform,
  selection, and pointer mapping use one coordinate system; vertical wheel input
  zooms around the cursor, horizontal or Shift-wheel input pans, and a
  proportional pointer/keyboard scrollbar moves the viewport directly. Outward
  wheel input is released at zoom limits. Zoom state cannot alter trim state or
  submitted actions. Longer loop selections remain valid above the 6.6-second
  minimum; compact pattern curves insert saved knots into backend samples so
  uniform preview sampling cannot hide imported reversals. Selections above the
  255 essential-knot storage bound fail before upload. Browser and backend
  validation reject unknown schemas,
  malformed metadata, missing/out-of-range actions, oversized files, and
  mismatched names instead of silently repairing them; sources up to 20,480
  actions remain inspectable when trimmed to the 4,096-action backend limit.
  Finite program imports preserve all selected knots. All 159 frontend tests,
  typecheck/build, and `go test ./...` pass. Relative to the merged Import-tab
  baseline, HTML/CSS/JS grew 9,330 raw / 2,708 gzip bytes to 418,100 / 118,025;
  the complete embedded payload is 862,336 / 555,442 using the established
  per-file level-9 method and unchanged artwork. Plain/stripped pure-Go binaries
  are 20,634,112 / 14,497,792 bytes. No transport path changed; real-device feel
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
