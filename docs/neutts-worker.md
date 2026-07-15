# NeuTTS Air Worker Setup

MagicHandy's NeuTTS provider is a separate Go worker that adapts the
non-Python `neutts-rs` `stream_pcm` runner. The core app remains pure Go and
continues to work when NeuTTS is absent or fails.

## Current capability boundary

The runner synthesizes from NeuCodec reference codes plus the exact reference
transcript. Although the encoder exported by `neutts-rs` is still a stub,
MagicHandy now ships a separate native reference worker around the pinned
DistillNeuCodec ONNX encoder. Settings accepts a local WAV and its exact
transcript, encodes the WAV without Python, validates the resulting token range
again in Go, and stores canonical app-managed `.npy` codes with the source WAV.
The transcript conditions synthesis but is not an encoder input.

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
MSVC, builds the eSpeak-enabled `stream_pcm` example, and stages:

```text
<data-dir>\voice\neutts\
  active\runtime\stream_pcm.exe
  active\runtime\magichandy-neucodec-encoder.exe
  active\runtime\DirectML.dll
  active\runtime\models\neucodec_decoder.safetensors
  active\runtime\runtime.json
  active\runtime\THIRD_PARTY_NOTICES.txt
  active\encoder\distill_neucodec_encoder.onnx
  active\encoder\distill_neucodec_encoder.onnx.data
  active\hf\hub\models--neuphonic--neutts-air-q4-gguf\...
```

`stream_pcm` does not call MagicHandy's managed `llama-server.exe`. The Rust
crate builds its own CPU llama.cpp binding through `llama-cpp-4`; tying NeuTTS
to the managed llama.cpp installer selection gives users one explicit
source-toolchain/download decision. NeuTTS remains CPU-only even when managed
chat uses CUDA.

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
succeeds. The resulting runtime/backbone/encoder uses about 1.9 GiB, with
several additional GB potentially needed during the build.

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

Prerequisites on Windows are Rust (MSVC target), Visual Studio Build Tools,
CMake, and Git. Build the reviewed `neutts-rs` revision and its Rust
eSpeak-enabled streaming example:

```powershell
git clone --branch v0.1.1 --depth 1 https://github.com/eugenehp/neutts-rs.git
cd neutts-rs
git rev-parse HEAD # ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f
# Change only the neutts root package entry in Cargo.lock from 0.1.0 to 0.1.1.
cargo build --locked --release --example stream_pcm --features espeak
```

Set **stream_pcm runner override** to the resulting
`target\release\examples\stream_pcm.exe`. Build the MagicHandy protocol
adapter with:

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
paths and transcript entry remain under Advanced. Confirm a custom **Air Q4**
setup works directly before selecting the provider:

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
4. Save, then use the TTS status row to start and load the worker.
5. Send a test request before enabling **Speak chat replies**.

Start validates the adapter, runner, decoder, codes, transcript, and exact
backbone cache, then probes `stream_pcm --help` for the required CLI contract;
it does not synthesize during readiness. **Send test** is the audible and
model-load verification. First synthesis can take minutes on CPU, so NeuTTS
requests have a five-minute worker timeout. The browser follows the backend
request until that terminal timeout instead of abandoning valid cold inference
after 15 or 30 seconds. Worker PCM streams while synthesis runs. The pinned
runner currently emits one bounded `NeuCodec decoder:` line on stdout before
PCM; the adapter strips only that known diagnostic. The core retains a bounded
copy and wraps it as a 24 kHz mono WAV for controller-owned browser playback.
Stop or request cancellation terminates the active runner process and
invalidates browser playback.

## Security and privacy

NeuTTS input and audio remain local after installation; see the network
limitation above. Paths and transcripts are ordinary local settings, not
secrets. Generated audio is retained only for the bounded recent request window
defined by the voice manager. Reference preparation and audio-preview endpoints
accept only loopback, same-origin, controller-owned requests; a remote client
cannot ask the server to read or preview an arbitrary host path.
