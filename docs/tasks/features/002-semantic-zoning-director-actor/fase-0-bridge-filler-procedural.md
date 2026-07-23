# Phase 0 — Procedural Bridge Filler (Hybrid Continuity)

## Objective

Eliminate motion gaps in **hybrid procedural** chat when segment duration expires before the next user message or LLM dispatch (P0-B from analysis doc). Reuse patterns from Chat Auto bridge (`chat_auto.go` loopBridge, `ApplyStaminaForBridge`).

## Prerequisites

- Feature 001 phase 1 complete
- Rule 04 device sync test baseline: `TestProceduralSyncUninterruptedOnRealDevice`
- Read `internal/httpapi/chat_auto.go` — bridge when `remaining < 10s` and queue empty
- Read [`procedural-chat-motion-analysis.md`](../../../procedural-chat-motion-analysis.md) §18 P0-B

## Design

| Component | Responsibility |
|-----------|----------------|
| `chatChaosBridge` | Watches `chatChaos.player` timeline; schedules filler segments |
| Filler physics | Gentle `tipo_batida: fluido`, same `regiao` as last dispatch, low `velocidade` |
| Duration | ~30s extension (match Chat Auto bridge), `useRecover=false` |
| Stop conditions | User stop, new LLM dispatch, transport drop, explicit mode stop |

**Not in scope:** ModeChat keepalive recovery after transport drop (separate task / ADR update).

---

## Task 0.1 — Bridge watcher in chat chaos runtime

**Action:**

1. Add `chatChaos.bridge` goroutine or hook in existing player tick (prefer reusing `manualqueue` debug callback).
2. When `player.Running()` and `TimelineEndMS() - playheadMS < 10000` and no dispatch in flight → queue filler build.
3. Use `buildChaosSessionForDurationFromPosition` with last known physics snapshot.

**Acceptance criteria:**

- Filler only triggers once per gap (debounce)
- Trace row `chat_chaos_bridge_filler`
- Unit test with fake transport + shortened session

---

## Task 0.2 — Preserve regiao/intent across filler

**Action:**

1. Store `lastChaosPhysics` on `chatChaosRuntime` after each successful dispatch.
2. Filler reuses `lastChaosPhysics` with reduced `velocidade` (e.g. max(20, last-15)).
3. Append via `AppendExtension`, not player restart.

**Acceptance criteria:**

- `hsp_plays` count does not increment on filler-only extension
- Position stays in same zone band (device test)

---

## Task 0.3 — Device sync validation

**Action:**

1. Extend `TestProceduralSyncUninterruptedOnRealDevice` OR add `TestProceduralBridgeFillerDevice`:
   - Start procedural motion, wait until timeline < 12s without new dispatch
   - Assert bridge fires and `player_running` stays true for additional 5s
2. Record in TRACKER session log.

**Acceptance criteria:**

- `ActiveRatio >= 0.85` over 20s window including natural segment end
- `MaxIdleGap <= 850ms`
- Rule 04 evidence in PR/tracker

---

## Task 0.4 — Domain rule 05

**Action:** Create `docs/domain_rules/05-procedural-bridge-filler.md` documenting trigger, duration, stop conditions.

**Acceptance criteria:** Index updated in `domain_rules/README.md`

---

## Phase completion

- Update feature 002 TRACKER; link from feature 001 task 2.2 if deferred there
