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

The Windows source installer builds `voice-neutts-worker.exe` only. It does not
install the external runner, model files, or reference assets, and the UI reports
that adapter-only state as incomplete rather than offering a Start button.

## Build the runner

Prerequisites on Windows are Rust (MSVC target), Visual Studio Build Tools,
CMake, and Git. Build the reviewed `neutts-rs` revision and its pure-Rust
eSpeak-enabled streaming example:

```powershell
git clone https://github.com/eugenehp/neutts-rs.git
cd neutts-rs
git checkout 65771f3a91a811725ccb51ba6f679528cd6e0325
cargo build --release --example stream_pcm --features espeak
```

Set **stream_pcm runner path** to the resulting
`target\release\examples\stream_pcm.exe`. Build the MagicHandy protocol
adapter with:

```powershell
go build -o voice-neutts-worker.exe ./cmd/voice-neutts-worker
```

Place `voice-neutts-worker.exe` beside `magichandy.exe`, under the app data
directory's `tools` folder, or select it with the advanced worker override.

## Install artifacts explicitly

Use the runner project's documented model download/conversion command before
starting MagicHandy. `models\neucodec_decoder.safetensors` must exist under the
`neutts-rs` project root; MagicHandy finds that root by walking upward from the
selected runner and launches `stream_pcm` there. Also obtain a compatible `.npy`
reference-code file and its verbatim transcript. Confirm the **Air Q4** backbone
used by MagicHandy works directly before selecting the provider:

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
3. Use **Browse...** beside the runner, optional reference WAV, and `.npy` codes;
   the chooser selects paths on the computer running MagicHandy. Enter the exact
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

NeuTTS input and audio remain local after its external runtime/model cache has
been prepared; see the network limitation above. Paths and transcripts are
ordinary local settings, not secrets. Generated audio is retained only for the
bounded recent request window defined by the voice manager.
