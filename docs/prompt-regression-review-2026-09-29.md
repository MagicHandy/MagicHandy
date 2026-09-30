# Prompt regression review — 2026-09-29

## Prior evidence reviewed

Read the recent MagicHandy Claude Code conversation, including the September
24–28 motion, Lab and video work, and its earlier voice-register findings.
The transcript was checked against these committed records rather than treating
every preliminary claim as a result:

- [Naturalness, holds, pace and recall](motion-naturalness-review-2026-09-24.md):
  Gemma 12B pace/variety cases reached 14/14 in each continuous mode; the
  established request suite remained 74/74. Standing holds, release, elapsed-time
  planning and recall were measured separately. Recall selected the intended
  previous score in only 4/8 trials. The initial speech-repetition claim was
  withdrawn after fixing a harness that had incorrectly published motion plans
  as dialogue. Do not revive that claim as evidence for a voice rewrite.
- [Depth mapping](motion-depth-review-2026-09-25.md): Creative v2 met 44/45 depth
  requests after the shared depth frame, versus 19/45 before. Layered met only
  9/45 and still has band-relative geometry limitations. These are known model
  and representation limits, not newly introduced video-chat regressions.
- [Lab stroke vocabularies](lab-stroke-modes-review-2026-09-26.md): all 216
  evaluated results were rendered through the engine; Plain words met 27/27
  final depth cases. These remain Lab experiments, not production replacements.
- [Voice parity](chat-voice.md) and the August 1 register follow-up: persona
  temperament must survive the selected voice and reaction style. Full stock
  replies in machine examples previously displaced the selected register.
- [Video ownership](decisions/0032-video-curation-and-remote.md): the earlier
  Gemma remote check declined 9/9 motion requests. It did not cover the new
  five-language matrix or schema/history contradictions found here.

## Findings and changes

1. **Disabled motion was still advertised at the end of the prompt.** The final
   guard listed start/update/target/stop even for chat-only turns. These turns
   now get a reply-only guard, with optional mood only when enabled. Providers
   that support schemas also receive the matching grammar. Backend capability
   enforcement remains in place for providers that ignore it.
2. **Old control envelopes reintroduced disabled capabilities.** Chat-only
   history could replay a previous motion command or serialize ordinary speech
   as `motion.action=none`. It now shares continuous mode's speech-only history,
   retaining the actual dialogue and the existing history limits. The same
   applies to chat-only Autopilot check-ins and their repair history.
3. **Video instructions repeated a known voice-regression pattern.** Late stock
   English replies and “every request gets the same answer” displaced natural
   responses. The note now describes authority without prescribing reply prose,
   explicitly retains voice/persona/language, and permits a brief factual
   explanation when the person asks about unavailable controls. The existing
   Warm, Intimate and Explicit register text is unchanged.
4. **Source selection was presented as live playback.** Selecting Script does
   not prove that a video is playing or that synchronization succeeded. The
   prompt no longer asserts that the script is moving the device “right now,”
   or promises that selecting Chat immediately takes control. The instructions
   distinguish opening the chat panel from choosing the Chat motion source.
   That explanation is localized in all five built-in languages.
5. **Autopilot repair lacked the interactive path's language reminder.** Speech
   repair now carries the selected language, voice and persona and asks to
   retain valid speech when only structure/timing needs correction. Motion-only
   repair does not acquire a reply instruction. Continuous modes still reject
   invalid proposals rather than gaining a repair pass.

Emergency Stop's deterministic path, model motion authorization, depth mapping,
standing holds, score recall, geometry, speed limits and dispatch are unchanged.

## Verification

The initial regression tests failed on all four groups: disabled command
instructions/history, absent chat-only schema, video stock replies/false running
claims, and missing Autopilot repair-language guidance. Unit coverage now also
checks all five languages and four voices for both video owners, including
preservation of persona text. The HTTP test still sends an unwanted model motion
command and verifies zero transport commands.

Compared 240 composed motion-enabled prompts against `9e36b2be`: five built-in
languages × four voices × four modes × interactive/motion-Autopilot/speech-
Autopilot composition. Every byte and hash was identical. This is a prompt
construction comparison, not a new stochastic motion-quality benchmark. The
existing continuous history, request mapping, parser, Stop and lifecycle tests
remain the executable regression checks. No new motion proposal was generated
or engine geometry changed, so this pass does not claim a new motion atlas or
physical feel review.

Live checks use the installed `n0404n0404` Gemma 4 12B IT heretic Q4_K_M artifact
with SHA prefix `239ec3629639`, through llama.cpp b9966 CUDA. A separate loopback
worker uses one 16,384-token slot, reasoning off and a 256-token output limit.
No user profile, credentials, conversation history or hardware is involved.

- Video capability/language: **20/20** final replies, two runs of Script/Off
  across English, Spanish, Brazilian Portuguese, Simplified Chinese and Japanese.
  Each passed the production `Service` without repair/fallback, had only `reply`,
  named the Chat source control and retained the selected language.
- Ordinary conversation: **4/4** Utility/Warm greetings stayed conversational,
  without setup or source instructions, under both video owners.
- Autopilot speech repair: **5/5** malformed timing fixtures were repaired through
  the production service in the selected language with no motion. The model
  rephrased the otherwise valid greeting despite the preservation instruction;
  exact wording preservation remains a model limitation, not a proven guarantee.

Manual reading complements the language heuristics. The first candidate passed
schema/language checks but two Japanese replies named the wrong control. Adding
localized source instructions corrected that in both final repeats. The baseline
also copied the English stock reply verbatim and emitted disabled motion fields
in two Portuguese replies; its parser suppressed those fields. One baseline
request failed with a transport EOF. Early language checks also falsely rejected
valid translations because their vocabulary assumed affectionate dialogue;
these new tests use vocabulary appropriate to capability explanations/greetings.
All intermediate logs remain available, including these failures.

This pass checks preservation of the Explicit register in composition; it does
not repeat the earlier explicit-prose quality corpus or claim new quality scores
for it. Live outcomes are limited to this model and the neutral test requests.

Reproduce the new optional live checks against a loopback model:

```powershell
$env:MAGICHANDY_LIVE_LLAMA_URL = 'http://127.0.0.1:18182'
go test -tags liveeval ./internal/chat -run '^TestLive(VideoChatCapabilityAndLanguage|VideoChatStillConverses|AutopilotSpeechRepairLanguage)$' -count=1 -v
```

Local evidence is under `.scratch/prompt-review-20260929/`: baseline regression
and model logs, final video/conversation/repair logs, before/after prompt hashes,
full checks and app readiness. Runtime files are deliberately not committed.

## Completed checks and review app

Full `go test ./...` and Windows `go test -race ./...`, `go vet`, default
golangci-lint, `CGO_ENABLED=0` build, frontend typechecking, all 737 frontend tests
and the frontend build pass. No embedded UI files changed; its existing 900 kB
advisory remains. An additional lint pass with the optional `liveeval` tag finds
16 existing issues in untouched live-evaluation files (length/complexity, artifact
paths and identifiers). The same tagged lint scoped to changes from `9e36b2be`
reports zero new issues; no gate or configuration was weakened.

The final app runs on `http://127.0.0.1:50255/#/chat` with fresh isolated data,
simulated motion, LLM motion Off and the remote listener disabled. It passed
`scripts/check-review-llm.ps1`. A real text-only request through the visible Chat
page completed in 725 ms (350 ms to first token), one provider call, with no
malformed response, repair or semantic fallback. The backend reports no active
motion; the review tab remains open on that reply. This is a single review
sample, not a latency benchmark.
