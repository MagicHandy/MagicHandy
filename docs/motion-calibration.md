# Motion speed and organic-loop calibration

Status: implemented calibration and C2 Creative compiler, awaiting matched
real-device confirmation (2026-08-22)

## Why this changed

The first shape-independent speed model fixed an important defect: pattern
identity no longer decided global pace. Its reference constant was nevertheless
the authored pace of MagicHandy's moderate `Stroke` loop, 180 semantic travel
percentage-points per second at 100%. That made the number honest relative to
that one curve, but not relative to the manual speed control users already knew.
A full-range 73% target requested only 131.4% travel/s, or about 145 mm/s over a
110 mm reference stroke.

Dynamic motion exposed a second mismatch. Every reversal inherited the catalog
authoring floor of 450 ms, regardless of stroke length, and nonzero variation
repeated one four-cycle sine motif. A short stroke therefore stayed slow while
its supposedly organic envelope became more obviously periodic as speed rose.
Slow finite plans had a separate quantization artifact: engine sampling rounded
phase back to a whole authored millisecond, so one authored millisecond could
become a visible plateau after time stretching.

The first range-envelope follow-up fixed long-period repetition but left two
independent physical-feel defects. An installed-session trace held almost the
same 31–35% stroke length for more than 30 reversals, and each leg spent most of
its time near constant velocity before a very short acceleration-budgeted guide
braked at the endpoint. The result was regular in distance and unnaturally
linear in velocity even though it was neither transport jitter nor sample noise.

## Comparison with other implementations

No cross-device protocol defines one universal `speed_percent`, so the useful
comparison is the physical meaning each implementation gives its control:

- The original Handy publishes a 32–400 mm/s sustained carriage range and
  110 mm travel. Handy 2 publishes 125 mm travel with a 32–400 mm/s Standard
  range and a 32–450 mm/s normal Pro range. Its HAMP SDK exposes velocity
  directly as 0–100% and explicitly notes that zero selects the slowest motion
  rather than Stop. That is an affine minimum-to-maximum control, not
  `maximum × percent` from a stationary zero. See
  [The Handy FAQ](https://www.thehandy.com/faq/),
  [the original Handy specification](https://www.thehandy.com/store/the-handy-eu/),
  [the Handy 2 specification](https://www.thehandy.com/store/the-handy-2-eu/),
  and the [official Handy JavaScript SDK](https://gitlab.com/sweettechas/platform/handy-js-sdk/-/tree/master).
- Buttplug's linear command does not invent a percentage speed at all. It sends
  a normalized target position and an explicit movement duration, so physical
  speed is distance divided by time. See the
  [Buttplug `LinearCmd` specification](https://stpihkal.docs.buttplug.io/docs/spec/generic/#linearcmd).
- MultiFunPlayer keeps range, waveform, and a continuous time multiplier as
  separate motion-provider inputs. It offers triangle/sine/other waveforms,
  PCHIP or Makima custom-curve interpolation, and a continuous OpenSimplex
  random provider rather than repeating a tiny fixed random table. See its
  [motion-provider base](https://github.com/Yoooi0/MultiFunPlayer/blob/36c08fbb99ac9398a63ff1cca1bbf68cd2228a94/Source/MultiFunPlayer/MotionProvider/AbstractMotionProvider.cs),
  [pattern provider](https://github.com/Yoooi0/MultiFunPlayer/blob/36c08fbb99ac9398a63ff1cca1bbf68cd2228a94/Source/MultiFunPlayer/MotionProvider/ViewModels/PatternMotionProvider.cs),
  and [continuous-noise provider](https://github.com/Yoooi0/MultiFunPlayer/blob/36c08fbb99ac9398a63ff1cca1bbf68cd2228a94/Source/MultiFunPlayer/MotionProvider/ViewModels/RandomMotionProvider.cs).
- StrokeGPT-ReVibed's non-pattern area-focus path emits full legs between
  extrema and samples them with monotone PCHIP. That makes velocity naturally
  fall to zero across the approach to a turn instead of preserving a linear
  body until a tiny endpoint ramp. Its bounded shape is a useful comparison;
  its client timer and separate motion path remain non-authoritative for this
  architecture. See its
  [non-pattern motion builder](https://github.com/mapledaemon/StrokeGPT-ReVibed/blob/02e6879d6235c27f786a072e5b1a0f59051cd232/strokegpt/motion.py)
  and [monotone sampler](https://github.com/mapledaemon/StrokeGPT-ReVibed/blob/02e6879d6235c27f786a072e5b1a0f59051cd232/strokegpt/motion_patterns.py).
- FunSync Player explicitly offers shape-preserving PCHIP for no-overshoot
  curves and Makima as a less aggressive alternative for oscillatory data. It
  reinforces that interpolation shape is a separate control from point timing,
  not something transport buffering should invent. See its
  [interpolation implementation](https://github.com/DaveMakesWaves/funsync-player/blob/main/renderer/js/interpolation.js).
- `handy-ai-motion` derives mm/s from position delta and duration, clamps it to
  a configured physical range, and tells its model to maintain rhythm coherence
  and avoid unnecessary mechanical repetition. Its useful lesson is explicit
  distance/time calibration; its client-owned timed-command loop and direct
  model-authored delays are not compatible with MagicHandy's one-engine safety
  architecture. See its
  [motion implementation](https://github.com/Fran31416/handy-ai-motion/blob/94125fccb59262c82e7b0e63252d73b878e55a1a/index.js).

MagicHandy retains its semantic target and shared sampled stream. None of these
implementations is copied as a transport or second motion path.

## Calibrated percentage

Loop targets and the optional media speed limiter now share one selected Handy
model calibration:

```text
progress = (speed_percent - 1) / 99
physical rate = minimum_mm/s + (maximum_mm/s - minimum_mm/s) × progress
semantic travel rate = physical rate / full_travel_mm × 100
```

The Connection menu exposes the profile as a three-part merged `Handy model`
radio control: Original, 2 Standard, and 2 Pro. Existing settings default to
Original Handy; Handy 2 owners must select their documented model. The backend
publishes the valid options, the selected profile's travel and normal speed sit
directly below the buttons, and a change applies immediately to an active loop
through the same retarget path.

| profile | published travel | published normal speed | semantic endpoints | physical rate at 73% |
| --- | ---: | ---: | ---: | ---: |
| Original Handy | 110 mm | 32–400 mm/s | 29.1–363.6%/s | 299.6 mm/s |
| Handy 2 Standard | 125 mm | 32–400 mm/s | 25.6–320.0%/s | 299.6 mm/s |
| Handy 2 Pro | 125 mm | 32–450 mm/s | 25.6–360.0%/s | 336.0 mm/s |

Thus the same percentage has the same documented physical interpretation on
the Original and Handy 2 Standard even though their semantic rates differ to
account for travel. Pro follows its higher supported normal ceiling. The
setting changes semantic timing only: positions remain 0–100 and transports
still receive no model-specific motion schema or raw physical payload.

Handy's product page mentions an optional 800 mm/s Pro overclock, but it is
deliberately not exposed. No official source found in this review publishes
per-model acceleration limits. The selector therefore does not invent them
from motor RPM or marketing descriptions; all three profiles retain the shared
conservative runtime acceleration envelope below.

The requested rate is not a promise that every shape can achieve it. The shared
planner lengthens a loop when its actual rendered cubic curve would exceed the
runtime envelope. The UI therefore continues to label position as a commanded
estimate rather than physical feedback.

## Two envelopes, for two jobs

The former implementation reused catalog-quality numbers as runtime limits.
Those concerns are now explicit:

| envelope | acceleration | reversal gap | purpose |
| --- | ---: | ---: | --- |
| stored catalog | ≤ 3000%/s² | ≥ 450 ms | reject patterns that feel like chatter at their authored reference pace |
| runtime plan | ≤ 7500%/s² | ≥ 100 ms | bound a user-selected faster playback while permitting calibrated manual-control speeds |

Runtime safety is evaluated against the exact rendered Hermite second
derivative after focus, time scaling, and speed-aware reversal guides. A cubic
segment's acceleration is linear, so checking both ends of every interval gives
the exact maximum without probe-step error. Reversal-ramp caps are expressed in
played milliseconds; a faster plan no longer shortens its physical ramp merely
because its authored clock was compressed.

Stored patterns keep those short ramps because they preserve authored waveform
and cadence. Creative selects a different profile inside the same curve
compiler: a monotone quintic Hermite segment shares velocity and acceleration
with its neighbors, reaches exactly zero velocity at a true endpoint, and
retains nonzero velocity through a pass-through anchor. The planner evaluates
the exact polynomial extrema, lengthening the period against the same exact
7500%/s² runtime acceleration ceiling and a Creative-only 150000%/s³ perceptual
jerk ceiling. The jerk number is a software quality bound, not an undocumented
Handy hardware specification. This is a content-level interpolation choice,
not a second engine or transport path.

Catalog acceptance remains on the quieter envelope. This is not a relaxation of
what may be stored or exposed to the model by default.

## Organic Creative motion (`dynamic` schema)

Creative exposes independent semantic axes instead of making one percentage do
every job:

1. `span_percent` or the named-anchor bounds set the outer reach;
2. `span_min_percent` plus `span_profile` (`breathe`, `wander`, or `contrast`)
   vary stroke length coherently inside that reach; `steady` clears the
   envelope; and
3. `variation_percent` controls loop-closed multi-harmonic center drift plus
   seeded grouped rhythm and direction asymmetry, limited to 0.65–1.45× local
   authored timing before global speed normalization. A square-root perceptual
   response makes ordinary 20–40 values meaningfully different without changing
   the model-facing 0–100 schema; and
4. an optional `sections` phrase combines 2–4 complete movement ideas, each
   carrying one bounded route/texture and 2–12 cycles.

The backend chooses enough cycles for center/rhythm variation to take at least
about eight seconds and for an explicit span envelope to take at least about 30
seconds at the maximum reference rate. That remains the non-repeating carrier
horizon, not the interval between visible changes. Narrow spans consequently
receive more cycles than broad spans. Breathe is one asymmetric long swell with
a restrained faster texture; wander and contrast choose new correlated range
levels roughly every two cycles, and each seeded three-choice block visits
tight, medium, and broad portions without repeating a fixed table. All are
deterministic, bounded, smooth, and loop-closed for traces, tests, and seamless
playback; none is random per-sample noise. Tests require every circular
six-cycle wander/contrast window to explore at least 18% of the available
floor-to-outer band and prevent a four-cycle window from settling below 8%.
A zero center/rhythm variation can therefore coexist with a changing explicit
span profile, while `steady` continues to mean a genuinely fixed stroke length.

A section phrase is compiled as one C2, loop-closed engine curve. Its authored
travel covers at least 60 seconds at the fastest supported reference rate, or
the longer selected decision horizon, subject to the existing 4096-point cap.
Later passes keep the model's macro order but derive a different deterministic
occurrence seed, preventing an exact micro-timing repeat. The range/rhythm
control interpolation uses quintic smootherstep, whose first and second
derivatives are zero at control knots, so long-horizon variability does not put
an acceleration corner back underneath the C2 carrier. Automated acceptance
requires at least six distinct whole-percent reversal lengths and reversal-
length coefficient of variation above 0.15 for the retained mixed-section
fixture; these are regression floors, not promises that every user phrase is
equally wild.

For whole-percent buffered transports, Creative's simplifier now measures two
errors: position at time and inverse time at position. The second term retains
the braking/acceleration timing of a short eased stroke even when its rounded
positions fit a straight chord. Redundant equal-position quantized edges are
then removed. This fitter is Creative-only; catalog, imported, program, and
media sampling keep their prior tolerances and shapes.

Older alpha.25 targets with no span profile retain their prior implicit small
span swell. New model responses use the explicit envelope. The model selects
the floor and profile; deterministic code only normalizes bounds and derives a
traceable phrase seed.

Engine phase now remains fractional through curve sampling. This removes
authored-millisecond stair steps without adding points, browser state, or a new
dispatch path.

## Control-surface rule

The compact Chat sidebar now distinguishes ordered and categorical settings:

- Motion change rate is a numbered 1–8 slider, spoken-check-in cadence and
  Gentle/Balanced/Intense style remain named discrete sliders. Their
  marks use the handle's real inset travel: the first and last marks are the
  actual endpoints, and intermediate marks use `index / (count - 1)` rather
  than the centers of equal-width label cells.
- Creative (`dynamic` in settings/API)/Pattern Library/Off and speech-motion
  authority are segmented radio choices, because rendering categories on a
  scalar slider would imply a false numeric relationship.
- Speech custom timing remains directly below its cadence slider. Motion level
  1 maps to 90–240 seconds, level 4 to the former 20–60 second Natural window,
  level 7 to 8–24 seconds, and level 8 to a tighter 8–16 seconds; all earlier
  points are unchanged explicit backend windows. Legacy motion presets/custom
  bounds migrate to the nearest level.
  Rapid set-point changes are serialized so the final choice cannot be lost
  behind an in-flight save.
- The global Connection menu contains one low-frequency three-part merged
  `Handy model` radio control. Its selected travel and normal maximum appear
  directly below the buttons, so the calibration choice remains visible
  without introducing another list selector.

All values still come from and return to backend settings snapshots.

## Evidence still required

The initiating feedback was qualitative: Dynamic felt robotic and a selected
73% felt slow. It did not include a transport, latency summary, or trace export.
Automated tests now cover the calibration points, exact runtime acceleration
and Creative jerk extrema,
runtime reversal spacing, fractional sampling, all explicit Creative span
profiles across all three Handy models, short-window stroke-length diversity,
C2 acceleration continuity, eased time-at-position after 1% wire quantization,
multi-section reversal-length diversity, long deterministic phrases, stateful
model updates, and the one-stream retarget path.

A review of the latest installed Cloud REST session found 439 sampled points
and 85 legs. Its contrast envelope contracted from about 80% to about 32%, then
held within roughly 31–35% for more than 30 reversals; adjacent request
acknowledgements were ordinarily about 330–341 ms, with one recorded network
error and a successful 330 ms user Stop. This evidence separates the regular
stroke-length plateau and linear endpoint shape from high-frequency jitter, but
it predates the correction and therefore is diagnostic rather than acceptance.

The installed managed 12B Gemma model passed the broader 25/25 first-response
Creative decisions without repair or transport dispatch. The exact recent
tip/full-length correction sequence then passed 9/9 repeated decisions after
semantic validation: reply-only claims are repaired, requested base/tip
coverage is checked against the effective geometry, and position-only
corrections cannot silently rewrite pace, span texture, variation, or horizon.

On 2026-08-22 the locally installed Ollama model also passed a focused four-case
section matrix without an engine or transport. Starts and running replacements
produced two complete sections; pace-only copied geometry was normalized away
so the current phrase survived; and an unsatisfied tip report had to change
geometry before its reply could claim success. The live run issued no motion or
device command. A broader legacy matrix remained stochastic, so this is focused
evidence for the new contract rather than a claim that all provider acceptance
is closed.

A capped matched-device A/B run must still record Handy model/profile,
transport, selected outer/floor/profile and speed, command latency,
`motion_trace.v3`, Stop behavior, and subjective feel.
Until that exists, this change corrects the demonstrable schema/timing mismatch
but does not close the real-device acceptance item or make Creative the default.
The cross-cutting model-profile decision is recorded in
[`ADR 0016`](decisions/0016-handy-model-speed-calibration.md).

## Plan compilation is fail-safe

Alpha.24 exposed a numerical edge at the boundary between Creative geometry and
the strict curve validator. A full `0..100` span at 96% variation could produce
the floating-point value `100.00000000000001`. Rejecting that point was correct;
discarding the compiler error and retaining a zero-value curve was not. The next
sample indexed an empty point list and could terminate the Go process.

Alpha.25 applies three layers at the shared planner boundary:

1. the final varied position is clamped after all center/span arithmetic, not
   only while its conceptual window is calculated;
2. plan compilation errors are retained, and Engine Start, Resume, retarget,
   and settings refresh reject the plan before any transport work; and
3. an empty curve samples as a stationary 50% midpoint with zero velocity, so a
   future invariant failure remains process-safe while the engine reports the
   compilation error.

The fallback is not a second motion path and is not dispatched for a rejected
engine plan. Tests compile and sample all 101 variation values against all three
Handy profiles, including the exact 96% full-span regression.
