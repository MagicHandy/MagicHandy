# ADR 0045: Device inventory and linked playback sessions

## Status

Accepted 2026-10-14. Supersedes the output-selection and companion-failure
behavior in ADR 0044. The [October 15 hardware review](../native-linked-hardware-review-2026-10-15.md)
adds native Handy/COYOTE playback and worker-loss testing in both policy
directions. Loaded output, physical radio loss and seamless recovery with a live
companion remain pending.

[ADR 0047](0047-intiface-vibrator-output.md) adds a vibrator. Participation
becomes the `device.playback_devices` set, with `output` kept as a projection
of the Handy and Coyote members. Linked sessions take any set of devices, with
the run clock on the first included one, and `continue_healthy` counts
survivors.

## Decision

The saved inventory, live transport connections and playback participation are
separate backend-owned states:

- `device.configured_devices` contains zero or one `handy` and `coyote` entry.
  Missing inventory migrates from the old output selection; an explicitly empty
  list stays empty across reloads.
- Existing `device.output` now selects playback participants: `none`, `handy`,
  `coyote`, or `handy_coyote`. An included device must be configured. The legacy
  output endpoint can add a missing participant for compatibility, but never
  deletes other saved connections.
- Connection status comes from the actual transport snapshot. Adding a row
  does not connect, include, or start anything. Connect prepares a device;
  **Use in playback** includes it in the next run. Excluding keeps its connection
  and zeroes Coyote output. Disconnect retains the saved row. Removing closes
  the connection and excludes the device; the last row can be removed.
- `none` permits text chat with the motion engine idle. Motion requests fail
  explicitly rather than silently selecting another device or a simulator.

The connection manager retains the additive flow with an icon-only **+** on
the title row. Each row has its protocol, connection controls, and separate
playback switch. Settings offers the configured playback devices and the
failure policy; **Manage connections** opens the same manager.

`device.linked_failure_policy` is `stop_all` by default. Any participant's
unexpected disconnection, unhealthy playback state, append/play failure or
readiness loss stops the linked session. Both owners receive Stop, with a
fault marker that also zeroes electrical output. Errors from either Stop are
reported. Ordinary Pause still preserves strength.

The explicit alternative, `continue_healthy`, retires the failed participant
from an established run and attempts its fault Stop, allowing the remaining
owner to finish. Both devices are still required for startup. A second failure
stops the run. The backend publishes a degraded-session status and the retired
device names. Reconnecting cannot rejoin this run; the user must Stop and start
a new one. Changing playback participation or policy stops the current run.
Emergency Stop always stops all outputs, independently of this setting.

Stopped failures remain in backend snapshots with a failed-session marker;
connection availability is reported separately. Unexpected COYOTE loss cannot
confirm Stop or zero, while the healthy participant still receives Stop.

Coyote engine handles bind to one link; browser Bluetooth commands bind to a
connection generation, including same-tab reconnects. Old append/Play/alignment
work cannot command a replacement connection. A fresh explicit run builds a
new session. A known zero strength is a valid silent baseline, while unknown
strength, a pending handshake, or unapplied limits block startup. Calibration
cannot overlap playback on another device.

## Architecture and scope

There is still one shared semantic engine and its existing health/dispatch
loop. Fan-out combines health and command results; it adds no independent
motion loop. Handy timing remains authoritative, and neither waveforms nor
sampling, offsets, fade-in, strength ceilings, or LLM prompts change here.

Native Coyote patterns, new LLM device vocabulary and hardware latency
calibration are separate work. No new dependency is required by this refactor.

## Validation

Regression coverage includes inventory migration/empty-state persistence,
add/include/exclude/disconnect/remove, permissions, startup rollback, both
failure policies in both directions, joined Stop errors, stale connection
commands, and the running engine's response to simulated link loss. UI tests
cover authoritative updates, removal of the last row, the title-row +, and
failure-policy persistence. The full repository gates and a simulation review
build are recorded in the accompanying review report.
