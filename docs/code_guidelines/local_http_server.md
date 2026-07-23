# Local HTTP Server Guidelines — MagicHandy

## Server overview

- Default bind: `127.0.0.1:49717` (override with `-addr`)
- Package: `internal/httpapi`
- Serves embedded SPA from `uibuild/dist` for non-API routes
- JSON API under `/api/*`
- Health: `GET /healthz`

## Handler conventions

1. Register routes in `Server` mount methods (grouped by domain: chat, motion, settings, llm, modes)
2. Use `decodeJSON(r, &req)` for POST/PUT bodies
3. Use `writeJSON(w, status, payload)` for success
4. Use `writeError(w, status, err)` for failures (see ADR-0013)
5. Set streaming headers before writing SSE bodies; check `http.Flusher`

## Authentication

Local-only app: no user auth middleware. Handy connection key is a **device credential** stored in settings, not session auth.

## Redaction

`GET /api/settings` and `GET /api/state` return redacted views. Never add handlers that dump raw settings without redaction.

## CORS

Not required for default embedded UI (same origin). If adding external clients, document in an ADR first.

## Main endpoints (reference)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Liveness |
| GET | `/api/status` | App status snapshot |
| GET | `/api/state` | Motion + session state |
| GET/PUT | `/api/settings` | Settings CRUD |
| POST | `/api/chat/stream` | Streaming chat + motion dispatch |
| POST | `/api/modes/start` | Freestyle / chat keepalive |
| GET | `/api/traces` | Diagnostic trace ring |
| POST | `/api/llm/load` | Load local model |

Full list: grep `HandleFunc` / `Handle(` in `internal/httpapi/`.

## Testing handlers

```powershell
go test ./internal/httpapi/... -run TestChat -v
```

Use `httptest.NewRecorder` and inject fake transport via test `Server` setup. See `server_test.go`, `chat_test.go` for patterns.

## Graceful shutdown

`cmd/magichandy/main.go` traps signals and shuts down HTTP server with timeout. Long-running chat auto loops must respect `context.Context` cancellation.
