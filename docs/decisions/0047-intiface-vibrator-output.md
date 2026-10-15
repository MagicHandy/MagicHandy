# ADR 0047: Intiface vibrator output

## Status

Accepted 2026-10-15. Implemented and tested against simulated Buttplug servers
only: no vibrator was available. Physical acceptance is open (see Validation).
Model replies still assume a linear stroker; the
[device-aware model responses plan](../device-aware-prompts-plan-2026-10-15.md)
covers that later work. Extends [ADR 0010](0010-transport-neutral-frames-intiface.md)
(Intiface), [ADR 0044](0044-coyote-estim-output.md) (companion helpers) and
[ADR 0045](0045-device-inventory-and-linked-sessions.md) (participation and
linked sessions).

## Context

Most devices Intiface Central supports are vibrators, but ADR 0010 maps only
`LinearCmd`. The owner asked for a Buttplug vibrator that plays with the
stroker (The Handy over any connection method, or a linear actuator through
Intiface), with the Coyote, with both, or alone.

Three facts shaped the design:

- **A vibrator holds its last level.** The Coyote stops 0.1 s after its last
  packet; a vibrator keeps running until it is told otherwise.
- **One client per server.** Intiface Central serves one client in practice,
  so the linear actuator and the vibrator must share a single connection.
- **The frame stays the only motion model.** As with the Coyote, the vibrator
  follows the shared motion frame through a conversion profile. The model
  never controls its intensity.

The [implementation plan](../intiface-vibrator-plan-2026-10-15.md) records the
design, the GPT-6-Astra and Gemini Argon reviews, and the owner's choices,
including vibrator-only playback.

## Decision

1. **Playback devices.**
   - `device.configured_devices` accepts `vibrator`, at most one.
   - Participation becomes a set, `device.playback_devices`. A missing set
     migrates from the legacy `output`; an explicit empty set stays empty.
     Every member must be configured, and a write that sets both fields
     inconsistently is rejected.
   - `output` remains a projection of the Handy and Coyote members. A legacy
     write never removes the vibrator.
   - The engine's owner key includes the playback set and the Coyote,
     Bluetooth and vibrator bindings (for the vibrator, the session
     generation and selection epoch). Reconnecting or reselecting one of them
     builds a new engine.
2. **Linked sessions for any set.**
   - `FanOut` holds an ordered list of labeled participants. The first
     included device in the order Handy, Coyote, vibrator is the run clock.
     A participant that is alone is its own clock, and a retired clock hands
     the role to the next active participant on the following Play.
   - Companions start on the clock's anchor before any output: the Coyote and
     the vibrator implement `PlayAligned`.
   - Stop reaches every participant concurrently and joins labeled errors.
   - `continue_healthy` counts survivors: an established run continues while
     one participant is healthy. `stop_all` is unchanged.
   - The vibrator declares a buffered lead but no minimum point interval, so
     its command cadence never coarsens the stroker's sampling.
3. **Companion utilities.** The bounded frame copy, the clock and the
   coverage tracking move from the Coyote code to `internal/companion`, with
   finite-value checks and atomic appends. The Coyote keeps its packet pacing,
   strength handshake and renderer.
4. **One shared Intiface session.**
   - **Ownership.** `intifaceSession` owns the socket, handshake, ping,
     reader, request IDs, writer and device registry. The linear owner and the
     vibrator owner bind to it through a small owner interface, and an
     `IntifaceHub` holds the three.
   - **Leases.** The Handy row (when its connection method is Intiface) and
     the vibrator row each hold a lease while connected. The socket closes
     when the last lease ends, or on an address change or shutdown.
     Disconnecting a row releases only its lease, stopping its device first.
   - **Bindings and claims.** Each discovered device gets a new incarnation
     whenever it appears, and owners bind to it. A device can have only one
     role: the linear owner and the vibrator cannot claim the same device.
     Selecting a device stops both the previous and the new device before it
     is ready.
   - **Fault isolation.** A rejected or timed-out response faults only the
     owner that sent it. Socket loss, a missed ping or an uncorrelated
     protocol error faults every owner. A Stop that cannot be confirmed still
     fails the session, so the server stops its devices.
   - **Capacity.** Up to 32 acknowledgement waiters, with 8 more reserved for
     Stop and ping.
   - **Parsing.** `ScalarCmd` features are parsed with their original index in
     the device's full list. The v3 encoding sits behind one seam, because
     spec v4 replaces it with `OutputCmd`.
5. **The vibrator owner.**
   - **Selection.** One device with at least one `Vibrate` actuator; v1
     drives all of them together. Other actuator types are refused.
   - **Wire.** Levels are `k/N` for each motor's `StepCount`. The pacer
     ticks every 100 ms and sends only on a step change. A nonzero level
     waits for the command in flight and for the device's advisory timing
     gap plus a margin. A zero bypasses both limits.
   - **Explicit zero.** Coverage loss, Pause, the end of Test, Exclude,
     removal and shutdown send an explicit zero.
   - **Stop.** Stop invalidates the playback generation, then sends
     `ScalarCmd` 0 and `StopDeviceCmd`, each with a 500 ms bound.
   - **Failures.** A rejected or missing acknowledgement marks the vibrator
     `stale` and stops it, and the failure policy applies.
   - **Watchdog.** It stops a pacer that has not run for 750 ms while
     playing and reports the run `stale`. A pacer that wakes up afterwards
     sends only silence until the engine stops the run. While the vibrator
     plays, the session sends Intiface's ping only while the pacer is alive,
     so the server's own watchdog also catches a stall.
6. **Conversion profiles** (`internal/vibration`):
   - **Follow speed** (default): absolute velocity averaged over 250 ms,
     through `1 - exp(-v / v0)`.
   - **Follow depth**: toward the chosen end, in semantic coordinates.
   - **Steady**: a constant level while covered motion plays.

   Every profile shares the same shaping:
   - asymmetric smoothing: 80 ms attack, 250 ms release
   - a 2 s fade-in on every Play and Resume
   - exact silence below 2% drive
   - a mapping into the range from the minimum to the live intensity
   - quantization with hysteresis

   The timing offset is ±300 ms, and positive values delay the vibrator. The
   renderer reads only covered frame data and never extrapolates. These
   constants are untuned starting points.
7. **Arming and permissions.**
   - **Settings.** A `vibration` block holds the profile, depth end, minimum
     (default 0), ceiling (default 40%) and timing offset.
   - **Live intensity.** It starts at 0 on every connection and selection,
     and only the user raises it, from the connection manager. At 0 the
     vibrator is silent whatever the motion does. The effective ceiling is
     the highest motor step at or below the ceiling.
   - **Raising.** Raising needs the controlling host and the current Stop
     sequence. A raise or Test prepared before a Stop, selection or session
     change is refused.
   - **Lowering.** Lowering is open to every client with control, needs no
     Stop sequence and never starts output.
   - **Test.** One second at the lower of the intensity and 25%, only while
     motion is idle. Test levels carry a generation like a run's, so a Stop
     discards a level that was composed but not yet written.
   - **Routes.** `POST /api/transport/intiface/vibrator/select` (host) and
     `/intensity` and `/test` (control). Connect, Disconnect and Scan take a
     `device` lease name.
8. **Emergency Stop.**
   - On a live session, Emergency Stop sends `StopAllDevices` beside the
     engine Stop, whatever the engine, the selections and participation are.
     That stops every device the server has, which is intended.
   - Before the write, the session closes admission and invalidates every
     owner. Admission stays closed until the Stop completes.
   - Emergency Stop also zeroes the live intensity.
   - An unconfirmed `StopAllDevices` fails the session and reports the
     failure.
9. **Simulation.** `-simulate-motion` builds start an in-process Buttplug
   server (`internal/buttplugsim`) with one simulated stroker and one
   two-motor simulated vibrator. Its 1 s ping exercises the watchdog path.

## Not in this decision

- Prompt changes for a vibrator or Coyote without a stroker (planned
  separately).
- `Rotate`, `Oscillate` and other actuator types; several vibrators; battery
  reads.
- A trace record per `ScalarCmd`. The status reports commanded, acknowledged,
  rejected and timed-out counts and the last error, but motion traces do not
  yet include scalar writes. Physical latency calibration will need them.
- Requiring a nonzero Intiface ping timeout. The status reports
  `ping_watchdog: false` and the vibrator row says what that means.
- No new dependency, and no change to the native Bluetooth worker
  ([ADR 0046](0046-shared-native-bluetooth-worker.md)).

## Validation

Automated coverage, all against simulated servers:

- **Shared session.** Fake Buttplug servers carry linear and scalar traffic
  on one socket. The tests cover:
  - per-owner faults, socket loss for both owners, and late responses for an
    old generation
  - a reused index becoming a new incarnation, and one-role claims
  - the emergency barrier, with no write after `StopAllDevices`
  - reserved Stop capacity, and the pacer-coupled ping
- **Vibrator owner.**
  - `k/N` quantization with original indices, and spacing by the device's
    timing gap
  - explicit zeros, the fade-in, and no `ScalarCmd` after Stop, including a
    test level composed just before it
  - the stall watchdog, silence after it, and stale handling of a rejected
    acknowledgement
  - control-generation fencing for raises and Test
- **Linked sessions.** Two- and three-participant sessions under both
  policies; the clock handoff; startup rollback; aligned companion starts.
- **Settings.** Migration of omitted, empty and conflicting fields, and
  legacy writes that keep the vibrator.
- **Renderer goldens.** Each profile, reversals, the threshold, hysteresis
  and the downward ceiling.
- **HTTP and UI.**
  - HTTP: leases, permissions, and the route admission matrix
  - UI: the connection manager's vibrator row, the vibrator panel, the
    Handy's lease state and the Settings profile

Physical acceptance, owner-run when hardware is available: latency and the
timing offset; Stop-to-silence times; Intiface stopping the toy on core crash,
server crash, missed ping, a half-open socket and radio loss; radio contention
beside the native Bluetooth worker; the vibrator with each Handy connection
method; and the feel of each profile alone and with the other devices. Record
the device, firmware and Intiface Central versions, and keep traces.
