# ADR 0044: DG-LAB Coyote 3.0 Output

## Status

> Amended by [ADR 0063](0063-perceptible-following-and-coyote-facts.md): pulse
> width and rate read speed on the saved Handy speed band, capped by the
> earlier curve so output can only soften at a given strength, with a 150 ms
> release. Models now get Coyote facts (no control) instead of no Coyote
> prompt, and the status reports why a connected Coyote is silent. Strength
> authority, ceilings, the handshake and Stop are unchanged.

Accepted on 2026-10-14. The implementation drives a simulated Coyote end to
end; physical acceptance on a Coyote 3.0 is open (see Validation).

The worker packaging and IPC ownership below are extended by
[ADR 0046](0046-shared-native-bluetooth-worker.md): the helper is now named
`bluetooth-worker`, version-3 IPC is device-neutral, and each Handy/COYOTE link
still gets its own process. The pulse/strength decisions in this ADR remain.

[ADR 0047](0047-intiface-vibrator-output.md) moves the Coyote's frame
timeline, clock and coverage helpers to `internal/companion`, shared with the
vibrator. The Coyote's packet pacing, strength handshake and renderer are
unchanged.

## Context

The DG-LAB Coyote 3.0 is a two-channel electrical stimulation box. Each
channel has a strength from 0 to 200, set by the user or by the device's own
switches, and plays waveform frames that the host streams: one 20-byte `B0`
write every 100 ms carries four 25 ms frames per channel (an output-unit
period of 10–1000 ms and a relative pulse width of 0–100). DG-LAB publishes the
Bluetooth protocol in
[dglab-bluetooth-protocol](https://github.com/dungeonlab-open/dglab-bluetooth-protocol)
(`coyote/v3`). The device holds nothing beyond the current packet: the 2.0
documentation states that output stops 0.1 s after the last write, and 3.0
clients rely on the same behavior.

MagicHandy's transport contract carries one moving position (ADRs 0002 and
0010), and exactly one dispatch owner drives it. AGENTS.md section 3 places
native Bluetooth behind the browser bridge or a worker process, never in the
core binary.

Two reviews informed this decision: a Codex investigation (2026-10-13) and a
Claude evaluation (2026-10-14). Both recommend a small Bluetooth worker as the
production path, user-owned ceilings outside the model's reach, disarmed
reconnects and Stop that invalidates every queued packet. The Codex review adds
that position must never become channel strength, that changing pulse rates
must respect the device's output window, and that native stimulation patterns
and model composition are the stronger long-term vocabulary. The user decided
not to implement the DG-LAB app relay, to make the Coyote alone the default
when a Coyote is chosen while keeping either choice one click away, and to
start the ceilings lower than other apps do.

## Decision

1. **Device state.** See [ADR 0045](0045-device-inventory-and-linked-sessions.md)
   for the saved inventory, independent playback selection, empty chat-only
   state, additive manager, and configurable linked-session failure policy.
2. **Connection.** A per-connection worker, `estim-ble-worker`, owns the radio
   through `tinygo.org/x/bluetooth` (WinRT on Windows, BlueZ over D-Bus on
   Linux, no cgo). It speaks NDJSON over stdio (`internal/estim/workerproto`)
   and is a thin pipe: it connects, writes the bytes the core composed and
   reports notifications, battery and loss. Protocol v2 acknowledges native
   completion for every write, so the core never queues old frames behind a
   slow native write. A write timeout kills the worker before returning.
   A Windows job object ends it with
   the core. If stdin closes without a shutdown, it writes the stored zero
   packet before disconnecting. The core remembers the device address, because
   recent firmware accepts a direct connection to a known address without
   pairing mode; scans match the exact normal-mode name, so the recovery-mode
   `47L121000_O3` never connects. A device without the documented service and
   both characteristics is refused.
   Rejected: browser Web Bluetooth (no device buffer, and hidden tabs both
   throttle timers and end our bridge), the DG-LAB app relay (user decision),
   and third-party bridges through the Intiface owner (single owner, no
   strength visibility).
3. **Owner and fan-out.** `transport.Coyote` is a dispatch owner. It keeps a
   read-only copy of the neutral frame, renders one packet per 100 ms tick and
   asks the engine for 500 ms of lead (1 s for media). `Stop` and Pause silence
   it without touching strength. With both devices, `transport.FanOut` is the
   engine's one transport: the Handy stays authoritative for timing and startup;
   the Coyote follows the Handy's playback clock, and both contribute to
   playback health. The default failure policy stops both; an explicit setting
   can retire the failed participant and continue on the healthy one. Stop
   reaches both concurrently and joins both outcomes.
4. **Conversion profile.** The renderer (`internal/estim`) is a deterministic
   mapping at the owner boundary, not a second motion model. The user picks one
   of three profiles. The default, *Stroke and speed*, sets the channel balance
   from position (channel A at the base, B at the tip, with a swap option) and
   the pulse width from smoothed speed. *Speed only* drops the balance and
   *Position only* keeps a steady width. No profile maps position or speed to
   channel strength.
   - The pulse rate follows tempo inside a user band (default 40–100 Hz). It
     moves in 5 Hz steps held for at least 500 ms, because DG-LAB documents that
     quickly changing periods longer than the 25 ms window are reshaped.
   - Every Play or Resume fades in over 2 s.
   - Output windows the motion frame does not cover are silent. The owner
     never holds the last value and reports starvation like the Intiface pacer.
5. **Strength authority.**
   - **Only the user changes strength.** Channel strength changes only on a
     user step or on the device's own switches. Automation shapes pulse width
     and rate beneath it.
   - **Ceilings.** The ceilings, 30 of 200 per channel by default, are written
     with `BF` on every connection. The device enforces them against every
     source and keeps them after disconnecting. Requested and applied limits
     are distinct. A failed live write zeroes and retires the connection;
     reconnect reapplies the saved limits before enabling output.
   - **Disarmed start.** Each connection begins with a zero command.
     Native COYOTE exposes Write Without Response, so native completion cannot
     establish a known baseline. It requires a device report before increases
     or playback. For acknowledged links, `zero_command` is an accepted write,
     not a device measurement. A timer
     never invents confirmed zero, and a newer report always takes precedence.
   - **One change at a time.** Steps travel one at a time and wait for the
     device's `B1` report. Missing confirmation drops queued increases and
     blocks new increases while the baseline is unknown.
   - **Test pulse.** Strength steps never start output. An explicit host-only
     Test action plays the existing 1.6 s calibration reference on one channel,
     through the owner's shared packet pacer and encoder. It changes no
     strength, requires idle output and a known baseline with applied limits,
     and carries Stop-generation fencing. This calibration is not an LLM motion
     source. No new waveform loop or transport bypass is introduced.
   - **Permissions.** Raising needs the controlling host. Lowering and zeroing
     are open to controllers, and the global Stop zeroes for everyone.
6. **Stop and loss.**
   - **Global Stop.** The global Stop runs `EmergencyZero` beside the engine
     Stop. A hold rejects increases, playback and test admission until both
     outputs finish. Generation bumps discard old motion, but cannot discard
     the safety zero. Both results are joined before completing any shared Stop
     reply. Failed writes retire the uncertain connection.
   - **Disconnect.** Disconnect zeroes first.
   - **Link loss.** No new packets are dispatched; physical output decay remains
     a hardware acceptance measurement. The owner reports
     `stale`, so a Coyote-only engine stops, and reconnecting starts at zero.
7. **Model control.** The model gets no Coyote-specific control or prompt in
   this version. It keeps driving motion, which the profile converts.
8. **Simulation.** `-simulate-motion` also simulates the Coyote. The simulated
   device follows the documented strength, ceiling and report rules. The
   worker's `-simulate` flag gives process-level tests the same device.

## Consequences

Positive:

- Chat, Autopilot, patterns, Freestyle and video scripts drive the Coyote
  without new prompts, and the Handy path is unchanged when the Coyote is not
  selected.
- One Stop path covers both devices; the protocol is isolated and checked
  against DG-LAB's published example packets.
- The core binary links no radio stack; the worker can be replaced without
  touching motion.

Negative:

- The installer gains `estim-ble-worker.exe` (about 3.3 MB). `go.mod` gains
  `tinygo.org/x/bluetooth` and its platform dependencies; only the worker links
  them.
- `BF` has no read-back, so writing the ceilings also overwrites the device's
  two balance parameters (MagicHandy owns them as settings, default 160/0).
  Ceilings persist on the device and limit other apps until changed.
- Windows Bluetooth adapters vary; the remembered-address path and scanning
  need confirmation on the user's hardware.
- The conversion profile's physical feel is unvalidated.

Follow-up, not implemented:

- native stimulation patterns and a model *compose* vocabulary (rhythm,
  texture, envelope, channel relationship), recurring motifs with Autopilot,
  comparison-based preferences, coordinated Handy and Coyote compositions and
  reviewer vocabulary; these extend the output contract and need their own ADR;
- a Coyote card on the phone remote and A/B lanes in the live visualizer;
- a Coyote 2.0 encoder, which would have no device-enforced ceiling.

## Validation

Automated:

- golden B0/BF vectors from DG-LAB's examples;
- strength handshake, renderer and timeline tests;
- the worker serve loop with a simulated adapter;
- the real worker process in `-simulate` mode;
- owner, fan-out and HTTP tests on the simulated device;
- UI component tests.

Open, run by the user on a Coyote 3.0:

- first connection in pairing mode, then reconnection by remembered address;
- ceilings enforced against the device's switches, and switch changes
  mirrored in the app;
- time from Stop to silence and to zero;
- silence after a dropped link;
- the feel of each profile, and the timing offset with the Handy and the
  Coyote together.

See [estim.md](../estim.md) for setup, behavior and protocol notes.
