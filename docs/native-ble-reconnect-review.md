# Native BLE reconnect and COYOTE playback review

This follows the [initial hardware investigation](native-handy-hardware-review-2026-10-14.md).
The user authorized repeated COYOTE 3.0 connections and playback, confirmed that
no electrode/output leads were connected, and allowed strength changes. Testing
used the isolated Windows app at `http://127.0.0.1:50235/#/settings/device` with
the real native worker. The Handy stayed disconnected and excluded from playback.
No hardware addresses, binding identifiers or user credentials appear here.

## Reconnect correction

The old Windows workaround relied on worker exit to release native resources
because TinyGo Bluetooth v0.16.0 could disconnect recursively from its status
callback. That prevented a crash but left GATT service ownership behind long
enough for the next worker to receive AccessDenied.

The module is now pinned to upstream commit
`9df563b9dd49efd0d697700c6bc938dcf1e90c6f`
(`v0.16.1-0.20261012180541-9df563b9dd49`). This is the reviewed, **unreleased**
upstream [service-disposal fix, PR 509](https://github.com/tinygo-org/bluetooth/pull/509),
including the earlier correction that removes recursive disconnect from the
native callback. Its Windows changes close discovered services synchronously,
wait for native connection readiness, and propagate failed characteristic
discovery. Relevant Windows source and module changes were inspected before
hardware testing. The BSD-3-Clause license is unchanged. There is no local fork
or native dependency in the core. The indirect macOS/embedded dependencies
follow the upstream module's versions and are not linked into Windows or Linux.
Track a tagged upstream release containing these fixes before removing this pin.

The worker now explicitly disconnects before process exit. A single close owner
fences writes and makes overlapping closes wait for disposal. Lost links remain
owned until teardown, even after they are removed from the active write path;
native resources are disposed outside the native callback. Failed discovery and
late canceled connections use the same disposal path. Process termination is
still the deadline fallback for a native call that cannot return.

Ten immediate disconnect/reconnect cycles succeeded with the disposal fix.
A later cold connection hit the old six-second deadline while the same device
remained visible in scans. The shared default connection budget is now twelve
seconds, including the fresh selected advertisement and native readiness. This
changes a deadline, not normal connection latency, packet pacing or Stop. Caller
cancellation and the outer process deadline still apply. Five further immediate
cycles passed with this budget. No retry loop or artificial gap was inserted.
Two more cycles and the complete playback sequence passed on the final build:
**17/17 immediate cycles** across these runs. Connections took **3,283–7,611 ms**
(median **6,200 ms**); disconnect requests completed in **46–62 ms**. Each
connection received a real zero report and all cycles had zero write failures.

## Playback with no output leads

The real device confirmed A=5/B=0 for the A test, A=0/B=5 for the B test, and
A=5/B=5 for engine playback. Saved ceilings stayed at 30/30. These are strength
values reported by the device; the separate commanded waveform intensities are
not electrical measurements.

- The A reference pulse commanded A only and was interrupted after about 600 ms.
- The B reference pulse commanded B only and ended after its built-in 1.6 seconds.
- The shared engine played `flow-full-sweeps` at 30% for four seconds, then
  retargeted to `flow-pace-wave` at 40% for four seconds.
- Both engine phases stayed in `playing`, with zero write failures and zero
  starved packets. The first playback sample reached 111 sent packets. The
  recorded pacer lateness rounded to 0 ms.
- Global Stop cleared commanded output and received fresh A=0/B=0 reports.
  The first run's three Stop HTTP times were 25, 23 and 65 ms; observed zero
  reports arrived at 185, 201 and 202 ms. Status polling was every 40 ms, so
  those times include polling and host delay.

The final-build repeat also passed all four phases, with zero write failures and
starvation. Stop HTTP times were **26, 23 and 47 ms**; fresh zero reports arrived
at **155, 262 and 172 ms**. The complete tests ran twice; no physical output was
connected during either run.

These checks establish native connection, dispatch, device-reported strength,
per-channel routing, bounded reference playback, engine retargeting and zero
reporting after Stop. They do not measure current through a load, waveform
accuracy, sensation, physical output-decay time, switch-limit enforcement, radio
loss during electrical output or concurrent Handy/COYOTE playback. The shared
sampler, motion mapping, buffering policy and model prompts are unchanged.

## Packaging startup correction

The earlier package job failed waiting for health during the Program Files
install/uninstall/reinstall stage. A subsequent unmodified rerun passed; the
original job did not retain app stderr, so its precise cause is unconfirmed.

The harness had a reproducible port-allocation defect: it checked only the main
port although the app also binds the adjacent remote port before serving health.
It now checks both ports, uses a fresh pair after over-install, starts with plain
logs, and reports process exit plus captured stderr instead of waiting through
the health timeout. Successful readiness verifies both listeners. Packaging CI
retains the lifecycle logs even when a job fails; checksum, signature, binary
vulnerability, installation and uninstall checks remain enforced.

The new regression occupies the remote port deliberately, verifies a prompt
failure that includes the actual bind error, then checks three successful
dual-listener starts. It runs against a real compiled app in isolated data and
is included in the full installer suite and packaged lifecycle acceptance.
Both PowerShell 5.1 and PowerShell 7 regression and installer suites passed.

## Evidence and review limits

Ignored evidence under `.scratch/native-handy-hardware/` includes the initial ten
and subsequent five reconnect samples, the retained cold-connection failure,
per-phase playback/status samples, Stop timing, a motion trace and a diagnostic
ZIP without chat. The trace contains engine records from the actual playback;
it is not a synthetic simulation. No runtime evidence is committed.

Software checks include native close/lost-link disposal, overlapping closes,
deadline cancellation, write fencing, worker isolation, full Go/race/lint,
architecture/goleak, and CGO-disabled Windows/Linux builds. The frontend was
unchanged. Review generation uses the available local Gemma endpoint. Repeated
Handy reconnect and physical Handy playback still need their own hardware check.
