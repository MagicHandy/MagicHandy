# DG-LAB Coyote 3.0

MagicHandy can drive a DG-LAB Coyote 3.0 electrical stimulation box alone or
alongside The Handy. The decision record is
[ADR 0044](decisions/0044-coyote-estim-output.md).

## Safety first

- Place electrodes below the waist only, never across the chest or near the
  heart.
- Do not use a Coyote with a pacemaker or other implant, a heart condition,
  epilepsy, or during pregnancy.
- MagicHandy never raises channel strength on its own. Only your steps in the
  app or the Coyote's own switches do. The model, modes, scripts and videos
  shape the pulses beneath the strength you set.
- Stop and `Esc` set both channels to zero. Pause only silences the output.

The connection manager shows these notes once; connecting is refused until the
host accepts them. Turning the acceptance off in Settings shows them again.

## Setup

1. Open the connection manager and use the small **+** in its title row to add
   **Coyote**. This saves a connection without connecting or joining playback.
   Turn on **Use in playback** when ready. Excluding a device keeps its
   connection; Disconnect keeps its saved row; Remove closes it and removes the
   row. Removing every row leaves chat available with motion idle.
2. Turn the Coyote on and disconnect the phone app. If the logo eyes are already
   flashing, leave the controls alone. Select **Connect Coyote**.
   If it cannot be found, select **Find nearby devices**. Holding both switches
   can turn this model off.
   The list ranks signal strength and distinguishes **Remembered COYOTE**,
   **Likely COYOTE 3.0** (advertised name) and **Possible COYOTE** (unnamed private
   advertisement). An unnamed match is only a candidate; select it explicitly.
   MagicHandy verifies the required service and characteristics before commands.
3. MagicHandy writes your ceilings to the device, sets both channels to zero
   and remembers the device. Later connections go straight to the remembered
   address, which recent firmware accepts without pairing mode. A failed
   remembered connection never switches devices automatically; use the picker
   to choose a different device. The official app's binding is left unchanged.
4. Raise each channel with its **+** button. Strength changes alone never start
   playback. With all playback paused, select that channel's **Test** button for a
   bounded 1.6 s reference pulse at its current strength. The reference uses the
   profile's widest pulse and its low and high rates; it is not a guarantee of
   the strongest sensation every pattern or electrode arrangement can produce.

The Coyote must not be connected to another app at the same time. The Bluetooth
helper (`bluetooth-worker.exe`) ships beside `magichandy.exe`; Linux builds use
BlueZ. macOS is not supported.

## Settings

Settings › Device › Coyote:

- **Channel A and B ceilings** (0–200, default 30): written to the device on
  every connection. The device enforces them against every source, including
  its own switches, and keeps them after you disconnect, so they also limit
  other apps.
- **Follows**:
  - *Stroke and speed* (default): the stroke position moves the sensation
    between channel A (base) and channel B (tip), and faster movement widens
    the pulses.
  - *Speed only*: faster movement widens the pulses on both channels equally.
  - *Position only*: a steady pulse width moves between the channels.
- **Pulse rate band** (default 40–100 Hz): the rate follows the pace within the
  band, in 5 Hz steps held for at least 500 ms.
- **Channel balance**, **Swap channels** and **Timing offset** (−300 to 300 ms;
  positive delays the Coyote relative to motion and video).
- **Coyote balance parameters** (advanced): DG-LAB's frequency and intensity
  balance, written with the ceilings. Defaults are 160 and 0.
- **Forget the remembered Coyote** on the next save.

## Behavior

| Event | Output | Strength |
| --- | --- | --- |
| Connect | silent | set to 0, ceilings written |
| Play or Resume | fades in over 2 s | unchanged |
| Strength step while idle | remains silent | explicit user change |
| Test channel | 1.6 s reference on the selected channel | unchanged |
| Pause or a transport Stop | silence command delivered, or failure reported | unchanged |
| Global Stop, `Esc`, console Stop | silenced, queued packets dropped; failure is reported | set to 0 by command; device reports are distinguished |
| Disconnect | silenced | set to 0 first |
| Link lost or worker ended | no further frames are sent; physical decay time needs hardware validation | unknown until the next connect/report |
| Motion frame runs out | silent frames, then `starved` | unchanged |

With both devices included, the Handy remains authoritative for timing and
both participants contribute to playback health. **Settings > Device > On
device failure** defaults to **Stop all devices**. An unexpected disconnection
or playback failure stops the linked run and attempts to zero Coyote strength.
**Continue on the healthy device** instead retires the failed participant from
an established run and lets the other continue. Both are required at startup.
The connection manager shows when a run continues without a device. Reconnect,
then Stop and start a fresh run to rejoin; old work cannot use a new connection.
Emergency Stop always stops every device. Changing participation or the failure
policy stops the run. See [ADR 0045](decisions/0045-device-inventory-and-linked-sessions.md).

Raising strength needs the controlling host. Lowering and **Set both to zero**
are available to every client with control, and the global Stop zeroes for
everyone. Test requires the controlling host and current Stop sequence, idle
output, an applied ceiling and a known strength baseline without a pending
change. A zero command accepted by the Bluetooth write is labeled separately
from an actual device strength report. Increases and tests wait for the zero
handshake window to end; a timeout never creates a strength value. Native COYOTE
uses Write Without Response, whose completion means queued rather than delivered.
Only a device strength report establishes its baseline; absent reports leave
strength unknown and block increases and playback.

A failed ceiling update zeroes and retires the connection. The saved setting
is retried on the next explicit connection before playback can be enabled.
Failed zero/silence writes are reported as failed Stop confirmation, including
to every overlapping Stop request, and the uncertain connection is retired.

## Diagnostics

`GET /api/state` includes `coyote_transport`. It holds the selected output,
whether the helper is available, and a status object with these fields:

- connection and battery;
- strengths and their source (`reported`, `zero_command`, or `unknown`),
  pending changes, the device's last applied ceilings, and `limits_applied`;
- the playback state;
- the last commanded pulse widths, which are not measurements;
- packet, write-failure and starvation counters, and the worst pacer lateness.

Non-host clients do not see the device address or error details.

## Protocol notes

Source: DG-LAB's
[dglab-bluetooth-protocol](https://github.com/dungeonlab-open/dglab-bluetooth-protocol)
(`coyote/v3/README.md`, `coyote/README.md`).

- **Device identity.** Normal advertisements can name `47L121000`; an already
  bound unit can instead advertise an unnamed private identifier. The official
  4.2.6 APK resolves that identifier against its saved bindings. No fixed
  manufacturer/company ID proves identity. Service `0x180C` has
  write characteristic `0x150A` and notify characteristic `0x150B`. The battery
  is service `0x180A`, characteristic `0x1500`.
- **`B0` packets** are 20 bytes, written every 100 ms.
  - They carry a 4-bit sequence and two 2-bit strength modes (keep, add,
    subtract, set).
  - Next come the A and B strength values (0–200), then four period bytes and
    four width bytes per channel.
  - Periods of 10–1000 ms are compressed to 10–240 with DG-LAB's integer
    formula.
  - An out-of-range waveform value makes the device drop that channel for the
    packet.
- **`B1` reports** carry the sequence and the actual strengths. One arrives for
  every sequenced change and for every change made on the device, with
  sequence 0. MagicHandy keeps one change in flight until its report arrives.
  An unchanged zero may produce no report. Only a GATT-acknowledged zero write
  may establish a labeled commanded baseline; a native Write Without Response
  cannot. Neither can overwrite a newer device
  report. The 1 s timer only ends the wait; it never confirms a value. A relative
  change without B1 becomes unknown after 1.5 s and discards queued increases.
- **`BF`** (7 bytes) sets both ceilings and both balance pairs. It has no
  reply, and DG-LAB requires rewriting it after every reconnection.

The core composes every byte (`internal/estim`); the worker only writes them
(`internal/ble/bleworker`, `cmd/bluetooth-worker`). The core-to-worker
protocol is NDJSON version 4 with an explicit GATT profile, negotiated write
limit, unacknowledged-write capability, and `hello`, `discover`, `connect`, `write`,
`battery`, `last_will`, `disconnect` and `shutdown`. The worker sends `notify`,
`battery`, `disconnected` and `error` back. Every write waits for the worker's
native-call completion, including GATT Write Without Response. This bounds
outstanding waveform work to one packet instead of buffering stale frames in
the pipe. A timed-out write retires and terminates the worker before returning.

The shared worker also serves native Handy links in separate processes; COYOTE
packet composition and strength control are unchanged. See
[ADR 0046](decisions/0046-shared-native-bluetooth-worker.md). Old version-2/3 helpers
are rejected rather than silently reused for the new protocol.

## Validation status

Automated tests cover the following on a simulated Coyote:

- DG-LAB's example packets;
- the strength handshake and the renderer;
- the worker in-process and as a real process;
- the owner and the Handy + Coyote fan-out;
- the HTTP routes and the UI panel.
- overlapping Stop/zero writes, failed ceiling retry/reconnect, missing or
  reordered strength reports, read-only lowering without output, explicit test
  permissions and Stop fencing, and a deliberately hung worker process.

On the user's Coyote 3.0, the unnamed device's GAP name and required service and
characteristics were verified. After adding Windows advertisement refresh and
connection-readiness checks, real connections read 100% battery and received
device reports of A=0/B=0. Five global Stops while already at zero received fresh
zero reports in 152–232 ms, including polling delay. This does not measure the
physical interruption of active electrical output. An immediate reconnect still
failed at GATT discovery. See the
[hardware report](native-handy-hardware-review-2026-10-14.md).

Still open:

- pairing and remembered reconnection;
- the ceilings against the device's switches;
- time from Stop to silence and to zero;
- silence on link loss;
- the feel of each profile;
- the timing offset with both devices.
