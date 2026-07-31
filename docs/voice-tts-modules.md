# Local TTS Modules

MagicHandy's core does not embed a speech model. Local TTS runs in an optional
server process and the bundled Go adapter calls its OpenAI-compatible
`POST /v1/audio/speech` endpoint.

## Choices

| Provider | Best fit | Default model | Runtime |
|---|---|---|---|
| Faster Qwen3-TTS | NVIDIA GPU, fastest recommended local cloning path | `Qwen/Qwen3-TTS-12Hz-0.6B-Base` | isolated Python/PyTorch module |
| Chatterbox Turbo | NVIDIA or CPU fallback | `ResembleAI/chatterbox-turbo` | isolated Python/PyTorch module |
| OpenAI-compatible | an existing local or remote service | server-defined | user managed |
| ElevenLabs | managed cloud quality and latency | ElevenLabs model | bundled Go HTTP worker |

Faster Qwen3-TTS is the default recommendation for a compatible NVIDIA GPU.
The 0.6B model limits VRAM use relative to the 1.7B model and is the first model
to test alongside a local LLM. Chatterbox Turbo is the fallback when the Faster
Qwen runtime is unsuitable. Neither module is required to run MagicHandy.

## Installation Contract

Choose a managed module in `install.ps1`, or run
`scripts/install-tts-module.ps1` directly. The shared module flow:

1. displays the selected project's license, model, hardware target, source
   revision, download implications, and install root;
2. repairs WinGet through Microsoft's supported path when needed, then installs
   `uv` after consent;
3. installs a managed Python runtime and creates a private virtual environment
   below the MagicHandy data directory;
4. installs a pinned upstream revision and dependencies;
5. downloads the chosen model only after consent;
6. configures the server for `127.0.0.1`, not all interfaces; and
7. calls the MagicHandy settings command so the provider, paths, port, and
   auto-launch choice are persisted in SQLite. Faster Qwen reference fields
   remain empty until the user completes them in Settings > Voice.

No preinstalled Python, uv, PyTorch, Git, or compiler is required. Faster Qwen
uses managed Python 3.11. The pinned Chatterbox dependency set uses Python 3.10
because its supported Windows Torch, torchvision, and ONNX packages are
available as wheels there; this avoids an accidental native build on a clean
machine.

`scripts/update-tts-module.ps1` reads the existing module choice, preserves it
by default, and asks before changing provider, model, port, or auto-launch. The
main app updater validates and reuses a selected installed module rather than
reinstalling several GiB. Both module scripts support non-mutating plan/check
modes.

The scripts do not reuse or alter a system Python environment. Removing a
provider from Settings does not delete its model files. Uninstalling large
module assets is a separate explicit operation.

## Auto-Launch And Ownership

Auto-launch means MagicHandy's TTS worker starts the configured module server
when the worker model is loaded. It waits for the configured health endpoint and
stops only the child process it created. If the port is occupied, startup fails
instead of attaching to or killing an unknown process.

For Chatterbox, readiness is not inferred from HTTP status alone. MagicHandy
probes `GET /api/model-info` and requires its `loaded` field to be `true`.
Existing settings that used the older UI-data health route migrate to this
model-aware endpoint.

With auto-launch off, start the configured service yourself. A scripted
provider can keep its provider-specific defaults while connecting to that
already-running endpoint, or an arbitrary service can use the external
OpenAI-compatible provider. MagicHandy probes but never owns or stops a server
it did not launch.

## Reference Audio

Faster Qwen3-TTS needs a local reference WAV plus its exact transcript for
zero-shot cloning. Use clean single-speaker speech without music or effects.
The command-line installer does not ask for either value. After installation,
open Settings > Voice, choose the WAV, enter its exact transcript, and save.
MagicHandy reports the runtime as installed but keeps the worker unconfigured
until both values are present. The app stores the path and transcript in its
settings database, not in installer state; conditioning is cached by the
resident server.

Chatterbox accepts a local reference WAV as a named voice. The installer copies
that source into the module's voice directory and stores the resulting voice
name. The original file remains untouched. Without a reference, the pinned
server's `Emily.wav` sample is installed as the initial voice.

For NVIDIA installs, the script selects the pinned upstream CUDA 12.1
requirements on RTX 20/30/40-series GPUs and CUDA 12.8 on compute-capability
12.x hardware such as the RTX 50 series. Explicit CUDA selection fails early
when NVIDIA driver tools are unavailable; `-Device cpu` remains the portable
fallback.

## OpenAI-Compatible Contract

MagicHandy sends:

```json
{
  "model": "server-model-name",
  "input": "text to speak",
  "voice": "voice-name",
  "response_format": "wav"
}
```

`model` and `voice` may be omitted when the server does not require them. The
worker accepts WAV, MP3, Opus, AAC, or FLAC responses. WAV is preferred
because it avoids optional browser codec differences. A streamed WAV with
unknown RIFF lengths is repaired after the bounded response is complete.
The scripted Faster Qwen server is limited to WAV, and the scripted
Chatterbox server is limited to WAV, MP3, or Opus. Settings enforces those
provider-specific format lists.

The managed Chatterbox launcher suppresses the upstream server's automatic
browser opening. MagicHandy remains the only user interface while the pinned
server continues to provide its normal API.

For a protected compatible endpoint, the API key is stored as a private setting
and passed to the worker only as `OPENAI_TTS_API_KEY`.

## Acceptance Checklist

- first load and first playable clip complete without blocking the core;
- warm repeated clips do not slur, truncate, or change speaker unexpectedly;
- cancellation clears the active request and the next request succeeds;
- worker stop terminates an auto-launched child and leaves an external server
  running;
- server listens only on loopback unless the user deliberately operates a
  remote endpoint;
- GPU memory leaves enough room for the selected chat model;
- browser playback succeeds in Firefox and Chromium.
