# ADR 0063: Perceptible speed following and truthful Coyote facts

Status: implemented with automated, simulated and live-model acceptance,
2026-10-29. Physical acceptance on the owner's vibrator and Coyote is open.

This amends the following-speed constants of
[ADR 0047](0047-intiface-vibrator-output.md) and decision 7 of
[ADR 0061](0061-model-controlled-vibration-level.md), and decisions 4 and 7 of
[ADR 0044](0044-coyote-estim-output.md): the Coyote's conversion profile and
"no Coyote-specific prompt". Coyote strength authority, ceilings, the strength
handshake and Stop are unchanged.

## Context

The owner's first physical test of alpha.79, with GPT in Layered, found that
vibration following The Handy's speed (the default profile) felt constant.
Advanced custom envelopes such as wave and pulse worked.

A review harness measured why. It ran the real Layered planner and buffered
sampler into the real renderer at the owner's 100 ms cadence, using the owner's
settings: The Handy original, a saved speed band of 1–100%, Max 70% and a
20-step motor.

- Drive moved only 0.05–0.12 within a stroke. The 250 ms speed average and
  250 ms release filled each turnaround's dip.
- The 35% band floor compressed pace: speeds 25 to 70 moved the drive from 0.51
  to 0.77.
- Follow ranges and Max multiply that swing, and quantization hysteresis
  removes the rest. A 0–100% follow held one motor step 59–95% of the time; a
  GPT-style 40–90% held one step all of the time.

The same review of the Coyote found the lessons of the vibrator work unapplied:

- Pulse width followed speed through the saturating curve that ADR 0061 had
  retired for the vibrator, with a 250 ms release, and ignored the saved band.
  The median width only went from 55 to 77 between speeds 25 and 70.
- No model was told a Coyote was playing. The output context existed only with
  a vibrator, and even then as a bare `estim_follows_motion`. A model could
  promise e-stim strength changes it cannot make.
- The panel could read Playing while both channels sat at zero, as every
  connection starts.
- The Channel balance hint said it shifts strength. It scales pulse width.

## Decision

1. **Following speed reads each stroke.** The vibrator's speed window is one
   pacer tick (100 ms), with a 40 ms attack and a 100 ms release. Strokes peak
   mid-travel and dip at each turnaround. A positive level keeps at least the
   motor's lowest step, so at gentle levels a turnaround does not switch the
   motor off and on with every stroke; only a zero level is silent.
2. **One speed band, lower floor.** Both outputs place speed on the saved Handy
   speed band through one helper, `companion.SpeedBandDrive`. It runs from 20%
   at the slowest saved speed (formerly 35%) to full drive at the fastest. The
   Handy's intensity for the depth and steady profiles uses the same scale.
3. **The Coyote spans the band without widening.** Its pulse width and rate use
   the band, capped at every speed by the earlier saturating curve, and its
   release is 150 ms. At a given strength the change can only soften output.
   The owner reconfigures when the saved band changes; unchanged ceilings are
   not rewritten to the device.
4. **Advanced reads the follow honestly.** The independent guide says `follow`
   rises toward `max_percent` mid-stroke and with faster pace, dips toward
   `min_percent` at each turnaround, and follows best with a wide range. No
   range is imposed. A first wording warned that moderate strokes stay near
   the middle of the range; GPT then flattened strength requests into steady
   levels, the opposite of following, so it was withdrawn.
5. **Coyote facts, never a control.** When a Coyote plays, every capability
   level is told:
   - it follows the movement, and only the user sets its strength or turns it
     off, in the device panel (Zero both);
   - how movement shapes it under each profile;
   - there is no separate e-stim pattern or channel command;
   - whether it is connected, and whether it is silent
     (`silent_until_user_raises_strength`, from the same backend reason the
     panel shows).

   Strength numbers are not sent, and the response schema gains nothing.
   Simple gets one compact paragraph. Prompts without a Coyote are byte for
   byte unchanged.
6. **The Coyote says why it is silent.** Its status reports `silent_reason`:
   `ceiling_zero`, or `strength_zero` once the device itself reports both
   channels at zero. A commanded zero is not a measurement (ADR 0044), so it
   never claims silence. While motion plays and nothing can be felt, the panel
   reads Silent instead of Playing and names what to raise. The balance hint
   now says pulses.

## Safety

The Coyote gains no authority. Its pulse width never exceeds alpha.79's at any
speed, its strength stays user-only (ADR 0044), and Stop still zeroes both
channels. The vibrator's Max still caps every output. The owner still sends at
most one command per 100 ms tick. At moderate speeds the number of step changes
rises from about 2–3 per second to about 6–9, within the existing bound.

## Evidence

Owner's settings as above. The motor column shows the 5% steps used. "Modal"
is how much of the time the most common step held.

| Case | Before | After |
| --- | --- | --- |
| Follow 0–100%, speed 25 | 30–40%, modal 59% | 10–35%, modal 28% |
| Follow 0–100%, speed 45 | 40–50%, modal 78% | 25–50%, modal 28% |
| Follow 0–100%, speed 70 | 50–60%, modal 95% | 35–65%, modal 34% |
| Follow 40–90%, speed 45 | 50% only, modal 100% | 40–55%, modal 54% |
| Mean drive, speed 25 → 70 | 0.51 → 0.77 | 0.39 → 0.74 |
| Coyote median width, speed 25 / 70 | 55 / 77 | 45 / 71 |

A renderer regression test uses eased strokes like the sampler's output. It
fails on the old constants (drive 0.56–0.73 over three motor steps) and passes
on the new ones. A Coyote test checks that the band never widens the pulses
past the earlier curve at any speed, and that pace contrast increases.

Live evaluation used the production prompt, schema and parser
(`TestEstimFactsLive`, liveeval tag). It ran six intents with a Coyote playing
beside The Handy: stronger, off with the strokes kept, "is it on?" at zero
strength, weaker, a slow start and faster. Strength intents must keep the
strokes; movement intents must still move. Replies were reviewed for claims.
See [the review](../follow-speed-coyote-review-2026-10-29.md) for the model
results.

## Open

Physical acceptance remains open:

- whether the stroke following is felt as intended on the owner's vibrator;
- the motor and radio lag against the 100 ms window (the timing offset can
  shift the vibrator);
- whether the 20% floor is too soft at slow speeds (Minimum while moving can
  lift it);
- the Coyote's feel with the band.
