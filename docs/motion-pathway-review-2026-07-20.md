# Motion Pathway Review - 2026-07-20

## Scope

This review followed every production motion source through pattern import,
normalization, interpolation, retargeting, frame generation, and the three
dispatch owners. It also compared the relevant ScriptPlayer implementation at
commit `19eb92ff76478c19de03933db7ab9be343918a52`.

The production path remains singular:

`chat / Autopilot / Freestyle / library / program -> MotionTarget -> MotionPlan -> Curve -> Engine frame -> Transport`

Only `internal/motion` calls `transport.AppendPoints`, `SetStrokeWindow`, or
`Play`. Pattern and program imports produce definitions; they never construct
wire commands. This audit removed the unused controller-only Cloud and
Bluetooth stroke/HSP add/play HTTP routes, which were an unintentional raw
motion bypass. Connection checks, state/events, diagnostics, and emergency
Stop remain available.

## Confirmed Jitter Sources And Fixes

1. **Fixed-grid aliasing.** The old 125 ms frame could miss authored peaks,
   dwells, or reversals between sampler ticks. `Hard and Regular` has meaningful
   125-167 ms knot spacing and exposed this directly. Buffered owners now merge
   authored knot times with 25 ms curve probes, then reduce the frame with a
   0.3 percentage-point vertical-error bound. Output is capped at 128 essential
   points per one-second nominal frame; pathological content fails clearly
   instead of flooding a transport.
2. **Loop-seam stops.** Loop PCHIP previously assigned zero velocity to both
   endpoints unconditionally. A loop whose final and first legs continued in
   the same direction therefore decelerated to a false stop once per cycle.
   Closed loops now use the cyclic weighted harmonic slope when the adjacent
   secants agree; true seam reversals still have zero velocity.
3. **Noise amplification during import.** A loop selection with only 1% source
   span could be expanded to the full semantic 0-100 range. Pattern import now
   requires at least 5% source span. Rapid extrema whose smaller adjacent swing
   is at most 2% are removed before bounded-error simplification. The cadence
   check is relative to cycle length and capped at 250 ms, so a slow, deliberate
   1-2% excursion is preserved. Finite programs remain source-exact.
4. **Discontinuous retargets.** The old path inserted one bridge point only for
   jumps over 12%; smaller changes snapped, while the sample after a large
   bridge could still jump. Retargets now use a 750 ms smootherstep crossfade
   between effective paths. Position and velocity are continuous at both ends,
   and rapid retarget phase selection uses the effective path position and
   direction rather than an underlying plan hidden by an earlier transition.
   Crossfading two moving curves can itself create a tiny extra turn, so the
   final transition frame removes rapid reversals at or below 2% while
   protecting both handoff endpoints. All catalog pattern-pair and 10% focus
   transitions are checked for that property at semantic and Cloud resolution.
   Transition history expires against estimated playback time, not merely when
   its future points enter the buffer, so a queued retarget still samples the
   effective path. The trace field remains `bridge_points` for schema
   compatibility.
5. **Premature and unaccounted Handy quantization.** The shared frame already
   carries `float64`, but both Handy owners rounded it to whole percent. Browser
   Bluetooth now keeps the semantic fraction until the protobuf codec maps it
   to firmware's native 0-1000 point scale, preserving 0.1% resolution. Cloud
   REST must still use the API v3 integer `PointPosition`; it now advertises
   that one-percent limit to the shared sampler. A second bounded reduction
   removes rounded dwell/catch-up knots only when the resulting Cloud line
   remains within 0.8% at every retained semantic probe. Curve fitting stays in
   the engine rather than becoming a second transport motion model.
6. **Immediate-mode cadence.** Intiface continues to use the selected device's
   `DeviceMessageTimingGap` plus its scheduler margin. The shared engine does
   not inject authored knots below that floor; tests inspect the emitted frame,
   not only the configured interval.

PCHIP interpolation itself remained shape-preserving and did not overshoot.
The Intiface pacer also schedules absolute `LinearCmd` segment deadlines and
does not wait for each acknowledgement before the next deadline; neither was a
reason to introduce another motion model.

## Quantitative Check

The catalog check covers all 29 built-ins over two cycles with a 10% focus
window, where integer output is most vulnerable to stair steps. Stationary time
is more useful than duplicate ratio alone because it measures how long a
whole-percent wire path dwells before catching up.

- Fixed 125 ms sampling: 3,385 points, 915 duplicate edges, and 113,001 ms of
  rounded stationary segments.
- Adaptive 25 ms probe plus 0.3% semantic reduction: 2,294 points, 528 duplicate
  edges, and 71,232 ms stationary, while preserving every tested semantic curve
  within 0.35%.
- Cloud-aware 0.8% bounded fitting: 1,420 points, 151 duplicate edges, and
  29,717 ms stationary. This cuts rounded stationary time by 74% relative to
  the old grid; worst measured wire error is 0.843% (`Waves`).
- The old fixed grid missed a `Hard and Regular` peak by 3.125%.

Whole-percent Cloud output still has a physical resolution floor. The fix
removes avoidable aliasing and premature rounding; it does not claim that a 10%
stroke window can contain more than roughly eleven distinct Cloud positions.

## ScriptPlayer Comparison

Useful ideas retained from
[ScriptPlayer](https://github.com/FredTungsten/ScriptPlayer):

- [`MainViewModel.cs`](https://github.com/FredTungsten/ScriptPlayer/blob/19eb92ff76478c19de03933db7ab9be343918a52/ScriptPlayer/ScriptPlayer/ViewModels/MainViewModel.cs)
  derives each action duration from the next timestamp and commands the next
  endpoint. This supports MagicHandy's absolute timed-point/segment model.
- Device command spacing and explicit late-command handling reinforce the need
  for bounded host pacing on immediate-mode owners.

Behavior deliberately not copied:

- [`Device.cs`](https://github.com/FredTungsten/ScriptPlayer/blob/19eb92ff76478c19de03933db7ab9be343918a52/ScriptPlayer/ScriptPlayer.Shared/Devices/Device.cs)
  replaces queued commands when transformed positions differ by less than 10.
  That can erase an entire subtle MagicHandy focus window.
- The common path transforms positions to bytes before dispatch, losing useful
  precision. MagicHandy keeps floats until the owner encoding boundary.
- [`HandyScriptServer.cs`](https://github.com/FredTungsten/ScriptPlayer/blob/19eb92ff76478c19de03933db7ab9be343918a52/ScriptPlayer/ScriptPlayer.HandyApi/ScriptServer/HandyScriptServer.cs)
  expands any partial source range to 0-100. MagicHandy allows this only for an
  explicitly selected loop with a meaningful source span; programs retain
  source amplitude.
- The current Buttplug position branch sends immediate position output while
  its duration-aware linear call is commented out. MagicHandy's deadline-paced
  `LinearCmd` owner is the stronger reference for linear actuators.
- ScriptPlayer's HSSP upload architecture is not a model for firmware-v4 HSP
  incremental buffering, retargeting, or Stop ownership.

## Residual Risk And Hardware Gate

- Cloud REST integer position encoding is unavoidable under the current API v3
  [schema](https://www.handyfeeling.com/api/handy-rest/v3/docs/). Browser
  Bluetooth and Intiface should be judged separately because they retain finer
  position resolution.
- Stroke-window and reverse settings are owner-level envelope changes. HSP may
  apply them to already buffered points immediately, while Intiface snapshots
  reverse at append time. A cross-owner ramped-envelope design needs matched
  hardware evidence; it should not be inferred from source code alone.
- Automated tests establish continuity, timing, bounded approximation error,
  Stop ownership, and goroutine teardown. Subjective feel still requires the
  same pattern and 10%/20% focus cases on Cloud, Browser Bluetooth, and
  Intiface, capped below 40% speed, with trace export and owner telemetry.
