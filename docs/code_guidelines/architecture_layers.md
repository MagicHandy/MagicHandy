# Architecture Layers — MagicHandy

See ADR-0011 for the authoritative decision. This file is the implementer's quick reference.

## Where to put new code

| If you are building… | Put it in… | May import… |
|---------------------|------------|-------------|
| HTTP route or SSE handler | `internal/httpapi` | Any internal package |
| Chat turn / motion dispatch | `internal/chat` | `motion`, `config`, `llm` — **not** `transport` |
| Chat Auto loop / roteiro | `internal/chatauto` | `chat`, `config` — **not** `transport` |
| LLM provider adapter | `internal/llm` | stdlib, HTTP client only |
| Semantic motion / retarget | `internal/motion` | `transport` types only via interfaces at playback boundary |
| HSP playback queue | `internal/manualqueue` | `transport` for dispatch |
| Handy API client | `internal/transport` | stdlib, HTTP — **not** `motion` or `chat` |
| Settings / SQLite | `internal/store`, `internal/config` | infrastructure only |
| React page or hook | `frontend/src` | `api/`, `hooks/`, `contexts/` |

## Composition root

`internal/httpapi.Server` wires dependencies from `Runtime`:

- `MotionTransport transport.Transport`
- `LLMProvider llm.Provider`
- Motion engine, chat services, mode manager

Handlers orchestrate; they should not contain motion math or HSP encoding.

## Known exception: procedural chat

```
httpapi (chat_chaos, chat_auto)
  → internal/motion (waypoint generation)
  → internal/manualqueue (Player)
  → transport (HSP)
```

This bypasses `motion.Engine`. Feature 001 targets consolidating telemetry, stop/pause, and recovery semantics across stacks.

## Verification

```powershell
go test ./internal/architecture/... -v
```

Any new package under `internal/` is automatically checked against boundary rules.
