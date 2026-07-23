# Task Tracker — MagicHandy

> **Central control point.** Consult at the start of every session.

## Legend

- `[ ]` Pending · `[~]` In progress · `[x]` Done · `[-]` Cancelled

## Overall Status

| Feature/Fix | Description | Progress | Status |
|-------------|-------------|----------|--------|
| [001-procedural-chat-consolidation](./features/001-procedural-chat-consolidation/TRACKER.md) | Unify procedural stack, remove legacy paths | 3/8 | Phase 1 done |
| [002-semantic-zoning-director-actor](./features/002-semantic-zoning-director-actor/TRACKER.md) | Zoning resolver, Director/Actor, bridge filler | 19/19 | Done |

## Completed phases (reference)

Phases 0–11B from [`IMPLEMENTATION_PLAN.md`](../../IMPLEMENTATION_PLAN.md) are complete on `main`. New work is tracked here as features/fixes.

## Session Log

### Session — 2026-07-13 (procedural sync device validation)

- **Activities:** Executed `TestProceduralSyncUninterruptedOnRealDevice` against real Handy via cloud REST; SQLite key from `.local-data/magichandy.db` (`MAGICHANDY_DATA_DIR`).
- **Sync evidence:**
  - Single HSP play stream (`hsp_plays=1`) across start + 2 chained `target` dispatches — append path, no transport restart
  - Core window: `ActiveRatio=1.0`, `MaxIdleGap=0`, `starvation_events=0`, `transport_errors=0`
  - Position range ~86% — physical motion confirmed
  - Max HSP point delta ~194ms (fluido pacing, within latency budget)
- **Command:** `go test -tags=integration ./internal/httpapi -run TestProceduralSyncUninterruptedOnRealDevice -v -timeout 5m` (set `MAGICHANDY_DATA_DIR` if key not in AppData)
- **Next steps:** Feature 001 phases 2–3 (throttle fix, chat auto stamina unification)

### Session — 2026-07-13 (bridge gap-zero + turbo/vibrate restore)

- **Fix:** Restored `buildVibrateStroke` turbo interpolation; bridge preserves turbo tipo, debounce 400ms + emergency at 800ms remaining, reset debounce on dispatch
- **Test:** `active_ratio=1.0`, `max_idle=0s`, vibrate/turbo `min_hsp_delta=1ms`; cabeca vibrate pos 73..96 (was ~1..56 with organic wave)

- **Test:** `TestChatContinuousSyncOnRealDevice5Min` — deterministic 9×7 matrix (lento → turbo/vibrate 1ms × todas as zonas), foco em movimento contínuo
- **Steps:** 61/63 (budget 5min atingido antes de `turbo_cabeca_base` e `turbo_full`); todos os 9 `tipo_batida` e 7 `regiao` exercitados
- **Sync evidence:** `active_ratio=0.963`, `position_range=99.0%`, `hsp_adds=516`, `starvation=0`, `max_idle=3.0s`
- **Warnings:** posição `cabeca` fora do esperado em 4 steps (lento/simples/vibrate/turbo); `max_idle_gap` acima do ideal 850ms mas dentro da tolerância 45s
- **Command:** `go test -tags=integration ./internal/httpapi -run TestChatContinuousSyncOnRealDevice5Min -v -timeout 12m`

### Session — 2026-07-13 (5-minute chat continuous device test — LLM)

- **Test:** mesma função com turnos LLM (Qwen2.5-7B-Instruct via llama.cpp)
- **Chat turns:** 6 (legacy procedural + director mode handjob/tip + riding/full)
- **Sync evidence:** `active_ratio=0.828`, `position_range=88.9%`, `hsp_adds=463`, `starvation=0`, `hsp_plays=3`
- **Warning:** `max_idle_gap=28.6s` between segment handoffs — bridge filler needs tuning for chat gaps


- **Activities:** SQLite persistence hooks for `MotionPreferences`; `BoundsFromRegiao` legacy compat; `MotionTargetFromCommandWithPreferences`; settings migration defaults; phase 1 tracker closed
- **Tests:** `go test ./internal/motion/semantic/... ./internal/chat/... ./internal/config/... -count=1` PASS

### Session — 2026-07-13 (feature 001 phase 1)

- **Activities:** Legacy generator isolated to tests; dual-stack docs; procedural `MotionTarget` maps regiao/velocidade; feature 002 planned (bridge + zoning + Director/Actor)
- **Tests:** `go test ./internal/motion/... ./internal/chat/... -run "MotionTarget|Chaotic"` PASS
