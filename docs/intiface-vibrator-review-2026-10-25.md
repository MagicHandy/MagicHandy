# Intiface vibration and independent model control — 2026-10-25

Follow-up: the [capability-level evaluation](model-capability-review-2026-10-25.md)
adds opt-in Intermediate and Advanced local contracts. The findings below record
the initial implementation; Simple remains the compact default, while a capable
local model may explicitly use the same independent tools as external models.

## Report and diagnosis

The alpha.73 report described an Edge 2 that worked with Test but usually did
not respond to chat, especially beside a Handy, and did not restart after Esc.
The supplied failure-only archive retains three successful starts, each stopped
as `starving` after roughly 200 ms (200.207–200.958 ms). The default speed-follow
renderer required 250 ms of history after a 120 ms pre-roll, while engine health
was checked at 200 ms. A fully buffered startup therefore looked exhausted.
This is reproduced with simulated Buttplug, both alone and beside an Intiface
Handy, and fixed without weakening genuine tail-exhaustion detection.

Four retained Layered geometry failures include three successful repairs and
one failed repair. The export contains neither the original prompts nor raw
replies, so it does not establish a precise model failure rate or the missing
edit. Repair feedback now identifies the selected geometry, its requirements
and the conflicting structured fields. No user-language classifier was added.
The reported E4B model was unavailable locally; the model experiments below use
an installed Gemma 12B and cannot establish E4B performance.

Esc intentionally resets live intensity to zero. A model must not rearm it.
The device panel now explains how to re-enable vibration, distinguishes a silent
connection, and displays each motor's last quantized command. Test availability
comes from the backend, including motor-step limits. A vibration-only session
uses motor readouts instead of animating a Handy that is not moving.

## Implemented behavior

Previously, all vibrator modes converted the shared linear timeline: speed,
depth or steady. Merely mentioning the vibrator in prompts did not give the
model an independent output. [ADR 0057](decisions/0057-independent-vibration-envelope.md)
adds a second semantic envelope to the existing engine, clock and Stop lifecycle:

- External Layered and Creative v2 chat/Autopilot can select steady, smooth wave,
  rounded pulse, off, or return to the saved follow profile. The envelope controls
  relative drive **within the live intensity the human has armed**. It cannot
  raise intensity/ceilings, select devices, or control individual motors.
- A vibration-only edit leaves the Handy plan, phase and buffered points intact.
  Starting vibration from idle does not position or play an included Handy.
  A later linear target, including Library or Autopilot, uses normal prepared
  startup and preserves the envelope. Pause/Resume retains the envelope;
  a fresh run without an envelope uses the saved follow profile.
- Backend state determines availability. Disconnected or retired outputs are
  not advertised as live; stopped envelopes are not presented as current.
  If vibration loses arming during a compound request, a valid linear update
  can apply with an explicit vibration error. Autopilot suppresses a spoken
  claim for a scalar change it could not apply.
- Follow is an immediate control command: it supersedes old pending replies.
  Model repair generations, Stop admissions and selected-device epochs fence
  stale edits. Cancellation before an envelope edit leaves healthy playback
  alone. No independent motion scheduler or chat-to-transport path is added.

The renderer now handles pre-roll/seek history correctly. Prepared writes are
rechecked against live limits at the actual writer; quantization never rounds
above intensity, ceiling or the Test cap. Test is covered by the pacer watchdog,
with completion grace. Late acknowledgements cannot clear a newer send or fault
a replacement run. Bounded scalar command/coverage/Stop events are included in
trace reports (128 events, with a dropped-event count); no keys or raw model text
are added to those events.

## Model formats and quality

Real requests used GPT 6.1 Sol, low reasoning, and installed
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`. The captures contain
synthetic requests only. Existing ChatGPT authentication was read in place;
credentials were not copied into review data. Tests call the production parser
and retain rejected selections, corrections and provider failures.

Three formats express the same engine capabilities: optional flat vibration,
required nullable flat vibration, and separate nested linear/vibration groups.
The retained contract is **flat with explicit nullable vibration**. OpenAI's
strict adapter already makes optional fields required-nullable on the wire;
optional versus explicit is therefore not a clean independent treatment there.

| Comparison | Result | Median generation time |
|---|---:|---:|
| GPT, optional flat, ten pattern requests | 10/10 intent checks | 5.178 s |
| GPT, explicit flat, same ten requests | 10/10 | 6.049 s |
| GPT, grouped, same ten requests | 10/10 | 5.177 s |
| GPT, explicit flat, six exact-preservation requests | 6/6 | small-sample follow-up |
| GPT, grouped, same six requests | 6/6 | small-sample follow-up |
| Gemma independent control, optional / explicit / grouped | 8/24 / 12/24 / 20/24 | no local capability enabled |

Claude Opus 5.5 Max independently reviewed blinded A/B/C pattern captures. It
found the formats effectively tied; grouped preserved unrelated Handy settings
better in one case, but the targeted six-case follow-up did not reproduce a
format advantage. All formats produced the counted two-short/one-full phrase,
full-device reach and separate vibration correctly in the matched quality set.
A schema pass alone is not the quality score: parameter choices, replies,
unrequested changes and the shared-engine plots were also inspected.

The GPT independent-control matrix passed 24/24 in one run. Later confirmation
had 23/24 with one provider timeout; that case passed when repeated. Earlier
runs also retained a provider timeout and a corrected strict-schema compatibility
failure for unarmed states. These are not hidden by aggregating only successes.
The live HTTP chat-to-engine-to-Buttplug-simulator sequence passed all four steps:
scalar-only start, explicit Handy start, scalar-only wave change with identical
Handy plan, and scalar off with Handy continuing. Final global Stop zeroed the
intensity. Four real GPT Autopilot hold/independent-edit cases also passed.

Grouped output materially helped Gemma, but its remaining failures were
misleading claims of restarting vibration while emitting off or requiring
repair at zero intensity. A repeat with the state-constrained schema still
passed only 20/24. Local prompts and schemas therefore remain unchanged, tested
across five modes and running/stopped states. Their existing shared-profile
control remains available; independent model control is external-only.

## Context and latency

The production HTTP path exposed a default-context failure missed by the
initial 128K trials: the full Layered contract plus independent output facts
exceeded the conservative 32K admission budget. The external minimum is now
64K, including migration of an explicit legacy 32K connection. Its history cap
is **still 64 messages / 24 KB**. 128K/256K selections retain their previous
larger history policies, and local context settings are untouched.

Full versus shorter guidance at the same 64K budget passed 10/10 pattern and
6/6 exact-preservation checks for each version. Pattern medians were 5.233 s
full versus 4.489 s short; preservation medians were 4.393 s full versus 5.308 s
short. This small comparison does not establish a consistent latency benefit.
The full guidance is retained. The budget is an application admission ceiling,
not a request to the provider to fill that window, and the compact tier sends
no additional history merely because its allowance increased.

## Visual review and artifacts

Ignored evidence is under `.scratch/intiface-vibrator-review-2026-10-25/`:
`all-model-captures.json`, `motion-atlas-final/`, `vibration-atlas.json`,
`vibration-plots/`, `gpt-app-independent-v2.json`, the blinded Claude review and
follow-up, and named raw/log files for every evaluation. Initial grouped-adapter
failures were caused by test normalization, and an early preservation predicate
mistook an explicit full-width ceiling for the equivalent implicit ceiling;
both failed captures remain alongside corrected tests.

Both final motion overview sheets and the vibration envelope overview sheets were
inspected. Counted phrases remain distinct; the full sweep reaches 0..100;
alternating-end choices expose the intended slower-lower/faster-upper structure.
The vibration plots show distinct rounded pulses and continuous waves, with
steady/off cases correctly flat. The 600 ms initial envelope transition is
visible; the plotted relative drive is before human intensity, transport fade
and motor quantization. It is not measured hardware smoothness. Actual app
captures record the resulting targets, dispatch diagnostics and Stop.

## Validation and remaining physical acceptance

Regression coverage includes solo/linked startup for all three follow profiles,
real exhaustion, offsets/seek, atomic frame rejection, live-limit races, coarse
steps, stalled Test, stale acknowledgements, selected-device epochs, independent
output isolation, inactive-Handy startup, later prepared linear startup,
canceled edits, Pause/Resume, stale repair, retired participants, model-role
resolution, Follow admission, default external budget, and honest motor readouts.

Final checks: `go test ./...`, `go test -race -timeout 20m ./...`, `go vet`,
`golangci-lint` (0 issues), typecheck, localization, all 989 frontend tests, the
canonical UI build and the pure-Go build passed. Windows race validation used
an isolated w64devkit compiler cache; it is not a runtime dependency. The current
review app completed a real local text-only chat in one provider call, without
repair/fallback or motion, plus `check-review-llm.ps1`. Claude completed the
independent code and blinded-format reviews; its attempted post-fix re-review
hit the CLI session limit, so there is no claimed final Claude sign-off.

No physical vibrator was connected in this work. Before release acceptance on
an Edge 2, verify first chat playback alone and beside each Handy transport;
low/high motor-step behavior; independent pulse/wave timing and transitions;
Stop-to-silence; user rearming; reconnect; missed acknowledgements/radio loss;
continue-healthy versus stop-all; and vibration-only to combined startup. Retain
Intiface version, device firmware, timing/ack statistics and a diagnostic export.
All motors in the selected vibrator still share one envelope. Pulse duty cycle
and independent per-motor control are not exposed in this first contract.
