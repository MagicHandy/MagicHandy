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

## Snapshot — 2026-07-11, managed llama.cpp runtime branch

### Goal 1: Maintainability

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| CI gates | gofmt, vet, golangci-lint (staticcheck, funlen, gocyclo, depguard), test, race, `CGO_ENABLED=0` build on every PR | **Met** | `.github/workflows/test.yml`; `.golangci.yml` (funlen 100/60, gocyclo 20) |
| Import boundaries | chat/llm/modes never touch transport; nothing depends on httpapi; no CGo | **Met** | depguard rules + `internal/architecture` boundary tests |
| Size norms — Go core | no core file over ~600-800 lines | **Met** | All authored Go files remain under the 800-line advisory target. Runtime build state is isolated in a 447-line manager and a 244-line embedded PowerShell helper rather than added to the provider adapter or HTTP server. |
| Size norms — web | same norms for `web/` | **Met** | React TS/TSX and authored CSS modules remain under 800 lines. The 544-line model panel and 372-line model stylesheet are separate from the routed Settings shell and shared controls; `web/dist` remains the single shipped build. |
| Size-norm enforcement | norms surface as findings, not manual review | **Met** | `internal/architecture.TestSourceFileLineBudgets` enforces 800-line defaults for `cmd`, `internal`, and `web`; no grandfathered source-file override remains. |
| God-object avoidance | no single struct owning unrelated state | **Met** | Packages match the target architecture; library persistence/import/feedback live in `internal/patterns`, while the engine owns playback and completion. |
| Phase discipline | scoped PRs, tests, docs per phase | **Met** | Phases through 13.8 are merged by PR; the Phase 14 branch carries code, tests, rendered UI evidence, migrations, risk updates, and budget measurements together. |

### Goal 2: Core Memory

All numbers exclude Ollama/llama.cpp/CUDA/TTS/ASR per the measurement rules.
Full rows in `docs/perf-baseline.md`.

| Item | Target | Status | Evidence |
| --- | --- | --- | --- |
| Python baseline | measured before claims | **Met** | StrokeGPT-ReVibed core idle 524.75-524.81 MB (2026-07-01, commit `6c56985`) |
| Go core idle RSS | < 40 MB | **Violated (waived)** | Managed-runtime stripped build idles at 52.73 MiB after `/healthz`, close to the Phase 14 52.49 MiB sample and below Phase 11B's 54.13 MB, but still over the original target. The fixed pure-Go SQLite waiver remains; re-evaluate if idle climbs past ~60 MiB. |
| Go core active RSS | < 80 MB | **Met** | Model-manager reads settle at 53.40 MiB after repeated `/api/llm/models` calls. This is an API-read sample, not a new real-device active run; earlier real-device samples remain 16.75-16.76 MB Cloud REST and 17.52-17.53 MB Browser Bluetooth before SQLite. |
| Sustained soak | 1 h RSS within +20% of active baseline | **Met** | 18.41-20.16 MB over 56 warmed samples; +9.53% growth (2026-07-02) |

Risk R11 (goals unmeasured) is substantially closed for memory, with the Phase
11B SQLite idle-RSS waiver now explicit.

### Goal 3: Binary Releases

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| Pure-Go core | `CGO_ENABLED=0` build always works | **Met** | CI gate; depguard denies `C` |
| Binary size | < 30 MB | **Met** | Managed-runtime build: 18,822,656 bytes plain and 13,031,936 bytes stripped with `-ldflags "-s -w"`; still well below 30 MB. |
| Cold start to serving UI | < 500 ms | **At Risk** | 556 / 534 / 533 ms over 3 runs (client-side probe: spawn + poll `/healthz` at 10 ms granularity via PowerShell, which includes process and HTTP-client overhead). Re-measure with server-side timestamps in Phase 16 before judging. |
| Release pipeline | portable zip, versioning, release workflow | **Pending** | Phase 16 |

### Safety Gate: Motion Goroutine Lifecycle

| Item | Status | Evidence |
| --- | --- | --- |
| goleak in motion and transport `TestMain` | **Met** | `internal/motion/goleak_test.go`, `internal/transport/goleak_test.go` |
| Stop-teardown coverage | **Met** | engine stop/unhealthy-playback/concurrent tests; owner-switch stops motion (`controller_test.go`); server `Close()` stops the loop on shutdown |
| Race tests in CI | **Met** | `go test -race` gate (CI runs it with CGO on Ubuntu) |

### Real-Device Milestone (Motion Core)

| Item | Status | Evidence / Notes |
| --- | --- | --- |
| Engine retarget checklist on hardware | **Met** | Phase 7 via `cmd/retarget-validate` |
| Full app path — Cloud REST | **Met** | 2026-07-02: browser UI + chat against a real Handy; visible connection check (`HSP ready / 540 ms`), Start via UI, SSE visualizer running, deterministic chat stop (`docs/perf-baseline.md`, "Full App Path Evidence") |
| Full app path — Browser Bluetooth | **Met** | 2026-07-02: visible Edge Web Bluetooth flow selected `OHD_hw0_29b3243120f4`; visible Check connection reported `Connected: HSP ready / Unknown / 0 ms` without queuing `hsp/state`; visible Start motion at 28% sent `stroke_window`, `hsp_add`, and `hsp_play` with `browser_ack`; chat `stop` returned `Stopping motion.` and Stop ACKed; repeat UI Start/Stop captured active RSS samples (`docs/perf-baseline.md`, "Full App Path Evidence"). |
| Controller ownership + owner-switch semantics | **Met** | Phase 9B controller lease, read-only clients, stop-first owner switch, motion SSE (`docs/controller-dispatch-semantics.md`, PR #16) |

### Functional Parity (UI/UX vs StrokeGPT-ReVibed)

Tracked row by row in `docs/ui-design.md`, "Functional Parity Baseline".
Summary: original regression rows 1-9 are closed. Phase 14 also restores the
reference app's functional pattern browser, finite program/funscript player,
freehand authoring, and visible/reversible training feedback while keeping one
backend-authoritative preview and motion path.

## Watch List

Ranked by threat to the stated goals:

1. **Cold start at the boundary.** Probably measurement overhead, but nobody
   has proven that yet; treat 500 ms as unconfirmed until Phase 16 measures
   it server-side.
2. **Browser Bluetooth endurance.** The full short UI/chat path now passes, but
   Web Bluetooth still depends on an active Edge tab, user-driven pairing, and
   browser GATT stability. Do not treat the short run as a one-hour BLE soak.
3. **Feature growth vs binary/memory/browser budgets.** The model manager and
   managed-runtime flow raise the embedded UI from 80,533 to 85,718 gzip bytes
   (+5,185 / 6.4%) and the stripped binary from 12,766,208 to 13,031,936 bytes
   (+265,728 / 2.1%). Both remain within their budgets; re-measure after curated
   downloads and setup-wizard UI rather than assuming this growth stays flat.

## History

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
