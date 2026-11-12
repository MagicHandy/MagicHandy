# VacuGlide native control and Autopilot completion review

Reviewed on `claude/vacuglide-native`, based on `f4e67ef6`, with the owner's
clarification that the finish target is **Session buildup**. The changes remain
on this branch. The signed-in ChatGPT profile on port 50360 was used in place
for model evaluations; its credentials were not copied into review data.

## Outcome

Keep the optional Session buildup bar. Always supplying elapsed-session facts
is useful, but is a different feature from asking the model to build toward a
finish. Verbal timing is useful too, especially with Advanced native tempo;
it is not sufficient evidence to remove the bar or promise precise deadlines
with every model and motion mode.

Claude's simpler Graphite Now playing / Play / Autopilot layout and automatic
timing remain. The manual motion-method choice remains. Native tempo replaces
position vocabulary only when a VacuGlide leads the run.

## Corrections made during the reviews

- Native Autopilot startup, continuation and speech use the tempo contract.
  The runtime planning message now uses that same vocabulary instead of
  inheriting Layered position edits and depth guidance.
- Native planning receives each recent request's age. Local planning history
  keeps those ages even if the saved motion selector is a catalog mode.
  Elapsed durations no longer round away up to half a minute.
- Native `next_check_seconds` uses the same change-rate-dependent minimum and
  300-second maximum as the scheduler, replacing a conflicting 5–600 schema.
- Editing or resetting buildup atomically invalidates an in-flight/prepared
  decision for the previous target. Current playback and Pause/Stop admission
  remain authoritative. Large valid buildup placements no longer overflow.
- Now playing uses backend-reported idle/retired participants, preserves the
  paused plan's name, labels an Edge hold before general Autopilot ownership,
  and distinguishes Ultra position playback from vibration-only playback.
  A stopped target cannot keep stale tempo/vibration controls visible after
  the selected devices change. Speech choices use a balanced two-column grid.
- The UI and native model guidance distinguish the finish target from the
  fixed Slow build routine. The bar does not automatically stop the device.
- Earlier completion work retained here includes Stop-fenced tempo controls,
  shared-engine air adjustments, conservative session/connection air budgets,
  cancellation/repair admission, seeded continuation, native Edge return,
  and continuous envelope phase across Pause/Resume and looping ramps.

No runtime language classification, phrase matching, forced numerical finish
target, new motion loop or transport bypass was introduced.

## Model evaluation

Production prompts, provider adapters and parsers were used without a physical
transport. Failed selections and rejected experimental guidance remain in the
captures; retrying a failed experiment does not turn the earlier result into a
pass.

| Final regression set | Passed | Median request latency |
|---|---:|---:|
| GPT-6.1 Terra Low, Advanced | 30/30 | 2.72 s |
| Gemma 12B, Intermediate | 29/29 | 0.78 s |
| Gemma E4B, Simple | 23/23 | 3.88 s, CPU only |

These cover startup, exact/relative pace, native routines, continuation/holds,
unsupported depth, connection/paused state, air permission, Stop and motor
pause; the higher tiers additionally cover ramps/waves/pulses and Advanced
sequences. The final explanatory-copy follow-up passed another 5/5 Terra,
5/5 12B and 4/4 E4B cases, including a text-only buildup explanation.

The five timing scenarios cover a target requested partway through a session,
later praise, a revised deadline, an elapsed gentle interval, and a new gentle
direction overriding a full buildup bar. Actual production tempo envelopes
are fed back between decisions. Category means and bounded model horizons
make the experiment reproducible; real scheduler jitter and network latency
remain separate concerns.

| Timed native tempo scenarios | Passed | Observed limitation |
|---|---:|---|
| Terra Low | 5/5 | One response authored each requested continuation |
| 12B Intermediate | 4/5 | Added too long a ramp after an already elapsed interval |
| E4B Simple | 3/5 | Also missed the target after unrelated praise; has no timed sequence vocabulary |

Two longer Intermediate timing-guidance variants were discarded because they
did not improve results. Simple and Intermediate did not acquire Advanced
capabilities merely to make a test pass. Timed request success is a small
sample, not a reliability guarantee.

The original finish-target evidence had a future-decision/backwards-time bug
and approximated tempo curves. The corrected checkpoint evaluation and its
limitations are recorded in [ADR 0072](decisions/0072-autopilot-timing-without-switches.md).
Layered pace is still developed at reconsiderations; its selected pace at the
eight-minute checkpoint was 72–76% in the final run, rising further afterward.
That is not evidence of exact-time finishing or measured physical speed.

## Visual and app review

Artifacts are deliberately ignored under `.scratch/claude-timing-review-20261112/`:

- `finish-{baseline,final}.json`: all finish-target calls and sampled intervals.
- `timed-*.json`: timing trials, including failed guidance variants.
- `tempo-{terra,12b,e4b}-final.json` and `tempo-help-*.json`: regression evidence.
- `tempo-atlas-complete/index.html`: 265 retained records, 96 distinct engine
  envelopes, five overview sheets; it includes failures and unscored planning
  checkpoints rather than silently grading them as passes.
- `position-atlas/index.html`: 42 retained decisions, 37 distinct position
  plots, three overview sheets using the production Cloud sampler.
- `app-trace.json`: 198 trace rows through simulated native playback,
  Pause/Resume, buildup reset and Edge. Subsequent Stop was verified in the UI.
- `review-chat.png`, `review-device-panel.png`: current review UI.

All overview sheets and detailed low/middle/high position plots, timed ramps,
failed late continuations and native routines were inspected. Geometry remained
bounded, with no newly introduced rests in position plans. Native ramps blend
from the actual preceding level; explicit pauses remain immediate. Timing
misses are visible as delayed arrivals, not hidden by sampling a future plan.
Older schema experiments and their failed routine substitutions remain in
`.scratch/vacuglide-completion/atlas/`.

The isolated app on **http://127.0.0.1:50363/#/chat** has working local Gemma 12B
and simulated devices. Verified: text-only generation without motion or repair,
routine playback, native Autopilot startup, paused clock, reset while paused,
resume, Edge labeling, and Stop. At 390 px width there was no horizontal
overflow and Emergency Stop stayed visible. The final app is stopped, with
the simulated VacuGlide connected and the eight-minute buildup setting visible.

## Validation and remaining hardware work

Normal and race Go suites, architecture/goroutine lifecycle gates, vet, lint,
frontend tests (136 files / 1,123 tests), typecheck, localization and the
`CGO_ENABLED=0` build were run. The final LLM readiness probe completed real
generation against the exact review app. Budget measurements are in
[the scorecard](goal-scorecard.md).

No real device was connected or commanded. Before hardware acceptance, verify
the VacuGlide motor range/stall floor, cloud response time, valve-step effect,
Stop during valve activity, reconnect behavior, mixed position-led playback,
and physical feel of routines and long ramps. Mixed sessions currently follow
the position lead; native tempo-only sessions leave selected Coyote/vibrator
companions idle. Independent mixed-device tempo prompting is not claimed by
this implementation.
