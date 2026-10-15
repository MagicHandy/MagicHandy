# ADR 0046: Shared native Bluetooth infrastructure with per-device workers

Date: 2026-10-14
Status: Accepted for software implementation; original Handy connection/Stop
verified, Windows rapid reconnect remains a hardware-validation blocker.

See the [authorized hardware findings](../native-handy-hardware-review-2026-10-14.md).

## Context

The COYOTE integration introduced a native worker with its discovery, packet
limit and process client under the electrical-output domain. Handy already uses
firmware-4 protobuf HSP in the browser. Adding another device-specific executable
would duplicate process supervision and platform discovery; putting every device
in one worker would couple their failure handling.

## Decision

Ship one `bluetooth-worker` executable and start one instance per connection.
`internal/ble` defines a byte-link interface and allowlisted GATT profiles;
`workerproto`, `workerclient` and `bleworker` own IPC, bounded writes, lifetime and
discovery. Only `cmd/bluetooth-worker` imports the radio library. There is no new
dependency and no CGo in the shipping core. The helper and core require IPC
version 4 and agree on the profile, maximum write size and whether native writes
lack a GATT acknowledgment before using the link. Version 3 helpers are rejected
because they cannot distinguish queued writes from acknowledged delivery.

Device semantics remain in core drivers: `internal/estim` for COYOTE pulse
packets and user-owned strength, `internal/handyble` for Handy RPC. The existing
browser HSP transport is generalized behind a small command-channel interface.
Both native and browser owners use `HandyBluetoothTransport` for the same
semantic point validation, one-percent position quantization, reversal, numeric
stream IDs, absolute point tails and motion admission. Browser API type aliases
preserve existing callers. The independent browser emergency Stop remains.

Profiles describe GATT services, characteristics, discovery names and framing
limits. They do not choose patterns, run motion timers or contain arbitrary
user-supplied UUIDs. Adding another device requires a profile and core driver,
not changes to the shared motion engine or a parallel output loop. The worker
simulator accepts injected connectors and has no device-specific protocol logic.

## Connection and Stop behavior

- Native Bluetooth uses the host computer's radio. Its connection is backend
  state, independent of browser tabs, and is another Handy protocol choice in
  Setup, Settings and the connection manager. Selection never connects.
- A remembered Handy address does not fall back to another device. A first scan
  with multiple matching names fails explicitly. Forgetting the device requires
  disconnecting first. Public status and diagnostic exports omit the address.
- Discovery verifies both service characteristics. Payload budgets use the
  negotiated ATT MTU minus its three-byte header, capped by the profile. Point
  batches are split into whole RPC messages, with correctly incremented tails.
  An insufficient/unknown Handy MTU fails before playback instead of assuming
  the firmware's 512-byte maximum. WinRT write properties choose an available
  write mode before submission; uncertain writes are never retried.
- Explicit COYOTE discovery uses a separate, short-lived scan-only worker.
  Unnamed advertisements with the official app's candidate shape are listed
  without claiming device identity. Host-only responses contain expiring opaque
  selection IDs and RSSI, never hardware addresses or private binding data.
  Remembered devices, advertised name matches and possible unnamed matches are
  distinguished. Selection is explicit and full GATT validation precedes writes.
  Neither an explicit selection nor a remembered address falls back to another
  device. MagicHandy does not overwrite the official app's private binding.
- COYOTE uses Write Without Response on Windows and Linux. The core receives
  that capability and cannot use a queued zero as a known strength baseline.
  An actual device report is required before increases or playback are admitted.
- Every write receives a worker completion receipt. It means the native API
  accepted the write, not a measured position or physical Stop confirmation.
  Firmware responses are separately correlated. Missing optional clock replies
  disable time compensation; they do not disable otherwise working write-only
  firmware. Late correlated rejections retire the session.
- Native initialization, emergency Stop, disconnect and last will use RPC 715
  (StopCurrentMode, firmware 4.0.13+). HSP-specific Stop rejects a device left
  in Idle or another mode. Stopping must not require starting an HSP stream.
- Stop fences unsent chunks and queued work. An already-submitted Handy write
  can finish within its fixed one-second deadline, leaving a healthy connection
  available for Stop; a write that exceeds that deadline kills its worker before
  returning. A retired transport cannot stop or command a replacement session.
- Linked failure policy, excluded-device Stop, removal, protocol changes and
  shutdown use existing backend lifecycle paths. Reconnection never hot-joins a
  running linked session. A failing Handy worker does not kill a COYOTE worker.

## Physical limitations and validation

A worker stores a driver-supplied Stop packet for orderly disconnect and stdin
EOF. This is best effort. Windows kill-on-parent-exit, a blocked native call,
radio loss or power failure can prevent that packet from reaching the device.
Unlike COYOTE's short pulse packets, Handy may retain buffered HSP motion.
Native Play disables looping and enables pause on starvation, but software
tests cannot establish the physical duration after a failure. Hardware review
must cover core crash, worker crash/hang, radio loss and global Stop before this
is called physically validated. Existing conservative buffering stays in place.

The initial native radio implementation targets Windows/WinRT and Linux/BlueZ.
macOS and firmware-3 Protocomm are outside this implementation. The UI reports an
unsupported host or missing helper rather than falling back to another protocol.

The [integration plan](../native-handy-bluetooth-plan-2026-10-14.md) records the
implementation order and acceptance checks. Browser/native golden wire fixtures,
small-MTU chunk tests, process isolation, linked failure tests and leak/race
checks provide software evidence. The hardware follow-up records original Handy
connection and Stop, plus the remaining Windows reconnect failure.
