# Phase 1 — Legacy Cleanup and Documentation Alignment

## Objective

Remove confusion from the procedural chat dual-stack by deleting or isolating dead code and aligning docs/contracts with the actual runtime path (`manualqueue.Player` + HSP).

## Prerequisites

- Read [ADR-0011](../../../adrs/0011-layered-architecture.md)
- Read [`procedural-chat-motion-analysis.md`](../../../procedural-chat-motion-analysis.md) §1, §18 (P0-A, P1-A, P1-B)
- Read [architecture_layers.md](../../../code_guidelines/architecture_layers.md)

---

## Task 1.1 — Isolate legacy chaotic waypoints generator

**Action:**

1. Confirm `GenerateChaoticWaypoints` in `internal/motion/chaos_waypoints.go` has no production callers (grep the repo).
2. If only tests reference it, move to `_test.go` helper or mark file with `//go:build test` / test-only package split per Go conventions.
3. Keep `GenerateStrokeWaypointsFromPosition` as the sole production entry in `stroke_waypoints.go`.
4. Run `go test ./internal/motion/...`.

**Acceptance criteria:**

- No production import of `GenerateChaoticWaypoints`
- All motion package tests pass
- `chaos_waypoints_test.go` still validates math if kept as test reference
- **Real device:** procedural dispatch observed; traces reviewed for sync (Rule 04)

---

## Task 1.2 — Update dual-stack documentation

**Action:**

1. Update `internal/chat/contract.go` package/comments to state:
   - Engine path: library / semantic retarget
   - Procedural path: `manualqueue.Player` + HSP (bypasses engine)
2. Add a short "Motion paths" subsection to `README.md` linking to `procedural-chat-motion-analysis.md`.
3. Do not claim all chat motion goes through `motion.Engine`.

**Acceptance criteria:**

- README and contract comments match analysis doc §1
- No contradictory "single stack" language in touched files

---

## Task 1.3 — Align MotionTargetFromCommand for procedural

**Action:**

1. Review `internal/chat/motion_target.go` — `proceduralMotionTarget` and `MotionTargetFromCommand`.
2. Map `velocidade`, `regiao`, `tipo_batida` from `MotionCommand` when `motion_generation_mode` is procedural, or document why `MotionTarget` is intentionally unused on that path.
3. If mapping is added, add unit tests in `motion_target_test.go` or extend `motion_generation_test.go`.
4. Run `go test ./internal/chat/...`.

**Acceptance criteria:**

- Callers consulting `MotionTarget` for procedural get consistent data OR explicit `ErrNotApplicable` / documented no-op
- Tests cover procedural command → target mapping (or explicit skip with comment in ADR/feature tracker)

---

## Phase completion

- Mark tasks 1.1–1.3 in [feature TRACKER](./TRACKER.md)
- Update [central TRACKER](../../TRACKER.md) progress to 3/8 if all done
