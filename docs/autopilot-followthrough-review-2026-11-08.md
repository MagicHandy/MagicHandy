# Autopilot follow-through review — 2026-11-08

## Observed session

An installed alpha.82 session ran for about 40 minutes with GPT-6.1 Terra
(reasoning low) for chat and Autopilot, Layered motion, the utility voice,
Motion change rate 8 with the scaled cadence (8–16 second reference stretches),
Auto timing and prepared decisions on. It had 34 requests, 31 chat replies
(median 6.8 s) and 37 Autopilot lines. Read-only exports of the chat, failure
records and motion trace were used; private chat, device identifiers and raw
exports remain outside git.

Several requests were followed poorly. The question was whether Autopilot
should call the model more often in Auto timing mode.

## Faster calls are not the fix

Autopilot already re-planned every 10–16 seconds, and each GPT decision took
3.5–7.9 seconds. The 8-second floor allows 7.5 calls a minute. Every failure
below was in what a decision chose, not when it came: more frequent calls would
have repeated the same choices faster. The floor stays. DeepSeek answers in
about 1–2 seconds, so a lower floor would be possible there, but this review
found no quality reason for it.

## Findings

The session's turns were replayed with the production prompt builders, the
recorded motion state and simulated time, against OpenRouter's
`openai/gpt-6.1-terra` at reasoning low.

1. **A build requested now, with a cue for later.** One request asked to bring
   the person toward the edge and to slow right down once told they were
   close. Chat changed nothing. In eight replays it built the pace in four,
   changed nothing in two and slowed down in two, reading "edge" as the tip of
   the range. From the reply the session actually gave, Autopilot first held
   and then raised the pace by 8–15 points every two or three decisions: 30–45%
   after a minute.
2. **Invented strokes ping-ponged.** Asked to keep inventing simple strokes at
   top speed, Autopilot alternated between two layer templates in all 18
   replayed decisions: short and long strokes, then strokes hopping between the
   ends. In the session it also eased below the requested top speed and narrowed
   the band to 15–85%. The planner saw only the current score, the
   continuation text calls an earlier alternation "not a user constraint", and
   phrases were framed as tools for counts the person spelled out.
3. **An ordered progression became a wave.** Asked to begin in the middle and
   widen the strokes, chat chose a smooth widen-and-narrow range wave in five
   of five replays instead of counted steps.
4. **Smaller misses, unchanged here.** A recall of an earlier pattern brought
   back only its pace; a request to lean faster also changed the stroke
   pattern; Autopilot eased from 88% to 82% about 36 seconds after a motif the
   person had asked for.
5. **Two replies were dropped.** A speed or stroke quick control used during a
   reply retired it as a settings change, and Edge canceled another. Both are
   by design; a separate change makes the Edge case say so.

## Change

See [ADR 0066](decisions/0066-hosted-planning-direction-and-recent-scores.md).
Hosted Layered planning sees up to three distinct scores from the last three
minutes before the current one, as observations without recall IDs or the
person's words, and is told to keep moving toward a destination the person
named, to keep stroke qualities they asked for across variations, and that
counted phrases are tools for inventing. Hosted chat is told to act now on the
immediate part of a request with a later cue, that being close or at the edge
describes arousal rather than a slider position, and that an ordered
progression is counted step by step. Local prompts and timing are unchanged.

## Validation

### Replays

| Situation | Before | After |
| --- | --- | --- |
| Chat turn for the build with a later cue | Built 4 of 8 | Built 8 of 8 (20% to 40–45%) |
| Autopilot after the session's actual reply | 30–45% after a minute | 60–70% after about a minute |
| Autopilot after the cue to ease off | Not replayed | Held about 18% for about 75 s, 4 of 4 |
| Inventing at top speed, 3 runs × 6 decisions | 18 of 18 on two templates | 7 of 18 on them; 8 of 18 phrases; 6 distinct ideas per run |
| Middle-out progression | Range wave, 5 of 5 | Counted widening steps, 5 of 5 |

The cue check guards against the direction guide rebuilding right after a
request to ease off.

The same prompts on DeepSeek V4.1 Flash helped inventing, but one of three
edge replays still read "edge" as a position, and 2 of 33 planning replies
failed validation. The DeepSeek sample is small.

### Suites

Seventeen default hosted live suites ran on both builds with the production
variants (constraint lifetime `current`, phrase join `guidance`, pace
`candidate`, range `baseline`):

| Build | Intent checks passed |
| --- | --- |
| Before | 230 of 265 (86.8%) |
| After | 235 of 265 (88.7%) |

Seven cases started passing and two stopped. One of the seven had failed only
because the provider's response did not complete, so it is not credited to the
change. One new failure is a planning reply that set its band to 50–50%, which
validation rejected, so the previous motion would have continued; the other is
a regional timing case that is weak on both builds (one of four, then none).
Five suites gained a case and none lost one overall.

Six opt-in suites ran only partly before the evaluation account's OpenRouter
credit ran out. Completed pairs: independent vibration 20 of 21, then 24 of
24; vibration with Autopilot 3 of 4, then 2 of 4, failing the same case.
After only: directional pace 9 of 9, natural conversation 9 of 9 (13 rows not
run), output awareness 9 of 11. Both output-awareness misses are
vibrator-only stopped states, which do not receive the new chat guidance.

### Visual review

Both builds' suite outputs were exported through the shared engine with
`cmd/motion-atlas` (265 cases each, including failed selections) and rendered
with `scripts/render-motion-atlas.py`: 201 and 197 distinct plots on 13
overview sheets each. Every overview sheet of the new build was inspected, and
each changed case was compared with its earlier output in detail.

- No accepted output stalls, collapses to a flat band or shows a knot jump.
  Counted phrases compile to distinct regional patterns, and pace stays
  within the usual velocity and acceleration ranges.
- The new passes are visible in the plots. An anchor-at-zero request that the
  earlier build compiled to a nearly stationary trace near the base now strokes
  normally from zero. A wandering-geometry request that stayed in a narrow
  20–50% band now wanders across most of the range. A continuation reaches the
  requested pace, and a regional mix now visits both ends.
- The new failures show no motion defect: the 50–50% band was rejected before
  compiling, and the regional timing reply compiled smoothly but did not keep
  the requested per-region timing.

The synthetic sessions in `TestHostedFollowThroughLive` are not in the atlas
because they were not run (see Tests).

### Tests

Unit tests cover the newest-three, three-minute window of recent scores; that
only hosted Layered planning receives them; that they carry no recall IDs or
the person's words; and that each new guide reaches only hosted advanced
motion, and the planning guide only Layered. `TestHostedFollowThroughLive`
(`liveeval`) repeats the four situations with synthetic requests and retains
every result for atlas review. It was not run: the OpenRouter evaluation
credit was exhausted and the ChatGPT evaluation sign-in had expired.

## Not covered

- Physical feel. All evidence is model output and compiled motion.
- The synthetic live evaluation above, and the opt-in suite rows that did not
  run.
- The smaller misses in finding 4.
- Whether a quick control should retire a reply in flight. That remains the
  documented stale-work rule.
