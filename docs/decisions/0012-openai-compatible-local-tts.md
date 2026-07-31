# ADR 0012: OpenAI-Compatible Local TTS Modules

## Status

Accepted. Supersedes ADR 0007's NeuTTS Air selection.

## Context

MagicHandy implemented NeuTTS Air through a persistent native runner to avoid
Python. Accelerated synthesis was fast enough, but repeated listening and live
use exposed inconsistent articulation, slurring, and fragile reference-code
conditioning. The implementation also coupled voice provisioning to the
llama.cpp build and added a large custom Rust, ONNX, codec, and cache surface.
That complexity no longer earns its maintenance cost.

Current local cloning models with materially better quality are primarily
Python and PyTorch applications. Keeping Python out of the Go process remains
important; requiring every optional worker to be non-Python is not. ADR 0003
already provides the correct isolation boundary.

The reviewed alternatives are:

- `faster-qwen3-tts`: an MIT-licensed Qwen3-TTS implementation with an
  OpenAI-compatible `/v1/audio/speech` example server. It supports zero-shot
  cloning and streams WAV or PCM. Its published Windows RTX 4060 result for the
  0.6B model is 413 ms time to first audio and 2.26 real-time factor.
- Chatterbox Turbo: an MIT-licensed 350M cloning model. The reviewed community
  server exposes `/v1/audio/speech`. Its reviewed Windows install paths support
  NVIDIA CUDA and CPU, providing a broader fallback than Faster Qwen3-TTS.
- `qwen3-tts.cpp`: a promising native GGML implementation, but it currently has
  no HTTP server, no release artifacts, and a young integration surface. A
  custom resident wrapper now would recreate the NeuTTS maintenance problem.
- Any user-managed OpenAI-compatible TTS service: useful for future models and
  remote/private deployments without another provider-specific core change.

## Decision

Remove NeuTTS from the release implementation and use one generic
OpenAI-compatible TTS worker for three provider choices:

1. **Faster Qwen3-TTS (managed)** is the recommended NVIDIA path. The separate
   installer creates an isolated `uv` environment, installs a pinned source
   revision, downloads the explicitly selected model after consent, and writes
   the resulting module settings through MagicHandy's settings command.
2. **Chatterbox Turbo (managed)** is the broader hardware fallback. Its
   installer uses the same isolated and pinned approach and writes a
   loopback-only server configuration.
3. **OpenAI-compatible (external)** connects to a server the user starts and
   owns. MagicHandy never stops that external process.

The bundled Go worker:

- speaks the ADR 0003 NDJSON protocol to the core;
- sends text to `/v1/audio/speech`;
- accepts model, voice, format, health path, an optional JSON readiness field,
  and an optional bearer key;
- buffers a bounded response and repairs streaming WAV length fields before
  handing it to browser playback;
- owns and stops only a managed child it started;
- launches child processes directly, never through a shell;
- binds scripted modules to `127.0.0.1` on a user-selected port; and
- keeps credentials in process environment variables, never arguments, status,
  diagnostics, or exports.

Managed model servers are optional modules, not core dependencies. The normal
Go build remains pure Go with `CGO_ENABLED=0`. The main installer does not
silently download Python, CUDA, models, or multi-gigabyte assets. Dedicated
scripts explain license, hardware, and disk implications before changing the
machine.

Settings exposes a provider dropdown for both scripted modules and the external
compatible endpoint. Auto-launch is an explicit independent preference; merely
selecting a provider does not grant process ownership. Enabling "speak replies"
may use an already running worker but does not silently change the auto-launch
choice.

Legacy settings that select `neutts_air` migrate to no TTS provider with
speaking disabled. This prevents an update from repeatedly trying to start a
removed runtime. Existing NeuTTS assets are left on disk for the user to remove;
the updater does not destructively delete user data.

## Consequences

Positive:

- one tested HTTP adapter works with two scripted modules and future compatible
  servers;
- better local clone quality without adding Python, Torch, CUDA, or CGo to the
  core binary;
- the 0.6B Qwen model gives a lower-VRAM default while Chatterbox provides a
  broader fallback;
- no custom codec, reference-code generator, or llama.cpp coupling remains;
- install and process ownership are explicit.

Negative:

- managed local TTS now requires an optional Python environment;
- model installation is larger and slower than the core install;
- first model load can take minutes and needs a longer worker load deadline;
- quality, latency, and VRAM coexistence still vary by GPU, model, reference
  recording, and server revision;
- the Chatterbox OpenAI endpoint currently returns a complete clip rather than
  incremental audio.

## Deferred

Reconsider a native Qwen3-TTS runner when it has a stable server contract,
tagged releases, cancellation, bounded streaming, and Windows artifacts. Do not
add a model-specific native wrapper merely to remove Python from an already
isolated optional process.

## Verification

- worker protocol tests cover load, health, synthesis, cancellation, size
  bounds, model-aware JSON readiness, key redaction, WAV repair, and managed
  child ownership;
- installer plan-only tests cover both scripted modules without downloading;
- settings migration tests prove NeuTTS is disabled safely;
- frontend tests cover provider-specific fields and auto-launch persistence;
- release acceptance records first-load time, warm time to playable audio,
  output quality, VRAM use alongside the selected LLM, cancellation, and clean
  worker/server teardown.

## Relationship

- ADR 0003 defines the worker and audio-ordering boundary.
- ADR 0007 still governs Parakeet ASR and ElevenLabs TTS.
- ADR 0011 governs the Windows installer shell and explicit consent.
- `docs/voice-tts-modules.md` documents installation and operator behavior.
- `docs/risk-register.md` R17 records the retired NeuTTS experiment and the new
  local TTS acceptance work.
