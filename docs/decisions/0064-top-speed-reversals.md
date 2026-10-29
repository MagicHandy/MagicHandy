# ADR 0064: Sharper reversals at the top of the speed range

Status: implemented with automated and rendered review, 2026-10-29, at the
owner's choice; amended by the [whole-path review](../motion-path-review-2026-10-29.md).
The first Original Handy Cloud comparison accepted full-range, short and
compound patterns at 100%; broader physical coverage remains open. See the
[hardware report](../top-speed-cloud-audition-2026-10-29.md).

This amends the top of the range accepted in the
[Layered speed-support review](../layered-speed-support-review-2026-10-26.md).
The accepted alpha.75 100% fit now plays at about 84% requested; at and below
75% every fit is unchanged.

## Context

The owner reported that speed scaling never reaches The Handy's maximum
effective speed. The speed scale defines 100% as mean travel equal to the
device's peak velocity: 400 mm/s over 110 mm on the Original Handy, or
363.6 %/s. No stroke with turnarounds can average that.

Measured with the real compiler on the Original Handy, effective pace stopped
rising at about 85% requested. Every request from there up played alike:

- 78.7% for full-range Layered and the library's Full sweeps;
- 67.9% for the default Layered field used in the follow-up matrix;
- 61.9% for a counted phrase;
- about 60% for Simple (Creative v2).

The cause was in the authored shape. A fixed envelope (7,500 %/s²,
150,000 %/s³) shaped the turns, and cruise strokes used ramps that are fixed
fractions of each stroke. A stroke reached the velocity ceiling with rounded
turns, and playback scaling can only slow a curve, never flatten it. Widening
the envelope alone changed almost nothing.

The owner was offered sharper reversals at the top, rescaling the slider, or
both, and chose sharper reversals.

## Decision

1. **Above a 75% knee the turn envelope widens smoothly.** It reaches 3×
   acceleration and 8× jerk at 100%. At and below the knee the envelope, every
   fitted curve and every sampled point are unchanged.
2. **Cruise strokes may shorten their ramps.** At 100% a ramp may shrink to 35%
   of its ordinary length. A shorter ramp is tried only for velocity-limited
   cruise strokes, and kept only when the joint fitter finds the whole score
   strictly faster inside velocity, acceleration, jerk and reversal spacing.
   Nested five-percent ramp steps replace the initial four-item menu. Previously
   eligible candidates stay available as the speed rises. Creative v2 measures
   its minimum worthwhile gain against the original rounded fit at these high
   speeds, then keeps the fastest eligible candidate.
3. **Shortened ramps keep 1.5% velocity headroom.** Such a stroke holds the
   ceiling for most of its length; the headroom leaves the quantized wire
   timing room to round.
4. **Immediate transports keep their commands.** The widened envelope never
   turns a stroke faster than 120 ms; an Intiface Handy needs 110 ms. A stroke
   the runtime envelope already turns faster keeps that timing.
5. **One envelope per requested speed for prepared motion.** It covers fitting,
   the prepared plan and Layered joins. Content fitted for a faster request is
   held to a slower request's envelope. Diagnostics also honor a content's
   narrower bound; legacy content does not inherit the new envelope.
6. **Sampler reserves come from the compiled trajectory.** Use its analytic
   acceleration bound, with the existing base reserve as a floor. Never infer
   it from already rounded/retimed append anchors: this densified slow motion
   in the initial proposal. Expired fast transition history releases its bound.
7. **Scope.** The change covers Layered and its phrases, the library's
   continuous recipes (Full sweeps and the rest), Freestyle, the Lab stroke
   score and Simple (Creative v2). Authored point patterns and original
   Creative are unchanged.

## Consequences

Constrained broad strokes gain additional speed between 75% and 100%. Their turnarounds are
sharper, and at 100% a long stroke cruises at nearly constant speed with short
reversals. Peak acceleration at 100% rises from about 0.8 g to about 1.3 g for
Layered and Full sweeps on the Original Handy, and to about 1.5 g for Simple.
These are software trajectory estimates, not manufacturer acceleration or
jerk ratings. Very short strokes still saturate at the reversal timing floor;
this decision does not make every one-percent request distinct for every
geometry. Integer-millisecond fitting can differ by one millisecond per leg.

The device velocity ceiling, the user's saved speed limits, Stop and every
transport path are unchanged. Anyone who preferred the earlier top-end feel
can save a maximum speed near 84%.

The first matched Original Handy Cloud comparison at 100% found both versions
smooth, B slightly faster on full and short strokes, and its compound rhythm
distinct and smooth. Short strokes added no reported chatter, vibration or
harshness. Intermediate speeds, live transitions and other devices/transports
remain separate acceptance steps.

## Evidence

Effective pace on the Original Handy, before and after. Values up to 75% are
identical.

| Content | 85% | 92% | 100% |
| --- | --- | --- | --- |
| Layered full range | 78.7 → 80.2% | 78.7 → 84.6% | 78.7 → 86.6% |
| Layered default score (`DefaultFlowSpec`) | 67.9 → 73.9% | 67.9 → 77.3% | 67.9 → 78.0% |
| Layered phrase | 61.9 → 70.8% | 61.9 → 73.0% | 61.8 → 74.1% |
| Simple (Creative v2) | 58.5 → 65.7% | 59.6 → 68.9% | 60.5 → 71.2% |

A rendered review compared position and velocity of the engine-compiled
trajectory and the sampled wire points at 100%, before and after, for full
range, the default score and Simple. Long strokes become a near-constant
cruise at about 98.5% of the ceiling with short reversals. Short strokes stay
rounded and turn faster. No velocity humps, stalls or overshoot appeared, and
wire points track the curve.

Tests:

- The auditioned six-cycle fits are unchanged at the knee. At 100% each is
  faster than its auditioned period and inside its own envelope.
- Full-range pace rises at the 75/85/92/100 checkpoints on all three Handy
  models and reaches at least 85%. An additional 390-fit integer-speed sweep
  checks for regressions beyond one millisecond of rounding per stroke.
- 50 ms and 110 ms immediate intervals still fit at 100%.
- Derivative-envelope tests assert the requested speed's envelope.
- Join-continuity tolerances scale with the join's jerk.

The follow-up additionally retains a 375-case transport matrix, 180 monotone
joins, simulated-latency retarget/Stop capture, independent filter-removal
experiments and their failures. All 198 matrix cases at or below 75% match
alpha.79 in both mathematical samples and transmitted points. See the linked
review for the distinction between useful chatter cleanup, sparse transmission
and the removed erroneous acceleration estimate.
