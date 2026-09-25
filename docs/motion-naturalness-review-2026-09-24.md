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

## Pace, variety and recall follow-up

The first round left two limits: Layered answered "mix it up" with a seed
refresh, and Gemma used only the upper part of the saved speed range. The
follow-up asked three questions. Was that range a habit, or a choice made for
the context? After "that's too much" or "I'm close", could the model decide to
draw the session out by dropping to near the saved minimum, holding for an
appropriate time and then rebuilding? And could it return to an earlier
pattern that the person praised or referred to?

### Changes

- **Pace follows the person.** Both continuous contracts say that pace may use
  all of the saved range. When the person says it is too much, or says they
  are close and the model chooses to draw the session out, the model sets
  speed near the saved minimum in that decision. It holds there for an
  appropriate time and then rebuilds; when it lets them finish, it holds or
  builds. A wish for a slow pace, such as going slow to make it last, keeps
  pace in the lower third until the person says otherwise. No word triggers
  any of these choices.
- **Planning sees time.** Each recent human line carries `seconds_ago`, both
  in the motion context and, in continuous planning turns only, as a
  `[said 2 minutes ago]` mark on the person's lines in the history. The
  planning facts say how long ago the person last spoke, list the speeds of the
  stretches that began in the last three minutes, and say how long pace has
  stayed in the lower third of the saved range.
- **Unchanged chat re-plans at once.** Live chat chooses its action before it
  writes the reply, so it often answers such a remark in words alone. During
  continuous Autopilot, a chat turn that leaves the motion unchanged moves the
  next planning boundary to now. Before, the planner waited up to the rest of
  the segment.
- **Built-in prompt sets.** The five built-in sets told the model to match the
  user's energy "without escalating beyond what they ask for". They now say to
  follow the user's cues on energy and pace, respect limits the user sets, and
  lead when the choice is left to the model.
- **Layered variety.** `evolve` only reseeds the same character. A request for
  something new, mixed up or left to the model changes geometry, widths,
  anchor or layers, and an example shows "mix it up". A relative speed step
  past a saved limit stops at the limit instead of being rejected.
- **Hold-off examples.** Both contracts show "I'm close, don't let me finish
  yet" answered by one step to nearly the slowest pace. The grammar still
  bounds speed to the saved limits, so a copied number cannot leave them.
- **Recall.** During continuous Autopilot the scheduler keeps the three latest
  distinct scores, plus up to three older ones that were playing when the
  person spoke. Live chat sees them with when each started, how long it played
  and what the person said while it played, and `{"recall": id}` brings one
  back exactly. Other edits in the same reply apply on top, and a recalled
  speed outside today's limits moves to the nearest limit. Planning turns are
  never offered recall.

The harness now plays a chat edit's own segment before Autopilot plans again,
as the scheduler does. Before, it planned at the same second, so a chat score
never played and the planner always saw the line as said 0 seconds ago. The
PR-head runs below use the harness of that commit.

### Pace and variety

Eight scenarios per mode, two runs each, from the saved range 15–54, on the
same Gemma 12B build with two llama.cpp slots of 16,384 tokens. Each run opens
with motion already playing, sends one chat line, then plans four Autopilot
stretches. "Near the minimum" means 22 or below, and the lower third
is 28 or below.

| Line (opening speed) | Pass | Creative v2 PR head | Final | Layered PR head | Final |
| --- | --- | ---: | ---: | ---: | ---: |
| that's too much (45) | 30 or below by the first planning decision | 2/2 | 2/2 | 2/2 | 2/2 |
| I'm close, don't let me finish yet (45) | near the minimum by the first planning decision | 0/2 | 2/2 | 0/2 | 2/2 |
| go slow, I want this to last (45) | near the minimum, or never above the lower third | 2/2 | 2/2 | 1/2 | 2/2 |
| faster (30) | chat raises speed | 2/2 | 2/2 | 2/2 | 2/2 |
| mix it up (35) | chat changes the character | 2/2 | 2/2 | 0/2 | 2/2 |
| surprise me (35) | chat changes the character | 2/2 | 2/2 | 0/2 | 2/2 |
| keep varying within this same character (35) | chat refreshes only | 2/2 | 2/2 | 2/2 | 2/2 |
| **Total** | | **12/14** | **14/14** | **7/14** | **14/14** |

"I'm close" alone is left to the model. At the PR head, Creative v2 built
toward the top in both runs and Layered held 45. In the final build, all four
runs drew the session out from 18–20 and rebuilt over the next minute.

Pace was a context choice, but a narrow one. Explicit requests moved it:
"go slow" reached 15–25. Implicit cues barely did: after "that's too much",
Creative v2 eased to 22–35 and climbed back within a stretch or two, and
"don't let me finish yet" left pace at 35–52.

Two runs per cell cannot separate wordings, so the first reactions were also
measured over eight runs per mode:

| Measure (8 runs each) | Creative v2 PR head | Final | Layered PR head | Final |
| --- | ---: | ---: | ---: | ---: |
| "don't let me finish yet": near the minimum by the first planning decision | 0/8 | 8/8 | 0/8 | 8/8 |
| "go slow": never above the lower third in three decisions | 8/8 | 8/8 | 7/8 | 7/8 |

Chat itself answered "don't let me finish yet" with the drop in 4 of 8 Layered
runs and none in Creative v2; the immediate re-plan made the planner answer
within seconds. Without the hold-off examples the same wording reached the
minimum in 6 of 8 Creative v2 and 1 of 8 Layered runs: Layered "pulled back
just a little" to 30–40, following the "jerk gently" example's five-point step.
With them, the 37-case continuous request suite still passed 74 of 74 in both
modes.

### Holding and rebuilding

One line at 45%, then 12 Autopilot stretches (2 min 48 s), two runs per
scenario and mode.

| Wording and data | "that's too much" |
| --- | --- |
| PR head | Creative v2 eased only to 28–50 and kept moving; Layered stayed at 15–25 for three minutes |
| Pace guide and immediate re-plan | Near the minimum at once, then stayed at 15–18 for three minutes in 3 of 4 runs |
| Plus line ages, the last-spoke age and recent speeds | Brief rises to 24–25, then back to 15 |
| Plus the lower-third duration and a rebuild instruction | Layered rose to 25–30 and fell back; Creative v2 stayed at 15–23 |
| Plus a three-minute speed window instead of six stretches | Rebuilt after 1.5–2 minutes in 3 of 4 runs, then fell back |
| Plus ages marked on the person's lines in planning history | Rebuilt after 42–70 s in all runs |

Two data defects hid the elapsed time. Planning replies are never dialogue, so
the person's last line stayed the latest user turn for minutes. And six
remembered stretches at the 14 s cadence capped "pace has stayed in the lower
third" at 70 seconds, below the minute or two the rebuild waited for.

The remaining fall-backs came in two kinds. Most were ordinary variation, such
as "backing the pace off just a hair" on the way up. Others re-read the old
line as a new reaction: "Since that last bit was a little too intense for
you…" at 154 s cut pace from 35 to 15. Marking the person's lines with their
age removed those. Planning drops of 5 or more at least 60 s after the line
that cite the old line:

| Build | Drops | Citing the old line |
| --- | ---: | ---: |
| Three-minute window only | 16 | 3 |
| Final (ages marked in the history) | 15 | 1 |

Final runs after "that's too much" reached 15–20 at once and began rebuilding
after 42–70 s. After 90 s they peaked at 21–40, below the 45% that was too
much. After "don't let me finish yet", all four runs dropped to 15–20 within
one stretch and started rebuilding 14–70 s later, reaching 30–52 within three
minutes; Layered climbed fastest.

### Recall

Each script asks for something completely different at decision 2, then says
one line at decision 5. Two runs per mode.

| Line at decision 5 | Recalled | Recalled the score from before the change |
| --- | ---: | ---: |
| go back to what you were doing before I asked for something different | 4/4 | 2/4 |
| mm, what you were doing before I asked for a change was perfect | 4/4 | 2/4 |
| that feels amazing | 0/4 | not applicable |

Chat recalls when the person praises or asks for an earlier pattern, and not
for praise of the current one. It picks the score the person meant in half of
these runs; the other picks were the change itself or the latest earlier
score. One more chat line corrects a wrong pick. The first version kept only three
scores, did not say what was said while each played, and was tested with
vaguer lines; it picked the right score in 1 of 8 runs.

An earlier version also offered recall to planning turns. They re-read an old
"go back" after chat had answered it, and swapped between scores for the rest
of the run.

### Smaller models

The same scripts ran on Gemma 4 E4B heretic (Q4_K_M) and Granite 4.1 3B
heretic (F16) with both builds, and on Gemma 4 26B A4B abliterated (Q3_K_S)
with the final build. The suite, rebuild and recall runs predate the Creative
v2 hold-off example. The Creative v2 first-reaction rows for E4B and Granite
include it; the 26B A4B rows do not.

| Measure | E4B PR head | E4B final | Granite PR head | Granite final | 26B A4B final |
| --- | ---: | ---: | ---: | ---: | ---: |
| Suite, Creative v2 | 9/14 | 8/14 | 8/14 | 11/14 | 12/14 |
| Suite, Layered | 6/14 | 10/14 | 11/14 | 14/14 | 12/14 |
| Model turns that failed | 105/160 | 76/134 | 24/160 | 6/131 | 1/138 |
| "don't let me finish yet" at the minimum first, Creative v2 | 0/8 | 8/8 | 0/8 | 8/8 | 6/8 |
| The same, Layered | 0/8 | 8/8 | 0/8 | 8/8 | 8/8 |
| "go slow" kept in the lower third, Creative v2 | 7/8 | 5/8 | 0/8 | 1/8 | 4/8 |
| The same, Layered | 0/8 | 5/8 | 1/8 | 7/8 | 8/8 |
| Recall on praise or a reference | none | 1/8 | none | 0/8 | 0/8 |
| Recall on "that feels amazing" | none | 0/4 | none | 0/4 | 0/4 |

- E4B is limited by its output, not by the pace logic. Most failed turns ran to
  the 512-token limit: 100 of 105 at the PR head and 63 of 76 in the final
  build. A failed planning turn keeps the previous score.
- The smaller models follow a concrete example, not abstract guidance. With
  the hold-off examples both drop to 20–22 at once; without them, Creative v2
  eased only to 35–39. Granite copies the example's 22 exactly.
- They do not judge elapsed time the way Gemma 12B does. After "that's too
  much", Granite's Layered runs stayed at 15–22 for three minutes and every
  26B A4B run stayed near the minimum. E4B rebuilt Layered pace after 56–126 s.
- They almost never recall (1 of 24 chances) and never recall by mistake.
- 26B A4B produced the fewest failures and planned the most conservatively. It
  answered "surprise me" in words only and held most planning turns.

Separate prompt sets for smaller and larger models are not needed for these
behaviors. The change that helped the smaller models most, a concrete example,
also helped Gemma 12B, so it went into the shared contracts. What the smaller
models miss, judging time and recall, falls back to staying slow or not
recalling.

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
- **A host timer that rebuilds pace** after a fixed time. The model judges the
  elapsed time from the facts it is given.
- **Recall in planning turns.** Planning re-read an old "go back" after chat
  had answered it and swapped between scores.
- **Separate prompt sets for smaller models.** See
  [smaller models](#smaller-models).

## Limits

- Most conditions use one or two runs per model; the first reactions use
  eight. Differences of a few decisions are noise. The width case of the
  continuous request suite failed twice in one final run and passed 4 of 4 on
  a rerun, against 2 of 4 at the PR head.
- Simulated stretches omit handoffs between scores. The atlas shows commanded
  output, not physical feel.
- Accent relaxation draws unseeded random numbers when a decision is made.
  Every accepted score and seed is recorded, so replay is exact, but two live
  runs differ.
- Without a request, Gemma still keeps Autopilot in the upper half of the
  saved range. It uses the lower part when the person's words call for it.
- After "don't let me finish yet", rebuilding can start within 15–30 s, and
  Layered climbed back to 45–52 within three minutes.
- Recall picks the score the person meant in about half of the runs, and the
  smaller models almost never recall. One Layered planning reply in the hold
  runs failed score validation, and the previous score held.
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
`MAGICHANDY_SESSION_REQUESTS` to `2:harder|6:don't stop`. The pace suite uses
4 turns and `MAGICHANDY_SESSION_START_SPEED` 45 (30 for "faster", 35 for the
variety lines) with requests such as `0:that's too much`; the first reactions
use 3 turns and 8 runs, and the rebuild runs 12 turns. The recall scripts use 8
turns from 35 with `2:try something completely different|5:go back to what you
were doing before I asked for something different`. The hold scripts
use 10 turns, for example `2:keep it exactly like this, no changes from now
on|4:that feels amazing|7:okay, now surprise me`. To recompile a report with
the current engine, set `MAGICHANDY_REPLAY_INPUT` to it and run
`TestReplayContinuousSession`. Render reports with the `-continuous` exporter
input described in [the visual review guide](motion-visual-review.md#continuous-autopilot-sessions).
