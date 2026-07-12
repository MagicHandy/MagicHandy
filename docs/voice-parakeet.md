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

`install.ps1` offers this setup explicitly after the core build and LLM
questions. It downloads only after consent, verifies both SHA-256 values, and
places the optional runner and model under the selected MagicHandy data folder:

| Artifact | Version / file | Size | License | Verification |
| --- | --- | ---: | --- | --- |
| Runner | `parakeet.cpp` v0.4.0 Windows CPU | 1.4 MB | MIT | GitHub release SHA-256 |
| Model | `tdt-0.6b-v3-q4_k.gguf` | 644 MiB | CC-BY-4.0 | Hugging Face LFS SHA-256 |
| Worker | `voice-parakeet-worker.exe` | built locally | GPL-3.0-only | Go build |

The installer builds the worker but does not enable or start voice. In
**Settings > Voice**, enable voice workers, select **Parakeet (managed, local)**
as the Speech input provider, and set the installed `parakeet-server.exe` and
GGUF model paths. The app resolves `voice-parakeet-worker.exe` automatically
when it is beside the core binary; **Advanced > Worker binary override** handles
nonstandard layouts. Save settings, then use Start and Load in the Speech input
row. The default managed port is `127.0.0.1:8990`; change the provider-scoped
Server port field only when that port is already in use.

The first installer path intentionally uses the portable CPU runner. Users can
replace it with a compatible parakeet.cpp CUDA or Vulkan runner later without
changing the worker contract. Acceleration selection belongs in a measured
hardware-fit slice, not in the first voice install path.

## Manual Setup

Build the worker:

```powershell
go build -o voice-parakeet-worker.exe ./cmd/voice-parakeet-worker
```

Run it in managed mode by selecting the managed provider and filling its
server/model fields in MagicHandy's voice settings. The worker launches:

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

Browser microphone capture and push-to-talk are implemented on localhost.
Recognized speech uses the same chat and motion safety path as typed chat; it
never bypasses limits, smoothing, controller ownership, or Emergency Stop.
However, browsers currently record WebM/Opus or Ogg while the pinned managed
runner path is documented with WAV. End-to-end format compatibility remains an
acceptance gap (risk R24); hands-free routing is still planned.

## Validation

The worker tests cover external `/v1/models` compatibility, parakeet.cpp
`/health` readiness, a managed child process starting only once, port conflict
errors, unload, shutdown, cancellation, and no-speech handling. A real-model
check remains required before managed push-to-talk is called validated: capture
the browser format, runner version, model checksum, CPU/GPU mode, load time,
transcription latency, and the no-speech result in diagnostics without recording
raw audio or credentials.

## Related

- ADR 0003 and [voice worker protocol](voice-worker-protocol.md)
- ADR 0007 (voice backend choice)
- [installation automation plan](installation-automation.md)
- Phase 13.4 in [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)
