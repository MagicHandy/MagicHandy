# Intiface / Buttplug Setup

MagicHandy can use a user-run Intiface Central server as a dispatch owner for
The Handy or another linear actuator, and as the connection for a vibrator
(see [Vibrator](#vibrator)). The Go core speaks Buttplug message spec v3
directly over WebSocket; Intiface Central continues to own Bluetooth/radio
access and device drivers.

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

1. Open **Settings > Devices**, then The Handy's page (select **Add device**
   first if it is not listed). Under Connection, choose **Intiface Central**
   and select **Use this method**.
2. Check the server address under **Settings > Devices > Connections** (by
   default `ws://127.0.0.1:12345`). A new address saves when you press Enter or
   leave the field.
3. Select **Connect**, then **Scan for devices** if the target is not already
   in the device list.
4. Choose one linear actuator and select **Use this device**.

The selected device and actuator are session state, not durable settings.
Buttplug device indices can change between discovery sessions, so persisting an
index in SQLite would create a stale-device safety risk. The server address and
dispatch owner are persisted normally.

## Behavior And Safety

- The linear owner drives one `LinearCmd` actuator. Vibration has its own
  owner (see [Vibrator](#vibrator)); rotation, oscillation and other actuator
  types are not mapped.
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
  scheduler margin. `StepCount` also reports physical endpoint resolution to
  the shared sampler. The sampler scales that resolution through the active
  stroke window before bounded reduction, avoiding commands that collapse onto
  the same physical step. Positions remain floats until `LinearCmd`; the owner
  does not apply a deadband or reshape the curve.
- **Emergency Stop** closes write admission, invalidates every owner and
  sends `StopAllDevices` on a live session, whatever the engine and the
  selections are. This requests a stop for every device the server has.
- Owner changes, disconnect, server shutdown, pacer underrun and rejected
  linear commands clear queued work and issue `StopDeviceCmd` where the
  session is still reachable. Selecting an actuator requests Stop for both
  the previous and the new device first. These actions establish local
  cancellation and a server request, not a physical stop: the October 15 audit
  below found a missing hardware Stop for the Handy driver in Central 3.2.2.
- Buttplug ping keepalive remains enabled. A failed ping marks the connection
  stale; MagicHandy does not silently fall back to another owner. While the
  vibrator plays, a stalled vibrator pacer stops the pings, so Intiface's own
  watchdog requests Stop too. That fallback still depends on the server's
  device-specific Stop implementation.

Connection state, selected actuator/resolution, queue depth and coverage,
playback state, pending ACKs, sent/acknowledged/rejected/timeout counts, send
lateness, ACK latency, coalesced segments, recent wire dispatches, and redacted
errors are available in **Settings > Diagnostics** and `GET /api/state` under
`intiface_transport`. Trace exports use `motion_trace.v3` and include the most
recent 32 paced wire records plus explicit sent/dropped counts.

## Vibrator

A Buttplug device with `Vibrate` actuators can play with the stroker, the
Coyote, both, or alone ([ADR 0047](decisions/0047-intiface-vibrator-output.md)).
It has not been tested on hardware yet.

1. Open **Settings > Devices**, select **Add device**, then **Vibrator**. Its
   page opens.
2. Select **Connect**, then **Scan for devices** if the device is not listed.
3. Choose the device and select **Use this vibrator**. Every vibration motor on
   it runs together.
4. In the device panel (the device chip at the top right), turn on **In
   playback**. **Max** (default 40%) is the strongest it goes; with **Model
   sets level** on, the chat model chooses the level beneath it.

Behavior and safety:

- **One connection.** The Handy (with the Intiface method) and the vibrator
  share one Intiface connection; each device holds its own lease. The
  connection closes when the last device disconnects, and **Disconnect** under
  **Settings > Devices > Connections** closes it for both. One device cannot be
  both the linear actuator and the vibrator. Its address is under the same
  Connections section.
- **Profiles.** On the vibrator's page in **Settings > Devices**, the level
  follows stroke speed (default; it spans your saved Handy speed range), stroke
  depth (strongest at the base or tip), or stays steady while motion plays.
  **Max**, **Model control** and **Minimum while moving** are saved there, and
  a ±300 ms timing offset under **Settings > Devices > Playback**. Following
  vibration maps between the minimum and the level: Max, or the model's share
  of it (ADR 0061). Simple models in Layered and Intermediate models set one
  level; Advanced models can also play their own patterns in % of Max.
- **Pacing.** Levels are whole steps of each motor's `StepCount`, sent through
  `ScalarCmd` on a 100 ms tick, only when the step changes and never faster
  than the device's advisory timing gap. A 2 s fade-in starts every Play and
  Resume.
- **Stopping.** Pause, Stop, loss of coverage, the end of Test, Exclude and
  removal send an explicit zero, because a vibrator keeps its last level.
  Stop also sends `StopDeviceCmd`. A rejected or missing response stops the
  vibrator and applies the linked failure policy. Emergency Stop also pauses
  the vibrator: your next manual chat request, **Resume now** in the device
  panel or choosing the device again resumes it; questions and background
  planning cannot. To keep it off, turn it out of playback. A later Stop still
  cancels an in-flight request. See
  [ADR 0061](decisions/0061-model-controlled-vibration-level.md).
- **Test** runs one second at the lower of Max and 25%, only while motion is
  paused.
- **Keepalive.** When Intiface Central reports no ping timeout, the vibrator
  row says that only a closed connection stops the vibrator if MagicHandy
  stops responding.

The vibrator's status is under `intiface_transport.vibrator` in
`GET /api/state`: the device and motors, Max and the effective Max, whether
model control is in effect and the model's level, whether Stop paused it, the
current commanded level, the playback state and the command counts.

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
- A final one-second 20% run recorded unconditional Stop requests: active commands
  `intiface-000005` and repeated idle Stop `intiface-000006` were distinct and
  acknowledged. Disconnect then recorded a third close-time acknowledgment.
  Those receipts did not independently establish a physical motor stop.
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

Keep automated or unattended real-device runs at or below 40% speed.

The [October 15 follow-up](release-motion-review-2026-10-15.md) includes matched
full-stroke 20%/35% runs on Original Handy firmware 4.3.0 and Central 3.2.2+46.
The operator found Cloud smoother, with clearer speed separation; Intiface
repeatedly hesitated and caught up. Resetting the firmware stroke window through
Cloud did not remove this behavior. Local writes showed no late/coalesced or
failed commands, but those measurements end at the server, before Bluetooth.

The actual server advertises 100 ms for this device, so MagicHandy uses 110 ms
including margin. The expanded offline matrix includes that floor alongside
Cloud and generic 50 ms Intiface. A speculative denser sampler produced only
modest gains on the failing run and was parked; no runtime speed multiplier,
lower timing floor, or change to the user's Intiface configuration was shipped.

The [subsequent physical sampling comparison](intiface-smoothness-review-2026-10-15.md)
revisited that denser fit; the operator found it worse. Wider spacing improved
full sweeps and pace waves, and a 180 ms **preference** with the original hard
floor was smoother and natural in the repeated Creative v2 trial. That
preference now applies to generated curves for `The Handy (FW4+)`. Fast turns
still use the advertised hard limit; buffered output, imported media and other
linear devices retain their fit. The report records the coarser intermediate
curves and the limits of the 35% physical evidence.

### Known Handy Stop limitation in Central 3.2.2+46

The release's buttplug_server 12.0.2 generates no Stop hardware command for its
position-only Handy feature. An empty Stop returns `Ok` without going through
the device task, so previously accepted motion can still be waiting for a GATT
write. App queue depth zero and a 2–3 ms Stop ACK do not prove that the device
stopped or that all downstream writes were canceled. Buttplug v4 reaches the
same server path and does not remedy the omission.

Do not rely on that acknowledgment as a verified emergency motor stop. Native
Handy Bluetooth and Cloud use the separate buffered HSP path with actual Stop
commands. Their physical stopping delay must still be distinguished from a
command receipt. The Intiface release acceptance check remains open until
downstream Stop ordering and physical behavior are established. A guessed
position target, automatic transport switch, or connection close is not a
substitute for a verified hardware Stop.

Remaining checks are downstream batch/GATT timing correlated with carriage
motion, restricted stroke windows, and a non-Handy linear device if available.

## September 2026 connection review

Session I/O and handshake live in `intiface_session.go`, separate from motion
pacing. Ping begins immediately after ServerInfo, including during a slow device
list request. The reader applies discovery and scan replies before delivering
them to callers, preserving later DeviceAdded, DeviceRemoved and ScanningFinished
events in wire order. Uncorrelated protocol errors retire the session, and
watchdog durations are checked before converting to Go durations.

Buttplug fixed a Handy 2 HDSP direction-timing problem in upstream commit
[`5a60f54`](https://github.com/buttplugio/buttplug/commit/5a60f5492800dbe9bcb5e9443c4db0a04b7239e6).
That device driver belongs to the installed Intiface engine. MagicHandy keeps
using the standard v3 LinearCmd contract and does not bypass it with raw BLE
commands. The October 15 follow-up checked the official Intiface Central
3.2.2+46 release's [Cargo lockfile](https://github.com/intiface/intiface-central/blob/5413bf0/rust/Cargo.lock):
it pins intiface-engine 5.0.5 and buttplug_server 12.0.2. The published server
crate was downloaded and its SHA-256 matched the lockfile. Its Handy driver
already sets `stop_on_target:false`, passes milliseconds unchanged, and maps
integer position to 0–1 once. Therefore that release includes the upstream
stop-on-target correction; an assumed missing fix is not a reason to alter
MagicHandy's pacing. This establishes the official release source, not measured
firmware dynamics or Bluetooth arrival timing.
See [the full review](bluetooth-intiface-review-2026-09-20.md).
