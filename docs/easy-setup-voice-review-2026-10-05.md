# Easy Setup voice selection and Qwen reference guide

## Finding and change

Easy Setup originally always chose Chatterbox Turbo. ADR 0037 favored its
included Emily voice, since Qwen required a reference recording in Settings.
It rated the LLM's VRAM independently of voice. This did not follow the requested
Qwen preference or account for simultaneous LLM/TTS memory.

The backend now returns both voice choices and a combined allowance. Qwen
0.6B Base is preferred on NVIDIA unless the allowance is insufficient.
Chatterbox is the fallback, on CPU if its GPU allowance also exceeds the card.
Unknown GPU or model memory remains unconfirmed, with Qwen still preferred and
a "Fit unconfirmed" badge rather than a claim that it fits. The user can select
Qwen below the allowance; the warning does not prevent installation. Platform,
installer and NVIDIA requirements still apply.

The formula is `max(LLM minimum, LLM measured VRAM + voice + 1024 MiB)`;
voice budgets are 4096 MiB for Qwen and 2048 MiB for Chatterbox. A skipped LLM
uses zero. These voice budgets are planning allowances, not measured peaks.
Measured catalog values apply only to a matching model content hash at the
default or smaller context. A selected imported/unmeasured model or larger
context is labeled unknown instead of using another model's measurement.

## Guided flow and consultation

Claude Code 2.1.280 was consulted through its CLI with the explicitly requested
`claude-opus-5-5`. The CLI returned `canonicalModel: claude-opus-5-5`, first-party
provider, and a successful response. This was a design-only consultation with
tools disabled; no user audio, credentials or chat history were included.

Accepted advice: retain four Easy Setup steps; expand reference fields inline
only for Qwen; put the mandatory audio/transcript disclaimer on the choice and
panel; offer an explicit defer action; preserve a saved reference; keep the
speech-enablement gate in Go; do not claim transcript recognition or optimal
performance. Opus suggested switching to Chatterbox when memory is unknown;
the user's explicit Qwen preference was retained, with an unconfirmed badge
and warning. Voice synthesis/listening stays available through Settings after
completion rather than adding another preview stage to this small setup flow.

```text
Speak replies aloud?
  No: no voice installation.
  Yes: Qwen3-TTS or Chatterbox (backend marks its preference).
    Chatterbox: included voice; install on the assessed GPU or CPU.
    Qwen3-TTS: audio sample + exact transcript required to speak.
      Configure now: choose a local WAV and type its exact spoken words.
      Set up voice later: install Qwen and keep spoken replies off.
Install the selected components, then Finish.
```

Both Easy and Custom setup use the same guide. The WAV check accepts regular
files up to 16 MiB, PCM/float audio, one or two channels and 1–30 seconds, with
3–10 seconds recommended. It checks container consistency, sample alignment
and duration. It does not recognize speech, detect every silent/bad recording,
or verify the transcript. Transcript input is trimmed and bounded to 8192 bytes.
The host-authorized check endpoint is read-only; reference data is kept out of
installer arguments, progress and reports. Installation rechecks the file
before changing settings or enabling Qwen speech. Deferral and invalid/deleted
references cannot enable spoken replies. No recording, upload or new dependency
was introduced.

## Verification

- Boundary matrix: exact fit, one MiB below, 16/12/8/6 GiB, GPU unknown,
  unmeasured model, voice-only, skipped LLM, CPU, and missing installer.
- Backend reference checks: valid WAV/duration, malformed container/chunks,
  unsupported channels/alignment, absent/oversized transcript, missing path,
  too-short/long sample, controller admission, no settings mutation on check,
  enablement with a validated reference, and no replacement after a deleted file.
- UI tests: Qwen preference, warning with a permitted manual override, CPU
  fallback payload, disabling speech removes the warning, unknown memory,
  unsupported NVIDIA case, required fields, explicit defer, reference payload,
  and saved-reference preservation. All 763 tests in 99 frontend files pass;
  typecheck, five-locale audit and production build pass.
- Full Go tests, vet, golangci-lint and pure-Go build pass. The new check route
  has an explicit administrator/host admission row; role-matrix checks pass.
  Full local race tests pass; the available LLVM-MinGW compiler is used only
  for race instrumentation, while the shipped core is built with CGo disabled.

The isolated review app uses the RTX 5070 Ti's detected 16,303 MiB and estimates
13,520 MiB for the default LLM/Qwen/reserve combination. Browser review covers
module switching, explicit defer, restoring the sample guide and the always
mounted Stop on the narrow app viewport. The exact review app at
`http://127.0.0.1:50196` completed a real readiness generation through local
Ollama `huihui_ai/granite4.1-abliterated:3b`. It stays idle in simulator mode.
No hardware, microphone capture, actual voice installation, voice synthesis or
simultaneous LLM/TTS peak-memory benchmark was run for this change.

Byte budgets are recorded in [the scorecard](goal-scorecard.md). The recommendation
is a transparent deterministic policy; hardware-specific optimality still needs
representative simultaneous inference and listening measurements.
