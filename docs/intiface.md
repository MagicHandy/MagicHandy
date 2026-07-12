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
- Consecutive neutral timed points become one scheduled `LinearCmd` each. The
  owner does not interpolate, resample, or reshape engine output.
- Stroke-window projection and reverse direction are applied once in the
  Intiface owner. Handy owners continue to apply their equivalent at the Handy
  encoding/device boundary.
- **Emergency Stop**, owner changes, disconnect, server shutdown, pacer
  underrun, and rejected linear commands clear queued work and issue
  `StopDeviceCmd` where the session is still reachable.
- Buttplug ping keepalive remains enabled. A failed ping marks the connection
  stale; MagicHandy does not silently fall back to another owner.

Connection state, selected actuator, queue depth, playback state, command
latency, and redacted errors are available in **Settings > Diagnostics** and
`GET /api/state` under `intiface_transport`.

## Validation Status

Automated tests use a fake Buttplug v3 server for protocol, precision,
backpressure, Stop/Close ordering, lifecycle, and HTTP/UI integration. No
Intiface Central process or port `12345` listener was available during the
implementation session, so these live checks remain required before Intiface is
recommended as equivalent to the Handy owners:

- run the same capped pattern on one Handy through Cloud REST and Intiface and
  record latency, trace, Stop behavior, and subjective matched feel
- run a capped pattern and Stop on one non-Handy linear device if available

Keep automated or unattended real-device runs at or below 40% speed.
