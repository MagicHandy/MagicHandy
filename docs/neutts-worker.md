# NeuTTS Air Worker Setup

MagicHandy's NeuTTS provider is a separate Go worker that adapts the
non-Python `neutts-rs` `stream_pcm` runner. The core app remains pure Go and
continues to work when NeuTTS is absent or fails.

## Current capability boundary

The runner can synthesize and stream from pre-encoded NeuCodec `.npy` voice
codes plus the exact reference transcript. Its public Rust reference encoder
is still a stub. MagicHandy therefore does not encode an arbitrary reference
WAV and does not invoke Python behind the scenes.

The reference WAV field records which clip produced the configured codes. It
is provenance, not a runtime input. MagicHandy does not bundle a reference
voice. A separately licensed, previously generated `.npy`/transcript pair can
be used with no Python installation at runtime.

The Windows source installer builds `voice-neutts-worker.exe` and, when managed
llama.cpp is selected, installs the runner, decoder, and Air Q4 backbone. It does
not install a reference voice. Skipping managed llama.cpp also skips NeuTTS.

## Installer-managed runtime

The source installer pins `neutts-rs` v0.1.1 at
`ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f`. Its Cargo lockfile is honored with
`--locked`; the older `65771f3...` revision previously shown here had mismatched
`llama-cpp-4` manifest and lockfile versions and is no longer supported.

The installer installs LLVM/libclang plus pinned Rust 1.94.0 for x64 Windows
MSVC, builds the eSpeak-enabled `stream_pcm` example, and stages:

```text
<data-dir>\voice\neutts\
  active\runtime\stream_pcm.exe
  active\runtime\models\neucodec_decoder.safetensors
  active\runtime\runtime.json
  active\runtime\THIRD_PARTY_NOTICES.txt
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
and is converted by the upstream pure-Rust converter. The
1.1 GiB source checkpoint and temporary Cargo/build trees are removed after the
atomic runtime stage succeeds. The resulting runtime/backbone uses about 1.4
GiB, with several additional GB potentially needed during the build.

No startup/status path downloads files. Rerun `update.ps1` with managed
llama.cpp selected to repair or update the pinned runtime. `-SkipLlamaBuild` and
declining the managed llama.cpp prompt skip all NeuTTS provisioning.
The installer rehashes the active runner, decoder, and GGUF before reuse, and
the app repeats those checks once when app-managed NeuTTS is configured. Custom
runner overrides retain their own explicit cache contract instead of being
silently paired with the app-managed Air model.

## Custom runner build

Prerequisites on Windows are Rust (MSVC target), Visual Studio Build Tools,
CMake, and Git. Build the reviewed `neutts-rs` revision and its Rust
eSpeak-enabled streaming example:

```powershell
git clone --branch v0.1.1 --depth 1 https://github.com/eugenehp/neutts-rs.git
cd neutts-rs
git rev-parse HEAD # ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f
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

For the app-managed runtime, leave the runner override blank. For a custom
runtime, use the runner project's model conversion command and ensure
`models\neucodec_decoder.safetensors` exists above the selected runner;
MagicHandy walks upward to find it. In either mode obtain a compatible, licensed
`.npy` reference-code file and its verbatim transcript. Confirm a custom **Air
Q4** setup works directly before selecting the provider:

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
   **Browse...** for a custom runner. Select the optional reference WAV and
   required `.npy` codes on the computer running MagicHandy, then enter the exact
   transcript.
4. Save, then use the TTS status row to start and load the worker.
5. Send a test request before enabling **Speak chat replies**.

Start validates the adapter, runner, decoder, codes, transcript, and exact
backbone cache, then runs a bounded synthesis probe before reporting ready.
**Send test** remains the audible verification. Worker PCM streams while
synthesis runs. The core retains a bounded copy and
wraps it as a 24 kHz mono WAV for controller-owned browser playback. Stop or
request cancellation terminates the active runner process.

## Security and privacy

NeuTTS input and audio remain local after installation; see the network
limitation above. Paths and transcripts are ordinary local settings, not
secrets. Generated audio is retained only for the bounded recent request window
defined by the voice manager.
