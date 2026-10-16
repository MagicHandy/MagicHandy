# Coyote 3.0 implementation review — 2026-10-14

This records the initial safety and UI review. The subsequent
[device-state review](device-state-review-2026-10-14.md) supersedes the
connection-list behavior and final validation counts below: removing the last
connection is now supported, adding a connection does not select it for playback,
and linked-session failure handling is configurable.

This review covers the native worker, transport owner, strength handshake,
global Stop, and connection controls. All device scenarios use test doubles or
the simulated connector. No real device has been scanned, paired, connected, or
commanded. Physical output and radio timing remain unverified.

## Reproductions and corrections

| Reproduction | Correction | Regression coverage |
| --- | --- | --- |
| Concurrent engine Stop invalidated the separate zero-strength write. | Safety writes depend on the current connection, not the motion generation. Global Stop closes control admission until all outputs have stopped. | Concurrent Stop/zero, held Stop, and two HTTP Stop callers receiving the same failed-zero result. |
| A failed ceiling write published the new setting as applied and allowed playback under the previous device ceiling. | Track requested and successfully applied limits separately. Suspend output during the update; on failure, attempt zero and close the connection. Reconnection reapplies saved limits. | Failed BF write, blocked playback, closed link, and successful ceiling application after reconnect. |
| A lost relative-strength acknowledgement still allowed a queued increase. | An expired acknowledgement makes strength unknown and drops queued increases. Reductions remain permissible; new increases require an eligible baseline. | Lost B1 response followed by queued increases and decreases. |
| A zero timeout replaced a newer physical-switch report with an invented confirmed zero. | Successful GATT delivery can establish an explicitly labeled commanded-zero baseline. Only device reports establish reported strength; a timeout never assigns a value or replaces a newer report. | Zero timeout without delivery, intervening physical report, and matching device report. |
| Idle strength adjustments implicitly began a reference waveform, including a reduction from a non-controlling client. | Strength adjustment stays silent. A separate channel Test action uses the existing bounded reference waveform and requires the controlling host and current Stop sequence. | Silent decreases, explicit Test permissions, Stop cancellation, and delayed pre-Stop controls. |

The worker protocol now returns an IPC completion receipt for every native
write, including GATT Write Without Response. The core therefore cannot build
an unobserved queue of motion frames ahead of Stop. An uncertain timed-out
write retires the session and terminates the worker before returning. Tests
exercise both acknowledged and unacknowledged native-write requests with a
hung test worker; they do not load a radio driver.

## What remains unchanged

The shared motion engine, semantic trajectory, profile mapping, pulse encoding,
strength ceilings, and 100 ms dispatch cadence are unchanged. Test uses the
existing reference waveform through the same dispatch pacer. It adds no
separate motion source or LLM control of electrical strength. Pause silences
the waveform; global Stop also requests zero strength.

No physical latency guarantee is inferred from the simulation. Before a motion
trial, connection-only hardware validation must check worker discovery,
Bluetooth ownership, ceiling application, the initial zero command, and
device-report handling. A successful connection alone does not validate
sensation, strength calibration, or Stop latency on hardware.

## Automated validation

The complete Go suite, race suite, vet, and golangci-lint pass with the
permanent regressions above. The transport and worker lifecycle tests keep
their goroutine-leak gates enabled. Runtime artifacts, test logs, and trace
exports remain in ignored `.scratch/codex-coyote-fixes/`.

## Connection manager

Claude Opus 5.5 Max refined the initial section layout, alignment, rules, and
protocol menu. Its session reached the usage limit before the additive-device
follow-up, which Codex implemented and reviewed. **+ Add connection** replaces
the manager's three-way selector; it offers only the missing supported device.
Each device has its own protocol/status section. Removing one retains the other,
and the sole device cannot be removed. The Handy's title menu changes protocol
through the existing backend preference API; Coyote's only supported method is
shown as text. Neither addition nor protocol selection initiates playback.

Focused UI tests cover keyboard selection, unchanged labels until backend
confirmation, serialized saves, removal failure, access restrictions, preserved
panel lifetimes, and the current Stop sequence supplied to explicit Test.

The final UI uses an outlined Coyote prerequisite notice, places the Handy
protocol beside its title, and leaves simulation identification in the top bar.
The frontend type check, localization audit (five locales), production build,
and all 883 tests in 115 files pass. The full test run uses four workers to
avoid host saturation; no assertions or safety gates are skipped. The Coyote
component tests explicitly control their polling clock rather than racing a
real 500 ms interval against assertions.

Both the core and native worker build with `CGO_ENABLED=0`. The exact final
review binary runs at `http://127.0.0.1:50232` with isolated data and
`-simulate-motion`; Handy Bluetooth and Coyote are disconnected. Its readiness
probe and a text-only chat complete with the available Ollama Gemma model. The
chat returns non-empty text in 397 ms, uses one provider call, and has no
repair/fallback or failure record. The final Coyote snapshot contains zero
packets sent. Runtime state and a trace export are retained with the ignored
test evidence.
