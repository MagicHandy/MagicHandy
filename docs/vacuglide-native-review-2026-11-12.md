# The VacuGlide as a main device: architecture review, 2026-11-12


This is the original investigation and implementation plan. For implemented
behavior, model results, corrections, current UI and remaining hardware checks,
see [the completion review](vacuglide-completion-review-2026-11-12.md). In
particular, independent mixed-device tempo prompting remains future work.

This review covers alpha.85 (`f4e67ef6`), which shipped VacuGlide support
([ADR 0069](decisions/0069-autoblow-cloud-devices.md)) with the release
review's hardening. It uses engine measurements and simulation only; no
VacuGlide was connected.

**Outcome:** the owner chose option B, the full motor range as the default,
and positions leading whenever a position-capable device plays. Phases 1–3 are
implemented in simulation as [ADR 0071](decisions/0071-vacuglide-native-tempo.md).
Hardware calibration (phase 4) is open.

## Answer

Yes, a control model native to the VacuGlide would work better, and tuning the
current design will not produce one.

Today the VacuGlide plays motion composed for The Handy. The shared engine
plans positions. The VacuGlide's owner then averages how fast those positions
move and turns that into a motor speed. Everything the motion meant, and
everything the VacuGlide can do besides go faster or slower, is decided for a
different device.

**Recommendation:** give the VacuGlide its own tempo-and-air channel in the
shared engine, built the way independent vibration was
([ADR 0057](decisions/0057-independent-vibration-envelope.md)). It gets its own
vocabulary, limits, routines, model guidance, Autopilot and UI, and runs on the
engine's clock and Stop lifecycle. It should not get a separate engine
(AGENTS.md §1, [R14](risk-register.md#r14-per-source-motion-path-divergence)).

## How the VacuGlide is secondary today

These numbers come from running the shared engine with the VacuGlide owner's
sampling policy and the shipped follower, over all 98 built-in patterns and the
release fixtures. They use the Handy Original reference and the default
Reference pace unless noted. The method is at the end.

| Finding | Measurement |
| --- | --- |
| Patterns collapse to one speed | At speed 50, the 98 patterns run the motor at 32.6% ± 2.4. 36 of them vary by less than 2 points (10th to 90th percentile). Pulse plays at 32–34%, Tease at 29–36%, Stroke at 32.4–32.6%. |
| Where the motion happens is lost | Lower, middle and upper strokes give identical motor speeds. Full sweeps at speed 60 run the motor at 38.8% over the whole range and 38.3% aimed at the tip or the base. The frame's stroke rate nearly triples (1.13 → 3.25 Hz); the VacuGlide does not change. |
| Intense motion can play slowest | Fast-down and fast-up strokes at speed 100 run the motor at 16% (26% on Full motor range), because the slow leg dominates the average. Minimum-width strokes also run at 16%. |
| The default leaves the top of the motor unused | At speed 100, Reference pace averages 51% of the motor across patterns, and no pattern exceeds 66% (90th percentile). Full motor range averages 83%, and 32 of 98 patterns reach 95%. |
| Its speed depends on The Handy's model setting | The same request at speed 50 runs the motor at 32.6% with the Handy Original setting and 29.3% with Handy 2. That is 10% slower, set by a device that may not be in the session. |
| Changes arrive late | A speed change lands after at least 1.2 s of frame that is already buffered. The 1 s average then reaches 90% of the new speed 1.2 s later when speeding up and 2.0 s later when slowing down. |
| Its own control has no source | Air, set by the valves, changes only by a manual step or through an experimental follower that guesses depth from Handy positions. The model cannot ask for tighter, looser, a build in tempo or a pause. Instead, it is told what not to claim. |
| Its limits and buttons are The Handy's | The Speed limit, Speed match and the device's speed buttons work in Handy terms. The buttons move the shared Speed limit. |

Two of these problems come from what a follower is. Tuning cannot fix them.

- **The information is gone before the owner sees it.** The frame carries
  positions. The VacuGlide's controls are tempo and air. The frame does not
  say whether a stretch of motion was meant to tease, build or edge, so the
  owner can only measure how fast it travels.
- **Neither translation is faithful.** On a device with a fixed stroke, the
  motor sets the stroke rate. Following travel, which is what MagicHandy does
  today and how Autoblow converts scripts, keeps the stroke rate only for
  full-range strokes. Across the patterns, the VacuGlide strokes at 78% of the
  frame's rate on average, 40% for strokes spanning 40% of the range, and 33%
  at worst. Following stroke rate instead would play short, light strokes as
  the device's fastest full strokes. The right mapping depends on intent, and
  only the planner knows the intent.

## Options

### A. Keep the follower and improve it

These are small, low-risk changes:

- Average over a window centered on the frame time instead of trailing it.
  Generated frames are buffered at least 1.2 s ahead and media scripts are
  known in advance, so this removes about half a second of lag.
- Convert travel on a fixed scale instead of the Handy model's scale.
- Default to the full motor range, and let the device's speed buttons move
  the VacuGlide's own top speed.

The device stays pace-only: patterns, intent and air are unchanged. This is
enough only if the VacuGlide is meant to remain a companion to a Handy.

### B. A tempo-and-air channel in the shared engine (recommended)

This option gives the VacuGlide its own control model next to positions and
vibration. The design follows below.

### C. A separate VacuGlide engine

Rejected. It would duplicate Stop, run admission, coverage and linked-session
handling. It would also break AGENTS.md §1: one engine and one motion path,
the rule written against the StrokeGPT failure (R14).

### D. Device-side modes or uploaded scripts

Rejected as the main path, for two reasons:

- **Local scripts are opaque.** The VacuGlide stores 30 local scripts (20
  speeds and 10 stroking modes). MagicHandy cannot know or show their tempo,
  and selecting one closes both valves.
- **Uploaded scripts are slow and stored remotely.** Uploads sit on Autoblow's
  servers for nine days and can take a minute. They are still Handy-shaped
  funscripts.

Device modes could become a separate, explicitly reviewed feature later, with
Stop also covering `local-script/stop`.

## Proposed design (B)

### Control model

The engine owns two semantic channels:

- **Tempo, 0–100.** It is mapped into the person's **Tempo range** on the
  motor. The range defaults to the full motor range, with a floor above the
  stall point.
  - Modes are modeled on vibration's: steady, ramp to a level over a few
    seconds, wave, pulse, pause (the motor stopped for a time), and follow
    (derived from a position plan).
  - Periods stay at a couple of seconds or longer, so changes stay within what
    the cloud can carry, until hardware shows otherwise.
- **Air, as relative steps.** Each step makes the stroke tighter and longer,
  or looser and shorter.
  - At most one step every two seconds.
  - At most a few steps from where the session began, and never more than the
    existing 12 per connection.
  - The device reports nothing back, so the UI and the model facts both say
    that air steps are relative to where the person set the device.

### Which device leads

Each session has one leading control model:

- **VacuGlide alone**, with or without a vibrator or Coyote: tempo leads.
  - Chat, Autopilot, routines and manual controls produce tempo-and-air
    targets.
  - A tempo-only run starts without a position plan, like a vibration-only
    run.
  - A vibrator set to follow follows the tempo.
- **With a Handy or Ultra:** positions lead, as now.
  - The engine derives the VacuGlide's tempo from the plan, using a centered
    window and a fixed scale; holds become pauses.
  - Advanced models can set the VacuGlide's tempo independently, as they can
    with vibration.
- **Media:** tempo is derived from the script ahead of time with a centered
  window. This is the conversion Autoblow's own sync uses, without the lag.
- **Later:** a lead choice for mixed sessions, where the Handy follows the
  VacuGlide with full strokes at its tempo.

### Engine and transport

- `motion` gains a tempo specification and plan beside `VibrationSpec`.
  These use the same clock, generation fencing, coverage, pause and resume,
  and Stop.
- `AppendPointsCommand` carries a tempo frame beside points and vibration,
  plus air requests fenced by revision.
- **The VacuGlide owner becomes a mapper.**
  - Tempo becomes `target-speed`, with the stall floor, the hold and the cap
    on send rate.
  - Air becomes valve pulses, with the existing serialization and closure.
  - The averaging renderer and the reach follower leave the owner. The
    position-led follower moves into the engine, where it can see the plan.
- Conversion at the owner suits secondary outputs, as with the vibrator's
  conversion profiles ([ADR 0047](decisions/0047-intiface-vibrator-output.md))
  and the Coyote's.
  For the session's main stroker, that conversion is the whole experience.
- There are no new goroutines. The rules for Stop, valve closure, health
  checks and reconnects are unchanged.

### Settings, controls and UI

- **Device page.**
  - Tempo: the Tempo range and the commanded tempo.
  - Air: Looser and Tighter steps, the net steps this connection, and whether
    chat and Autopilot may change air.
  - Connection is unchanged.
  - Speed match and the reach follower are retired.
- **Device buttons.** The speed buttons move the top of the Tempo range, not
  the shared Speed limit.
- **Main view while the VacuGlide leads.**
  - The speed control sets tempo.
  - Position-only controls (the stroke window and zones) are hidden, and the
    Air steps are shown.
  - The visualizer draws commanded tempo over time, both recent and planned.
    It is labelled as commanded, not measured.
- **Library.** It offers VacuGlide routines. Position patterns are marked
  pace-only while a VacuGlide leads.
- Everything follows [UI design](ui-design.md) and its guidelines.

### Routines

A small native library of tempo-and-air plans with seeded variation: steady,
slow build, swells, bursts, stairs, edge and release, wandering. Autopilot uses
these routines and the model's own specifications.

### Model vocabulary

- VacuGlide facts and vocabulary appear only when a VacuGlide is connected.
  Prompts for a Handy alone keep their bytes.
- **By tier:**
  - Advanced: tempo modes, sequences and air.
  - Intermediate: tempo modes and the direction of air changes.
  - Simple: a tempo level or a routine.
- Each tier is admitted by evaluation, as in ADR 0057 and
  [ADR 0061](decisions/0061-model-controlled-vibration-level.md): the
  production parser, every response retained, and the compiled envelopes
  rendered, on the tier models.
- When the VacuGlide leads, this vocabulary replaces the pace-only guidance.
  In mixed sessions it adds independent tempo for Advanced models.

### Safety

- **Stop** works as now: the engine's Stop plus the owner's cloud Stop.
  Tempo frames from before Stop are fenced by generation.
- **Coverage:** a tempo frame that runs out stops the motor, as now.
- **Air** is bounded per session and per connection. Stop closes the valves,
  and nothing changes air after Stop.
- **New risk:** chat and Autopilot could tighten the sleeve. Bounded steps, a
  permission that is off by default, and the existing admission keep this
  within the person's choice. The risk register needs an entry for it.

## Phased plan

1. **Native control model.**
   - Engine: the tempo channel, tempo-led runs when the VacuGlide plays alone,
     and the engine-side follower for position-led sessions and media.
   - Device: the owner as a mapper, the Tempo range and the buttons.
   - UI: the device page and the tempo visualizer.
   - Docs: the ADR, UI design, risk register and scorecard.
   - Accepted in simulation: lifecycle, races with Stop, coverage, goroutine
     leaks and linked sessions.
2. **Routines and manual control.** The native library, the library UI and the
   quick controls.
3. **Model and Autopilot.** Advanced first, then Intermediate and Simple as
   evaluations allow.
4. **Hardware calibration.** Measure stroke rate against tempo, the stall
   floor, response time, the effect of one air step, and whether the air
   permission is needed. Then tune the defaults.
5. **Later.** A lead choice for mixed sessions, device modes, and other
   speed-only strokers if any are added.

Phase 1 alone removes the dependence on the Handy model, the unused range, the
lag, and the Handy-terms buttons and limits. It also makes sessions where the
VacuGlide plays alone tempo-led with manual control. Phases 2 and 3 give the
VacuGlide its own routines and its own model vocabulary.

## Open questions

- In mixed sessions, should positions lead by default (proposed), or the
  VacuGlide?
- Should chat and Autopilot be allowed to change air at all? If so, should
  their steps be undone when the session ends? They cannot be undone after
  Stop.
- Should the Tempo range default to the full motor range (proposed), or to a
  gentler top until the hardware has been measured?
- Will a VacuGlide be available for phase 4?

## Method

- A labs-tagged scratch test does the sampling and rendering. It was removed
  with the alpha.85 renderer it measured:
  - It samples the shared engine for 14 s per target, with the sampling
    capabilities of the VacuGlide owner.
  - It renders the shipped `vacuglide.Renderer` every 100 ms, like the
    owner's tick, and summarizes from 2 s onward.
- **Stroke rate** counts the turning points in the commanded frame. The
  VacuGlide's implied stroke rate uses Autoblow's own scale: 600 units per
  second at 100% is three full down-and-up cycles per second. Fidelity is
  therefore a ratio that does not depend on the physical top speed.
- **Step responses** use full-range strokes whose pace changes at a stroke
  boundary. They are timed from that change in the frame.
