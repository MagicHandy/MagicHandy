# ADR 0007: Voice Backend Selection

## Status

Accepted and implemented. Updated 2026-07-15: managed ASR uses parakeet.cpp;
NeuTTS Air uses the Slice 13.6 Go adapter around pinned `neutts-rs` v0.1.1
`stream_pcm`; a separate Rust/ONNX worker generates reference codes from WAV.

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
   GPU). A Go ADR 0003 adapter runs the reviewed `neutts-rs stream_pcm` process
   and forwards live PCM without Python. A separate first-party Rust worker runs
   a pinned DistillNeuCodec ONNX encoder to create the required `.npy` reference
   codes from a local WAV. The exact transcript remains separate conditioning
   input for synthesis.
3. **Cloud TTS (premium) — ElevenLabs**: HTTP from Go, expressive and
   high-fidelity instant cloning, low latency, no Python and no local VRAM.

The core app, Parakeet, ElevenLabs, and the installer-managed NeuTTS runtime run
with **no Python present**. Selecting managed llama.cpp in the source installer
also builds the NeuTTS runner and installs its decoder/backbone; skipping
llama.cpp skips NeuTTS. Reference generation does not require Python; the
upstream Rust stub is not used for that path.
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
  option in the survey: CPU real-time avoids GPU contention with the LLM. The
  implemented adapter proves non-Python decode and streaming. The pinned ONNX
  encoder closes the arbitrary-WAV reference gap without adding CGo or Python to
  the core.
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
- fits beside a local LLM on a shared 12 GB GPU by running TTS on CPU in a
  separate worker process
- no Torch/CUDA/NeMo install path in the core

Negative / risks:

- NeuTTS Air's subjective cloning quality remains unproven; arbitrary-WAV
  reference encoding is implemented and compatibility-tested, but needs wider
  sample-quality acceptance (R17)
- NeuTTS source provisioning is coupled to managed llama.cpp and adds Rust plus
  about 1.9 GiB of installed voice assets. MagicHandy compensates for the pinned
  runner not enforcing `HF_HUB_OFFLINE=1` with exact-cache preflight, but it
  reloads in a new process for every request; network-denied evidence,
  persistent preload, and subjective voice acceptance remain R17
- expressive emotion *tags* on a cloned voice are not covered by the initial set;
  that stays a cloud (ElevenLabs) or optional-Python capability
- ElevenLabs needs internet + API key and sends text/reference audio to a cloud
  service (privacy) - clearly optional
- Parakeet v3 covers ~25 languages vs Whisper's 99; users needing other languages
  need a later optional Whisper provider rather than the first managed runner

## Implementation Note

The NeuTTS Air spike, Slice 13.6 protocol adapter, and checksum-pinned Windows
source installation are complete. A pinned DistillNeuCodec ONNX worker now
generates compatible reference codes from WAV without Python; setup and manual
pre-encoded fallback are documented in `docs/neutts-worker.md`. Network-denied
evidence, persistent preload, and subjective quality remain open; if they fail
acceptance, use a documented non-Python fallback or an optional Python worker
while keeping ElevenLabs as the premium path.

The first Parakeet integration is documented in `docs/voice-parakeet.md`: a
managed parakeet.cpp v0.4.0 process with explicit, checksum-verified installer
downloads. It is a worker-owned runner, not a CGo link or a new motion path.

## Relationship

- ADR 0003: the worker boundary and delivery-ordering rules this builds on
- `docs/voice-tts-survey.md`: the evidence base
- `docs/risk-register.md`: R17 (NeuTTS Air cloning/codec spike)
