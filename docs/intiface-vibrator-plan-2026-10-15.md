# Intiface vibrator integration plan

Status: implemented as [ADR 0047](decisions/0047-intiface-vibrator-output.md)
on 2026-10-15 and tested in simulation only. This plan was written before the
runtime changes and is kept as the design and review record. The
implementation differs in three places:

- Each row connects and disconnects its own lease on the shared session
  instead of sharing one Connect control.
- Scalar dispatches are counted in the vibrator status but are not yet traced.
- As for the Coyote, lowering is open to every client with control; a
  read-only client cannot lower.

Base: the device stack in PRs #319 (COYOTE output and linked device sessions,
[ADR 0045](decisions/0045-device-inventory-and-linked-sessions.md)) and #321
(native Handy Bluetooth, [ADR 0046](decisions/0046-shared-native-bluetooth-worker.md)).
Hardware connection is out of scope until the owner authorizes it. The draft
was reviewed by GPT-6-Astra (Codex CLI, max reasoning, read-only against commit
554d141f) and Gemini Argon (High); see [Review record](#review-record).

## Outcome

A Buttplug vibrator reached through a user-run Intiface Central becomes a
playback device. It can play with the stroker (The Handy over any connection
method, or a Buttplug linear actuator through Intiface), with the Coyote, with
both, or alone. It follows the shared motion frame through a conversion
profile, like the Coyote: there is no second motion model, and the model never
controls its intensity.

Owner decisions so far:

- **Vibrator-only playback is supported** (2026-10-15). Both reviewers were
  asked; Codex recommended secondary-only and Gemini recommended allowing it.
- **Model replies are later work.** They still assume a linear stroker; the
  [device-aware model responses plan](device-aware-prompts-plan-2026-10-15.md)
  covers the prompt changes and their test matrix. Until it lands,
  vibrator-only and Coyote-only replies can describe strokes the person does
  not feel.

## Current implementation this builds on

- **Inventory and participation** (`internal/config/device_connections.go`):
  `configured_devices` holds at most one `handy` and one `coyote`; `output` is
  a four-value participation enum (`none`, `handy`, `coyote`, `handy_coyote`);
  `linked_failure_policy` is `stop_all` (default) or `continue_healthy`.
- **Linked playback** (`internal/transport/fanout.go`, `linked_session.go`):
  one engine; `FanOut` holds exactly one primary and one companion, named
  "Handy" and "Coyote" in errors, retirement and status. Timing takes the
  stricter of both owners' point interval and buffered lead; sampling, startup
  state and the playback origin follow the primary. Any participant reporting
  `starving` or `starved` is unhealthy at once (Coyote's 750 ms only changes
  its own label). Coyote-only playback bypasses `FanOut`: the Coyote is the
  engine's only transport and its own clock.
- **Intiface** (`internal/transport/intiface*.go`, [ADR 0010](decisions/0010-transport-neutral-frames-intiface.md),
  [intiface.md](intiface.md)): one struct owns the WebSocket session, the
  device map, one linear selection and the `LinearCmd` pacer. Only `LinearCmd`
  features are parsed. Selection rejects zero `StepCount` and timing gaps over
  5 s, and does not stop the newly selected device. Pings run only when the
  server reports a nonzero `MaxPingTime`. A Stop that cannot be confirmed fails
  the whole session. The server connects Intiface only when the stroker's
  dispatch owner is `intiface`, and closes it when that owner changes.
  Non-Handy linear devices still lack physical acceptance.
- **Emergency Stop** zeroes the Coyote beside the engine Stop and stops a
  connected Handy that is outside the engine. Nothing covers other Intiface
  devices.

## Architecture

### Playback devices

- Participation becomes a set, `device.playback_devices`, of stable device IDs
  with display labels. Migration keeps today's rules: a missing value derives
  from `output`, an explicit empty set stays empty, and every member must be in
  the inventory. Conflicting new and legacy fields in one write are rejected.
- The legacy `output` remains a read projection of the stroker and Coyote
  members. A legacy write changes only those members and never removes the
  vibrator; it is no longer the routing authority. Every `UsesHandy` and
  `UsesCoyote` consumer is audited.
- The web UI moves to the set. Three places derive participation from the enum
  today, and `normalizeOutput` reads any unknown value as `handy`
  (`web/src/shell/ConnectionManager.tsx`).
- The engine's owner key includes the participant set and each participant's
  connection and selection epoch, so a changed binding always builds a new
  engine.

### Linked sessions and the run clock

- One linked session holds a **clock participant** and ordered companions.
  The clock is the first included device in the order stroker, Coyote,
  vibrator. The Coyote already acts as its own clock when alone; the vibrator
  owner gets the same `PlaybackStartTime` and pre-roll behavior.
- `FanOut` generalizes to labeled participants. Stop reaches every participant
  concurrently and joins labeled errors. Startup requires every included
  device and rolls back a partial start. All companions are aligned to the
  clock's start **before** any output is enabled, replacing today's
  Play-then-align sequence.
- The vibrator declares a minimum buffered lead (covering its offset and
  lookahead) and **no** minimum point interval: its command cadence must not
  coarsen the stroker's sampling. A normal run must never report `starving`,
  because a linked session treats that as a failure.
- Failure policy. `stop_all` is unchanged. Under `continue_healthy` an
  established run continues while at least one participant is healthy; the
  two-device `retired[1-index]` rule is replaced by counting survivors. Losing
  the shared Intiface socket retires every Intiface participant at once. The
  established clock is kept, a retired device cannot rejoin the run, and the
  Coyote keeps its distinction between a fault (zero strength) and Pause
  (strength kept).

### Shared Intiface session

Split into two steps, because only the first is free of behavior change:

1. **Extraction.** One session runtime owns the connection, handshake, ping,
   reader, request IDs, writer, response correlation, the device registry and
   scanning. The linear owner binds to it. Existing test assertions are kept;
   fixtures that reach owner internals are rewired.
2. **Lifecycle.**
   - **Leases.** The session is not a playback participant and not a
     connection-manager row. Each configured device that uses Intiface holds a
     lease while it is connected, including during discovery and while it is
     excluded from playback. The socket never closes between Connect, Scan and
     Select, or because playback stopped. It closes when the last lease ends,
     or on explicit disconnect, an address change or shutdown, each of which
     invalidates every dependent owner.
   - **Bindings.** Each owner binds to an immutable token: session generation
     plus device incarnation. A device index that disappears and returns is a
     new incarnation. Indices are never persisted (ADR 0010).
   - **One device, one role.** The same device cannot be both the linear and
     the vibration output in v1, since a device-level Stop from one owner would
     stop the other. Selecting a device stops both the previous and the new
     target before it is ready; the linear selection gains the same rule.
   - **Fault isolation.** A rejected or timed-out response faults only the
     owner that sent it. Socket loss, a missed ping or an uncorrelated protocol
     error faults every owner. A Stop that cannot be confirmed still closes the
     session, because the server then stops its devices.
   - **Emergency barrier.** Before any Stop write the session closes admission
     and invalidates every owner's generation. Each write rechecks session,
     incarnation, playback generation and limits under the writer lock.
     Admission stays closed until every Stop has finished, so no owner can
     write after `StopAllDevices`. Admitted writes stay bounded rather than
     cancelled, because cancelling a WebSocket write can break the connection
     that Stop needs.
   - **Capacity.** Bounded acknowledgement waiters and writer backlog, per
     owner and overall, with capacity reserved for Stop and ping, fair
     scheduling, session-wide unique IDs and generation-scoped handling of late
     responses. Device timing gaps are enforced per device.
   - **Parsing.** `ScalarCmd` features are parsed (`ActuatorType`,
     `StepCount`, descriptor) and the v3 wire encoding sits behind one internal
     seam: spec v4 replaces these messages with `OutputCmd`, and Intiface
     still accepts v3 clients.

### Vibrator owner

- **Selection:** one device with at least one `Vibrate` actuator; all of its
  vibrate actuators are driven together in v1. The row says which motors will
  run. Preselection happens only when exactly one candidate exists; using it
  is still an explicit choice.
- **Wire:** `[{"ScalarCmd":{"Id":1,"DeviceIndex":7,"Scalars":[{"Index":0,"Scalar":0.2,"ActuatorType":"Vibrate"}]}}]`.
  `Index` is the actuator's position in the device's full `ScalarCmd` feature
  list, kept after filtering. Only `Vibrate` is accepted. A level is `k/N` for
  `StepCount` `N`; a feature with zero steps is rejected. `Ok` confirms server
  processing, not motor output.
- **Pacer:** about every 100 ms, never faster than the device's advisory
  `DeviceMessageTimingGap` plus a margin. It renders a level, quantizes it
  with hysteresis, and sends only on a step change, with at most one command
  in flight; only an unsent target is coalesced, and an ambiguous command is
  never retried. Server-side deduplication or throttling is unverified, so the
  client paces itself. A missing or rejected response marks the vibrator
  unhealthy and the failure policy applies.
- **The level persists.** Unlike the Coyote, whose output stops after its
  last packet, a vibrator holds its last level. Coverage loss, Pause, the end
  of Test, Exclude, removal and shutdown therefore send an explicit zero that
  bypasses smoothing, the minimum, deduplication and pacing delays; the
  deduplication state resets on every restart and reselection. A watchdog
  detects a stalled pacer and stops the device. While the vibrator plays, the
  session's ping depends on the pacer being alive, so Intiface's own watchdog
  also catches a stall when the server enables it.
- **Stop:** invalidate the playback generation, then send `ScalarCmd` 0 for
  its actuators and `StopDeviceCmd`, each with a bounded wait. No `ScalarCmd`
  from an older generation can follow the Stop.

### Conversion profiles

| Profile | Level follows |
| --- | --- |
| Follow speed (default) | average absolute velocity over a short window, through a saturating curve (`1 - exp(-v / v0)`) |
| Follow depth | position toward the chosen end, in semantic base/tip coordinates, honoring reverse direction |
| Steady | a constant level while covered motion plays |

- Speed averages absolute segment velocity; endpoint differences would cancel
  across a reversal. Semantic speed is not physical motor speed once the
  stroke window scales it.
- Smoothing uses elapsed-time coefficients with a faster attack than release
  (the Coyote renderer already does both). The smoothing time, the 100 ms
  cadence, the curve constant `v0` and the timing offset are tuning hypotheses
  with no measured defaults; calibration combines smoothing, radio and motor
  lag.
- The timing offset is ±300 ms with a documented sign. The renderer only
  reads covered frame data and never extrapolates.
- Output below a small threshold is exactly 0. Above it the level maps into
  the range from the minimum to the live intensity (see Safety). The minimum
  defaults to 0 and, when raised, works as a motor spin-up floor. Silence
  always overrides the minimum and the fade.
- A 2 s fade-in starts every Play and Resume, and filters reset on seek and
  resume.
- Stationary but covered dwells keep the profile's output (steady and depth
  hold; speed falls toward the minimum).
- Ceilings quantize downward: a coarse actuator delivers its nearest step at
  or below the limit. There is never an automatic kick above the ceiling.

### Safety and permissions

- **Arming.** Modeled on the Coyote. A persisted **ceiling** in Settings
  (default 40%, quantized down) and a live **intensity** that starts at 0 on
  every connection or selection. The user raises it toward the ceiling, with
  Test, from the connection manager. The profile's level maps into the range
  from the minimum up to the live intensity, so at an intensity of 0 the
  vibrator is silent whatever the motion does.
- **Transactional admission.** Raising the intensity, changing the profile or
  minimum, and Test require the controlling client. They are fenced against
  Stop, controller turnover and stale request bodies, with the same
  Stop-sequence and owner-generation checks as the Coyote. Lowering is atomic
  against the current value, clamps pending output at once, never starts idle
  output, and stays available to read-only clients without accepting a
  disguised increase.
- **Test** plays 1 s at the lower of the live intensity and 25%, only while
  motion is idle.
- **Emergency Stop** addresses the live Intiface session independently of
  engine construction, selection and participation: it sends `StopAllDevices`
  alongside the other stops and joins their failures. `StopAllDevices` stops
  every device the server has, not only those MagicHandy selected, which is
  the intended emergency behavior. Failed delivery still leaves the engine
  stopped and reports silence as unconfirmed.
- **Actuators.** `Vibrate` only in v1. `Rotate` and `Oscillate` wait for
  hardware to validate them; `Constrict` and `Inflate` are excluded.
- **Server backstop.** Buttplug documents that the server stops devices when
  a client misses its ping deadline or drops. Whether that stop reaches the
  toy is a physical acceptance check. When the server reports no ping
  watchdog (`MaxPingTime` 0), the session status says so (open decision 5).

## Settings, API and status

- `device.configured_devices` accepts `vibrator` (at most one in v1).
- A `vibration` settings block: follow profile, depth end, minimum, ceiling
  and timing offset.
- Routes, each with admission-matrix rows: select the vibrator device, Test,
  raise or lower the live intensity, plus Connect and Scan through the
  session.
- `intiface_transport.vibrator` status reports:
  - the selected device and its motors with their step counts
  - the live intensity, the ceiling and the effective quantized ceiling
  - the current level and the playback state
  - commanded, acknowledged, rejected and timed-out counts
  - the last error
- Traces record each scalar dispatch.

## UI

- **+** gains **Vibrator (Intiface)**.
- The vibrator row has:
  - "Intiface Central" with a shared marker when the stroker also uses
    Intiface
  - the device picker, listing only devices with vibrate actuators and
    stating which motors will run
  - **Use in playback**
  - the live **Intensity** control, with the effective ceiling shown
  - **Test**
  - its status
- There is one authoritative Connect/Scan control for the shared session. It
  sits in the vibrator row when only the vibrator uses Intiface; when both
  rows do, it appears once and the other row shows the shared status.
  Disconnect on a row releases only that row's lease.
- Read-only clients can still lower the intensity.
- Settings > Device > Vibrator: follow profile, depth end, minimum, ceiling
  and timing offset.
- The top bar names the actual participants and shows each one's status, so a
  retired device stays visible.
- With no stroker in playback, the Motion quick controls keep only Speed, as
  for Coyote-only.

## Implementation order

Smaller PRs than the first draft, so each refactor is reviewable on its own:

1. **Playback-device set and N-way linked sessions.** Migration, the
   generalized `FanOut`, the run clock, survivor counting and
   align-before-enable. No new device.
2. **Companion timeline utilities.** Extract the bounded frame copy, clock and
   coverage helpers from the Coyote code, and harden timeline validation
   (finite values, atomic appends). The Coyote's packet pacing, strength
   handshake and renderer stay separate.
3. **Intiface session extraction.** No behavior change.
4. **Intiface session lifecycle.** Leases, connecting without the stroker,
   bindings, fault isolation, the emergency barrier, capacity rules,
   `ScalarCmd` parsing, and stopping on linear selection.
5. **Vibrator owner.** Renderer, settings, routes, status, UI and docs,
   including ADR 0047. Updates ADR 0010, ADR 0044 (the extracted helper), ADR
   0045, intiface.md, IMPLEMENTATION_PLAN, the UI docs, the risk register,
   simulation support and the scorecard's resource budgets. No new dependency
   and no change to the native Bluetooth worker (ADR 0046).
6. **Device-aware prompts** (later), per the
   [prompts plan](device-aware-prompts-plan-2026-10-15.md).

Start step 1 after #321 merges into #319, because it touches the same files.

## Simulation tests

- A fake Buttplug server with `ScalarCmd` features, mixed actuator types and
  coarse step counts, carrying linear and scalar traffic on one socket.
  - A withheld `ScalarCmd` response faults only the vibrator while the linear
    owner continues.
  - Socket loss faults both.
  - A late response for an old generation is ignored.
  - A reused device index is a new incarnation.
- Blocked writes with a concurrent Stop or Test; no write after
  `StopAllDevices`; no `ScalarCmd` after Stop, including mid fade-in.
- Emergency Stop sends `StopAllDevices` with no engine, with an excluded
  vibrator and with an unselected device.
- Every three-participant failure combination under both policies, plus
  vibrator-only and Coyote + vibrator runs with the clock from the first
  included device, startup rollback, and restart and resume.
- Migration: omitted versus empty sets, conflicting fields, and a legacy
  `output` write keeping the vibrator.
- Renderer goldens per profile: reversal handling, threshold, hysteresis,
  fade-in on resume and the downward ceiling quantization.
- The pacer watchdog, and the pacer-coupled ping.
- goleak for every new worker, with no self-join; HTTP, admission and UI
  tests.

## Physical acceptance (owner-run, later)

Use a named test inventory: one vibrator, preferably also a coarse-step or
multi-motor unit, and a non-Handy linear device before claiming that
combination. Record firmware and Intiface Central versions, and keep traces.

- Latency calibration and the timing offset.
- The distribution of time from Stop to silence.
- Intiface stopping the toy on core crash, server crash, missed ping, a
  half-open socket and radio loss.
- Radio contention: three outputs, and the native Bluetooth worker beside
  Intiface Bluetooth on one adapter.
- All four Handy connection methods with the vibrator.
- Restart and resume.
- The feel of each profile: vibrator alone, with The Handy, with the Coyote,
  and with an Intiface linear device if available.

## Open decisions

2. **Arming and default feel:** follow speed, a 40% ceiling, and a live
   intensity that starts at 0 on every connection (Codex: no universally safe
   default; start disarmed).
3. **Scope of v1:** one vibrator device, with all its vibrate motors driven
   together and listed in the row.
4. **Test hardware:** which vibrator and which non-Handy linear device are
   available.
5. **Ping watchdog:** whether vibrator playback requires a nonzero Intiface
   ping timeout, or only reports when it is off. The default in current
   Intiface Central builds is unverified.

## Review record

**GPT-6-Astra (Codex CLI, max, read-only), adopted:**

- **Timing:**
  - the vibrator declares no point interval
  - linked sessions fault on `starving` immediately
  - fade-in on every Play and Resume
  - align companions before enabling output
- **Intiface session:**
  - extraction and lifecycle as separate PRs
  - leases held by configured rows, including discovery
  - device-incarnation bindings
  - one role per device, and Stop on selection
  - the shared emergency barrier, with bounded writes
  - capacity reserved for Stop and ping
  - an unconfirmed Stop still closes the session
- **Vibrator output:**
  - explicit zero, because the level persists
  - the pacer watchdog
  - Emergency Stop independent of the engine
  - transactional admission, with lowering open to read-only clients
- **Wire:**
  - original `ScalarCmd` indices
  - `k/N` quantization
  - the advisory timing gap
  - pinning the Intiface Central version
- **Participation:**
  - migration validation
  - the engine owner key including epochs
  - survivor counting
- **Mapping:**
  - window-averaged speed with saturation
  - depth in semantic coordinates
  - dwell behavior
  - minimum 0
  - downward ceiling quantization
- **Testing:** the test and acceptance lists.

Corrected or not adopted:

- Its secondary-only recommendation, because the owner chose vibrator-only
  support.
- Its suggestion to extract all Coyote pacing, narrowed to the timeline,
  clock and coverage utilities as it advised elsewhere.

**Gemini Argon (High)**, in the existing concurrent-device review
conversation, adopted:

- the session as leases rather than a saved row
- per-owner fault isolation
- `ScalarCmd` 0 plus `StopDeviceCmd` on Stop, and `StopAllDevices` on
  Emergency Stop
- the generalized `continue_healthy` rule and the legacy-write trap
- asymmetric smoothing, the spin-up floor, the zero threshold and hysteresis
- deferring battery reads, and the idle-only Test level
- per-participant status
- the shared-socket, three-participant and post-Stop tests
- the process-kill and radio contention checks
- the v3/v4 seam

Corrected:

- It called one client per server documented. The maintainer confirms that
  multiple clients ["are not a feature of Intiface or buttplug in general"](https://discuss.buttplug.io/t/can-multiple-clients-connect-to-intiface-on-ios/982),
  but the spec does not say so.
- It said a stroker failure must stop every device because the stroker owns
  the clock. The engine keeps the run clock after start, and ADR 0045 already
  lets the remaining device finish.
- Its no-op clock primary is unnecessary, because the Coyote and the vibrator
  can each be the clock.

Verified against the Buttplug documentation: [spec v3 generic commands](https://buttplug.io/docs/spec-v3/spec/generic/)
(`ScalarCmd`, `StopDeviceCmd`, `StopAllDevices`), [ping](https://buttplug.io/docs/spec/status)
("the server will disconnect and stop all currently connected devices"),
[sessions](https://buttplug.io/docs/dev-guide/architecture/sessions-and-components)
(the server stops a device until its client reconnects) and the
[buttplug v10 release notes](https://buttplug.io/blog/2025/10/19/buttplug-v10b2-and-intiface-central-v3b2-released)
(rebuilt backward compatibility for older specs).
