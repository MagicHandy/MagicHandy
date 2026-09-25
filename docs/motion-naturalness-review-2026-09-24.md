# Motion naturalness review — 2026-09-24

## Finding

Creative v2 was extended so the model could make a fast stroke up into the
upper range with a slower return. Physical feedback reported that sessions
now keep doing it: a quick sweep into the same target point, stroke after
stroke. The request was for a capability, not a tendency.

The reporter's retained Creative v2 Autopilot trace from 2026-09-20 (Handy
Cloud, 110 s) shows the habit. From about 49 s, every stretch uses the same
gesture. Over the whole trace, 54% of up/down stroke pairs have a return at
least 1.5 times slower than the upstroke, none run the other way, and
near-full strokes land on exactly 10.0 and 85.0 percent each time.

Simulated sessions through the production path reproduce it. Location is not
the cause; Gemma moves the focus almost every turn. The timing accent rides
along with every location. Five mechanisms combine:

1. **Model prior.** Both test personas chose tip-faster timing on the first
   decision, with no human input.
2. **Accents never expire.** Omitted edit groups persist, and the model edits
   one to three groups per turn, so a contrast chosen once stays all session.
3. **Recipe wording.** The contract described unequal timing as a recipe for a
   faster sweep and slower return.
4. **Engine regularity.** Contrast and inertia were constant for a score, and
   near-full strokes landed on the band edges exactly. Default variation was 35.
5. **A word lock.** Once a human line matched motion words, Creative v2
   Autopilot could only refresh its seed, and neither continuous mode could
   raise speed or widen the band for the rest of the session. "harder" froze a
   session; "slow down a bit" did not.

Two further defects surfaced. Seventeen of 40 baseline decisions were rejected
because a narrowed band no longer fit the kept focus width, so the old score
held. Layered Autopilot produced only two or three distinct characters per 20
decisions. Its whole-cycle timing fit also counted incidental reversals, which
capped the pace of alternating geometries.

## Changes

[ADR 0031](decisions/0031-model-judged-autopilot-continuity.md) records the
decisions. Each change maps to a proposal from the initial review:

| Proposal | Implementation |
| --- | --- |
| Let the model judge recent requests | Continuous Autopilot sees up to the eight latest human lines, unfiltered, and keeps what still applies. The word classifiers, request filter and continuation validator are removed. |
| Let unchosen accents fade | On planning turns that change other controls, contrast, rebounds and strong inertia the model did not choose again relax by a random amount. Holds and seed-only refreshes keep them. |
| Clamp focus width | A narrowed band clamps the width to `max(10, band)` instead of rejecting the decision. |
| Neutral wording | The sweep, variation and example wording no longer suggest a tip recipe. |
| Pace spread | Continuous Autopilot facts ask the model to use the saved speed range's width. |
| Per-stroke variation | Seeded landing points vary inside the band; a held anchor lands exactly. Default variation is 50. |
| Speech-only history | Earlier replies are replayed as speech, not as `{"edits":[]}` envelopes. |
| Phrasing inside a stretch | Seeded pace breathing that only eases off, flurries of two to four quicker strokes, and rests of 110–280 ms. At the default variation, a flurry starts at 3% of strokes and a rest occurs at 2.5% of turns; both rates double at variation 100. |
| Layered parity | Layered uses the same judgment and history. Its examples are neutral, and its Flow compiler fits legs between actual turning points. |

The initial review's speech proposal was dropped (see
[corrections](#corrections-to-the-initial-draft)). Two proposals were not
implemented as proposed; see [not selected](#not-selected).

Live tests found that a prompt-only exact hold lapsed after an unrelated
remark. A structural standing hold replaces it. While continuous Autopilot
composes, live chat must answer `stay_unchanged` after each reply. The server
remembers `true` for that conversation, and planning boundaries then hold
without inference. Autopilot shows "Keeping the motion as you asked". A later
`false`, a new Autopilot run or another conversation releases it.

## Measurements

### Setup

- llama.cpp b9966 (CUDA) on an RTX 5070 Ti, one slot, 32,768-token context.
- Gemma 4 12B IT heretic Q4_K_M (SHA-256 prefix `239ec3629639`), the
  reporter's managed model. Reasoning off, 512 output tokens.
- `TestLiveContinuousAutopilotSession` with the reporter's saved values:
  speed 15–54, stroke 0–100, change rate 8, Original Handy, explicit voice.
- Decisions every 14 s, matching the retained trace, with spoken check-ins
  every 52–103 s. Two invented personas, Mara and Theo.
- Stretches are sampled from the shared plan at 20 Hz. Positions are
  commanded semantic values, not carriage telemetry; no device was used.

The baseline is `main` at `970ccee3` (alpha.47), measured with the corrected
harness. Stroke pairs are consecutive up/down strokes of at least 3 points; a
pair is a fast upswipe when the return takes at least 1.5 times as long.

### Creative v2 without requests

Two runs of 20 decisions per build:

| Measure | Baseline | Final |
| --- | ---: | ---: |
| Decisions rejected | 17 / 40 | 0 / 40 |
| Distinct characters (seed excluded) | 23 / 40 | 40 / 40 |
| Unequal timing: tip / base faster | 33 / 0 | 16 / 4 |
| Median contrast of unequal decisions | 65 | 32.5 |
| Contrast of 40 or more | 33 | 9 |
| Stroke pairs: fast up / fast down / even | 70% / 0% / 30% | 15% / 5% / 80% |
| Consecutive top landings within 1 point | 68% | 25% |
| Mean spread of top landings per stretch (SD) | 2.0 points | 4.3 points |
| Speed: lowest / median / highest | 35 / 46 / 52 | 35 / 47 / 52 |

The swipe is still available and still chosen, but as one accent among
others. Speed stays in the upper part of the saved 15–54 range: the pace line
did not widen the range Gemma uses.

### Engine replay

`TestReplayContinuousSession` recompiled the 40 baseline decisions, with
their seeds, through the final engine. Fast-upswipe pairs moved only from 70%
to 68%, so the timing habit came from the decisions. Consecutive top landings
within 1 point fell from 68% to 32%, so landing regularity came from the
engine.

### Directed session

One run of 16 decisions, with "harder" before decision 3 and "don't stop"
before decision 7:

| Measure | Baseline | Final |
| --- | ---: | ---: |
| Creative v2 distinct characters | 3 / 16 | 16 / 16 |
| Creative v2 speed: lowest / median / highest | 40 / 54 / 54 | 35 / 50 / 54 |
| Creative v2 consecutive top landings within 1 point | 95% | 12% |
| Layered distinct characters | 2 / 16 | 3 / 16 |
| Layered speed: lowest / median / highest | 25 / 45 / 45 | 25 / 54 / 54 |

On `main`, the word lock left Creative v2 with seed refreshes of one character
after "harder". In the final build, chat answered "harder" by raising speed and
declared no standing wish for either line. Creative v2 Autopilot then kept
varying reach and location while pace moved between 45 and 54. Layered raised
pace to the top of the saved range and then refreshed seeds; see the limits.

### Layered without requests

Two runs of 20 decisions per build. No decision was rejected in either build.

| Measure | Baseline | Final |
| --- | ---: | ---: |
| Distinct characters (seed excluded) | 5 / 40 | 18 / 40 |
| Speed: lowest / median / highest | 25 / 25 / 25 | 25 / 25 / 25 |
| Consecutive top landings within 1 point | 54% | 50% |

Layered now develops anchor, widths and layers, but never changed speed
without a request in either build.

### Layered pace

Layered geometries at four seeds, 64 cycles each, with limits 1–100:

| Geometry | Speed | Mean travel before | After |
| --- | ---: | ---: | ---: |
| Alternating ends | 10 | 50.2 %/s | 46.8 %/s |
| Alternating ends | 45 | 56.9 %/s | 78.4 %/s |
| Alternating ends | 85 | 56.5 %/s | 78.5 %/s |
| Full strokes with tip work | 45 | 135.1 %/s | 128.0 %/s |
| Full strokes with tip work | 85 | 154.1 %/s | 146.2 %/s |
| Tip anchor, centered, wander | 45 | 146.1 %/s | 145.3–145.9 %/s |

Alternating ends no longer plateaus near 57 %/s at higher speeds, and its
reversals per cycle fall from 1.96 to 1.71 as incidental reversals merge.
Mixed full and tip geometries run 5% slower, and the library's variable-reach
recipes loop 2–3% longer. Fixed-width geometries are within 1%.

### Visual review

The final engine was exported and rendered with `cmd/motion-atlas`: 135
Creative v2 cases (122 plots, 8 overview sheets) and 69 library and Flow
experiment cases (69 plots, 5 sheets). All overview sheets were inspected,
alongside the detailed plot for the highest-speed roaming case.

- Roaming cases vary landing points, reach and timing across all three device
  profiles. Anchored fixtures keep their held end exact. The variation-0
  fixture still repeats exactly.
- Rests are true stops: velocity and acceleration reach zero at both ends. The
  85% Original Handy roaming case peaks at 5,990 %/s² with no velocity or
  acceleration jump at any knot, inside the 7,500 %/s² runtime limit.
- At 85%, a rest lasts about as long as a stroke and reads as a distinct beat.
  At 10% it is barely visible. Physical feedback should decide whether rests
  scale with pace.
- Library recipes and Flow experiments keep their shapes. Variable-reach
  recipes loop 1–3% longer, as in the pace table above.

The plotting run used matplotlib 3.10.9 and NumPy 2.4.6, which were already
installed, instead of the pinned review versions. That affects rendering only.

## Standing hold

The same scripted conversations ran in both modes: a hold request at decision
3, an unrelated remark, then a request for change. A fourth script tested
false positives with "keep going, don't stop", "yes, just like that" and "mmm,
more of that". Each script ran once per mode, 10 decisions each.

| Contract | Declared | Kept through remarks | Released | False-positive lines |
| --- | ---: | ---: | ---: | ---: |
| Optional `keep`, after the reply | 3 / 6 | 3 / 3 | 0 / 3 | not run |
| Required `keep`, before the reply | 6 / 6 | 6 / 6 | 4 / 6 | 3 wrong of 3 |
| Required `stay_unchanged`, after the reply | 6 / 6 | 6 / 6 | 6 / 6 | 0 wrong of 6 |

The optional field came after the reply, and the model usually closed the
object instead of adding it: Layered declared 0 of 3. Placed before the reply,
`keep` read "keep going, don't stop" as a hold and froze a session. The
selected contract asks after the reply, with a name that describes the effect.
The hold request "slow down a little and then keep it like that" applied the
edit and then held. In Creative v2, planning resumed with new characters after
each release. Layered resumed with seed refreshes only (see limits).

## Corrections to the initial draft

The initial review draft made two wrong claims. It said motion-turn replies
were published as speech; only the speech clock's check-ins are published, and
motion-turn replies are only traced. It also said check-ins received raw score
JSON, but the speech facts are already written in words. The first harness had
published motion replies into history, which inflated "I…" openings (98–100%)
and tip mentions (57–81%). The harness was corrected, every baseline here was
measured again with it, and the speech proposal was dropped.

With the corrected harness, Creative v2 check-ins still mentioned the tip or
head in 4 of 5 baseline lines and 5 of 5 final lines, and Layered in none.
Five lines per condition is too few to act on.

## Not selected

- **Pace drift tied to session buildup.** A host pace trend that follows
  elapsed time is the hidden escalation that
  [the guardrails](goals-and-guardrails.md) rule out. The breathing field
  only eases pace below the chosen speed and recovers, so it cannot escalate.
- **Scheduler sway for continuous scores.** Sway retargets speed mid-segment,
  which recompiles the whole score and competes with its own pace fields. The
  phrasing fields vary tempo inside the plan.
- **Any word rule**, including for exact holds, per the user's direction.
- **The `keep` contracts** measured above.

## Limits

- One model, one or two runs per condition and one run per hold script.
  Differences of a few decisions are noise.
- Simulated stretches omit handoffs between scores. The atlas shows commanded
  output, not physical feel.
- Accent relaxation draws unseeded random numbers when a decision is made.
  Every accepted score and seed is recorded, so replay is exact, but two live
  runs differ.
- Gemma still leaves the lower part of the saved speed range unused.
  Creative v2 used 35–52 of 15–54, and Layered stayed at 25 without a request.
- Layered often answers requests for variety with a seed refresh alone. The
  realization changes but the character does not. One Layered planning reply
  in the hold runs failed score validation, and the previous score held.
- The standing hold relies on the model's reading. A misread shows in the
  Autopilot status, and one chat line or a fresh run clears it.

## Reproduction

From `internal/httpapi`, with a loopback llama.cpp server:

```powershell
$env:MAGICHANDY_LIVE_LLAMA_URL = 'http://127.0.0.1:18180'
$env:MAGICHANDY_SESSION_MODE = 'creative_v2'  # or layered
$env:MAGICHANDY_SESSION_RUNS = '2'
$env:MAGICHANDY_SESSION_TURNS = '20'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = '..\..\.scratch\session.json'
go test -tags 'liveeval magichandy_labs' -run 'TestLiveContinuousAutopilotSession$' -v -timeout 60m .
```

For directed runs, set `MAGICHANDY_SESSION_TURNS` to 16 and
`MAGICHANDY_SESSION_REQUESTS` to `2:harder|6:don't stop`. The hold scripts
use 10 turns, for example `2:keep it exactly like this, no changes from now
on|4:that feels amazing|7:okay, now surprise me`. To recompile a report with
the current engine, set `MAGICHANDY_REPLAY_INPUT` to it and run
`TestReplayContinuousSession`. Render reports with the `-continuous` exporter
input described in [the visual review guide](motion-visual-review.md#continuous-autopilot-sessions).
