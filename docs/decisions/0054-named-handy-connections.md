# ADR 0054: Named Handy connections on one motion timeline

- Status: Accepted for the alpha.71 candidate
- Date: 2026-10-23

## Context

The linked session already supports several device types, but settings and
connection ownership previously allowed only one Handy. A second physical
slider also invalidates two assumptions: the first slider's measured startup
position cannot describe its peers, and serial network requests add a round
trip per peer to every frame.

## Decision

Keep the original `handy` instance and its existing configuration/API as the
compatibility adapter. Additional instances have opaque `handy-<12 hex>` IDs,
a name, method, model, and private connection credentials or native selection.
The inventory is bounded to eight Handys including the primary slot. Additional
instances support Cloud REST, Native Bluetooth, and Intiface in any mixture;
the primary slot also supports the existing singleton Browser Bluetooth bridge.
This is an inventory/resource bound, not a guarantee that a host radio can
maintain eight Bluetooth links.

Every participating device receives the same semantic sequence from the shared
engine. There is no extra generator, LLM request, or private motion loop. Range,
direction and speed limits are shared. Effective planning uses the most
conservative normalized travel-speed calibration among the participating Handy
models; an excluded device cannot lower the pace. The primary's stored model is
unchanged. Chat, reviewer, Autopilot, media, library and preview paths use that
same effective policy.

Each additional connection owns a transport incarnation. An engine captures a
generation-bound lease; retired work cannot command a replacement connection.
Native selections use expiring, instance-bound opaque discovery tokens. Reusing
a saved Cloud key or native address for another instance is rejected. Physical
identity cannot be compared across Cloud and Bluetooth, so users must assign
different physical Handys when mixing protocols.

Intiface instances bind separate selected linear outputs to the existing shared
socket and claim registry. Adding an output after the socket connects starts
its existing transport pacer; releasing it cancels that worker and claim while
other leases survive. Native instances use the existing worker protocol, with
one worker process per connection. Nothing native enters the pure-Go core.

The shared engine obtains startup telemetry from each capable participant and
compiles that slider's bounded lead-in itself. Another slider's startup path is
never broadcast. Every participant must prepare successfully before the main
sequence starts, even under Continue healthy. Owners without position telemetry
retain their existing startup behavior. Preparations are sequential and may
increase startup time with several Cloud devices.

Window/append I/O runs concurrently across participants and drains before
return or rollback. Play establishes the first participant's clock, then starts
companions concurrently at its elapsed stream position. Cloud computes that
position after clock refresh. Sampling honors every participant's limits, and
refill latency includes the slowest connection. Physical synchronization remains
an estimate subject to transport, radio and network delays.

## Stop, lifecycle and permissions

The existing linked failure policy applies per instance. A failed participant
cannot hot-join a run after reconnection. Changing participation, model, method,
credentials or the shared Intiface address stops the affected session; renaming
does not stop it. Connecting or adding does not include or start a device.

Global Stop cancels pending connection attempts and reaches connected instances
outside the current engine too. Failure to construct one fallback owner does
not suppress Stop on the remaining reachable devices. An excluded Cloud setup
placeholder with no key is not a prepared device. Instance removal stops and
disposes its owner before discarding runtime state.

New settings and connection mutations follow the existing host/controller
authorization policy. Runtime snapshots are readable to full-interface users;
keys and hardware addresses never appear in them or public settings. Diagnostic
redaction includes every saved instance's private values. The existing
Intiface/Buttplug device-Stop limitation remains unchanged.

## UI and scope

Settings > Devices retains the additive flow and gives each Handy its own page,
name, method, model, connection state and playback participation. The compact
device panel keeps the shared limits and permanent Stop. The shared Intiface
row lists every lease. Browser Bluetooth's one-device scope is explained where
additional methods are chosen. No independent per-Handy patterns, phase offsets
or arbitrary per-device speed multipliers are introduced.

See [implementation review](../multi-handy-review-2026-10-23.md) for tests,
simulation evidence, measured weight and the outstanding physical validation.
