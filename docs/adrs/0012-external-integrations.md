# ADR-0012: External Integration Pattern

## Status

Accepted

## Context

MagicHandy integrates with Handy Cloud REST, browser Web Bluetooth, local LLM runners (llama.cpp, Ollama), and (future) voice workers. Each integration has different failure modes, credential handling, and timeout requirements.

## Decision

### Integration inventory

| Service | Client location | Auth | Timeout strategy |
|---------|-----------------|------|------------------|
| Handy Cloud REST | `internal/transport/cloud_*.go` | Connection key in settings (redacted on read) | Per-request via `http.Client` on `Server.Runtime` |
| Browser Bluetooth | `internal/transport/browser_bluetooth.go` | Browser session | Bridge callbacks; no server-side timeout |
| Managed llama.cpp | `internal/llm/managed_llama_cpp.go` | None (local process) | Process lifecycle + HTTP to runner |
| External llama.cpp | `internal/llm/llama_cpp.go` | Optional API key | Configurable base URL |
| Ollama | `internal/llm/provider.go` | None (local) | Native `/api/chat` endpoint |
| Intiface (optional) | `internal/transport/intiface` | Local WebSocket | Connection-scoped |

### Patterns

1. **Inject clients** — `httpapi.Runtime` holds `CloudHTTPClient`, `LLMHTTPClient`, and provider interfaces. Handlers do not construct `http.Client` ad hoc.
2. **Redact secrets** — Connection keys and tokens never appear in `GET /api/settings`, traces, or logs. Use `<set-locally>` in docs.
3. **Explicit load/unload** — LLM models are loaded via `POST /api/llm/load`; the core does not auto-download GGUF files.
4. **Fake transport for tests** — `internal/transport/fake` implements the same interface as cloud transport for CI and unit tests.
5. **No invented API shapes** — HSP command structure follows `docs/hsp-v4-invariants.md` and real-device traces.

### Credential storage

- Settings persisted in SQLite (`magichandy.db` under OS config dir or `-data-dir`).
- Legacy JSON files imported once and renamed `*.migrated`.

## Consequences

### Positive

- Integrations are swappable behind interfaces (`transport.Transport`, `llm.Provider`).
- Tests run without network or device when using fake transport and mock LLM.

### Negative

- Each new external service needs an explicit client package and ADR note if it crosses trust boundaries.

### Neutral

- Voice workers (ADR-0003) will follow the same inject-and-boundary pattern when implemented.
