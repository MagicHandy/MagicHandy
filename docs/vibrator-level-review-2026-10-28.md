# Vibrator level and motion-mode continuation review

Claude's stopped session left the implementation on `claude/vibrator-model-level`,
based on alpha.76 (`43ac5d58`). Its final requested scope was model-chosen
starting vibration (or following Handy intensity until chosen), model control
under a saved user Max, original Creative hidden behind a restore setting,
and Creative v2 displayed as Simple. ADRs 0061 and 0062 describe that behavior.

## Follow-up findings and fixes

- Failed manual starts could clear Stop suspension after applying a model level,
  because rollback confused a model-level revision with a newer human resume.
  Resume now has its own revision; changing level cannot defeat rollback.
- Turning model control off left independent envelopes active. The transport
  now follows the movement profile when the user selects manual control, even
  if a prior model envelope remains in the saved target. Stop is unchanged.
- Paused playback accepted local scalar edits despite the independent-envelope
  contract forbidding them. Schema and dispatch now agree on paused authority.
- The Intermediate model often expressed one speed twice: absolute 35 and
  relative -10 against 45. The parser accepts that exact equivalence once;
  differing results and malformed/duplicate fields still fail. It reads no
  user language and does not pick a preferred speed or pattern.
- Independent envelopes still inherited a 600 ms retarget fade. That fixed ramp
  is removed to honor the requested Advanced control; startup fade, rounded
  wave/pulse shapes, Max and the existing write/Stop fences remain.
- The connection panel could say “Follows The Handy” while an Advanced envelope
  was active. It now reads the backend's current envelope. Stale “raise the
  intensity” confirmation copy is replaced, and Stop help says motion requests
  can resume instead of promising that every message resumes.
- Live UI testing found that a question's echoed positive level could clear
  suspension without starting playback. The final fix preserves command
  acceptance and rolls back suspension only when dispatch finishes without
  playback. It adds no language classifier, resume keyword or new user step.
- Continuation testing exposed an older problem: stopped chat discarded the
  engine's last score and fell back to a newly seeded default. It now receives
  the saved continuous score with `running:false`, and Advanced models also
  receive a separately named `previous_envelope`. These are context, not
  permission to run. The guide clarifies that movement-following vibration
  needs a timeline start; a follow-only settings change cannot start idle
  output.

## Real-model evaluations

`TestVibrationLevelLive` uses the production prompt, schema and parser. Its
13 cases include stronger/gentler/off/full, compound slower-strokes/stronger-
vibration, faster only, questions, soft/plain/strong starts, a little more,
back on, and a stronger request after an actual start-style conversation.
Passing requires the intended output, preservation of unrelated motion, one
provider call, and no repair or fallback.

| Model and contract | Final results | Observed median request time |
| --- | --- | ---: |
| Gemma E4B Q4_K_M, Simple, 16K / 256 output | Layered 24/26 | 1.304 s |
| Gemma 4 12B Q4_0, Intermediate, 32K / 512 output | Layered 26/26; Simple motion 26/26 | 0.738 s |
| GPT 6 Sol, Advanced, Low reasoning, 64K | Layered 13/13; Simple motion 13/13 | 3.304 s |

Local cases use seeds 10252026 and 10262026. These are small, warm-host samples,
not guarantees across hardware, quantizations or future provider versions.
The E4B failures are retained: a soft-start truncation and a strong-start
geometry conflict whose repair truncates. Longer E4B guidance gave 20/26 and
a 384-token budget gave 23/26; neither ships. Simple capability therefore
keeps the better compact guide and remains excluded from scalar vibration in
Simple motion (formerly Creative v2), where Claude's earlier trial was 8/20.

GPT's independent vibration-only start is valid with linear action `none`.
The fixture was corrected to accept that for “Start with strong vibration”
instead of requiring The Handy to start too. This changes scoring, not dispatch.

The additional `TestManualVibrationResumeProductionChatLive` suite exercises
the real HTTP chat path, shared engine and in-process Buttplug simulator.
Gemma 12B and GPT 6 Sol each pass **16/16** final turns: questions interleaved
with “continue where we left off”, “resume both devices”, same-rhythm
vibrator-only continuation, and a new-pattern request. Unchanged continuations
must retain the actual saved FlowSpec, not merely report running. Questions
must succeed without clearing suspension; no repaired or fallback response
counts as a pass. Initial captures remain alongside the final reports: GPT
originally emitted a follow-only change that claimed to resume but never
started its timeline, and the initial fixture was strengthened to catch
default-pattern substitution as well as failure to start.

## Response-boundary follow-up

The next review looked for rejection of semantically valid output, while
preserving the user's preference for natural continuation and avoiding
language classifiers. Three small parser changes passed comparison:

- Intermediate Layered can place several distinct edit groups in one array
  item, as Simple motion already permits. They still form one transaction;
  repeated groups, duplicate keys, unknown controls and invalid values fail.
- Hosted geometry accepts explicit controls that agree with the selected
  geometry (for example, full-slider travel plus endpoints 0 and 100). The
  shared geometry validator compares resolved values, including relative
  values against a recalled score. Contradictory values still fail atomically.
- An explicit `vibration:null` means no change even when vibration control is
  unavailable. Non-null edits retain all capability, readiness and Stop checks.

The model prompts and advertised schemas are unchanged. There is no extra
context, inference call, keyword rule, substituted pattern or preferred speed.
State/action mismatches, missing recall targets, conflicting geometry,
ambiguous repeated fields and unsafe phrase joins remain rejection boundaries.

`TestResponseBoundariesLive` completed **8/8 baseline and 8/8 fresh final
requests per model**, Gemma 12B Intermediate and GPT 6 Sol Advanced: 32/32
unrepaired requests in the scored runs. Cases cover full travel, both anchors,
compound range/width/pace changes, relative speed, continuation and questions
while running/stopped. The first fixture incorrectly treated an unset widest
stroke as zero instead of the full band; those two false failures and all raw
responses remain in the initial capture, and the scorer was corrected.

For a controlled before/after comparison, **22 equivalent representations of
the baseline model replies went from 0/22 accepted to 22/22**, producing
exactly the original commands. This is a structural replay result, not a claim
that 22 naturally generated replies failed. Fresh final replies contributed
another 23 equivalent representations, all accepted with identical commands.
The negative tests retain conflicts, invalid limits, duplicates, unavailable
output edits and paused-state rejection. No physical device was used.

## Engine and visual evidence

Ignored artifacts are under `.scratch/claude-vibrator-followup/` in this
worktree. `visual.json` and `plots/index.html` retain 368 model decisions and
structural replays from accepted and rejected experiments, with raw attempts and 45 distinct outputs
compiled by `motion.ReviewMotionOutput`. The starting scores for the earlier
reports are reconstructed from the exact evaluation fixture and fixed seed;
new captures include `before`, `limits`, and `running_before` directly.

All four overview sheets and detailed position/velocity plots were inspected.
Vibration-only accepted edits retain the same linear score. The rejected E4B
guidance experiment visibly accelerated strokes on a vibration-only “full”
request, so it was discarded. No new linear generator or sampler is introduced.

`plots/speed-coupling.png` compares the old saturation curve and new saved-band
mapping on shared buffered full-sweep samples at 20, 50 and 80%, at the existing
100 ms vibration cadence. Low-speed output separates more clearly from high
speed while keeping the startup fade and turnaround smoothing. These curves
are commanded estimates before motor-step quantization, not measured output.
`TestCapturedVibrationEnvelopes` retains the GPT envelope choices with the
shared engine sampler; follow choices use the motion profile rather than an
independent envelope curve.

## Validation and review app

Full Go tests and race tests passed before the boundary follow-up. Its final
chat and HTTP API race suites pass, along with all other packages in the
ordinary full run. Windows security blocked the ordinary HTTP API executable
and two child-process voice-install fixtures as potentially unwanted software;
a normal retry was also blocked. The same HTTP API tests pass under race.
No security setting was changed and the ordinary-run blocker is retained in
`boundary-go-all.log` and `boundary-httpapi-repeat.log`.
Vet, golangci-lint, frontend typechecking,
localization checks, 1,069 frontend tests and the embedded production UI build
pass. No dependency, worker, runtime motion loop or model call is added.
The size measurements are in `goal-scorecard.md`.

The isolated review uses the in-process Intiface simulator and local Gemma 12B
at Intermediate. The real-generation review probe passes. A text-only app chat
produces a nonempty answer without repair/fallback or motion; a subsequent
slow/gentle start chooses a level, and “stronger, keeping the strokes exactly
as they are” changes level without a new linear plan. No credential was copied.

The final boundary build also passes the real-generation probe and an app
text-only request (1.378 s). Its compound request sets speed 35%, bounds 20–90,
width 15–25 and level 40% of Max in one unrepaired reply (2.313 s). After Stop,
a question stays idle and suspended (1.284 s); “Continue where we left off”
restores the exact saved FlowSpec and level (1.315 s). It is left stopped at
`http://127.0.0.1:50305/#/chat`, with both in-process simulated outputs ready.

Physical acceptance remains open. For physical acceptance of this change, check an
actual vibrator's minimum usable motor step, perceived level glide,
speed-following at low/middle/high Handy pace, Advanced pattern transitions,
Stop-to-silence, and restarting after Stop. Also check simultaneous-device
failure policy. No real hardware was connected or commanded in this review.
The alpha publication assessment is recorded in the
[release review](alpha77-release-review-2026-10-28.md); it does not close these checks.
