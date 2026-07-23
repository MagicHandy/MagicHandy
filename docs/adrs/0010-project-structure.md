# ADR-0010: Project Structure and Repository Layout

## Status

Accepted

## Context

MagicHandy is a monorepo with a Go core, embedded frontend assets, CLI tools, and extensive documentation. Contributors and AI agents need a single reference for where code belongs and what each top-level path owns.

## Decision

### Top-level layout

| Path | Purpose |
|------|---------|
| `cmd/magichandy/` | Main application entrypoint |
| `cmd/retarget-validate/` | Real-device retarget validation runner |
| `cmd/lso-import/` | Legacy StrokeGPT import utility |
| `internal/` | All non-exported application packages |
| `frontend/` | React SPA source (dev/build only) |
| `uibuild/dist/` | Built static assets embedded by Go (`//go:embed`) |
| `docs/` | ADRs, guidelines, tasks, domain docs |
| `scripts/` | Dev/setup scripts (e.g. `ensure_llama_cpp.ps1`) |
| `.github/workflows/` | CI (format, vet, lint, test, race, build) |

### Internal package map

| Package | Role |
|---------|------|
| `httpapi` | HTTP edge adapter — routes, JSON, SSE |
| `chat`, `chatauto` | Chat orchestration and autonomous sessions |
| `llm` | LLM provider adapters (llama.cpp, Ollama) |
| `motion` | Semantic motion engine and retargeting |
| `manualqueue` | HSP playback queue (procedural path) |
| `transport` | Handy cloud, fake, browser Bluetooth |
| `modes` | Freestyle and mode planners |
| `config`, `store`, `memory`, `library` | Settings and persistence |
| `diagnostics` | Trace ring and diagnostic bundles |
| `architecture` | Import-boundary and file-size enforcement tests |

### Rules

- No production code outside `cmd/` and `internal/`.
- Frontend source stays in `frontend/`; only `uibuild/dist/` is embedded.
- Documentation for architectural decisions goes to `docs/adrs/` only.
- Feature work is planned in `docs/tasks/features/` before implementation.

## Consequences

### Positive

- Clear separation between build artifacts and source.
- AI agents can locate packages without scanning the whole tree.
- Import-boundary tests (`internal/architecture/`) enforce the layout.

### Negative

- Frontend developers must run `npm run build` in `frontend/` before Go embed picks up changes.

### Neutral

- `IMPLEMENTATION_PLAN.md` at repo root remains the phase roadmap companion to ADRs.
- `docs/decisions/README.md` redirects stale links to `docs/adrs/`.
