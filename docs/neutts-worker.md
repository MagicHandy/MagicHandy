# NeuTTS Air Worker Setup

MagicHandy's NeuTTS provider is a separate Go worker around a first-party,
persistent runner built against pinned `neutts-rs`. The model process loads
once, serves multiple synthesis requests over a bounded framed protocol, and
stays outside the pure-Go core. The app continues to work when NeuTTS is absent
or fails. Legacy custom `neutts-rs stream_pcm` executables remain compatible,
but they start a new model process for every request.

## Current capability boundary

The runner synthesizes from NeuCodec reference codes plus the exact reference
transcript. Although the encoder exported by `neutts-rs` is still a stub,
MagicHandy now ships a separate native reference worker around the pinned
DistillNeuCodec ONNX encoder. Settings accepts a local WAV and its exact
transcript, encodes the WAV without Python, validates the resulting token range
again in Go, and stores canonical app-managed `.npy` codes with the source WAV.
The transcript conditions synthesis but is not an encoder input.

Phonemization uses the official eSpeak NG 1.52 engine that NeuTTS expects. The
installer provisions it through WinGet and the persistent runner invokes it
directly; Python is not involved. The runner preserves punctuation and verifies
known English pronunciations during installation. The optional pure-Rust
`espeak-ng` crate in pinned `neutts-rs` is deliberately not enabled because its
output dropped words and mispronounced common words in the acceptance corpus.

The older pure-Go preparation path remains available to advanced/manual
clients. It can normalize a separately licensed official sample-style Torch
ZIP `.pt` or compatible one-dimensional int32 `.npy` without executing pickle.
MagicHandy does not bundle a reference voice.

The Windows source installer builds `voice-neutts-worker.exe` and, when managed
llama.cpp is selected, installs the runner, decoder, Air Q4 backbone, native
reference worker, and pinned ONNX encoder. It does not install a reference
voice. Skipping managed llama.cpp also skips NeuTTS.

## Installer-managed runtime

The source installer pins `neutts-rs` v0.1.1 at
`ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f`. That tag changed the manifest
package version to 0.1.1 but left its root `Cargo.lock` package entry at 0.1.0.
After verifying the commit, the installer requires and corrects exactly that one
metadata entry, then builds with `--locked`; all dependency versions remain
pinned. The older `65771f3...` revision previously shown here had mismatched
`llama-cpp-4` manifest and lockfile versions and is no longer supported.

The installer installs LLVM/libclang plus pinned Rust 1.94.0 for x64 Windows
MSVC. It copies the reviewed runner in `workers/neutts-runner`, applies the
exact pinned CUDA offload patch, and builds one of these variants:

- managed CPU llama.cpp: system eSpeak NG, CPU backbone, and CPU NeuCodec;
- managed CUDA llama.cpp: system eSpeak NG, all-layer CUDA backbone offload, and WGPU
  NeuCodec.

It stages the persistent executable under the stable `stream_pcm.exe` filename
so existing settings and custom-runtime discovery remain compatible:

```text
<data-dir>\voice\neutts\
  active\runtime\stream_pcm.exe
  active\runtime\ggml-*.dll          # CUDA build only
  active\runtime\llama.dll           # CUDA build only
  active\runtime\magichandy-neucodec-encoder.exe
  active\runtime\DirectML.dll
  active\runtime\models\neucodec_decoder.safetensors
  active\runtime\runtime.json
  active\runtime\THIRD_PARTY_NOTICES.txt
  active\encoder\distill_neucodec_encoder.onnx
  active\encoder\distill_neucodec_encoder.onnx.data
  active\hf\hub\models--neuphonic--neutts-air-q4-gguf\...
```

The NeuTTS runner does not call MagicHandy's managed `llama-server.exe`; its
`llama-cpp-4` binding owns a separate model context. The selected managed
llama.cpp backend determines the NeuTTS build too. CUDA substantially reduces
speech latency but keeps additional VRAM resident while chat speech is enabled;
CPU avoids that VRAM cost but can be much slower than real time. The runtime
manifest records the selected backend, runner protocol, backbone/codec
acceleration, phonemizer/version, and checksums for every required native DLL.
Schema-3 and older runtimes are intentionally rebuilt by `update.ps1` so the
inaccurate bundled phonemizer cannot remain active after an update.

The Air Q4 GGUF is downloaded from immutable Hugging Face revision
`008555972590ff2c599dd43736ba31c81df3f0bf` and verified as
`bf66dc21b7588fe720cbdfeac1595e7b7c780515f8d8f1ff9a29062e4ac9119e`.
The NeuCodec checkpoint comes from revision
`30c1fdd19e68aee65d542cf043750d4c0165893e`, is verified as
`30c3ea13ceeb2de693c56e5e33a1b7e00d44c95dcdd08a4ed0d552d0bf59ebdf`,
and is converted by the upstream pure-Rust converter. The reference encoder is
the Apache-2.0 DistillNeuCodec ONNX export pinned at
`2cd5cf022b7a1e689e561f0492787768cfe8395d`; both graph and external weights
are checksum-verified before publication. The 1.1 GiB source checkpoint and
temporary Cargo/build trees are removed after the atomic runtime stage
succeeds. The CPU runtime/backbone/encoder uses about 1.9 GiB. The CUDA runner
and its five llama/ggml DLLs add about 147 MiB, for about 2.0 GiB installed;
several additional GB may be needed during the build.

No startup/status path downloads files. Rerun `update.ps1` with managed
llama.cpp selected to repair or update the pinned runtime. `-SkipLlamaBuild` and
declining the managed llama.cpp prompt skip all NeuTTS provisioning.
The installer rehashes the active runner, decoder, GGUF, reference worker, and
encoder assets before reuse and atomically publishes the verified runtime.
Application startup validates the manifest and required paths without rehashing
large assets before the HTTP listener can start. Explicit integrity verification
and installer updates still perform the full hashes. Custom runner overrides
retain their own explicit cache contract instead of being silently paired with
the app-managed Air model.

## Custom runner build

The supported route is the source installer because it pins, patches, builds,
packages, and verifies the full native dependency set. For development, the
same persistent build can be reproduced after cloning the reviewed upstream
revision beside the MagicHandy checkout:

```powershell
git clone --branch v0.1.1 --depth 1 https://github.com/eugenehp/neutts-rs.git
cd neutts-rs
git rev-parse HEAD # ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f
# Change only the neutts root package entry in Cargo.lock from 0.1.0 to 0.1.1.
git apply ..\MagicHandy\workers\neutts-runner\neutts-rs-v0.1.1-cuda.patch
Copy-Item ..\MagicHandy\workers\neutts-runner\main.rs .\examples\magichandy_neutts.rs
cargo build --locked --release --example magichandy_neutts --features cuda,wgpu
```

Adjust the repository-relative paths when the checkouts are elsewhere. For a
CPU build, use `--features backbone`; the CUDA patch is inactive when that
feature is absent. Install eSpeak NG 1.52 or newer and ensure `espeak-ng` is on
`PATH` (or beside the runner). Copy the executable and every generated
llama/ggml DLL together,
then set **stream_pcm runner override** to that executable. Build the MagicHandy
protocol adapter with:

```powershell
go build -o voice-neutts-worker.exe ./cmd/voice-neutts-worker
```

Place `voice-neutts-worker.exe` beside `magichandy.exe`, under the app data
directory's `tools` folder, or select it with the advanced worker override.

## Reference voice and custom assets

For the app-managed runtime, leave the runner override blank. Open **Generate
reference voice**, choose a clean 1-30 second WAV sampled at 16-48 kHz, and
enter the words exactly as spoken. Three to fifteen seconds with one speaker,
little noise, and few long pauses is the quality target. The encoder downmixes
up to eight channels and resamples to 16 kHz before inference. It runs in a
short-lived process; a measured 7.45 second reference encoded in about one
second after warm filesystem caches and peaked near 1.3 GiB working set.

For a custom runtime, use the runner project's model conversion command and
ensure `models\neucodec_decoder.safetensors` exists above the selected runner;
MagicHandy walks upward to find it. The managed reference encoder remains
available when its installer-owned files are present. Manual pre-encoded `.npy`
paths and transcript entry remain under Advanced. Legacy one-shot `stream_pcm`
remains supported for advanced users. Confirm a custom **Air Q4** setup works
directly before selecting the provider:

```powershell
.\stream_pcm.exe --codes C:\voices\reference.npy `
  --ref-text "The exact words in the reference recording." `
  --text "NeuTTS setup test." `
  --backbone neuphonic/neutts-air-q4-gguf `
  --gguf-file neutts-air-Q4_0.gguf > test.pcm
```

MagicHandy requests `HF_HUB_OFFLINE=1` and supplies the exact GGUF filename to
avoid repository discovery, but the pinned upstream `hf-hub` client does not
enforce that environment variable. MagicHandy compensates by requiring the exact
GGUF cache entry before it starts the runner, so a missing cache cannot fall
through to an implicit download. The process is not network-sandboxed; a
network-denied integration test remains R17 exit evidence before NeuTTS is
described as fully app-managed/offline.

## Configure and test

1. Open **Settings > Voice** and enable voice workers.
2. Under **Speech output (TTS)** choose **NeuTTS Air (local)**.
3. Leave the runner override blank for the app-managed runtime, or use
   **Browse...** for a custom runner. Open **Generate reference voice**, select
   the source WAV, and enter exactly the words heard. Generate the codes, preview
   the stored audio, correct the transcript if needed, and apply. Manual
   pre-encoded paths remain under **Advanced**.
4. Save. **Start** still loads a newly changed configuration immediately. On
   later app launches, enabled speech input autoloads ASR, and enabled **Speak
   chat replies** autoloads TTS. A startup failure appears in worker status
   without preventing the app from serving its UI.
5. Send a test request before enabling **Speak chat replies** for regular use.

Load validates the adapter, runner, decoder, codes, transcript, and exact
backbone cache, then starts the persistent managed runner. **Send test** remains
the audible verification. The shell unlocks a persistent Web Audio context on
the first real pointer or keyboard gesture, then uses that sink after the
asynchronous request finishes; this avoids browser autoplay rejection without
creating a second speech queue. On the RTX 5070 Ti test host, the previous CPU
one-shot path took 127.27 seconds wall time, with first audio at 90.86 seconds
and a 66.72x real-time factor. The CUDA/WGPU build loaded in 1.90 seconds and a
one-shot synthesis completed in 2.45 seconds. Those early timing probes used an
inaccurate experimental phonemizer and independent codec chunks, so they are
retained only as performance history, not quality evidence. With system eSpeak
and Neuphonic's overlap-aware 25-token stream, four random controlled requests
reached first audio in 1.06-2.05 seconds and synthesis completed in 2.06-3.89
seconds. Output duration was 3.10-6.08 seconds and can overlap synthesis during
playback; synthesis time alone is not the listener's total completion time.
Managed Parakeet recovered every substantive target word in all four clips,
including two exact sentence transcriptions. These are single-host engineering
measurements, not universal latency or subjective cloning-quality claims.

CPU requests retain the five-minute timeout because fallback synthesis can be
slow. PCM stays bounded and streams as 24 kHz mono samples. Cancellation sends a
request-scoped cancel command instead of tearing down the loaded model; a live
test canceled after the first chunk and the same process completed the next
request with valid PCM. Unload, worker shutdown, and Emergency Stop terminate
the persistent child and invalidate browser playback.

## Security and privacy

NeuTTS input and audio remain local after installation; see the network
limitation above. Paths and transcripts are ordinary local settings, not
secrets. Generated audio is retained only for the bounded recent request window
defined by the voice manager. Reference preparation and audio-preview endpoints
accept only loopback, same-origin, controller-owned requests; a remote client
cannot ask the server to read or preview an arbitrary host path.
