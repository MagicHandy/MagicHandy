# ADR-0013: Error Handling and HTTP Mapping

## Status

Accepted

## Context

The HTTP API serves both the embedded UI and diagnostic tools. Errors must be consistent, safe (no secret leakage), and map domain failures to appropriate status codes.

## Decision

### HTTP error envelope

All handler errors use `writeError` in `internal/httpapi/server.go`:

```json
{ "error": "<message>" }
```

Status codes follow conventional mapping:

| Condition | HTTP status | Example |
|-----------|-------------|---------|
| Malformed JSON / validation | 400 | Invalid `offset_ms`, bad motion command |
| Missing auth / connection key | 401 / 403 | Cloud transport without key |
| Resource not found | 404 | Unknown prompt set ID |
| Domain conflict / wrong state | 409 | Mode already running |
| Dependency unavailable | 503 | Motion engine not ready, LLM not loaded |
| Internal failure | 500 | Unexpected panic recovery, stream setup failure |

### Domain errors

- Use `errors.New` / `fmt.Errorf` with stable messages in domain packages.
- Wrap with context at the edge: `fmt.Errorf("dispatch chat motion: %w", err)`.
- Do not expose stack traces or internal paths in JSON responses.
- Log structured details with `slog` at the handler or service boundary.

### Streaming endpoints

- SSE/chat streams signal malformed LLM JSON via UI flags (repair pass, then user-visible indication).
- Transport errors during motion may be recorded in `GET /api/traces` without aborting the HTTP connection when safe.

### Chat Auto errors

- `setChatAutoError` publishes session-level errors to the auto status snapshot.
- Autonomous loop should recover or stop gracefully; never leave the player in an undefined state.

## Consequences

### Positive

- Frontend can rely on `{ error: string }` for non-streaming routes.
- Redaction rules (ADR-0012) apply before any error body is written.

### Negative

- No error code enum yet; clients parse English message strings (acceptable for local-only app).

### Neutral

- Device transport errors are also surfaced through diagnostics/traces for debugging.
