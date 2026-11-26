# ElevenLabs Cloud Speech

ElevenLabs is MagicHandy's cloud speech-output option (ADR 0007). The protocol
worker `cmd/voice-elevenlabs-worker` speaks replies, and the core reads the
account's plan, models and voices for Settings (ADR 0075). Both are pure Go, with
no Python and no CGo, and they share `internal/voice/elevenlabs`.

The option is cloud-only and never a default or a silent fallback. Choosing it
sends reply text to ElevenLabs. The settings page says so, and points to the
local Faster Qwen3-TTS and Chatterbox modules for speech that stays on the
computer.

## Setup

1. Create an API key in the ElevenLabs dashboard (**Developers → API keys**).
   A key restricted to **Text to Speech** can speak. Add **Voices (read)**,
   **Models** and **User (read)** to browse voices and see the plan here, and
   **Voices (write)** to add Voice Library voices from MagicHandy. A key with
   none of the extra permissions still works, with a manual Voice ID field.
2. In **Settings → Voice**, enable voice, choose **ElevenLabs (cloud)** for
   speech output and paste the key. **Check key** reads the plan, models and
   the selected voice without spending credits. It can check a typed key
   before saving, and it never stores that key.
3. Choose a voice and model, save, and start the speech worker. The core runs
   the worker bundled beside it. A worker-path override no longer outranks
   the bundled worker.
4. Turn on **Speak replies** in Chat. Turning it on starts the speech worker.
   If speech fails, Chat keeps the reason on screen with a link to Settings.

## Key handling

The key is a private credential, handled like the Handy connection key:

- It is write-only through the settings API. Reads only report whether a key
  is set.
- It reaches the worker only through the `ELEVENLABS_API_KEY` environment
  variable, never on a command line.
- It is redacted from worker errors, core errors, setup reports and
  diagnostics.

Settings saves compare a salted credential revision. Saving unrelated
settings therefore no longer makes the next key save fail with a conflict.
Keys are trimmed when saved.

## Voices

The default voice is **Jessica** (`cgSgspJ2msm6clMCkdW9`), one of the default
voices every plan can use through the API. The earlier default ID
`21m00Tcm4TlvDq8ikWAM` now resolves to a Voice Library voice, which ElevenLabs
refuses for free plans with `402 paid_plan_required`. Saved settings that
still hold it move to the new default.

**Change voice…** opens a browser with two sources:

- **Your voices** lists the account's default, cloned, designed and saved
  voices, each with a free preview. Legacy default voices are labeled.
- **Voice Library** searches ElevenLabs' shared voices. Suggested searches
  are aimed at a companion voice speaking to one listener: Warm & close, Soft &
  breathy, Playful, Low & steady, ASMR and Conversational. Filters cover
  gender, age, language and sort. Voices that screen what they say (ElevenLabs
  live moderation) and voices that cost extra credits are hidden by default.
  **Add & use** saves a library voice to the account. Library voices don't
  take a custom-voice slot.

ElevenLabs lets only paid plans (Starter or higher) use Voice Library voices
through the API. On a free plan the browser says so, previews still play, and
own and default voices keep working.

Previews are fetched by the core from ElevenLabs' public preview storage
(`storage.googleapis.com/eleven-public-prod/` and `/eleven-public-cdn/`). The
URL is rebuilt on that fixed host, other paths are refused, and no credentials
are sent.

## Models and delivery

| Model | Use | Request limit in MagicHandy | Continuity across split requests |
| --- | --- | ---: | --- |
| Eleven v4 Turbo (default) | Expressive, quick to start, half the credits | 4,000 characters | surrounding text |
| Eleven v4 | Most expressive | 4,000 characters | surrounding text |
| Eleven v3 | Previous expressive generation | 5,000 characters | surrounding text |
| Multilingual v2 | Steady, with style and speed controls | 10,000 characters | previous request IDs |
| Flash / Turbo v2.5 | Fastest start, half the credits | 40,000 characters | previous request IDs |

The model list comes from the account. A model the account cannot use is
reported in Settings and refused on load with the available alternatives.
**Delivery** shows only the controls the selected model honors: stability,
similarity, style, speed and a language hint. Settings the model ignores are
never sent. An unset control keeps the voice's own setting.

## Speaking

- **Load** checks the key against the model list. A key that cannot read
  models is still allowed to speak. An invalid key, plan limit, exhausted
  credits or paused free use fails with ElevenLabs' reason.
- **Speak** sends a reply that fits the model's limit as one streamed
  request. Short replies (an estimated 150 seconds or less) arrive as 24 kHz
  PCM, which the browser plays as it arrives. Longer replies use 128 kbps MP3,
  and replies beyond about eight minutes use 64 kbps MP3. If an account
  refuses PCM, the worker retries once as MP3.
- Longer replies split at paragraph, sentence, clause or word boundaries,
  never inside an audio tag, and the request carries continuity context.
- Rate limits, server errors and dropped connections are retried up to three
  times, honoring `Retry-After` up to 10 seconds. A stream that delivers
  nothing for 20 seconds is ended as stalled. A spoken reply may run for up to
  five minutes.
- Errors keep ElevenLabs' request ID and reason, and become structured
  protocol errors. They never enter chat history (ADR 0003).

## Core endpoints

All are host-only. Reads are cached per key fingerprint, the model list for an
hour and voice lists for two minutes, with at most two upstream calls at once.

| Route | Purpose |
| --- | --- |
| `GET /api/voice/elevenlabs/account`, `POST` with a typed key | Key status, plan and credits, permissions, selected voice and model problems |
| `GET /api/voice/elevenlabs/models` | Speech models with their supported controls and languages |
| `GET /api/voice/elevenlabs/voices` | The account's voices |
| `GET /api/voice/elevenlabs/library` | Voice Library search |
| `POST /api/voice/elevenlabs/library/add` | Add a library voice (controller required) |
| `GET /api/voice/elevenlabs/preview` | Proxied voice preview from the allowlisted storage paths |

## Content policy

ElevenLabs' Prohibited Use Policy (updated 9 October 2026) applies to
everything spoken through it. MagicHandy does not filter reply text for
ElevenLabs. Library voices with live moderation screen what they say and can
refuse a reply, which is why they are hidden by default.

## Verification

`go test ./internal/voice/elevenlabs/... ./internal/voice/elevenlabsworker/...`
covers error classification, retries, streaming, stalls, request shaping,
splitting and the worker protocol against a mock API. The opt-in live check
runs against a real key:

```powershell
$env:ELEVENLABS_API_KEY = '<key>'
go test -tags liveeval -count=1 -run TestLiveElevenLabs -v ./internal/voice/elevenlabs/
```

It lists models and voices and speaks short PCM and MP3 samples, which spends
a small number of credits. It writes a report when `ELEVENLABS_LIVE_REPORT`
names a file.

## Related

- [ADR 0007](decisions/0007-voice-backend-selection.md): why ElevenLabs is the cloud option
- [ADR 0075](decisions/0075-elevenlabs-catalog-in-core.md): account and catalog reads in the core
- [ADR 0003](decisions/0003-voice-worker-boundary.md) and [the worker protocol](voice-worker-protocol.md)
- [Local speech modules](voice-tts-modules.md)
