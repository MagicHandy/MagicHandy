# Native Handy and linked-device hardware review — 2026-10-15

This extends the [COYOTE reconnect review](native-ble-reconnect-review.md).
The user authorized repeated Handy connections, playback, simultaneous devices,
Stop and linked-failure testing. They confirmed that the Handy was clear and
could move freely, and that no COYOTE output leads were connected. Tests used
the Windows native workers and isolated app on port 50235, an original Handy
with firmware 4.3, and a COYOTE 3.0. No Cloud or browser Bluetooth dispatch was
used. Device identifiers and credentials are excluded from this report.

## Corrections found during testing

The Handy HTTP route still overrode the shared twelve-second connection budget
with six seconds. The first immediate reconnect series failed discovery on
cycle 2 after about 6.4 seconds. Handy now uses `ble.DefaultConnectTimeout`, with
a 45-second outer request bound covering discovery, setup and optional clock
work. Successful operations return immediately. Stop and native write deadlines
are unchanged.

The motion API previously omitted an engine as soon as it stopped, including its
linked-session failure marker. It now retains a stopped `failed` session while
continuing to report actual transport availability. The connection manager can
therefore show why the session stopped. An HTTP regression verifies both the
retained marker and `available=false` while the Handy link is down.

COYOTE loss also exposed a confirmation bug: a retired session with no surviving
link returned successful Stop, and the global zero path treated that loss as a
harmless disconnected state. Unexpected loss now returns an explicit
unconfirmed-Stop error from the owner, session and emergency zero paths. A
never-connected owner remains a no-op, and an old session still cannot command a
replacement connection. Transport and HTTP/console regressions verify that the
healthy Handy receives Stop even when COYOTE cannot acknowledge it.

## Normal reconnect and playback

Ten consecutive immediate Handy disconnect/reconnect cycles passed after the
deadline correction, with no retry or artificial gap. Each established a
509-byte write budget and clock synchronization, completed Stop, and retired its
worker on disconnect.

| Measurement | Result |
| --- | --- |
| Connection | 2,492–6,111 ms; median 4,911.5 ms |
| Stop HTTP completion during the cycles | 115–178 ms |
| Disconnect HTTP completion | 360–422 ms |

The first harness assertion expected the label `stopped`; native Stop correctly
uses `stop_requested` because a completed write is not physical braking feedback.
That assertion was corrected and retained separately from the real six-second
connection failure.

Handy-only playback used the shared engine's `flow-full-sweeps` at 30%, then
`flow-pace-wave` at 40%. Firmware reports showed `playing`, and the engine stayed
healthy. A separate observer client with no controller epoch/generation issued
global Stop. The initial Stop request completed in 102 ms. A repeat with two
five-second phases completed Stop in 250 ms; the user observed that repeat and
confirmed smooth motion and correct stopping.

Simultaneous playback used the same two patterns, with COYOTE strengths reported
as A=5/B=5 and existing ceilings 30/30. Both participants stayed playing through
the target change. COYOTE reported zero write failures and zero starvation.
Observer-client Stop completed in 116 ms; the subsequent status check confirmed
engine stopped, Handy Stop dispatched, and actual COYOTE A=0/B=0 by 123 ms.
The final-build simultaneous repeat also passed, with no write errors or
starvation: Stop completed in 143 ms and the zero/status check by 205 ms.

## Failure handling and recovery

Fault injection terminated only the identified worker belonging to the selected
device in this review app. The harness verified its executable path and parent
PID before termination. It did not disable the Bluetooth adapter or disturb
unrelated apps. These tests exercise a worker crash on real connections; they
do not substitute for moving a device out of radio range or measuring electrical
output decay.

The four-case policy matrix, second-failure checks and recovery results are
recorded with per-phase status, traces and diagnostic reports. Policy timing
includes process identity verification, termination and status polling; it is
not a measurement of physical braking or electrical decay.

| Policy | Failed worker | Verified behavior | Policy observed | Healthy Stop/zero observed |
| --- | --- | --- | --- | --- |
| Stop all | Handy | Engine stopped; COYOTE reported zero | 279 ms | 559 ms |
| Stop all | COYOTE | Engine stopped; Handy Stop dispatched; COYOTE Stop unconfirmed | 353 ms | 403 ms |
| Continue healthy | Handy | COYOTE kept playing; Handy retired | 283 ms | Observer Stop checked separately |
| Continue healthy | COYOTE | Handy kept playing; COYOTE retired | 270 ms | Observer Stop checked separately |

The second worker failure stopped the continuing session in 405 ms for the
Handy-first case and 297 ms for COYOTE-first. Recovery after releasing both
connections permitted a fresh linked run in both directions. The COYOTE-loss
cases and simultaneous baseline were repeated after the Stop-confirmation fix.

Under `continue_healthy`, losing Handy left COYOTE playing; losing COYOTE left
Handy playing. The backend published the retired participant and degraded
state. Losing the remaining worker then stopped the engine and published a
failed session. Global Stop remained available from the observer client. When
a worker was gone, the app returned an unconfirmed-Stop error rather than
claiming that an unreachable device acknowledged Stop. Healthy outputs were
still stopped, and COYOTE reported zero whenever its connection remained usable.

**Crash recovery remains limited on this Windows host.** Reconnecting the killed
device while the companion worker remained connected repeatedly failed. A
bounded cache-refresh experiment also reached native discovery failure and did
not solve this; it was reverted. The shipped code still refreshes only the
selected advertisement and performs full native readiness/GATT validation.
Microsoft documents that address lookup uses the system cache and that native
connection work can outlast a caller; those facts alone do not establish the
cause of this host's crash-recovery failure. See
[Windows address lookup and connection behavior](https://learn.microsoft.com/en-us/uwp/api/windows.devices.bluetooth.bluetoothledevice.frombluetoothaddressasync).

After both connections were released, both devices could reconnect and start a
fresh linked run. The first immediate recovery attempt sometimes exhausted its
deadline; a second explicit Connect succeeded. These attempts are recorded and
are not counted among the ten successful normal reconnect cycles. No automatic
retry, radio reset or silent interruption of the healthy participant was added.
For now, recovery after a worker crash can require Stop, disconnecting both
connections, and explicitly reconnecting. Seamless recovery with a live companion
is an open hardware acceptance item. The reconnect-without-hot-join invariant
retains software coverage; its physical mid-run acceptance is blocked by this
reconnect limitation.

The harness waits separately for engine Stop and healthy-device Stop completion;
engine state intentionally changes first. COYOTE error counters are cumulative
across connections, so playback verification compares against the new
connection's baseline instead of treating an injected prior write failure as a
new playback failure. Earlier assertions and all failed recovery attempts remain
in the local evidence.

## Evidence and scope

Ignored artifacts under `.scratch/native-handy-hardware/` contain
`handy-reconnect-cycles.*`, `linked-handy*`, `linked-both*`, and
`linked-fault-*` status samples, traces, timing logs and diagnostic ZIPs without
chat. No runtime evidence is committed. The review app retains a working local
Gemma provider, verified by an actual generation, and is left stopped with both
devices disconnected after testing.

Full `go test ./...`, `go test -race ./...`, vet and golangci-lint pass, including
architecture and goroutine-leak gates. CGO-disabled Windows and Linux core/worker
builds pass. Added regressions cover lost-link Stop/zero honesty, replacement
connection fencing, observer/console Stop and the retained failed-session API
snapshot. No frontend source or embedded asset changed. Browser automation
still fails before DOM inspection on this host; the failure-marker change was
verified through the actual API, and the existing device-settings review tab is
retained.

The shared sampler, patterns, point quantization, stroke envelope, HSP buffering,
COYOTE waveform mapping, strength ceilings, model prompts and model routing did
not change. No dependency or frontend asset changed. The Windows stripped
CGO-disabled binaries measure 22,928,384 bytes for the core (+1,024 bytes) and
3,093,504 bytes for the unchanged worker. There is no new idle process or
polling loop.

Remaining physical acceptance includes loaded electrical waveform/decay and
switch-limit enforcement, physical RF loss, core crash/hang, longer sessions,
and quantitative alignment between device outputs. Native write completion and
reported zero are not measurements of motor braking or delivered current.
