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
is provenance, not a runtime input. A bundled or previously generated `.npy`
voice can be used with no Python installation at runtime.

## Build the runner

Prerequisites on Windows are Rust (MSVC target), Visual Studio Build Tools,
CMake, and Git. Build the reviewed `neutts-rs` revision and its pure-Rust
eSpeak-enabled streaming example:

```powershell
git clone https://github.com/eugenehp/neutts-rs.git
cd neutts-rs
git checkout 65771f3
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
starting MagicHandy. Also obtain a compatible `.npy` reference-code file and
its verbatim transcript. Confirm `stream_pcm` works directly before selecting
the provider:

```powershell
.\stream_pcm.exe --codes C:\voices\reference.npy `
  --ref-text "The exact words in the reference recording." `
  --text "NeuTTS setup test." > test.pcm
```

MagicHandy launches the runner with `HF_HUB_OFFLINE=1`. A missing model fails
clearly; load, status, and speech requests never download files.

## Configure and test

1. Open **Settings > Voice** and enable voice workers.
2. Under **Speech output (TTS)** choose **NeuTTS Air (local)**.
3. Set the runner, reference WAV provenance, `.npy` codes, and transcript.
4. Save, then use the TTS status row to start and load the worker.
5. Send a test request before enabling **Speak chat replies**.

Worker PCM streams while synthesis runs. The core retains a bounded copy and
wraps it as a 24 kHz mono WAV for controller-owned browser playback. Stop or
request cancellation terminates the active runner process.

## Security and privacy

NeuTTS input and audio remain local. Paths and transcripts are ordinary local
settings, not secrets. Generated audio is retained only for the bounded recent
request window defined by the voice manager.
