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
   Git and `uv` after consent; every discovered executable must pass a version
   probe, and an unusable WinGet alias is bypassed through the real package
   binary;
3. installs a managed Python runtime and creates a private virtual environment
   below the MagicHandy data directory;
4. installs a pinned upstream revision with app-owned constraints for the
   dependency versions that are sensitive to the selected runtime;
5. verifies generated launchers, package compatibility where upstream metadata
   is reliable, native Python imports, and CUDA access before downloading the
   model;
6. downloads the chosen model only after consent;
7. configures the server for `127.0.0.1`, not all interfaces; and
8. calls the MagicHandy settings command so the provider, paths, port, and
   auto-launch choice are persisted in SQLite. Faster Qwen reference fields
   remain empty until the user completes them in Settings > Voice.

No preinstalled Python, uv, PyTorch, Git, or compiler is required. Faster Qwen
uses managed Python 3.11. The pinned Chatterbox dependency set uses Python 3.10
because its supported Windows Torch, torchvision, and ONNX packages are
available as wheels there; this avoids an accidental native build on a clean
machine.

The managed services request and return WAV audio. FFmpeg is therefore not a
runtime dependency. `qwen-tts` can print a SoX warning while it registers its
older 25 Hz tokenizer, but MagicHandy's pinned Qwen model uses the separate 12
Hz tokenizer and does not use native SoX for audio processing. The package's
import-time availability check may still attempt `sox -h`; that warning does
not mean setup omitted a required component.

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
seed and an optional Base-model tone instruction, then consumes one discarded
codec frame through the complete streaming path before reporting ready.
The warm-up prevents one-time model initialization from changing the first
Stopping after one frame avoids decoding an entire throwaway utterance while
retaining the warm first-visible-request behavior.

Retries also reuse a source checkout and managed environment left by a failure
before `module-state.json` was written. The installer records only its known
package-metadata directory in the checkout's private Git excludes. It never
cleans the checkout or ignores arbitrary files, so tracked edits and unrelated
untracked files remain a hard stop. New clones use a sibling staging directory,
and retries complete the empty no-checkout worktree produced by older installers
without misclassifying Git's deleted-file report as a user modification.
The verified Git directory is also added to the installer process PATH so
`uv` can resolve pinned `git+https` dependencies even when Git was discovered
through an absolute fallback path.

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

Settings that change process ownership or model conditioning, such as the
provider, module, device, port, reference WAV, or reference transcript, stop the
old worker and asynchronously restore every role whose auto-launch policy says
it should be ready. Faster Qwen seed mode, seed value, and tone instruction are
request controls instead: saving them does not reload the model or discard its
reference cache. A health check also verifies the managed server child rather
than trusting the Go adapter's cached state. If that child exits, the worker is
reported as not ready, the failure remains visible until reload or deliberate
unload, and the next explicit Start can launch and load it again.

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
Saving a changed tone applies it to the next request without restarting the
managed worker. The prompt is persisted with the other voice settings and is
never sent to generic OpenAI-compatible TTS providers.

Chatterbox accepts a local reference WAV as a named voice. The installer copies
that source into the module's voice directory and stores the resulting voice
name. The original file remains untouched. Without a reference, the pinned
server's `Emily.wav` sample is installed as the initial voice.

For NVIDIA installs, the script selects the pinned upstream CUDA 12.1
requirements on RTX 20/30/40-series GPUs and CUDA 12.8 on compute-capability
12.x hardware such as the RTX 50 series. Explicit CUDA selection fails early
when NVIDIA driver tools are unavailable; `-Device cpu` remains the portable
fallback.

Chatterbox's pinned engine and server intentionally disagree on one legacy
metadata bound: `descript-audiotools` declares `protobuf<3.20`, while ONNX 1.16
requires a newer protobuf and the server maintainers validate that newer
runtime. MagicHandy mirrors the pinned server's repair with protobuf 4.25.8,
then imports the original and Turbo engine paths, ONNX, audio libraries, and
the selected PyTorch backend before any model download. It does not run a
metadata-wide `pip check` for that provider because the known obsolete bound
would reject the supported runtime; Faster Qwen retains the strict check.

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
`"instruct": "Speak quietly and close to the microphone at an unhurried pace."`.
The built-in tone presets are written this way on purpose: naming the delivery
mechanics (volume, pace, pitch range, mic distance, breath) reads better than a
bare emotion adjective, which the model tends to act out. The Go adapter adds these
nonstandard fields only for that provider.

**Keep an `instruct` short — this matters more than any individual phrasing.**
Every clause is a constraint the model has to satisfy simultaneously and hold for
the length of the utterance. A preset stacking five or six of them leaves only an
extreme corner of the model's range to satisfy them all in, and extreme corners
are where the artifacts live: straining, shouting, nasality. One defining
mechanic, one contour rule, and the shared ease anchor is the whole budget.
Anything more belongs in a Custom prompt.

That budget was learned late, because the presets kept previewing clean and
failing in use. **The TTS preview button now speaks a two-sentence sample for
exactly this reason.** It used to speak four words, over which there is barely one
intonation contour to get wrong, so a preset that came apart over a real
multi-sentence reply still sounded fine in the settings panel. Judge a tone change
on something at least as long as the replies it will actually speak.

**Lock the speaker identity explicitly.** Every built-in preset ends with a clause
saying the directions are about delivery only, not about who is speaking, and it
is the single most effective thing here. An `instruct` is a natural-language
description of *how someone speaks*, and in a multilingual model trained on
described audio that kind of text correlates with **who** speaks that way, not
only how — adjectives like relaxed, easy, or unhurried describe a speaker as much
as a delivery. Without the lock, the model is free to pick a matching speaker, and
did: Commanding arrived sounding Brazilian on one seed, and Warm faintly Jamaican
on wording it had previously been fine on. Five earlier rounds each chased the
individual word that seemed to be the cue, and each time a different preset
drifted somewhere else. Unlike a prosodic demand, the lock constrains the search
space rather than pushing the voice anywhere, so it adds nothing to sustain.

The word-level rules below still matter — they are what stops a preset asking for
non-English prosody outright — but treat them as second line now, not first.

Four phrasings to keep out of an `instruct`, because a multilingual model reads
them as a cue to change accent rather than delivery. They were all found the hard
way, from a Commanding preset that arrived in an audibly foreign accent on one
seed and sounded timid on the rest:

- **Flattening the pitch contour** (`level`, `flat`, `monotone`, `evenly`,
  `steadily`, `uniform`, `very little pitch movement`). English declaratives close
  on a falling contour; a level close is the prosody of a syllable-timed language.
  It also costs the tone its conviction, since a sentence that never resolves
  downward sounds tentative. To rule out uptalk, ask for a *falling* close, never
  a flat one. Watch the synonyms especially: Commanding shipped saying "Speak
  **evenly**" and came back with an accent, having walked straight past a guard
  that only matched the literal words above.
- **Relaxing articulation** (`loose articulation`, `slurred`). Consonant
  precision is one of the strongest accent cues a synthesizer has. Put lightness
  in pace and pitch, not in diction.
- **Making the word the prosodic unit** (`every word`, `rather than rushed
  together`) — the same mistake from the other side. Excited asked to keep "every
  word clearly articulated rather than rushed together" and got exactly that:
  each word released separately with an abrupt stop at the end, and a thin, nasal
  quality from the sustained effort. English runs words together inside a phrase,
  and forbidding that buys careful diction at the cost of sounding synthetic. Aim
  articulation at the phrase and let the words connect within it.
- **Shifting the pitch baseline** (`lifted pitch`, `raise the pitch`). This
  changes the apparent speaker rather than the delivery, and raising it thins the
  voice toward sounding younger. Ask for movement *within* the range instead:
  wider on stressed words, gentler across a phrase.

A fifth failure is not about accent but about phonation: **don't stack the
reducers.** Quiet, slow, low, and falling all push the voice the same direction,
and the bottom of that stack is where phonation gives out into press or creak,
which is heard as straining. Tender asked for softly *and* slowly *and* low
volume *and* audible breath *and* a falling close, with nothing holding the voice
up. Every preset now ends with a shared ease anchor naming the ceiling — relaxed
and unforced the whole way through, at a comfortable volume — and it says "the
whole way through" because sustaining the delivery is the part that fails.

**State that anchor positively.** It first read "never pushed, strained, or louder
than it needs to be", and Warm came back stressed, having been fine on the older
wording "relaxed and unforced". The working theory is that negating a continuous
acoustic attribute puts it in play: there is no discrete "strained" setting to
switch off, so naming loudness and strain mostly makes them salient. Negation
still earns its place in the framing clause, which rules out a whole speaking
register the model can recognise and step away from. Rule of thumb: **negate a
register, describe an effort level.**

**Do not drop "or an announcement" from the framing.** Commanding once needed a
separate framing constant to opt out of it, back when that preset earned its
authority from volume and any cue to back off worked against it. Rewriting
Commanding around steadiness made the opt-out unnecessary, and merging the two
framings quietly removed those three words from the four presets still relying on
them — Warm immediately came back sounding like a sports announcer. A clause that
has stopped applying to one preset may still be load-bearing for the others, so
`TestTTSTonePresetsResolveToReviewedInstructions` pins this phrase separately from
the constant that contains it.

Authority is the case where this bites hardest, and Commanding has now failed
three separate ways. It first earned authority from volume — "more weight and
volume" on important words over a full chest tone — which across a real reply is
a repeated push, and it strained. Rewriting it around calm evenness then hit two
more: "**evenly**" flattens the contour exactly as "level pitch" would, and
"steadiness **rather than force**" negates a continuous effort attribute, which
is the Warm lesson again. Both shipped.

What is left is authority with nothing to sustain: a measured, unhesitating pace,
space around the phrases that matter, and sentences that arrive at a settled
ending rather than trailing off. Pacing and pausing cost the voice nothing to
hold for three sentences, where loudness and a deep drop on every sentence do.
The general shape of the mistake: **if a preset asks for authority, check what
the voice has to spend to produce it.**

`TestTTSTonePresetsAvoidAccentDriftLevers` holds the built-in presets to the four
accent levers, and `TestTTSTonePresetsStayShortAndAnchored` to the length budget
and the ease anchor. A seed makes a bad sample reproducible, but the instruct
text is what decides whether that sample was reachable at all, so re-test a
prompt change on the same seed rather than a fresh one.

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

When chat speech and a local LLM share one GPU, a new message can either
interrupt active/queued speech (the default) or let speech finish. This is an
explicit Settings > Voice tradeoff. Interruption preserves completed request
history and playable audio but cancels unfinished TTS work. The managed Faster
Qwen wrapper uses a two-chunk producer queue, propagates a disconnected HTTP
consumer to the producer, and closes the upstream streaming generator so an
abandoned utterance does not retain the CUDA inference lock for all remaining
text. The currently executing model chunk is not forcibly interrupted.

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

Follow-up evidence from 2026-08-01 on an RTX 5070 Ti with the installed
0.6B Base module: the prior full hidden warm-up reached ready in 14.82 seconds.
The one-frame warm-up reached ready in 13.06 seconds while preserving about
0.40 seconds to first audio and 0.86 seconds total for the same 3.6-second WAV.
A tone and seed save completed in 7 ms with the worker start timestamp
unchanged; a request through the current Go core, worker, and managed server
then produced a valid 24 kHz mono WAV in 1.52 seconds. These are local
development measurements, not a cross-hardware release guarantee.
