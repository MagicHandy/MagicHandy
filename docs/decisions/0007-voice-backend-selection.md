# ADR 0007: Voice Backend Selection

## Status

Accepted for the rewrite plan. Updated 2026-07-09: the first managed ASR
implementation uses parakeet.cpp rather than a direct sherpa-onnx binding.

## Context

StrokeGPT-ReVibed's voice stack (Chatterbox, faster-whisper, Parakeet-via-NeMo)
was Python/Torch and a major source of dependency instability. The models are not
the problem — the NeMo/Torch packaging was; Parakeet now has mature non-Python
runners (parakeet.cpp, sherpa-onnx, achetronic/parakeet). MagicHandy wants voice
without making Python a core dependency, usable on a 12 GB GPU (e.g., RTX 5070)
that is also running the local LLM.

ADR 0003 established the optional, language-agnostic worker boundary. This ADR
picks the concrete engines to implement first. The full landscape and the
reasoning behind these picks are in `docs/voice-tts-survey.md`.

## Decision

Implement three non-Python voice backends behind the ADR 0003 worker protocol:

1. **ASR - Parakeet-TDT-0.6B-v3** via a managed **parakeet.cpp** server.
   parakeet.cpp publishes a small Windows runner with a local GGUF model,
   loopback health endpoint, and OpenAI-compatible transcription endpoint, so
   the first implementation stays outside the Go process with no CGo or Python.
   **achetronic/parakeet** remains an external OpenAI-compatible alternative;
   **sherpa-onnx** remains the future multi-model/VAD candidate. Parakeet is
   faster than Whisper, matches or beats its English WER, and, critically for
   hands-free use, does not hallucinate on silence. Whisper is an optional
   alternate for heavy noise/accents or
   languages outside Parakeet's ~25; Canary-1B-v2 is the optional max-accuracy
   upgrade.
2. **Local TTS (cloning) — NeuTTS Air**: a 748M Qwen-backbone speech LLM +
   NeuCodec, real-time on CPU (so it does **not** contend with the LLM for the
   GPU), zero-shot cloning from ~3 s of reference audio, contextual
   expressiveness. Runs via the llama.cpp-family runner pattern (ADR 0005) plus a
   native codec decoder.
3. **Cloud TTS (premium) — ElevenLabs**: HTTP from Go, expressive and
   high-fidelity instant cloning, low latency, no Python and no local VRAM.

The core app plus this stack install and run with **no Python present**.
Kokoro/Piper (non-cloning) may be added later as an instant fallback but are not
in the first implementation set.

## Door Left Open (optional, later)

- **Optional Python workers** — Chatterbox, CosyVoice 3, Dia, Parakeet — behind
  the same versioned protocol, for users who want maximum local
  expressiveness/cloning and accept the install. Never required; opt-in only;
  never on the core install path.
- **Other non-Python engines** — F5-TTS (ONNX), Orpheus-on-llama.cpp,
  sherpa-onnx — can be added as providers if they prove out.

## Rationale

- **Parakeet-TDT-0.6B-v3** is now non-Python (parakeet.cpp / sherpa-onnx /
  achetronic-Go) and beats Whisper on speed (non-autoregressive TDT, ~10x) and
  English WER, while avoiding silence hallucinations - the exact failure the old
  hands-free stack kept patching. parakeet.cpp is the first managed engine
  because its Windows binaries already include a loopback OpenAI-compatible
  server. A direct sherpa-onnx path would add a native runtime/binding and a
  custom HTTP surface to this first slice.
- **NeuTTS Air** is the best "fast + cloning + non-Python + fits a shared 5070"
  option in the survey: CPU real-time avoids GPU contention with the LLM, the
  Qwen backbone reuses the llama.cpp runner, and 3-second cloning covers the core
  use case. Its one integration cost is a native NeuCodec decoder.
- **ElevenLabs** covers the "expressive AND faithfully-cloned voice together"
  case that no mature local non-Python model does today, at the lowest latency,
  at the cost of cloud/privacy — an explicit user choice, not a default.
- Together they give a private local option and a premium cloud option, both with
  zero Python.

## Requirements (from the survey and ADR 0003)

- sentence-level streaming: speak sentence 1 while sentence 2 renders
- keep TTS off the GPU the LLM is using where possible (NeuTTS Air on CPU does
  this for free)
- providers report missing-dependency/failure without crashing the core
- audio playback uses the single-owner lease; model/provider errors stay out of
  chat history, TTS, and motion (ADR 0003, Message And Audio Delivery Ordering)

## Consequences

Positive:

- non-Python default voice *with* cloning; private local + premium cloud
- fits a shared 12 GB GPU; reuses the llama.cpp runner
- no Torch/CUDA/NeMo install path in the core

Negative / risks:

- NeuTTS Air's cloning quality and its native codec decoder are unproven for this
  app — prototype early (tracked as R17)
- expressive emotion *tags* on a cloned voice are not covered by the initial set;
  that stays a cloud (ElevenLabs) or optional-Python capability
- ElevenLabs needs internet + API key and sends text/reference audio to a cloud
  service (privacy) - clearly optional
- Parakeet v3 covers ~25 languages vs Whisper's 99; users needing other languages
  need a later optional Whisper provider rather than the first managed runner

## Implementation Note

NeuTTS Air integration (codec decoder + cloning quality/latency) is an explicit
spike in Phase 13. If it fails to meet quality/latency, fall back to F5-TTS
(ONNX) or an optional Python worker while keeping ElevenLabs as the premium path.

The first Parakeet integration is documented in `docs/voice-parakeet.md`: a
managed parakeet.cpp v0.4.0 process with explicit, checksum-verified installer
downloads. It is a worker-owned runner, not a CGo link or a new motion path.

## Relationship

- ADR 0003: the worker boundary and delivery-ordering rules this builds on
- ADR 0005: the llama.cpp runner pattern NeuTTS Air reuses
- `docs/voice-tts-survey.md`: the evidence base
- `docs/risk-register.md`: R17 (NeuTTS Air cloning/codec spike)
