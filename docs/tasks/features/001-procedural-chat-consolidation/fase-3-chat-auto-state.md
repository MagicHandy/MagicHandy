# Phase 3 — Chat Auto Stamina and Clock Unification

## Objective

Reduce stamina display drift and segment timing mismatch in Chat Auto (P1-G, P1-H).

## Prerequisites

- Phase 2 complete (or throttle fix merged independently)
- Read [`procedural-chat-motion-analysis.md`](../../../procedural-chat-motion-analysis.md) §14, §18 (P1-G, P1-H)
- Read `internal/httpapi/chat_auto.go` and `internal/chatauto/stamina.go`

---

## Task 3.1 — Unify stamina sources

**Action:**

1. Map all writes to `state.Stamina` vs `prepared.stamina` in chat auto loop.
2. Choose single source of truth for UI snapshot (`ChatAutoStatus` JSON).
3. Ensure bridge filler (`useRecover=false`) does not silently desync stamina.
4. Add/update tests in `stamina_test.go` and `chat_auto` handler tests.

**Acceptance criteria:**

- UI stamina matches player state within one tick interval under test scenarios
- No duplicate conflicting stamina fields in status snapshot without documentation

---

## Task 3.2 — Align segment clocks

**Action:**

1. Compare `segmentEndsAt` (wall-clock) vs `player.TimelineEndMS() - playheadMS` usage in prefetch and bridge triggers.
2. Prefer HSP timeline for playback decisions; use wall-clock only for LLM timeout/prefetch throttle.
3. Add test or trace assertion when bridge fires early/late.
4. Run `go test ./internal/chatauto/... ./internal/httpapi/... -run Auto -v`.

**Acceptance criteria:**

- Prefetch/bridge triggers documented with chosen clock source
- Test covers at least one bridge trigger path
- No regression in `roteiro_test.go`

---

## Phase completion

- Mark feature 001 complete in [feature TRACKER](./TRACKER.md) and [central TRACKER](../../TRACKER.md)
- Consider extracting domain rules `03-chat-auto-stamina.md`
