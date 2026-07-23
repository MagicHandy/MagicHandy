# AI-GUIDELINE — MagicHandy

> Consolidated index for AI agents. **Do not duplicate** full ADR content here — follow links.

---

## How to use

**Read order for every session:**

1. This file (skim)
2. [`docs/context.md`](docs/context.md) — vision, stack, ecosystem
3. [`docs/tasks/TRACKER.md`](docs/tasks/TRACKER.md) — what to work on now
4. Relevant ADRs in [`docs/adrs/README.md`](docs/adrs/README.md)
5. [`docs/code_guidelines/README.md`](docs/code_guidelines/README.md)
6. [`docs/domain_rules/README.md`](docs/domain_rules/README.md) — rules 01–03 when touching chat/motion; **rule 04 for all acceptance criteria**
7. Active feature phase in `docs/tasks/features/XXX-slug/fase-N-*.md`

**Implement only** when a task doc exists and dependencies are `[x]` complete.

---

## Vision (summary)

MagicHandy is a Go-first rewrite of StrokeGPT-ReVibed: local HTTP server, embedded React UI, local LLM chat, and Handy device motion via semantic engine + HSP transport. Goals: maintainability, single-binary distribution, lower baseline memory, race-tested concurrency.

---

## Architecture (layers)

```
httpapi (edge) → chat / chatauto / llm / modes → motion / manualqueue → transport → device
```

Import boundaries enforced by `internal/architecture/import_boundaries_test.go`.  
**Known exception:** procedural chat bypasses `motion.Engine` — see feature 001.

---

## ADR map

| ADR | Topic |
|-----|-------|
| 0001–0014 | [`docs/adrs/`](docs/adrs/) — all architectural decisions |

Full index: [`docs/adrs/README.md`](docs/adrs/README.md)

### Domain rules

| Rule | Topic |
|------|-------|
| [01](docs/domain_rules/01-motion-command-json-contract.md) | LLM motion JSON contract |
| [02](docs/domain_rules/02-procedural-vs-library-routing.md) | Procedural vs library routing |
| [03](docs/domain_rules/03-chat-auto-stamina.md) | Chat Auto stamina |
| [04](docs/domain_rules/04-real-device-sync-testing.md) | **Real device + sync testing (mandatory)** |

---

## Code guidelines

[`docs/code_guidelines/README.md`](docs/code_guidelines/README.md)

Key files: `go.md`, `typescript_react.md`, `architecture_layers.md`, `design_patterns.md`, `local_http_server.md`, **`testing.md`**

---

## Features (status)

| ID | Slug | Status |
|----|------|--------|
| 001 | procedural-chat-consolidation | Phase 1 done (3/8) |
| 002 | semantic-zoning-director-actor | Pending (0/19) |

Tracker: [`docs/tasks/TRACKER.md`](docs/tasks/TRACKER.md)

---

## Cursor integration

Always-on rule: [`.cursor/rules/magichandy-ai-guideline.mdc`](.cursor/rules/magichandy-ai-guideline.mdc) (`alwaysApply: true`). Cursor agents load this automatically and follow links to this file.

---

## Operational prompts (Portuguese BR)

| File | When |
|------|------|
| [`docs/prompts/DON'T READ/preprompt.criacao.tasks.md`](docs/prompts/DON'T%20READ/preprompt.criacao.tasks.md) | Planning |
| [`docs/prompts/DON'T READ/preprompt.executar.tasks.md`](docs/prompts/DON'T%20READ/preprompt.executar.tasks.md) | Implementation |
| [`docs/prompts/DON'T READ/preprompt.bugfixing.complexo.md`](docs/prompts/DON'T%20READ/preprompt.bugfixing.complexo.md) | Hard bugs |

---

## Task workflow

| Item | Convention |
|------|------------|
| Branch | `BEESCLO-XXXX` |
| Commit | `feat(TICKET): English description` |
| Feature dir | `docs/tasks/features/XXX-kebab-slug/` |
| Status | `[ ]` pending · `[~]` in progress · `[x]` done |

---

## Quick reference

### Run

```powershell
go run ./cmd/magichandy
# http://127.0.0.1:49717
```

### Test

```powershell
go test ./...          # structural precondition only
go test -race ./...
cd frontend && npm run test
```

**Acceptance:** every test session must use a **real Handy device** and include **synchronization analysis** (traces, playhead, buffer, handoffs). CI green alone is not sufficient. See [Rule 04](docs/domain_rules/04-real-device-sync-testing.md).

### Key endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Health |
| `GET /api/status` | Status |
| `POST /api/chat/stream` | Chat + motion |
| `GET /api/traces` | Diagnostics |

### Deep dives

- Procedural motion: [`docs/procedural-chat-motion-analysis.md`](docs/procedural-chat-motion-analysis.md)
- HSP invariants: [`docs/hsp-v4-invariants.md`](docs/hsp-v4-invariants.md)
- Roadmap: [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md)

---

## Universal prohibitions

- No implementation without `docs/tasks/features/` or `docs/tasks/fixes/` doc
- No implementation with pending tracker dependencies
- No architectural changes without ADR + explicit approval
- No invented API/library behavior
- No secrets in commits
- No duplicate utilities already in codebase
- **No marking tests complete without real device + sync analysis** (Rule 04)
