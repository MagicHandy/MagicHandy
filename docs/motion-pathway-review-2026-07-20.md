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
   Bluetooth now maps to firmware's native 0-1000 point scale, preserving 0.1%
   resolution. Cloud REST must still use the API v3 integer `PointPosition`; it
   advertises that one-percent limit to the shared sampler. Both owners quantize
   the semantic position before mirroring the native step, so reverse mode is
   an exact reflection instead of differing by one step at half-way ties. A
   second bounded reduction removes rounded dwell/catch-up knots only when the
   resulting wire line remains within the owner-specific error bound. Curve
   fitting stays in the engine rather than becoming a second transport motion
   model.
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

- Fixed 125 ms sampling: 3,413 points, 788 duplicate edges, and 97,191 ms of
  rounded stationary segments.
- Adaptive 25 ms probe plus 0.3% semantic reduction: 1,767 points, 438 duplicate
  edges, and 36,633 ms stationary, while preserving every tested semantic curve
  within 0.35%.
- Cloud-aware 0.8% bounded fitting: 1,247 points, 106 duplicate edges, and
  14,135 ms stationary. This cuts rounded stationary time by 85% relative to
  the old grid; worst measured wire error is 0.818% (`Hard and Regular`).
- The old fixed grid missed a `Hard and Regular` peak by 3.125%.

## Follow-Up Review - 2026-07-21

The first review's algorithms were retained, but four boundary defects were
confirmed and fixed:

1. API v3 returns `play_state` as enum integers (`0..4`). Treating only strings
   as valid made an online Handy look HSP-unavailable. The state parser now maps
   the documented enum, and stopped/not-initialized are accepted only during
   startup. Either state during an established run forces the shared recovery
   Stop.
2. The Cloud engine clock began before stroke setup, HSP setup, initial add, and
   Play. On the live device it ran about 1.4-1.5 seconds ahead of HSP
   `current_time`, causing avoidable old-plan buffering and late-feeling
   retargets. Cloud and Browser Bluetooth now report the successful Play request
   midpoint as their stream origin, matching the existing Intiface post-anchor
   clock contract.
3. Intiface exposed `StepCount` in diagnostics but did not expose it to the
   shared sampler. A 100-step actuator behind a 20-80% stroke window has an
   effective semantic resolution of about 1.67%, not 1%. The engine now scales
   physical resolution through the current window before its bounded reduction.
   Positions remain floats until the owner boundary; this is not a transport
   deadband or a second motion model.
4. Cloud accepted engine frames up to 128 points although API v3 permits at
   most 100 points per `/hsp/add`. The owner advertises and validates the
   100-point cap, so dense content fails before an oversized HTTP request.
5. The lead calculation spent its 250 ms safety allowance on a dispatch loop
   that can notice a low buffer 200 ms late. In the captured Cloud trace,
   ordinary adds were sometimes acknowledged with only 80-125 ms left in the
   prior buffer; three acknowledgements crossed the tail, including -540 ms on
   a 958 ms request. With `pause_on_starving` enabled, those underruns are the
   reported micro-stops. Cloud now declares a 1.5-second minimum accepted lead,
   startup prebuffers to it, the dynamic calculation reserves a dispatch tick,
   and checks use the actual last emitted timestamp instead of a nominal frame
   boundary.
6. API v3 can return an HTTP 200 response containing an `error` envelope. The
   Cloud owner treated that as accepted and advanced its cumulative point
   index, leaving an unfilled interval in the device buffer. Error envelopes
   now fail the command without advancing the tail; response messages are not
   copied into errors or diagnostics.

A catalog probe covered all 29 built-ins at 5%, 10%, 20%, and full focus;
20%, 40%, and 100% speed; and both 1% Cloud and 1.67% effective Intiface
resolution. No rapid one-native-step reversal remained at normal 20% or wider
focus. Very narrow 5-10% focus can intentionally collapse a pattern to only a
few physical steps; it was not replaced with ScriptPlayer's fixed 10-unit
deadband because that would make the requested subtle motion static.

Two capped 20% Cradle clock runs and one full 30%-maximum retarget checklist
were completed over Cloud REST. Before alignment, sampled engine/device clocks
were about 1.4-1.5 seconds apart. After alignment they were 120-160 ms apart
while the state read itself crossed the network. The full checklist completed
15 transport commands with zero failures, ten add batches / 90 points, Cloud
latency of 307-355 ms, retarget lead of 668-1,141 ms, and an explicit successful
Emergency Stop. The pre-fix comparison emitted 13 add batches / 125 points.

Whole-percent Cloud output still has a physical resolution floor. The fix
removes avoidable aliasing and premature rounding; it does not claim that a 10%
stroke window can contain more than roughly eleven distinct Cloud positions.

## Follow-Up Review - 2026-07-22

The reported paired-script failure was reproduced from the retained Cloud
trace and the source actions around video time 1:17. The source sequence is
strictly increasing and alternates cleanly: `76157/46 -> 77134/81 -> 78088/41`.
Starting at 61,863 ms, the engine emitted those actions at relative times
14,294, 15,271, and 16,225 ms with the configured motion-scale projection. No
duplicate timestamp, plateau, cubic interpolation, or synthetic return was
introduced by parsing or media slicing.

The same trace exposed only about 2.1 seconds of accepted Cloud coverage and
one `/hsp/add` request per second. Every observed request returned HTTP 200,
but `pause_on_starving` makes any downstream delivery jitter visible as a
physical stop. Clock-locked media now prebuffers and maintains ten seconds,
batches multiple sampler windows under the owner's 100-point cap, and refills
with up to four seconds of extra headroom. Interactive targets retain the
1.5-second horizon because their future queue is also their retarget latency.

[Syncopathy's HSP player](https://github.com/ofs69/syncopathy/blob/main/lib/player/handy_native_hsp_mixin.dart)
was reviewed as a reference. It keeps up to 30 seconds eager-buffered, groups
15 actions per buffer, primes multiple buffers before playback, and periodically
resynchronizes the device clock. MagicHandy adopts the deeper fixed-stream
buffering lesson, not Syncopathy's separate motion path or
`pauseOnStarving: false`; starvation remains fail-stopped here.

A separate pattern defect matched the report that slow chat patterns lingered
at reversals. Zero PCHIP slope at an extremum shaped the entire interval on
both sides, producing a smooth but prolonged endpoint ease. Loop patterns now
use backend-only trapezoidal velocity guides capped at 75 ms per side. The
stroke body remains constant-speed, authored extrema and the exact
zero-velocity instant remain intact, and internal guides are not forced onto
the wire. Quantized retarget frames receive the same final <=2% chatter cleanup
as semantic frames. Finite programs and linear media timelines are unchanged.

Automated tests now pin the reported 1:17 action sequence, media-only lead
selection, batched prebuffering and owner point caps, reversal profile,
acceleration/no-overshoot limits, approximation error, whole-percent stationary
time, and post-quantization retarget chatter. A capped post-fix hardware run is
still required before calling either subjective issue closed.

The first capped run also exposed a separate startup defect before those
continuity checks could be judged: the very first action moved abruptly toward
the pattern's first position. Its trace began with semantic position 0 at
stream time zero while the physical position was unknown. HSP timing constrains
segments after that anchor; it cannot constrain acquisition of the anchor
itself. The old start order also applied the requested 20-80% stroke window
before any physical observation, which could independently clamp a parked
slider outside that range.

Cloud startup is now position-aware. It first issues an HSP Stop, reads the
physical slider and existing stroke window, uses a non-narrowing union window
for a speed-bounded two-point HSP lead-in when needed, verifies arrival, and
only then applies the requested window and starts the main stream. At the
observed 20% cap, full physical travel requires at least five seconds. The
lead-in is cancelable through the ordinary engine Stop barrier, and media time
does not begin until it completes. Automated tests cover the observed
90%-to-20% case, no-op alignment, malformed Cloud state, and Stop during the
lead-in.

The first corrective hardware run also caught an API-coordinate trap and
failed stopped before the main stream: live firmware's `position` value was
relative to its active 17-80% window rather than full travel. Startup now uses
the absolute slider position and absolute stroke endpoints to reconstruct the
full-travel coordinate. In the repeat capped run it observed 29.33 mm, issued a
500 ms bounded lead-in toward the 24.57 mm first target, verified 24.67 mm with
zero reported speed inside the final 20-80% window, and did not issue the main
Play until after that verification and final-window application. The remaining
validation sequence completed and its final emergency Stop returned HTTP 200.
The objective startup gate therefore passes; subjective confirmation that the
first action no longer feels abrupt remains open.

This evidence and correction apply to Cloud REST. Browser Bluetooth does not
currently perform a slider-state read because that probe has destabilized live
GATT sessions, and Intiface has no actuator-position feedback even though its
initial `LinearCmd` has a duration. Those owners still need a separately
validated way to bound worst-case first-point acquisition; the engine keeps
their existing behavior rather than claiming an unavailable measurement.

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
  Bluetooth has 0.1% HSP encoding. Intiface resolution is device-specific and
  is now included in shared sampling after stroke-window projection.
- Stroke-window and reverse settings are owner-level envelope changes. HSP may
  apply them to already buffered points immediately, while Intiface snapshots
  reverse at append time. A cross-owner ramped-envelope design needs matched
  hardware evidence; it should not be inferred from source code alone.
- Automated tests establish continuity, timing, bounded approximation error,
  Stop ownership, and goroutine teardown. Subjective feel still requires the
  same pattern and 10%/20% focus cases on Cloud, Browser Bluetooth, and
  Intiface, capped below 40% speed, with trace export and owner telemetry.
