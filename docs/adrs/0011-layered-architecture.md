# ADR-0011: Layered Architecture and Import Boundaries

## Status

Accepted

## Context

StrokeGPT-ReVibed mixed HTTP handlers, motion math, and transport encoding. MagicHandy enforces layers so semantic intent, motion planning, and device commands remain testable in isolation. `internal/architecture/import_boundaries_test.go` encodes these rules as executable tests.

## Decision

### Layer model

```
┌─────────────────────────────────────────┐
│  httpapi (edge adapter)                 │
├─────────────────────────────────────────┤
│  chat · chatauto · llm · modes          │  ← orchestration (no transport import)
├─────────────────────────────────────────┤
│  motion · manualqueue                   │  ← semantic motion (no httpapi/chat/llm)
├─────────────────────────────────────────┤
│  transport                              │  ← device encoding (no motion/chat/httpapi)
├─────────────────────────────────────────┤
│  config · store · diagnostics · logging │  ← shared infrastructure
└─────────────────────────────────────────┘
```

### Enforced import rules

| Rule | Packages affected | Forbidden imports |
|------|-------------------|-------------------|
| Edge isolation | All except `httpapi` | `internal/httpapi` |
| Orchestration purity | `chat`, `llm`, `modes` | `internal/transport` |
| Motion independence | `motion` | `chat`, `httpapi`, `llm`, `modes` |
| Transport bottom | `transport` | `chat`, `httpapi`, `llm`, `modes`, `motion` |

### Semantic vs transport (ADR-0002)

- LLM and mode planners produce **semantic targets** only.
- `motion.Engine` samples plans and retargets active streams.
- `manualqueue.Player` encodes procedural timelines as HSP batches.
- `transport` serializes commands and owns device recovery state.

### Procedural chat exception

Procedural chat motion (`chat_chaos`, `chat_auto`) bypasses `motion.Engine` and uses `manualqueue.Player` directly. This is a known dual-stack (see feature 001). New work should converge stacks, not add a third path.

## Consequences

### Positive

- Regressions from "handler called Handy API directly" are structurally impossible.
- Packages can be unit-tested without HTTP or device mocks when boundaries are respected.
- Race tests target transport and motion loops independently.

### Negative

- Procedural path legitimately crosses layers via `httpapi` → `manualqueue` → `transport`, requiring explicit documentation and tests.
- Adding a new cross-cutting concern (e.g. unified telemetry) needs an ADR update.

### Neutral

- `httpapi` may import any internal package; it is the composition root for HTTP.
