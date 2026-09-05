# Conversation Lab and continuous catalog review — 2026-09-05

This review implements the request to make the LLM Lab a normal conversation
workspace, test its selected contract with Autopilot, move lengthy guidance
into Help, expand the catalog, and disable old built-ins on upgrade. It updates
draft PR #250; no merge, version bump, release tag or deployment is included.

## Result and boundaries

The title bar selects one of seven contracts. Model, schema, prompt and interval
options are behind Configure. Conversation, a compact session bar and the
composer dominate the page; response details and plotted output are collapsed.
The separate Help tab explains conversation, modes, Autopilot, motion, guided
tests, and storage. Context links open the appropriate topic. Observations and
test answers persist in the displayed SQLite path. They are evidence, not
automatic training, prompt changes or motion preferences.

Live motion starts explicitly. Accepted replies retarget the same shared
engine run. Preview-only Autopilot is also available: its backend scheduler
requests a continuation of the current conversation after a quiet interval.
Manual messages cancel automatic inference. Invalid automatic proposals pause
the scheduler; proposals cannot increase speed or widen the requested band.
Each turn makes one provider request, without repair or fallback. Production
Autopilot keeps its own planning policies.

The 81 legacy built-ins are forced disabled during library seeding after
preference promotion. Resolution, enablement edits, feedback undo and manual
motion requests cannot restore them. Saved names, weights and exports survive;
user-authored content and saved preferences for continuous recipes survive.
The current isolated app reports 98 built-ins: 17 continuous and 81 deprecated,
with zero enabled deprecated rows. Historical Motion Lab generators remain
explicit reference experiments, separate from selectable library patterns.

## Numeric context instead of hardware-name inference

The Lab previously serialized all `MotionSettings` into each model request.
That exposed a Handy version name, physical stroke-window settings, reverse,
style and video options to a model whose task is choosing semantic motion.
It now receives only saved numeric speed bounds and an `engine_envelope` with
semantic coordinates and a profile-derived peak velocity ceiling. The motion
package calculates that reference using the same profile function as the
shared sampler. It is neither a requested pace nor measured device velocity.

Each turn refreshes the values from current settings. The engine still owns
calibration, physical mapping, curve fitting and safety enforcement. A provider
capture regression changes the profile and limits between calls, verifies that
the envelope changes, that coordinates remain semantic 0–100 despite a saved
20–70 physical window, and that irrelevant settings stay out of model input.
Full settings remain in trial exports so visual output can be reproduced.

## Live local-model evaluation

All calls below used the isolated app's actual `/api/labs/llm/chat` and session
paths, with schema guidance, temperature 0.1 and reasoning off. No physical
device or simulator was started. The review provider is local Ollama. Models:

- `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` (Gemma 12B).
- `huihui_ai/granite4.1-abliterated:3b` (Granite 3B).

The selection set has ten geometry/timing requests, repeated with action names
and descriptive IDs. A pass requires the correct recipe and preservation of
speed 25. Fluent prose or a valid schema alone is not a pass.

| Prompt/context | Gemma selections | Granite selections |
| --- | ---: | ---: |
| Initial expanded-catalog prompt | 20/20 | 5/20 |
| Selection-first examples and distinctions | 20/20 | 13/20 |
| Final prompt with numeric planning context | 20/20 | 11/20 |

Granite's final split is 5/10 with action names and 6/10 with descriptive IDs.
Wrong outputs include an omitted recipe ID, a nearby but incorrect shape, and
a speed increase when the request asked for a pace wave. The results are a
small reused tuning set, not a held-out reliability estimate. The numeric
context does not solve weak semantic selection, and the two-result change
does not isolate a causal effect from model variability.

The final harness completed 58 validly parsed turns: 40 selections, eight
multi-turn conversation replies, six independent compound/no-change checks,
and four real scheduled Autopilot replies. Both models completed the four-turn
conversation (tip anchor/drift, softness 70, five points slower, then explain
without changes). Both produced two valid scheduled continuations without an
automatic speed increase or widened band. No repair or fallback was used.
Gemma's median response time was 580 ms (maximum 3,624 ms); Granite's was
135 ms (maximum 1,324 ms), including loading effects. These are host samples.

A final UI probe found a significant compound-edit failure: Gemma claimed
softness 70 but emitted only tip anchor and drift. Expanding the prompt's main
field list alone did not fix it: each model passed only the no-change case in
a three-case follow-up. A compound example plus a consistent control-key
ordering hint improved the tested set to 3/3 for Gemma and 2/3 for Granite,
including with the final numeric context. Granite still omitted anchor,
softness and cadence while claiming them on the center/drift/softness/steady
beat request. The ordering result is empirical; it does not establish a
specific grammar-runtime defect. The strict schema already contained all the
fields. Raw failures remain in the report and rendered atlas.

Reproduce on a fresh isolated app with Labs enabled and an available provider:

```powershell
python scripts/evaluate-lab-conversation.py --base-url http://127.0.0.1:49846 `
  --model '<installed-model>' --output .scratch/lab-conversation-current-live.json
```

The script takes controller ownership of that app and explicitly uses
preview-only sessions. A `finally` block stops Autopilot on polling failures.
Reports preserve raw text, before/after scores, limits, expected controls and
intent results. An intent miss is reported as evidence, not repaired or hidden.

## New recipes and visual findings

All 17 continuous recipes were rendered at speed 10, 25 and 43. The seven new
recipes received detailed low/middle/high-speed inspection, alongside the
overview, distinct model outputs and captured startup/retarget/Stop timeline.
Each plot comes from the shared engine: whole-loop position, first-12-second
detail, phase plot, velocity, acceleration and quantized command points.

The final atlas contains 221 steady cases (51 catalog cases and all 170 model
replies), 62 distinct steady plots, two overview sheets and one captured
timeline. Thirty-eight retained intent mismatches stay labeled. Six distinct
outputs added by the final prompt probes were also inspected in detail: wrong
base/center geometry remains visibly wrong, while the accepted centered
softness/cadence combination has a slower, even rhythm. The final tip-drift
softness-70 output at speed 25 reaches jerk 18,387; reducing speed to 20
reduces it to 13,906. Both remain continuous and inside the flow budgets.

| New recipe | Character and observed tradeoff |
| --- | --- |
| Base-anchored drift | One endpoint remains at the base while reach wanders over long gradual irregular trends. No one-direction impulse appeared in the rendered seed. |
| Tip-anchored drift | Mirrored base-drift behavior with the tip fixed. Its upper/lower symmetry is visible across all three speeds. |
| Centered drift | Both endpoints move around the same midpoint; width changes gradually without a fixed endpoint. |
| Soft turnarounds | Symmetric longer residence near both ends, with more travel through the middle. At equal requested pace, peak velocity can be higher than ordinary sweeps. |
| Even-beat variety | Changing width with a steadier cycle duration. Shorter strokes reduce average travel: at speed 25, about 66.9 percentage points/s versus 109.4 for centered drift. |
| Three-zone tour | Four lower, middle, upper and middle cycles with blended section changes. Large region transitions cause velocity surges and make this the conspicuous derivative outlier. |
| Breathing window | A moving window whose width varies between 20 and 65; both endpoint location and span change gradually. It is distinct from a fixed-width traveler or fixed anchor. |

Soft turnarounds at speed 25 commanded mean travel 109.8, peak velocity 216.4,
peak acceleration 727 and peak jerk 5,232 in semantic percentage-point units
per second to the respective power. At speed 43, peak velocity was 308.8,
acceleration 1,480 and jerk 15,198. A softer endpoint does not imply lower
peak speed everywhere in the stroke.

Three-zone tour at speed 25 reached peak velocity 263.8, acceleration 2,220,
and jerk 22,171. At speed 43, values were 271.6, 2,378 and 23,995, close to
the existing 24,000 flow jerk budget. Retiming limits the effective speed
increase. This needs separate physical feedback and is not a claimed default
natural-motion improvement. Breathing window at speed 43 stayed below the
same budgets (acceleration 1,930, jerk 19,877).

The inspected curves stayed in range and had continuous knot/loop derivatives.
Whole-percent wire sampling shows expected staircase velocity; it is not
carriage telemetry. A floating-point softness blend could exceed a normalized
endpoint by a few ulps; clamping the blend to 0–1 fixed the new normalization
test without changing the carrier or bypassing validation.

## Lifecycle and app-path evidence

The captured test transport records one stroke-window command, two point
appends, one Play and one Stop for startup, an accepted live retarget and Lab
disablement. Seven trace rows retain the scenario. Fake latency is 0 ms and
does not measure a physical link. This is a deliberately short dispatch test:
queued future points visible in its chart were canceled by Stop, not executed
on hardware. Physical comfort and transport timing remain unmeasured here.

Conditional retargeting checks the expected plan and active epoch under the
engine lock. Start admission, Stop, disablement, controller changes and
shutdown cancel pending inference and prevent late output from restarting or
overwriting another run. The original `starting` guard is retained; the
existing lifecycle suite exposed a deadlock when an early refactor omitted it.
The final tests cover live one-engine playback, preview-only Autopilot,
automatic rejection, manual preemption, late reply discard, disablement and
Stop teardown. All motion still uses the existing sampler, sanitizer and
transport interface. No transport mapping or retry policy was changed.

## Verification and retained artifacts

Full Go tests, full race tests, vet, golangci-lint, architecture boundaries,
frontend typecheck, 472 frontend tests, five-locale audit and production build
passed. Final affected-package checks and race checks passed after the session
and numeric-context changes. Ordinary pure-Go and atlas builds passed. The
current review app completed real provider generation and production text chat
with one call, non-empty text, no repair/fallback and `motion: null`.

The rendered desktop chat, explicit preview-only Autopilot flow, Help topic
navigation, storage explanation and global Stop were inspected. The review
UI retained two manual replies and five scheduled continuations, then Stop
ended the preview session. The compound edit took effect, the numeric-limits
question changed no controls, and the automatic replies preserved the score.
The review
app remains isolated at `http://127.0.0.1:49846/#/labs/chat`; older user app
processes and data were preserved. There is no copied device key.

Runtime evidence stays ignored under `.scratch/`, never in the embedded UI:

- `lab-conversation-live.json`, `lab-conversation-live-v2.json`, and
  `lab-conversation-live-v2-gemma.json`: initial and selection-prompt comparisons.
- `lab-final-ui-conversation.json`, `lab-combined-controls-v3.json`, and
  `lab-combined-controls-v4.json`: the failed compound request and prompt steps.
- `lab-conversation-current-live.json`: final 58-turn API evaluation.
- `lab-conversation-current-ui.json`: final visible chat and Autopilot replies.
- `lab-conversation-atlas-current.json` and `lab-conversation-atlas-current/`:
  full retained visual review; failed intent remains labeled beside actual output.
- `lab-conversation-capture.json`: captured fake-transport lifecycle and trace.
- `lab-current-readiness.json`, `lab-current-chat.sse`, `lab-final-*` and
  `lab-numeric-context-*`: readiness and required check logs.
- `lab-conversation-final-budget.json`: fresh-data stripped build measurements.

Footprint and the existing unclosed memory/physical-measurement limits are in
[the scorecard](goal-scorecard.md). Architectural decisions and the user flow
are in [ADR 0022](decisions/0022-lab-conversation-sessions.md) and
[Labs workspace](labs-workspace.md).
