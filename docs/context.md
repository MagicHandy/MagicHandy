# MagicHandy — Project Context

> **Audience:** AI agents and contributors. Read this before any implementation session.  
> **Last updated:** 2026-07-13 (AI-governed docs bootstrap)

---

## Vision

MagicHandy is a **Go-first ground-up rewrite** of StrokeGPT-ReVibed. It runs as a local HTTP server with an embedded React SPA, orchestrates local LLM chat, and drives Handy device motion through a semantic motion engine and HSP transport layer.

The rewrite exists to deliver: maintainable architecture, a shippable single-binary release, lower non-ML baseline memory, and explicit concurrency boundaries — without claiming Go alone fixes cloud latency or model memory.

---

## Ecosystem

| Component | Role | Relationship |
|-----------|------|--------------|
| **StrokeGPT-ReVibed** | Legacy Python app | Behavioral reference; parity target for motion/transport invariants |
| **Handy Cloud REST / HSP v4** | Remote device API | Primary transport for cloud-connected devices |
| **Browser Web Bluetooth** | Local dispatch owner | Alternative transport path (Edge/Chrome) |
| **llama.cpp** (managed) | Primary local LLM | External runner process; Go core manages lifecycle |
| **Ollama** | Secondary local LLM | Cross-platform compatibility path |
| **SQLite** (`magichandy.db`) | Persistence | Settings, memories, prompt sets, UI prefs |
| **Voice workers** (future) | ASR/TTS | Optional; behind worker boundary per ADR-0003 |

```
┌─────────────┐     HTTP/SSE      ┌──────────────────────────────────┐
│  Browser    │◄─────────────────►│  MagicHandy (Go)                 │
│  React SPA  │                   │  httpapi → chat/llm/modes        │
└─────────────┘                   │         → motion / manualqueue   │
                                  │         → transport → Handy/HSP  │
                                  └──────────┬───────────────────────┘
                                             │ HTTP (external)
                                  ┌──────────▼──────────┐
                                  │ llama.cpp / Ollama  │
                                  └─────────────────────┘
```

---

## Domain

High-level business rules live in feature docs and ADRs. Detailed numbered rules will be added to [`domain_rules/`](./domain_rules/README.md) as they are formalized.

Core domain concepts:

- **Semantic motion intent** — what the user/LLM wants (speed, region, pattern, action)
- **Physical transport output** — HSP timed points, stroke windows, reverse mapping
- **Operation modes** — `hybrid` (per-message chat), `auto` (autonomous Chat Auto session)
- **Motion generation** — `procedural` (math/physics) vs `library` (imported patterns)
- **Dispatch owners** — cloud REST, browser Bluetooth, fake transport (tests)

See also: [`hsp-v4-invariants.md`](./hsp-v4-invariants.md), [`procedural-chat-motion-analysis.md`](./procedural-chat-motion-analysis.md).

---

## Problem

StrokeGPT-ReVibed accumulated a large Python stack (~525 MB idle core), mixed semantic/transport concerns, and fragile long-running motion loops. Releases required a Python environment; motion regressions were hard to isolate.

Without MagicHandy:

- No single-binary distribution path
- No enforced import boundaries between motion, transport, and HTTP
- No race-tested concurrent transport scheduler
- Procedural chat motion and engine motion coexist without unified recovery semantics

---

## Solution

| Layer | Packages | Responsibility |
|-------|----------|----------------|
| **Entry** | `cmd/magichandy` | Process lifecycle, flags, graceful shutdown |
| **Edge** | `internal/httpapi` | HTTP routes, JSON encoding, SSE streams, embedded UI |
| **Orchestration** | `internal/chat`, `internal/chatauto`, `internal/llm`, `internal/modes` | Chat, autonomous sessions, LLM providers, mode planners |
| **Motion** | `internal/motion`, `internal/manualqueue` | Semantic engine, retargeting, HSP playback queue |
| **Transport** | `internal/transport` | Handy cloud, fake, browser Bluetooth, command shaping |
| **Persistence** | `internal/store`, `internal/memory`, `internal/library` | SQLite, user data |
| **Frontend** | `frontend/` → `uibuild/dist` | React SPA built and embedded by Go |

Key endpoints: `GET /healthz`, `GET /api/status`, `POST /api/chat/stream`, `POST /api/modes/start`, `GET /api/traces`.

---

## Out of Scope (this repo)

- Automatic model downloads (user-initiated only)
- CGo / libllama linked into the core binary
- Python as a core runtime dependency
- Voice workers implementation (Phases 12–13 — planned)
- Pattern library authoring UI (Phases 14–15 — planned)
- Release packaging (Phase 16 — planned)

---

## Stack

| Area | Technology |
|------|------------|
| Core language | Go 1.25+ (`CGO_ENABLED=0` for release builds) |
| Frontend | TypeScript 5, React 18, Vite 5, Vitest |
| HTTP | `net/http`, Gorilla WebSocket |
| Persistence | `modernc.org/sqlite` (pure Go) |
| LLM | Managed llama.cpp (primary), Ollama (secondary) |
| Logging | `log/slog` structured JSON |
| CI | gofmt, go vet, golangci-lint, `go test`, race tests |
| Tests | Structural: `go test ./...`, Vitest — **Acceptance: real device + sync analysis** ([Rule 04](./domain_rules/04-real-device-sync-testing.md)) |

---

## References

| Document | Purpose |
|----------|---------|
| [ADRs index](./adrs/README.md) | Architectural decisions (0001–0014) |
| [domain_rules/](./domain_rules/README.md) | Numbered business rules (01–04) |
| [code_guidelines/](./code_guidelines/README.md) | Implementation rules |
| [tasks/TRACKER.md](./tasks/TRACKER.md) | Central task tracker |
| [goals-and-guardrails.md](./goals-and-guardrails.md) | Measurable targets (memory, binary size) |
| [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) | Phase roadmap |
| [procedural-chat-motion-analysis.md](./procedural-chat-motion-analysis.md) | Deep dive on procedural chat motion |
| [hsp-v4-invariants.md](./hsp-v4-invariants.md) | Executable transport invariants |
| [perf-baseline.md](./perf-baseline.md) | Memory/performance measurements |
| [model-management.md](./model-management.md) | LLM model handling |
| [AI-GUIDELINE.md](../AI-GUIDELINE.md) | Consolidated index for AI agents |
