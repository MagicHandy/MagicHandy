# Parakeet ASR Worker

MagicHandy's local speech-recognition path is a small, pure-Go worker
(`cmd/voice-parakeet-worker`) around the existing
[parakeet.cpp](https://github.com/mudler/parakeet.cpp) runner. The worker
speaks the ADR 0003 protocol over stdio; the model stays in a separate
`parakeet-server` process. No Python, CGo, ONNX Runtime, or model code enters
the MagicHandy core.

## Runner Choice

The first managed path is `parakeet.cpp` v0.4.0 with the local
`tdt-0.6b-v3-q4_k.gguf` model.

- It publishes small Windows CPU, CUDA, and Vulkan runner archives and includes
  `parakeet-server`, which accepts local GGUF files and exposes
  `POST /v1/audio/transcriptions`.
- The server has a narrow, stable readiness endpoint (`GET /health`), binds to
  loopback when requested, and needs no downloader at worker load time.
- `achetronic/parakeet` remains compatible as an externally managed server, but
  requires an ONNX Runtime deployment. It is not the installer default.
- `sherpa-onnx` remains a strong future provider for model breadth, VAD, and
  streaming. Its native runtime/binding distribution and lack of this
  ready-made OpenAI server would make the first Windows path materially larger.

The worker is therefore a lifecycle wrapper, not a custom inference port. It
owns only the `parakeet-server` process it starts and never kills an external
server.

## Installer

`install.ps1` builds the small Parakeet Go adapter with the rest of the app, then
offers the external runner/model setup explicitly after the LLM questions. It
downloads only after consent, verifies both SHA-256 values, and places the
optional assets under the selected MagicHandy data folder. `update.ps1`
preserves this choice unless the user elects to reconfigure it:

| Artifact | Version / file | Size | License | Verification |
| --- | --- | ---: | --- | --- |
| Runner | `parakeet.cpp` v0.4.0 Windows CPU | 1.4 MB | MIT | GitHub release SHA-256 |
| Model | `tdt-0.6b-v3-q4_k.gguf` | 644 MiB | CC-BY-4.0 | Hugging Face LFS SHA-256 |
| Worker | `voice-parakeet-worker.exe` | built locally | GPL-3.0-only | Go build |

The installer does not enable or start voice. In
**Settings > Voice**, enable voice workers, select **Parakeet (managed, local)**
as the Speech input provider, and keep **Runtime source > MagicHandy module**.
The backend discovers the installer-owned `parakeet-server.exe`, GGUF model, and
`voice-parakeet-worker.exe`; no custom paths are required. The module status
distinguishes a complete installation from an adapter-only or otherwise
incomplete setup and directs the user back to `update.ps1` when repair is
needed. Save settings, then use **Start** in the Speech input row for immediate
use; Start also loads the model and succeeds only when ASR is ready. On later
app launches, enabled speech input starts and loads its configured ASR worker
automatically. The default managed port is `127.0.0.1:8990` under Advanced.

Installation, enablement, and process start remain separate actions: the
installer writes files only and voice remains opt-in. Startup autoload is driven
only by saved enabled/provider settings; installing assets alone does not start
a worker. The installer prints the exact Settings > Voice sequence after
provisioning.

The first installer path intentionally uses the portable CPU runner. Users can
replace it with a compatible parakeet.cpp CUDA or Vulkan runner later without
changing the worker contract. Acceleration selection belongs in a measured
hardware-fit slice, not in the first voice install path.

## Manual Setup

Build the worker:

```powershell
go build -o voice-parakeet-worker.exe ./cmd/voice-parakeet-worker
```

For a manual parakeet.cpp setup, choose **Runtime source > Custom local server**
and provide the server/model paths. The worker launches:

```text
parakeet-server --model <local GGUF> --host 127.0.0.1 --port 8990
```

For a user-managed compatible server instead, select **OpenAI-compatible
server** and set its Base URL and model name. Raw worker paths and argument
lists are exposed only by the **Custom worker** provider. The load response
names missing paths, model files, unreachable servers, and port conflicts
directly.

## Worker Behavior

- Load starts a managed runner, then waits for `GET /health`. External servers
  are checked through `/health` first and `/v1/models` as a compatibility
  fallback. No readiness check sends a transcription request.
- Unload, worker shutdown, and stdin EOF cancel in-flight work and stop only
  the managed child process. The core remains usable when a voice worker fails.
- The current parakeet.cpp example server accepts WAV audio. The worker rejects
  missing audio and whitespace-only transcripts as `no_speech`; it never puts
  an empty transcript into chat.
- Cancellation aborts the in-flight HTTP request. The runner serializes
  inference, while the core-owned worker queue remains bounded and visible.
- The Settings test action sends a valid, silent WAV to exercise the real
  decode path without inserting spoken content into chat or motion.
- A failed automatic load fails Start and stops the just-started adapter; the UI
  never treats a running but unloaded worker as microphone-ready.
- Browser MediaRecorder output is decoded in the browser, then downmixed,
  resampled to 16 kHz, and encoded as PCM16 WAV in one output-rate pass. The
  raw-WAV HTTP upload avoids browser base64/JSON expansion. The core validates
  the bytes and stages a private, process-session `audio_ref` that is removed on
  completion, cancellation, failure, or clean shutdown; stale crashed sessions
  older than the bounded request window are reaped at startup. The managed API
  rejects a compressed payload or fake WAV header instead of forwarding it to
  the runner and surfacing its opaque HTTP 400.

Browser microphone capture is implemented on localhost. The Chat control
defaults to a user-started hands-free session that transcribes successive
silence-delimited phrases until the user stops it. An AudioWorklet supplies mono
PCM to a bounded browser VAD; sensitivity, end-of-speech delay, noise
suppression, input selection, level, and queue status are available in the
microphone menu and persisted by the backend. Hold-to-talk remains available
and capped at 30 seconds. This does not claim background auto-start or recording
without a visible user-owned session.
Recognized speech uses the same chat and motion safety path as typed chat; it
never bypasses limits, smoothing, controller ownership, or Emergency Stop. The
Stop path also discards browser capture, invalidates voice results, cancels
in-flight chat, and fences stale request generations before dispatch. The WebM/Opus-to-WAV
mismatch and repeated cold-start path from R24 are fixed in code and covered by
capture-lifecycle, WAV encoder, and managed-boundary tests. A real Chrome/Edge
plus pinned-model transcription smoke test remains release evidence rather than
an unresolved format design.

## Validation

The worker tests cover external `/v1/models` compatibility, parakeet.cpp
`/health` readiness, a managed child process starting only once, port conflict
errors, unload, shutdown, cancellation, and no-speech handling. On 2026-07-15,
the final stripped app started the installed CPU runner and pinned
`tdt-0.6b-v3-q4_k.gguf` model through the production API. The official 7.45 s
Dave WAV was normalized to the browser contract (16 kHz mono PCM16 with a
canonical 44-byte header), accepted as a raw `audio/wav` upload, completed in
the managed queue, and produced a recognizable transcript. The API then
unloaded the worker and no MagicHandy, adapter, or `parakeet-server` process
remained. This validates the app/worker/runner boundary with a real model; it is
not evidence for microphone permission, browser DSP, VAD segmentation, or
first-word behavior. A real Chrome/Edge microphone run remains required before
closing R24 and should record runner version, model checksum, CPU/GPU mode,
load time, phrase latency, and the no-speech result without retaining raw audio
or credentials.

## Related

- ADR 0003 and [voice worker protocol](voice-worker-protocol.md)
- ADR 0007 (voice backend choice)
- [installation automation plan](installation-automation.md)
- Phase 13.4 in [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)
