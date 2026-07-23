# Design Patterns — MagicHandy

Patterns already in use — prefer extending these over introducing new frameworks.

## Provider (LLM)

`llm.Provider` interface with implementations:

- `managed_llama_cpp` — spawns and monitors local `llama-server`
- `llama_cpp` — external OpenAI-compatible server
- Ollama — native chat API

**Usage:** `httpapi` selects provider from settings; chat packages call interface methods only.

## Adapter (Transport)

`transport.Transport` abstracts:

- Cloud REST + HSP (`cloud_*.go`)
- Fake (`fake.go`) for tests
- Browser Bluetooth bridge (`browser_bluetooth.go`)

Motion and manualqueue depend on the interface, not cloud specifics.

## Edge adapter (HTTP)

`httpapi.Server` adapts HTTP JSON/SSE to domain services. Handlers are thin:

1. Decode request
2. Call service / engine
3. Map error to `writeError` or stream event

## Queue / Player (Procedural)

`manualqueue.Player` plays `[]transport.TimedPoint` as HSP sessions:

- `Start` / `Stop` / `AppendExtension`
- `Continuous` mode for Chat Auto
- Buffer-ahead and starvation detection

Do not reimplement HSP batching in handlers.

## Scene director / roteiro (Chat Auto)

`chatauto` maps LLM `Intent` → motion segments via `MapIntent`, stamina, and prefetch queue. State machine spans `httpapi/chat_auto.go` and `chatauto/*`.

## Trace ring (Diagnostics)

`diagnostics.TraceRing` records structured motion/transport decisions for `GET /api/traces`. New motion paths must emit trace rows for debuggability.

## Settings redaction

API views use redacted settings structs — secrets stripped before JSON encode. Pattern: load full settings internally, redact for `GET` responses.

## When to add a new pattern

Only with an ADR if it changes layer boundaries or introduces a new global singleton.
