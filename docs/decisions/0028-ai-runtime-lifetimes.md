# ADR 0028: AI runtime ownership, bounded prompts and progressive speech

Date: 2026-09-08

Status: Proposed for PR review

## Context

The AI module audit found lifecycle races shared across providers: a canceled
voice request could become complete again, a retired managed LLM handle could
restart its old model, and an installer could replace packages used by a live
Python process. Full-utterance buffering also hid the local speech servers'
streaming benefit. Individually valid saved memories could collectively exceed
the selected model's prompt capacity.

## Decision

- Treat managed LLM `Close` as permanent retirement and `Unload` as reusable.
  Chat resolves its provider after coordinator admission. Settings transitions
  cancel the old generation and hold admission closed through retirement.
  Read-only status must not replace a runtime used by a lab model override.
- Keep all voice terminal transitions under the request mutex. Cancellation
  invalidates retained audio, queued work and blocked stdin writes. Deadlines
  include write admission and pipe I/O; a wedged pipe retires the owned session.
  Share a small bounded job tracker between the three real Go adapters, without
  moving provider-specific readiness or synthesis into the core.
- Admit LLM input with a conservative UTF-8 byte budget, reserving output,
  reasoning when bounded, schema/framing and repair overhead. Select complete
  memories by lexical relevance then recency, with a 6 KiB aggregate cap that
  shrinks for small contexts. Remove oldest history when necessary; retain all
  system messages and the current exchange, including a repair's original user
  turn. Fail an oversized essential prompt explicitly. Do not mutate storage or
  introduce a tokenizer or embedding model into the pure-Go core.
- Stream PCM speech across each boundary: model adapter, worker frames, bounded
  core retention, controller-gated HTTP chunk reads and Web Audio scheduling.
  Keep the existing ordered delivery/acknowledgment path. Stop, lease loss,
  backend loss or cancellation revokes pending presentation. Unsupported WAV
  encodings and compressed audio retain complete-clip playback.
- Prepare TTS installations in permanent `runtimes/<id>` directories under the
  module home. Never relocate a built venv. Verify files/imports before applying
  settings that retire the old worker, then promote the small home index. Keep
  old runtimes for manual recovery. Share app-owned uv downloads; copy verified
  Qwen model files into the candidate rather than hardlinking writable weights.
- Expose only the managed TTS worker routes, with loopback binding, loopback Host
  checks and rejection of browser Origin headers. Chatterbox's upstream server
  is imported as a model library; its management and upload UI is not mounted.
  Synchronous inference runs through a shared bounded cancellation-aware bridge.

## Consequences and limits

No Go or browser dependency is added. Native inference stays in optional worker
processes. The shared engine, transport ownership and motion contracts remain
authoritative.

Byte accounting deliberately underuses many tokenizers' real capacities.
Unknown external contexts use a 32,768-byte application policy, not a claim about
the remote server's actual context. Exact tokenizer admission and model-specific
GPU/KV policies remain separate, measured work. Diagnostics report available
provider phase timings and token counts without retaining hidden reasoning.

Audio remains bounded to 8 MiB per request. Chunk reads copy at most 32 KiB and
poll at 100 ms when empty; scheduling stays approximately 1.25 seconds plus one
chunk ahead. Already audible speech cannot be recalled, but cancellation stops
future playback. A running synchronous GPU call must return before its producer
can observe cancellation; subsequent generation is skipped.

Versioned installations cost additional disk space. This is not an automatic
rollback system or a cross-file/database transaction: settings are the runtime
authority, and an index-promotion error is reported explicitly. New Qwen installs
can finish without reference conditioning, so real generation is validated on
model load after the user configures that reference. Actual model listening,
shared-GPU contention and full download/update acceptance remain necessary.

See [implementation and validation](../ai-runtime-improvements-2026-09-08.md)
and [voice worker protocol](../voice-worker-protocol.md).
