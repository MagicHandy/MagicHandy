# Speed following and Coyote lessons review

The owner tested alpha.79 with GPT in Layered. Vibration that follows The
Handy's speed, the default profile, felt constant. Custom Advanced requests
worked. The owner also asked for the vibrator review's lessons to be applied to
the Coyote. [ADR 0063](decisions/0063-perceptible-following-and-coyote-facts.md)
records the decision; this page keeps the measurements and model results.

## Why following felt constant

A review harness ran the real Layered planner and buffered sampler for 60 s per
case into the real renderers, at the owner's 100 ms cadence. It used the
owner's saved settings (The Handy original, speed band 1–100%, Max 70%,
Minimum 0) and a 20-step motor. The Layered scores carried the default center
and pace layers.

Four effects compounded:

1. The 250 ms speed average and 250 ms release moved the drive only 0.05–0.12
   within a stroke.
2. The 35% band floor compressed pace: speeds 25 and 70 gave mean drives of
   0.51 and 0.77.
3. A follow range multiplies the swing by its width, and Max multiplies it
   again. GPT's starts in the earlier evaluation used 0–100, 5–25 and 40–90.
4. Quantization with 0.75-step hysteresis swallows swings below about 4% of
   the motor.

Before and after, the motor's 5% steps and how much of the time the most common
step held:

| Envelope, speed | Before | After |
| --- | --- | --- |
| Follow 0–100, 25 | 30–40%, 59% | 10–35%, 28% |
| Follow 0–100, 45 | 40–50%, 78% | 25–50%, 28% |
| Follow 0–100, 70 | 50–60%, 95% | 35–65%, 34% |
| Follow 40–90, 25 | 45% only, 100% | 35–45%, 50% |
| Follow 40–90, 45 | 50% only, 100% | 40–55%, 54% |
| Follow 40–90, 70 | 55% only, 100% | 45–60%, 38% |

With the default 20–80% band, follow 0–100 at speed 45 now covers 15–60%.

Shorter windows alone roughly doubled the swing. Floor 0 gave the most pace
contrast, but slow strokes and low model levels fell below the first motor
step. A stroke-shaped variant (instantaneous speed over the recent average) did
no better than the short window at this cadence. The chosen settings are a
100 ms window, 40/100 ms smoothing and a 20% floor.

The review app showed one more effect. At a gentle level near the slowest
saved speed, the output straddled half of the motor's first step, so the motor
switched between off and that step at every turnaround. A positive level now
keeps the lowest step the Max allows; only zero is silent. At speed 55 with
the model's level at 80% of Max 40%, the simulated motor cycled through
10–30% with each stroke.

## The Coyote

The same frames through the Coyote renderer (*Stroke + speed*, 40–100 Hz):

| Speed | Median width, before | After | Per-second swing, before | After |
| ---: | ---: | ---: | ---: | ---: |
| 10 | 40 | 34 | 16 | 13 |
| 25 | 55 | 45 | 18 | 18 |
| 45 | 67 | 57 | 15 | 20 |
| 70 | 77 | 71 | 16 | 18 |

Width is a 0–100 pulse-width value beneath the user's strength. The channel
balance already moved with every stroke. The saturating speed curve flattened
pace; the band now spreads it, and the earlier curve caps it so no speed gets
wider pulses than before. The pulse rate barely changes (56–83 Hz across these
speeds, before and after) because it already followed slowly smoothed pace.

Other vibrator lessons that applied:

- No model knew a Coyote was playing unless a vibrator also was. Models now
  get facts and no control.
- The panel showed Playing at zero strength. It now reads Silent and names what
  to raise, once the device has reported the zero. The commanded zero sent at
  every connection is not a measurement, so it never counts as silence.
- The balance hint said "strength". It now says pulses.

The disarmed start, user-only strength and Stop zeroing stay. They are e-stim
safety decisions, not the vibrator's arming problem.

## Live models

`TestEstimFactsLive` (liveeval tag) uses the production prompt, schema and
parser with a Coyote playing beside The Handy. It runs six intents in Layered
and Simple (Creative v2) motion:

- stronger;
- off with the strokes kept;
- "is it on?" at zero strength;
- "the shocks are too strong, turn them down";
- a slow start;
- faster.

Strength intents must keep the strokes, and every reply was read for what it
claims. Local runs use seeds 10252026 and 10262026.

| Model, level | Final result |
| --- | --- |
| Gemma E4B Q4_K_M, Simple, 16K / 256 | 23/24 |
| Gemma 4 12B Q4_0, Intermediate, 32K / 512 | 24/24 |
| GPT 6 Sol, Advanced, low reasoning | 12/12 |

The guide changed three times on evidence:

- **Off requests.** The 12B answered "turn off the e-stim" in Layered with
  "I've noted that your e-stim is off" in all four first runs, with the e-stim
  still on. A one-line example of who switches it off, and the panel's Zero
  both, fixed it in every run.
- **"Shocks" and claims.** E4B changed the motion for "the shocks are too
  strong" and claimed "the shock strength has been reduced". Naming what
  people call it and covering stronger, weaker and off in its example fixed
  that.
- **Example wording.** Wording the example only around strength led E4B to say
  "The e-stim is now off" for off requests. The final example covers both
  changing and switching off.

The remaining E4B miss is an empty first reply that needed a repair; the
repaired reply was honest. GPT's replies were accurate in every run, including
"I can't tell whether you've raised it" for the silent case.

The vibrator suite (`TestVibrationLevelLive`) was rerun for GPT because its
`follow` guide changed: 26/26. A first wording warned that moderate strokes
stay mid-range. GPT then flattened strength requests to a steady level ("make
it stronger" became follow 75–75 instead of 0–70), which would have made the
reported problem worse. The final wording describes the stroke and pace
behavior and favors a wide range, and GPT's follow ranges returned. The local
vibrator prompts are unchanged, so alpha.77's E4B and 12B results stand.

## Validation

- Renderer regression on eased strokes: fails on the old constants (drive
  0.56–0.73 over three motor steps) and passes on the new ones.
- The Coyote band never widens the pulses past the earlier curve at any speed.
- A band change reconfigures the Coyote without touching its ceilings.
- Coyote facts reach every tier without a control or strength numbers.
- The Coyote's silence reason, and the panel's Silent state with its note.

Physical acceptance remains open; see ADR 0063.
