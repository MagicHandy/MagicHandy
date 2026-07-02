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

## Snapshot — 2026-07-02, `main` @ `f5441ba` (post Phase 9B PRs #15-#17)

### Goal 1: Maintainability

| Item | Target | Status | Evidence / Notes |
| --- | --- | --- | --- |
| CI gates | gofmt, vet, golangci-lint (staticcheck, funlen, gocyclo, depguard), test, race, `CGO_ENABLED=0` build on every PR | **Met** | `.github/workflows/test.yml`; `.golangci.yml` (funlen 100/60, gocyclo 20) |
| Import boundaries | chat/llm/modes never touch transport; nothing depends on httpapi; no CGo | **Met** | depguard rules + `internal/architecture` boundary tests |
| Size norms — Go core | no core file over ~600-800 lines | **At Risk** | `transport/browser_bluetooth.go` 875 (over cap), `transport/cloud_client.go` 645 (gray zone). Split when next touched. |
| Size norms — web | same norms for `web/` | **Violated** | `web/app.js` **1120 lines and growing** (870 at Phase 9; +250 from the 9B parity pass). The planned BLE-session extraction has not happened. Owner: Phase 9B close-out. |
| Size-norm enforcement | norms surface as findings, not manual review | **Violated** | No automated file-length check exists; the `app.js` drift happened silently. Recommended: a file-size test in `internal/architecture` with a grandfathered ceiling so files can only shrink. |
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
| Full app path — Browser Bluetooth | **Unmeasured (blocked on BLE visibility)** | Chromium requires a real chooser selection, but the 2026-07-02 Edge/Windows session saw no selectable `OHD`/Handy device: DevTools `DeviceAccess` returned empty chooser lists, a visible manual Edge page never connected, and Windows BLE advertisement scanning saw zero advertisements (`docs/perf-baseline.md`, "Full App Path Evidence"). |
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

1. **`web/app.js` size drift (Violated).** The only guardrail currently being
   violated, and it regressed further during the phase that was supposed to
   fix it. Extract the BLE session code and add the automated file-size check
   before Phase 10 UI work makes it worse.
2. **Bluetooth app-path validation (blocked on BLE visibility).** Everything
   else in Phase 9B is done; the next session needs Windows/Chromium to see the
   `OHD`/Handy advertisement, then a real chooser selection can finish the UI
   and chat validation.
3. **Cold start at the boundary.** Probably measurement overhead, but nobody
   has proven that yet; treat 500 ms as unconfirmed until Phase 16 measures
   it server-side.
4. **Feature growth vs binary/memory budgets.** Voice workers, pattern
   libraries, and the model manager all add weight; re-measure size and
   active RSS at each phase completion so growth is a trend line, not a
   surprise.

## History

- **2026-07-02** — Initial scorecard @ `f5441ba`. Memory goal fully measured
  and met (idle 8.96 MB, active 16.76 MB, soak +9.53%, Python baseline
  525 MB). Binary 10.59 MB / cold start 411-522 ms measured ad hoc. Cloud
  REST app path validated on hardware; Bluetooth later refined from "manual
  gesture needed" to "BLE visibility needed" after Edge/Windows saw no
  selectable `OHD`/Handy advertisement. Size norms violated by `web/app.js`
  (1120); no automated size enforcement yet.
