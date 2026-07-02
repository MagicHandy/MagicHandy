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

## Snapshot — 2026-07-02, Phase 9B live-validation follow-up branch

### Goal 1: Maintainability

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| CI gates | gofmt, vet, golangci-lint (staticcheck, funlen, gocyclo, depguard), test, race, `CGO_ENABLED=0` build on every PR | **Met** | `.github/workflows/test.yml`; `.golangci.yml` (funlen 100/60, gocyclo 20) |
| Import boundaries | chat/llm/modes never touch transport; nothing depends on httpapi; no CGo | **Met** | depguard rules + `internal/architecture` boundary tests |
| Size norms — Go core | no core file over ~600-800 lines | **Met** | Browser Bluetooth was split into bridge and transport files; all Go source files are now under the automated 800-line budget. `transport/cloud_client.go` remains in the gray zone at ~645 lines; split when next behavior change touches it. |
| Size norms — web | same norms for `web/` | **Met** | BLE session extracted to `web/bluetooth-ui.js`; current major JS modules are `app.js` 620, `bluetooth-ui.js` 592, `motion-ui.js` 448, `chat-ui.js` 379. |
| Size-norm enforcement | norms surface as findings, not manual review | **Met** | `internal/architecture.TestSourceFileLineBudgets` enforces 800-line defaults for `cmd`, `internal`, and `web`; no grandfathered source-file override remains. |
| God-object avoidance | no single struct owning unrelated state | **Met** | Packages match the target architecture; largest structs are scoped (bridge, engine, server). Re-check when modes land. |
| Phase discipline | scoped PRs, tests, docs per phase | **Met** | 17 PRs, one scope each; docs updated in the same PR as behavior. |

### Goal 2: Core Memory

All numbers exclude Ollama/llama.cpp/CUDA/TTS/ASR per the measurement rules.
Full rows in `docs/perf-baseline.md`.

| Item | Target | Status | Evidence |
| --- | --- | --- | --- |
| Python baseline | measured before claims | **Met** | StrokeGPT-ReVibed core idle 524.75-524.81 MB (2026-07-01, commit `6c56985`) |
| Go core idle RSS | < 40 MB | **Met** | 8.96 MB — ~1/58th of the Python core |
| Go core active RSS | < 80 MB | **Met** | 16.75-16.76 MB (real Cloud REST motion + SSE + chat stop, 2026-07-02) |
| Sustained soak | 1 h RSS within +20% of active baseline | **Met** | 18.41-20.16 MB over 56 warmed samples; +9.53% growth (2026-07-02) |

Risk R11 (goals unmeasured) is substantially closed for memory.

### Goal 3: Binary Releases

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| Pure-Go core | `CGO_ENABLED=0` build always works | **Met** | CI gate; depguard denies `C` |
| Binary size | < 30 MB | **Met (early)** | Measured 2026-07-02 @ `f5441ba`: 10.59 MB plain, 7.50 MB with `-trimpath -ldflags "-s -w"`. Will grow with features; re-measure each phase. |
| Cold start to serving UI | < 500 ms | **At Risk** | 411 / 518 / 522 ms over 3 runs (client-side probe: spawn + poll `/healthz` at 10 ms granularity via PowerShell, which inflates the number). Sits at the boundary; re-measure with server-side timestamps in Phase 16 before judging. |
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
| Full app path — Browser Bluetooth | **Unmeasured (blocked after BLE connect)** | 2026-07-02 follow-up reached Edge chooser selection and a ready `OHD_hw0_29b3243120f4` bridge. A non-moving Bluetooth Stop command ACKed in 102 ms. Full motion still did not complete: the first capped app-path start exposed a semantic stream-ID mapping bug, then later retests exposed dead command-loop recovery and shared Bluetooth client IDs across tabs; those are now patched. The remaining live blocker is the Edge/device GATT link dropping or reporting `hsp/state` timeout before the start sequence can run (`docs/perf-baseline.md`, "Full App Path Evidence"). |
| Controller ownership + owner-switch semantics | **Met** | Phase 9B controller lease, read-only clients, stop-first owner switch, motion SSE (`docs/controller-dispatch-semantics.md`, PR #16) |

### Functional Parity (UI/UX vs StrokeGPT-ReVibed)

Tracked row by row in `docs/ui-design.md`, "Functional Parity Baseline".
Summary: rows 1-4, 6, 8 (backend-loss banner + control lock, scrollback
stickiness, visible connection check, estimate labeling, copyable
diagnostics, Esc hint) closed by the Phase 9B UI pass; row 5 (pause/resume)
is Phase 11; row 7 (reset to defaults) is Phase 10; row 9 (server-side chat
continuity) is Phase 12.

## Watch List

Ranked by threat to the stated goals:

1. **Bluetooth app-path validation (blocked after BLE connect).** Windows/Edge
   can now select the `OHD` device and the browser bridge can ACK a non-moving
   Stop command. Full motion/chat validation remains open because the live GATT
   link drops or reports `hsp/state` timeout before the capped start sequence
   can complete.
2. **Cold start at the boundary.** Probably measurement overhead, but nobody
   has proven that yet; treat 500 ms as unconfirmed until Phase 16 measures
   it server-side.
3. **Feature growth vs binary/memory budgets.** Voice workers, pattern
   libraries, and the model manager all add weight; re-measure size and
   active RSS at each phase completion so growth is a trend line, not a
   surprise.

## History

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
