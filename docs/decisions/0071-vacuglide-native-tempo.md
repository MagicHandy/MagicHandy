# ADR 0071: The VacuGlide as a main device: a tempo-and-air channel

## Status

Accepted 2026-11-12 at the owner's direction. Implemented and tested in
simulation only; hardware calibration is open. Amends
[ADR 0069](0069-autoblow-cloud-devices.md). The
[architecture review](../vacuglide-native-review-2026-11-12.md) records the
measurements behind it.

## Context

ADR 0069 made the VacuGlide a follower of motion planned for The Handy: the
engine planned positions, and the VacuGlide's owner averaged how fast they
moved into a motor speed. The review measured what that meant on alpha.85:

- At speed 50, all 98 built-in patterns played at 32.6% ± 2.4 of the motor.
  Pulse, Tease and Stroke were indistinguishable, and so were tip, base and
  middle strokes.
- Fast-down strokes at speed 100 played at 16%, the slowest the device went.
- The default mapping left the top of the motor unused, and the result moved
  about 10% with the Handy model setting.
- A speed change took 1.2 s of buffered motion plus 1.2 to 2.0 s of averaging
  window.
- Nothing chose the air, and the device's speed buttons moved The Handy's
  Speed limit.

The owner directed that the VacuGlide "shouldn't be considered a device
secondary to The Handy but rather a main device of its own". They chose the
full motor range as the default, and that positions lead whenever a
position-capable device is present.

## Decision

1. **A tempo channel in the shared engine.** Tempo is semantic: a level from
   0 to 100 percent of the person's Tempo range, where 0 stops the motor.
   - The vocabulary is `follow`, `steady`, `ramp`, `wave`, `pulse`, `pause`,
     `sequence` (up to 12 timed sections, optionally looping) and `routine`
     (a built-in plan varied by a seed).
   - A new tempo starts from the current level and blends in over a second.
     A pause stops at once.
   - The engine samples tempo on its own clock and lifecycle, beside positions
     and vibration, exactly as it does vibration
     ([ADR 0057](0057-independent-vibration-envelope.md)).
   - There is no new goroutine and no path from chat to a transport.
2. **Positions lead when a Handy or Ultra plays.** A run with a tempo device
   and no explicit tempo follows the positions.
   - The engine derives tempo from its own plan over a centered two-second
     window, averaged again over one second, so follow neither lags nor
     hunts.
   - The scale is the selected Handy model's top pace, so the model setting
     no longer changes the VacuGlide's pace.
   - A tempo set by the person or by a model replaces follow until the run
     ends.
3. **Tempo leads when only the VacuGlide plays.** It may play beside a
   vibrator or a Coyote.
   - A tempo-only run starts from idle like a vibration-only run. It drives
     only tempo owners, never positions an idle slider, and keeps Stop,
     pause, resume and coverage.
   - Position content chosen during a tempo-only run promotes it through the
     ordinary Stop and start.
4. **The owner is a mapper.** The VacuGlide stores the engine's tempo frames
   and maps each level onto its Tempo range.
   - It keeps the stall floor (5%), stops below level 0.5, and caps
     requests at four a second.
   - The averaging renderer and the experimental reach follower are removed.
   - A tempo that runs out stops the motor, as before.
5. **Settings.** Speed match and Follow the motion's reach are retired.
   - **Tempo range**: a minimum and a maximum motor speed, defaulting to
     5–100%.
   - **Chat and Autopilot may adjust the stroke**: off by default.
   - The device's speed buttons move the top of the Tempo range by 10. The
     Ultra's buttons still move the shared Speed limit.
6. **Air.** Chat and Autopilot may step the valves only when the person allows
   it.
   - Steps are allowed only while playing, at most three either way from where
     the session began, inside the existing 12 per connection.
   - Manual steps are unchanged, and Stop closes the valves as before.
7. **Routines.** Six built-in routines with seeded variation: Slow build,
   Swells, Bursts, Stairs, Edge and release, and Wandering. They play from
   the main view or the model.
8. **Model vocabulary.** When tempo leads, chat and Autopilot use a tempo
   contract in place of the position contract.
   - The contract keeps the persona's sections and replaces only the motion
     vocabulary.
   - Simple gets steady, pause and routines.
   - Intermediate adds ramp, wave, pulse and air steps.
   - Advanced adds sequences.
   - Autopilot tempo choices ride a hold, so the positions are left alone.
   - Sessions led by positions keep their prompts byte for byte.
9. **UI.**
   - The VacuGlide page shows its Tempo range, the motor speed sent, the
     stroke steps and the air permission.
   - While tempo leads, the main view draws the tempo now and over the next
     eight seconds, with the motor speed labelled as sent rather than
     measured.
   - It offers the routines, a Tempo slider and Pause the motor.

## Consequences

- With only a VacuGlide, motion is native: tempo, its shape over time, and air
  within the person's permission. Patterns built for positions still play and
  are followed, as media is.
- A tempo change takes effect within one owner tick and the cloud trip. A
  follow change still waits for the buffered positions.
- A vibrator or Coyote in a tempo-only run stays idle; following tempo with
  them is future work. The Handy does not yet follow the VacuGlide's tempo.
- The device's built-in local scripts and Autoblow's script upload remain
  unused: they are opaque to the engine and its Stop.

## Validation

- `internal/motion`:
  - tempo validation;
  - envelope bounds and continuity across sections and routines;
  - routines for every seed;
  - tempo-only start, pause, resume and promotion;
  - follow on both Handy model settings;
  - `ApplyTempo` admission.
- `internal/transport`: mapping across the Tempo range, the pause, revision
  fencing, and motion air bounded by permission and session.
- `internal/httpapi`:
  - the tempo routes;
  - a routine started and changed in place through the simulator;
  - the range bound and the device buttons;
  - chat tempo with and without air permission;
  - the route admission matrix.
- `internal/chat`: each tier's shapes, the schemas, the prompt swap, the
  repair, the Handy prompt unchanged, and Autopilot holds.
- The frontend's VacuGlide page, the tempo readout, and the strings in five
  locales.
- Hardware (open): stroke rate against tempo, the stall floor, response to
  tempo changes, and the effect of one air step.

## Completion, 2026-11-12

Codex finished the implementation after Claude's session stopped, and the
owner then asked for a review and a Graphite realignment:

- **Air goes through the engine.**
  - `Engine.ApplyAir` sends a relative step as an engine command with its own
    revision, to tempo owners only.
  - The owner consumes the revision before any write, so an uncertain reply
    is never repeated.
  - A refused step does not fail the run.
- **Tempo-only runs carry an inert position channel,** not a placeholder
  Handy pattern.
- **Local models get one schema branch per tempo shape,** with the mode
  first. Gemma 12B passed 28 of 28 cases, E4B 23 of 23 and GPT 35 of 35,
  including continuing after Stop and spoken check-ins.
- **A stopped tempo can be resumed when asked.**
- **Edge and Autopilot speak tempo.**
  - Edge holds a low steady tempo and can return to the saved plan.
  - An Autopilot tempo choice can start a tempo-only run from idle, and while
    tempo leads, a planning failure holds instead of falling back to Handy
    patterns.
  - Spoken check-ins use the tempo vocabulary.
- **The person's tempo controls send the Stop sequence they were admitted
  under.**
- **UI, realigned to Graphite** (see [UI design](../ui-design.md)):
  - one Now playing card for every device mix;
  - a separate Play card while tempo leads;
  - one status-bar readout;
  - device panel rows grouped by run, with the stroke permission beside the
    Tempo range.
- **Autopilot timing switches** are removed by
  [ADR 0072](0072-autopilot-timing-without-switches.md).
