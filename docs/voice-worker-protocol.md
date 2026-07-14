# Voice Worker Protocol (v1)

The wire contract between the MagicHandy core and an optional voice worker
process, per [ADR 0003](decisions/0003-voice-worker-boundary.md). Workers are
separate local executables in any language; the core owns lifecycle, queueing
policy, cancellation, timeouts, and status display. A missing or failed
worker never affects chat, settings, motion, transport, or diagnostics.

Authoritative Go definitions: `internal/voice/protocol` (wire format, a leaf
package with no core dependencies), `internal/voice` (core-side lifecycle and
queue), `internal/voice/stubworker` + `cmd/voice-stub-worker` (the reference
model-free implementation).

## Transport And Framing

- The core launches the worker only after the user enables voice and selects a
  provider. Built-in providers resolve their worker binary automatically with
  an advanced path override; custom providers use explicit path + args. Workers
  never start implicitly.
- Frames are NDJSON: exactly one JSON object per line. Requests arrive on the
  worker's stdin; responses leave on stdout. stderr is free-form logging —
  the core captures a bounded tail and shows it on crash.
- A frame must stay under 1 MiB. Real providers pass audio by reference
  (`audio_ref`), not inline base64; inline `audio_b64` exists for stubs and
  tests. Browser audio references point into a private process-session staging
  directory and are removed on every terminal path and core shutdown.
- Workers must ignore unknown JSON fields (forward compatibility). Version
  compatibility is decided once, at hello — never silently degraded.
- Logging on both sides excludes secrets and raw audio payloads.

## Session Lifecycle

1. Core sends `hello` with its protocol version; worker answers `hello` with
   its version, provider identity, role, and capabilities. A version or role
   mismatch ends the session (the core stops the process).
2. Core polls `health`; sends `load`/`unload` to manage the model; submits
   `speak`/`transcribe` work; cancels by request ID.
3. Core sends `shutdown` for a graceful stop, then kills the process after a
   grace period. A process exit that the core did not request is a crash:
   visible state, exit status, stderr tail, and all in-flight requests fail.

## Requests (core → worker)

Every request carries a `type` and a core-assigned, role-prefixed ID (for
example, `asr-3` or `tts-3`); responses echo it as `request_id`. The prefix keeps
the shared API request namespace collision-free while IDs remain opaque to
workers.

| `type` | Fields | Purpose |
| --- | --- | --- |
| `hello` | `protocol_version` | version/identity negotiation; must be first |
| `health` | — | state probe; answered with `health` |
| `load` / `unload` | — | model lifecycle; answered with `health` |
| `speak` | `text`, `voice`, optional `delay_ms` (stub) | TTS; streams `audio_chunk` frames, ends with `done` |
| `transcribe` | `audio_b64` or `audio_ref`, `audio_format`, optional `delay_ms` (stub) | ASR; ends with one `transcript` |
| `cancel` | `target_id` | cancel a queued or active request |
| `shutdown` | — | graceful stop; worker answers `done` and exits |

## Responses (worker → core)

| `type` | Fields | Terminal? |
| --- | --- | --- |
| `hello` | `protocol_version`, `provider`, `provider_version`, `role`, `capabilities` | yes |
| `health` | `model_state` (`unloaded`/`loading`/`ready`), `queue_depth` | yes |
| `audio_chunk` | `seq`, `audio_b64`, `audio_format` | no |
| `transcript` | `candidates` (`text` + `confidence`), or `rejected` | yes |
| `done` | — | yes |
| `canceled` | — | yes |
| `error` | `error.code`, `error.message`, `error.retryable` | yes |

Rules the stub demonstrates and tests enforce:

- **No empty transcripts.** Silence / no speech is a `transcript` with
  `rejected: "no_speech"` (or `low_confidence`) and no candidates — the core
  never forwards a rejected transcript to chat.
- **Model gating.** Work submitted while the model is unloaded fails with a
  retryable `model_not_loaded` error.
- **Cancellation is prompt.** Workers process control frames (`health`,
  `cancel`, `shutdown`) while work runs, and interrupt an active request with
  a `canceled` frame quickly — not after finishing the clip.

## Error Codes

`protocol_mismatch`, `invalid_request`, `model_not_loaded`,
`missing_dependency`, `canceled`, `timeout`, `internal`. Structured errors
terminate their request and stay in the core's voice request log — they never
enter chat history, TTS playback, or motion (ADR 0003).

## Core-Side Guarantees

- One serialized work request per worker; the core queue is bounded and rejects
  new work when full for catch-up flood protection.
- Per-request timeouts: handshake and ordinary control 5 s, work 60 s (then a
  cancel frame + a `timeout` failure). Model load honors a longer caller
  deadline (30 s for Start) because a managed local server may boot inside it.
- Status surfaces every lifecycle state (`disabled`, `not_configured`,
  `stopped`, `starting`, `running`, `crashed`) plus provider identity, model
  state, queue depth, last error, and the crash stderr tail — in
  `GET /api/voice/status`, `/api/state`, and the Settings → Voice UI.

## HTTP Surface

- `GET /api/voice/status` — both workers with a live health probe.
- `POST /api/voice/workers/{role}/start|stop|restart` — lifecycle (controller
  lease required). Start includes model load; on load failure the just-started
  adapter is stopped and the request fails, so success means ready to serve.
- `POST /api/voice/workers/{role}/model` — `{"loaded": bool}`.
- `POST /api/voice/workers/{role}/test` — `{"text": "...", "delay_ms": n}`;
  submits a stub-scale request, returns its ID.
- `GET /api/voice/requests/{id}` / `POST /api/voice/requests/{id}/cancel`.
- `GET /api/voice/requests/{id}/audio` — retained clip bytes; gated by the
  single-owner audio lease (the active controller), so two tabs never speak
  the same clip. Retention is bounded (per request and to the newest few
  requests).

## Trying It

Build the stub and point settings at it (Settings → Voice):

```powershell
go build -o voice-stub-worker.exe ./cmd/voice-stub-worker
# TTS worker path: <full path>\voice-stub-worker.exe, arguments: -role tts
```

Stub test knobs: `-start-loaded`, `-fail-start` (simulates a missing
dependency), `-advertise-protocol N` (mismatch testing), request `delay_ms`,
and the magic text `__stub_crash__` (mid-request crash).

## Relationship To Other Docs

- ADR 0003: the boundary, lifecycle, and delivery-ordering rules
- ADR 0007: the concrete engines that will implement this protocol (Phase 13)
- `IMPLEMENTATION_PLAN.md` Phase 12 (this substrate) and Phase 13 (providers,
  plus the shared-log/cursor/audio-lease delivery-ordering foundation)
- `docs/risk-register.md` R15
