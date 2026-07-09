# Parakeet ASR Worker

The speech-recognition path from ADR 0007, implemented as a Phase 12
protocol worker (`cmd/voice-parakeet-worker`). Pure Go, no Python, no CGo:
the worker proxies to an external **OpenAI-compatible transcription
server** — the same external-runner pattern as the llama.cpp LLM path — so
the ASR model runtime never enters the core install.

ADR 0007's recommended engine is **Parakeet-TDT-0.6B-v3** served by
[achetronic/parakeet](https://github.com/achetronic/parakeet) (a Go server
exposing the OpenAI transcription API). Any server implementing
`POST /v1/audio/transcriptions` works the same way (sherpa-onnx's server,
speaches, Whisper-family servers) — Whisper remains the documented
alternate for heavy noise/accents or languages outside Parakeet's ~25.

## Setup

1. Install and start an OpenAI-compatible ASR server, e.g.
   achetronic/parakeet with the Parakeet-TDT model (see its README; model
   downloads are its explicit setup step, never something MagicHandy pulls
   silently).
2. Build the worker:
   ```powershell
   go build -o voice-parakeet-worker.exe ./cmd/voice-parakeet-worker
   ```
3. In **Settings → Voice**: set the **ASR worker path** to the binary and
   the **ASR worker arguments** to point at your server, e.g.:
   ```
   -base-url http://127.0.0.1:8765
   ```
   (`-model <name>` optionally selects a server-side model.) Save, Start,
   Load — load pings the server so "server not running" is an immediate,
   clearly worded state.

## Behavior behind the protocol

- `load` requires `-base-url` (there is no safe default port to guess) and
  verifies the server is reachable.
- `transcribe` accepts inline `audio_b64` (test-scale) or an `audio_ref`
  file path, uploads multipart to `/v1/audio/transcriptions`, and returns
  one transcript candidate.
- **No empty transcripts, ever** (ADR 0003): missing audio and
  whitespace-only server output are rejected as `no_speech` — the exact
  hands-free failure mode the old app kept patching. Parakeet-TDT itself
  does not hallucinate on silence, which is why ADR 0007 picked it.
- Cancellation aborts the in-flight HTTP request; failures are structured
  protocol errors that never crash the core.

## What is deliberately not here yet

Microphone capture UI (push-to-talk / hands-free) is the next voice UI
step and rides the hands-free permission model from the plan; recognized
speech will route through the same chat/motion path as typed chat — voice
never bypasses limits, smoothing, or Stop. Until then, the worker is
exercised through the voice test endpoint and `audio_ref` files.

## Relationship

- ADR 0007 (engine selection and the silence-hallucination rationale)
- ADR 0003 / `docs/voice-worker-protocol.md` (the boundary this implements)
- ADR 0005 (the external-runner pattern this mirrors)
