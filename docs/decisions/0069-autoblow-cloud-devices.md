# ADR 0069: Autoblow cloud devices: the Autoblow Ultra and the VacuGlide

> Amended by [ADR 0071](0071-vacuglide-native-tempo.md):
>
> - The VacuGlide plays the engine's tempo channel across its own Tempo range.
> - Speed match and the reach follower are retired.
> - The VacuGlide's speed buttons move its Tempo range.
> - With only a VacuGlide playing, chat and Autopilot choose its tempo and,
>   when allowed, its air.

## Status

Accepted 2026-11-11. Implemented and tested in simulation only: no Ultra or
VacuGlide was available, so hardware acceptance is open. The
[review](../autoblow-devices-review-2026-11-11.md) records the investigation,
the mapping and the acceptance list.

## Context

The owner asked for support for Autoblow's Autoblow Ultra and VacuGlide, for
an investigation of the VacuGlide's depth and range controls, and directed
that Autoblow's cloud-only Stop is outside MagicHandy's control and is to be
treated as intended behavior.

Autoblow's API V1 is the only documented path to either device:

- HTTP through regional clusters, found through `latency.autoblowapi.com`, with
  a per-device token that gives full control. No LAN or Bluetooth API is
  documented, and no Buttplug support was found.
- The devices take commands only while they are online. A VacuGlide enters and
  leaves online mode by holding its mode button for about 2.5 seconds. While
  online, the buttons are sent as server-sent events instead of acting on the
  device.
- The **Ultra** takes positions: go-to (position and speed), an oscillator
  between two points, and position scripts. Autoblow documents 400 position
  units per second as its fastest move and 70 ms as its shortest.
- The **VacuGlide** takes a motor speed (target speed, 0–100%) and has two
  stroke valves. Its script player turns each move's velocity into motor
  speed: 600 units per second is 100%, and the motor stalls below about 4%.

The VacuGlide's construction, from Autoblow's support pages and reviews: a
crank-driven piston moves air through a hose into a receiver tube, and the
air pulls and pushes the sleeve. The crank arm length, set with an Allen key,
fixes how much air each stroke moves, so it caps the stroke. The cap's valve
controls airflow; drawing air out of the sealed sleeve makes the stroke
longer and tighter, and letting air in makes it shorter and looser. Autoblow's
own modes and Autopilot drive the valves, and its support page describes
pressing the stroke button to suck air out after a leak. Depth and stroke
length are therefore one coupled axis set by air, with no position or air
measurement reported.

## Decision

1. **Two device kinds.** `autoblow` (the Ultra) and `vacuglide` join the
   inventory and playback set like the other devices, each saved at most once,
   in run-clock order: the Handys, the Ultra, the VacuGlide, the Coyote, the
   vibrator. Each has its own token, saved with an explicit Save like the
   Handy connection key and never returned to a client.
2. **One cloud client** (`internal/autoblow`). It looks the device up on the
   latency host, then sends every request to that cluster. Without a test
   override, it sends the token only to `*.autoblowapi.com` over HTTPS and
   refuses redirects for lookup, commands and events. Its errors
   never contain the token. The event stream carries the token in its URL, so
   that URL is never logged. Rate-limit headers are recorded, and a 429 is
   reported, never retried in a loop.
3. **The Ultra is a position owner.** It follows the transport contract
   ([ADR 0002](0002-motion-transport-contract.md)) like the Intiface owner:
   - Each stroke leg is one go-to: to the next turning point, or to the
     furthest point within 600 ms of a long leg. It is sent half the measured
     round trip early, plus the user's offset, at the speed that covers the
     leg in time on Autoblow's 400 units-per-second scale.
   - The stroke window and reverse direction are applied host-side, exactly
     once.
   - Points are at least 100 ms apart, with sparse curves preferred.
   - A 900 ms preroll brings the slider to its first point at 30%.
   - A frame that runs out is stopped and reported, like the other pacers.
   - Stop is `oscillate/stop`, which stops whatever the Ultra is doing.
4. **The VacuGlide is a pace owner.** Its motor follows the frame's travel,
   averaged over about one stroke (1 s), on Autoblow's own scale:
   - The **Speed match** setting sets the factor. The default, Reference pace, maps MagicHandy's 1–100% to about 5–61% of the motor with the
     Original reference. Full range stretches MagicHandy's top speed to the
     motor's.
   - A moving frame never drops below 5%; a frame that holds still stops the
     motor.
   - Changes are capped at four a second.
   - A frame that runs out stops the motor at once.
   - Stop attempts `target-speed/stop` and closes both valves, even if the
     motor Stop fails. A lost valve-open acknowledgement also triggers closure.
5. **Depth and range on the VacuGlide are air.**
   - **Stroke steps** open a valve for 300 ms. Both directions require the
     controlling administrator and the current Stop sequence. Valve direction
     is not a measured reduction in pressure, so neither bypasses admission. A connection allows at
     most 12 net steps either way.
   - **Follow the motion's reach** is experimental and off by default. It
     reads how far the frame's strokes reached over 2.5 s and takes at most
     one step every two seconds toward it, 20% of reach per step, within three
     steps of where playback began. It is not tuned on hardware.
   - Nothing else changes the valves. Starting a session never resets them,
     because the person's setting is the baseline.
6. **Device buttons.** The Ultra's pause button and the VacuGlide's mode button
   run Emergency Stop, like Esc. The speed buttons move the Speed limit's
   maximum by 10, like the device panel. The Ultra's mode button does nothing.
7. **Cloud-only Stop is accepted as intended**, at the owner's direction.
   - Emergency Stop reaches every connected Autoblow device beside the
     engine's Stop, whether or not it plays.
   - A command already on the wire gets 300 ms to finish so Stop lands after
     it. Past that it is canceled and Stop is sent again 750 ms later, because
     the cloud may still apply it.
   - Closing the app stops connected devices.
   - A health read every five seconds marks a device that left online mode as
     lost. Three consecutive read failures also attempt Stop and retire the
     connection. A reconnect never resumes its previous timeline.
   - The device pages say that Stop needs the internet and Autoblow's cloud.
     [R41](../risk-register.md#r41-autoblow-cloud-only-stop) records the
     residual risk.
8. **Model facts.** The Ultra counts as a stroker. The VacuGlide counts as a
   pace-only stroker: when it is the only stroker, replies describe pace and
   rhythm, not depth or stroke length. The Handy-only prompt keeps its bytes.
9. **Simulation.** `-simulate-motion` builds point both owners at an
   in-process imitation of Autoblow's cloud (`internal/autoblowsim`), so no
   request reaches Autoblow and nothing moves.

## Consequences

- The Ultra plays every motion source through the shared engine. Its shape
  within a leg is the device's own move, and a leg can arrive a round trip's
  jitter early or late.
- The VacuGlide plays pace only. Positions, zones, span and the Stroke limits
  do not reach it, and patterns that differ only in shape feel alike. Its
  stroke length is the person's, through the valves, unless they opt in to
  following.
- Videos play through the same live paths. Autoblow's script upload, which
  stores scripts on its servers for nine days, is not used.
- Stop depends on the person's internet connection and Autoblow's cloud. The
  VacuGlide's motor keeps its last speed if both fail, until it is switched off
  or its four-hour limit stops it.
- Shipping size measurements are recorded in the
  [scorecard](../goal-scorecard.md); the simulation helper is isolated from
  production dispatch unless `-simulate-motion` is selected.

## Validation

- `internal/autoblow`: lookup and cluster checks, request shapes, status
  mapping, token redaction, rate-limit headers and events.
- `internal/vacuglide`: pace on Autoblow's scale, the stall floor, holds,
  coverage and reach.
- `internal/transport`, against the simulator:
  - go-to per leg, the projected stroke window, starvation, and nothing sent
    after Stop;
  - motor speed and Stop, and valve steps;
  - sessions retired by a reconnect, a device lost when it leaves online mode;
  - button events, the test, and the goroutine-leak gate.
- `internal/httpapi`:
  - token save and redaction, connect preconditions, stroke permissions;
  - Emergency Stop reaching an idle device, the device buttons, a new token
    disconnecting;
  - the engine playing through each device alone, and route admission.
- `internal/chat`: the pace-only guide, with Handy-only bytes unchanged.
- The device kit models, the VacuGlide page and the visualizer choice, in five
  locales.
- Hardware acceptance (open): see the review.
