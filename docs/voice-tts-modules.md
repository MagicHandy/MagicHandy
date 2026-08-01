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

Model downloads do not require Windows symlink privileges. The installer uses
one Hugging Face file-finalization worker on Windows to avoid a first-use
symlink-probe race, retries transient failures three times, and keeps the
resumable cache when all attempts fail. Rerunning either installer reuses files
that already finished.

Managed Faster Qwen startup resolves the configured Hugging Face repository ID
to the cache revision recorded in `refs/main`, verifies the model and speech
tokenizer files, and passes that local snapshot directory to the server. The
server remains in Hugging Face and Transformers offline modes after installation;
startup never depends on a network metadata request. A legacy cache without a
revision ref is accepted only when it contains exactly one complete snapshot.
MagicHandy's small launcher wrapper is copied beside the module and refreshed
by ordinary app updates without touching the Python environment or model cache.
It extends only the managed Faster Qwen endpoint with an unsigned generation
seed and an optional Base-model tone instruction, then performs one short
discarded streaming warm-up before reporting ready.
The warm-up prevents one-time model initialization from changing the first
user-visible fixed-seed clip; the pinned upstream model remains unchanged.

Retries also reuse a source checkout and managed environment left by a failure
before `module-state.json` was written. The installer records only its known
package-metadata directory in the checkout's private Git excludes. It never
cleans the checkout or ignores arbitrary files, so tracked edits and unrelated
untracked files remain a hard stop.

When the main installer invokes a TTS module script, it uses a child Windows
PowerShell process. This keeps the module script's support-module reload and
private schema state separate from the parent provisioner, while preserving
interactive prompts, output, and exit failures.

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
zero-shot cloning. Use clean single-speaker speech without music or effects,
ideally 3 to 10 seconds and close to the model's advertised short-reference
use case. A longer WAV is accepted, but extra pauses, delivery changes, or room
conditions can make cloning less consistent; test a short clean excerpt before
attributing variation to the seed.
The command-line installer does not ask for either value. After installation,
open Settings > Voice, choose the WAV, enter its exact transcript, and save.
MagicHandy reports the runtime as installed but keeps the worker unconfigured
until both values are present. The app stores the path and transcript in its
settings database, not in installer state; conditioning is cached by the
resident server.

Managed Faster Qwen defaults to **Fixed** seed mode with seed `1337`, matching
the pinned project's examples. The same text, reference, and settings therefore
reuse the same sampling seed. **New seed** chooses and saves another unsigned
32-bit seed; **Varied** mode chooses a new seed for every request. Seed control
can help isolate stochastic delivery or quality changes, but it cannot repair a
noisy, mismatched, or overly long reference. Generic OpenAI-compatible servers
do not receive MagicHandy's nonstandard `seed` field. Varied seeds can fail to
emit an end token and produce unusually long or degraded speech, so the managed
wrapper also applies a generous text-proportional 12-to-160-second generation
ceiling. Fixed mode remains the recommended default.

### Tone prompts

Settings > Voice exposes a reviewed set of Faster Qwen delivery prompts:
Natural, Warm and intimate, Playful and teasing, Soft and reassuring,
Confident and commanding, and Excited and energetic. **Natural** sends no
instruction and therefore preserves the behavior of installations created
before this control existed. **Custom** reveals a bounded free-text prompt for
pace, emotion, pitch, emphasis, or other delivery guidance.

The managed Faster Qwen Base model accepts these instructions while cloning in
the existing in-context-learning mode. Instruction following is experimental:
the reference WAV, exact transcript, generated text, and seed still materially
affect delivery, and a prompt cannot repair a noisy or mismatched reference.
Saving a changed tone reconfigures the managed worker before the next request.
The prompt is persisted with the other voice settings and is never sent to
generic OpenAI-compatible TTS providers.

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

The managed Faster Qwen wrapper also accepts `"seed": 1337` and an optional
`"instruct": "Speak in a warm, intimate tone."`. The Go adapter adds these
nonstandard fields only for that provider.

`model` and `voice` may be omitted when the server does not require them. The
worker accepts WAV, MP3, Opus, AAC, or FLAC responses. WAV is preferred
because it avoids optional browser codec differences. A streamed WAV with
unknown RIFF lengths is repaired after the bounded response is complete.
The worker bounds one HTTP response at 32 MiB. The core retains at most 8 MiB
per playable clip (about 2 minutes 55 seconds of 24 kHz mono 16-bit WAV) and at
most nine clips, for a 72 MiB worst-case retained-audio ceiling. Larger output
fails explicitly instead of growing process memory without bound.
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

Development evidence from 2026-07-31 on Windows with the managed CUDA 0.6B
model: offline startup reached model-ready in 6.8 seconds, a manual stop/start
cycle returned to ready in 7.2 seconds, and Chromium completed the audio fetch
and `/played` acknowledgement. The first 2.88-second clip after process startup
took 15.4 seconds; a warm 1.52-second clip took about 0.8 seconds. SoX was not
installed and its upstream warning did not prevent synthesis. This single
reference check does not close the representative-reference listening,
Firefox, cancellation, or GPU/LLM coexistence items above.
