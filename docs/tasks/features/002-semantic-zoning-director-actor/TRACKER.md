# Feature 002 — Semantic Zoning, Director/Actor, Bridge Filler — Task Tracker



> See [central TRACKER](../../TRACKER.md). Depends on [feature 001](../001-procedural-chat-consolidation/TRACKER.md) phase 1 complete.



## Vision



Separate LLM responsibility from physical math:



1. **LLM returns enums only** (`Action`, `Location`, `Intensity`) — no stroke percentages

2. **Go resolver** maps enums + user `MotionPreferences` → `[min, max]` stroke bounds

3. **Organic engine** (Perlin) generates motion inside those bounds

4. **Director & Actor** split cuts time-to-first-movement: hardware moves before dialogue streams

5. **Bridge filler** keeps hybrid procedural motion uninterrupted between chat turns (P0-B)



## Dependencies



- Feature 001 phase 1 ✅ (legacy cleanup, dual-stack docs, procedural `MotionTarget` mapping)

- Existing: `motion.OrganicConfig`, `OrganicConfigFromPhysics`, `manualqueue.Player`, `chatauto` bridge pattern

- ADR-0002 (semantic vs transport), ADR-0011 (layers), Rule 01/02/04



## Feature Status



| Phase | Description | Progress | Status |

|-------|-------------|----------|--------|

| Phase 0 | Procedural bridge filler (hybrid continuity) | 4/4 | Done |

| Phase 1 | Semantic resolver (prefs + LLMIntent + bounds) | 6/6 | Done |

| Phase 2 | Director & Actor (zero TTFP) | 5/5 | Done |

| Phase 3 | End-to-end integration + device sync | 4/4 | Done |



## Task Files



- [fase-0-bridge-filler-procedural.md](./fase-0-bridge-filler-procedural.md)

- [fase-1-semantic-resolver.md](./fase-1-semantic-resolver.md)

- [fase-2-director-actor-zero-latency.md](./fase-2-director-actor-zero-latency.md)

- [fase-3-integration-organic-pipeline.md](./fase-3-integration-organic-pipeline.md)



## Architecture (target)



```

User message

    → AskDirector (fast JSON / grammar) → LLMIntent

    → ResolveMotionBounds(intent, MotionPreferences) → min, max

    → OrganicConfig{StrokeMin, StrokeMax, ...}

    → manualqueue.Player (procedural) + bridge filler when idle risk

    → AskActor (stream, dynamic system prompt with intent) → SSE UI

```



## Task Summary



- [x] 0.1 — Extract bridge filler pattern from `chatauto` for hybrid procedural

- [x] 0.2 — Trigger filler when player timeline < 10s and no pending dispatch

- [x] 0.3 — Device sync test: no idle gap > 850ms during 15s hybrid session

- [x] 0.4 — Document bridge behavior in domain rule 05

- [x] 1.1 — `MotionPreferences` struct + SQLite persistence

- [x] 1.2 — `LLMIntent` struct + enum validation

- [x] 1.3 — `ResolveMotionBounds` with action overrides

- [x] 1.4 — Unit tests for resolver matrix

- [x] 1.5 — `OrganicConfigFromIntent` adapter

- [x] 1.6 — Map legacy `regiao` → new `Location` enums (compat layer)

- [x] 2.1 — `AskDirector` llama.cpp JSON-schema call

- [x] 2.2 — `AskActor` streaming with dynamic system prompt

- [x] 2.3 — Chat handler orchestration (director blocking → motion → actor goroutine)

- [x] 2.4 — Race-safe state updates (mutex / generation tokens)

- [x] 2.5 — SSE events: `intent` before `token` stream

- [x] 3.1 — Wire resolver into `playChatChaoticMotion`

- [x] 3.2 — Feature flag / settings toggle director mode

- [x] 3.3 — Device validation: TTFP < 500ms + uninterrupted 10s

- [x] 3.4 — Update domain rules + ADR if contract changes

