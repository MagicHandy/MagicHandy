# ElevenLabs Cloud TTS Worker

The premium cloud TTS path from ADR 0007, implemented as a Phase 12 protocol
worker (`cmd/voice-elevenlabs-worker`). Pure Go, no Python, no CGo. Cloud
and clearly optional: it sends reply text to ElevenLabs, so it is a
deliberate privacy choice the user makes by configuring it — never a
default or a silent fallback.

## Setup

1. Create an ElevenLabs account and API key (their dashboard → API keys).
2. Build or download the worker binary:
   ```powershell
   go build -o voice-elevenlabs-worker.exe ./cmd/voice-elevenlabs-worker
   ```
3. In **Settings → Voice**: enable voice workers, set the **TTS worker
   path** to the binary, paste the **ElevenLabs API key**, and save. Then
   Start the worker and Load (load validates the key against the account
   endpoint, so a bad key fails immediately with a clear message).
4. Turn on **Speak chat replies** to hear replies (the reply text is what
   gets sent to ElevenLabs — see the privacy note above).

## Key handling (private credential)

The API key is stored like the Handy connection key: written through the
settings API (write-only), reduced to a `set/unset` flag on every read, and
handed to the worker process **only** via the `ELEVENLABS_API_KEY`
environment variable at spawn — never on the command line (visible in
process listings), never in worker status, stderr tails, or protocol
frames (the worker scrubs it from error text). Covered by tests at the
config, worker, and process level.

## Voice and model selection

The routine settings surface stays minimal (one key field). Voice, model,
and output format are worker arguments:

```
-voice-id 21m00Tcm4TlvDq8ikWAM   (default: "Rachel", a stock voice)
-model-id eleven_multilingual_v2 (default)
-format   mp3_44100_128          (default)
```

Set them in the **TTS worker arguments** field. Instant voice cloning uses
a voice created in the ElevenLabs dashboard: clone there, then point
`-voice-id` at it.

## Behavior behind the protocol

- `load` validates the key (401 → clear "check the key" error;
  missing key → "add the API key in Settings → Voice").
- `speak` splits the reply into sentences and streams one API call per
  sentence, so first audio arrives after the first sentence renders, and
  cancellation lands between or inside sentence streams (live HTTP request
  canceled mid-stream).
- Failures are structured protocol errors that terminate in the voice
  request log; they never crash the core app and never enter chat history
  (ADR 0003).
- Audio chunks are MP3 by default; the core's bounded retention and the
  controller-only (audio lease) playback endpoint apply unchanged.

## Relationship

- ADR 0007 (selection rationale: expressive + high-fidelity cloning, no
  local VRAM, explicit cloud trade-off)
- ADR 0003 / `docs/voice-worker-protocol.md` (the boundary this implements)
- `docs/neutts-air-spike.md` (the local cloning counterpart)
