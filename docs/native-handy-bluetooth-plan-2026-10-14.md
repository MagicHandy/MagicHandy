# Native Handy Bluetooth integration plan

Status: implementation plan, written before runtime changes (2026-10-14).
Base: COYOTE/device-session work in PR #319. Hardware connection is explicitly
out of scope until the user authorizes it.

Implemented as [ADR 0046](decisions/0046-shared-native-bluetooth-worker.md).
Software results and remaining physical checks are recorded in the
[implementation review](native-handy-bluetooth-review-2026-10-14.md).

## Outcome

Add **Native Bluetooth** to the existing Handy protocol list in Setup, Settings,
and the connection manager. It uses the radio on the computer running MagicHandy,
including when the UI is on another computer. Selecting the option saves it and
stops the previous run; it does not scan, pair, connect, or start playback.

## Architecture

1. Extract the COYOTE worker's process protocol, client, server, and link contract
   into device-neutral Bluetooth packages. One shared executable supports named
   GATT profiles; one process owns one device connection. A hung native write can
   therefore retire that link without terminating the other linked device.
2. Keep device protocol decisions in the core. COYOTE retains its pulse driver;
   Handy gets a bounded protobuf RPC codec/session. Profiles describe discovery,
   services, characteristics and write limits, not motion. The core remains pure
   Go and does not link the native radio stack.
3. Share the existing Handy BLE HSP transport mapping through a small command
   channel interface. Both browser and native channels receive the same validated,
   quantized points, absolute tail indices, stroke window and Stop fencing. Do
   not introduce a second sampler or a native-only motion model. Preserve the
   browser's independent local emergency Stop path.
4. Give native Handy a backend-owned connection lifecycle and status snapshot.
   Integrate it into saved device inventory, global Stop, prepared-device Stop,
   settings transitions and both linked-session failure policies. Retired sessions
   cannot command a replacement connection. Reconnection requires a new run.

## Protocol and lifecycle constraints

- Use firmware-4 Handy RPC/HSP, matching the existing browser path. Legacy
  firmware-3 Protocomm is not included. Check the required GATT service and both
  characteristics before reporting readiness.
- Version the worker handshake and validate its supported profile. Retain an
  explicit, actionable error for an old or missing helper. Do not silently route
  native requests through a browser or another transport.
- Preserve completion receipts for every GATT write. A receipt means the native
  API completed the write, not that the motor reached a commanded position.
  Firmware responses are correlated separately; tolerate write-only firmware
  where the existing browser path does, and fail the session on correlated
  asynchronous firmware rejection.
- COYOTE's 64-byte limit cannot be reused for HSP. Encode complete, bounded RPC
  frames and split point lists into independently valid messages with correct
  tail indices. Never fragment arbitrary protobuf bytes into GATT writes.
- Use optional clock synchronization only when a valid device reply exists;
  otherwise omit server time and label playback timing as an estimate. Preserve
  current buffering initially; lower latency is a measurement, not an assumption.
- Keep bounded queues and write deadlines. Stop invalidates queued work; uncertain
  writes are not retried. A stuck worker is killed before returning a retired
  connection, preventing a delayed command from reaching a later session.
- A last-will Stop is best effort. In particular, Windows kill-on-parent-exit can
  prevent it from executing. Handy can have buffered motion after a radio/process
  failure: require starvation pause, disable looping, record this limitation, and
  validate physical failure behavior before claiming hardware readiness.
- Discovery must not silently switch to another Handy when a remembered device
  is absent. Ambiguous discovery fails with guidance rather than choosing an
  arbitrary matching device. Hardware identifiers stay out of public snapshots
  and diagnostics.

## Implementation order

1. Shared BLE worker extraction and COYOTE parity tests; worker packaging update.
2. Handy RPC codec/session, simulated link, shared HSP channel and lifecycle tests.
3. Backend integration, native status/connect/disconnect API, compact protocol UI
   and capability/error handling in all three configuration entry points.
4. Regression checks, Windows/Linux pure-Go builds, worker/core size measurements,
   and a simulated review app with a verified working LLM.

## Acceptance evidence

- Codec fixtures cover the manufacturer's field numbers, percent units, omitted
  zero values, malformed frames, unknown fields, chunk tails, and reverse mapping.
- Session tests cover write-only firmware, optional clock failure, rejected RPCs,
  queued/blocked writes, Stop races, link loss, stale completions, teardown and
  reconnect. Existing goleak and race gates remain enabled.
- Integration tests cover native Handy plus COYOTE under both failure policies,
  excluded-but-connected devices receiving Stop, protocol changes, removal,
  authorization and unavailable helpers. Selection alone must not connect.
- Frontend tests cover protocol persistence, compact manager placement and clear
  host-radio wording. Inspect desktop and narrow layouts.
- Run Go tests/race/vet/lint, architecture checks, frontend tests/typecheck/build,
  localization checks and CGO_ENABLED=0 builds. Record affected shipping budgets.
- After separate permission: test discovery, connection, Stop, restart, link loss,
  core/worker failure and concurrent COYOTE operation on real firmware-4 hardware.
  Capture transport mode, latency summary and diagnostic trace. Until then,
  software validation must not be described as physical-device validation.

## References

- Existing browser codec/session and `internal/transport/handy_hsp_contract.go`.
- [Manufacturer RPC schema](https://gitlab.com/sweettechas/platform/handy-public-rpc),
  inspected at commit `2398008c10d5f43903ec150e654968501f0ebc83`.
- [Firmware 4 HSP and protobuf overview](https://www.thehandy.com/blog/firmware-4-is-here/).
- [Pinned native BLE library](https://github.com/tinygo-org/bluetooth/tree/v0.16.0).
