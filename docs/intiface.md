# Intiface / Buttplug Setup

MagicHandy can use a user-run Intiface Central server as a dispatch owner. The
Go core speaks Buttplug message spec v3 directly over WebSocket; Intiface
Central continues to own Bluetooth/radio access and device drivers.

## Prerequisites

1. Install and start [Intiface Central](https://intiface.com/central/).
2. Configure its WebSocket server. MagicHandy's default is
   `ws://127.0.0.1:12345`.
3. Make the device available to Intiface Central. Do not connect the same
   Bluetooth device to the browser Bluetooth owner at the same time.

Intiface Central is not bundled or started by MagicHandy. A remote `wss://`
endpoint is accepted, but the default loopback endpoint is the smallest and
safest setup.

## Connect

1. Open **Settings > Device** and choose `intiface` as **Dispatch owner**.
2. Set the Intiface Central server address and save settings.
3. Select **Connect**, then **Scan devices** if the target is not already in
   the returned device list.
4. Choose one linear actuator and select **Use actuator**.

The selected device and actuator are session state, not durable settings.
Buttplug device indices can change between discovery sessions, so persisting an
index in SQLite would create a stale-device safety risk. The server address and
dispatch owner are persisted normally.

## Behavior And Safety

- Only `LinearCmd` actuators are supported in this phase. Scalar vibration,
  rotation, and simultaneous multi-device output are not mapped.
- Consecutive neutral timed points become one absolute-deadline `LinearCmd`
  each. The first point is established through a generation-guarded startup
  anchor of at least 250 ms (raised to the device timing gap when necessary)
  before the stream clock begins. Stop invalidates that generation before its
  wire barrier. The owner does not add interpolation or a private motion
  generator.
- Reverse direction is snapshotted as each point enters the owner, matching the
  Handy encoding paths. The min/max stroke envelope remains live and immediate,
  matching the documented quick-settings contract.
- Linear writes do not wait for the preceding acknowledgement. Up to eight
  responses are correlated asynchronously with a 650 ms deadline; a missing or
  rejected response invalidates the stream and forces Stop without retrying an
  ambiguously delivered movement.
- Expired points are discarded rather than burst onto the device. A live late
  segment may use only its remaining scheduled duration and never more than 25%
  compression; otherwise playback reports `starved` and stops.
- The unsent queue is capped at 64 segments. A device's
  `DeviceMessageTimingGap` raises the shared engine sampling interval with a
  scheduler margin. `StepCount` is reported as physical resolution but positions
  remain floats; MagicHandy does not apply a second quantizer.
- **Emergency Stop**, owner changes, disconnect, server shutdown, pacer
  underrun, and rejected linear commands clear queued work and issue
  `StopDeviceCmd` where the session is still reachable.
- Buttplug ping keepalive remains enabled. A failed ping marks the connection
  stale; MagicHandy does not silently fall back to another owner.

Connection state, selected actuator/resolution, queue depth and coverage,
playback state, pending ACKs, sent/acknowledged/rejected/timeout counts, send
lateness, ACK latency, coalesced segments, recent wire dispatches, and redacted
errors are available in **Settings > Diagnostics** and `GET /api/state` under
`intiface_transport`. Trace exports use `motion_trace.v3` and include the most
recent 32 paced wire records plus explicit sent/dropped counts.

## Validation Status

Automated tests use a fake Buttplug v3 server for protocol, precision,
backpressure, Stop/Close ordering, lifecycle, and HTTP/UI integration.

Live evidence from 2026-07-12:

- Intiface Central on `127.0.0.1:12345` discovered `The Handy (FW4+)`, device
  index 0, with one `Position` actuator (100 steps).
- An isolated Phase 14B build enforced a 10–20% speed range. The shared stroke
  pattern ran at 20% with a 20–80% window, paused, resumed with phase preserved,
  then accepted an immediate 30–70% reverse-direction refresh before Stop.
- The historical workflow produced 19 `motion_trace.v2` rows: neutral `points_add` and
  `points_play` commands, successful Pause/Resume/refresh/Stop results, no
  `starved` state, and queue depth zero after Stop. Local WebSocket command
  queue-admission latency rounded below the diagnostics' one-millisecond
  resolution. That old metric did not measure paced `LinearCmd` writes or ACKs.
- A final one-second 20% run proved unconditional Stop: active Stop commands
  `intiface-000005` and repeated idle Stop `intiface-000006` were distinct and
  successful. Disconnect then recorded a third successful close-time Stop.
- Trace exports and process logs remained temporary runtime artifacts and were
  not committed. No connection key or API credential was used by Intiface.
- A same-Handy Cloud REST comparison then repeated the 20% pattern,
  Pause/Resume, 30–70% reverse refresh, active Stop, and repeated-idle Stop.
  Its 23 trace rows contained 19 successful transport results with no
  starvation. Pause, active Stop, and idle Stop completed in 317, 311, and
  310 ms; the Intiface run's local command latency rounded below 1 ms.
- The Cloud credential moved only in memory from an existing ignored local
  profile into the temporary datastore. It was not printed, logged, exported,
  or added to the repository.

Remaining acceptance checks:

- record the operator's subjective matched-feel judgment for the completed
  Cloud REST and Intiface runs
- run a capped pattern and Stop on one non-Handy linear device if available
- repeat the matched Handy run on this deadline-driven pacer and retain the
  `motion_trace.v3` send-lateness/ACK distributions

Keep automated or unattended real-device runs at or below 40% speed.
