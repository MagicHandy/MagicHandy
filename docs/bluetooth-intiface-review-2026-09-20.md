# Bluetooth, Intiface and shared Handy protocol review

Review baseline: `68fb56e94d7e5bd72066bd70f11ecad3ea2b1c93`.
The changes preserve the shared motion engine and transport-owner contract.
No real device was connected or commanded during this review.

## References checked

- Manufacturer [public RPC definitions](https://gitlab.com/sweettechas/platform/handy-public-rpc/-/tree/06a2d4a6b83bee4903040f335b191478b96fd225/public/proto),
  including `Point`, HSP state, clock messages, request IDs and notifications.
  The repository calls itself a draft; its concrete wire definitions were
  cross-checked against the upstream Buttplug implementation and manufacturer SDK.
- Manufacturer [API v3 client and types](https://gitlab.com/sweettechas/platform/handy-apiv3-bundle/-/tree/040beac8ae6d02e455e90d38996b73898534ccd2/src/_HANDYAPI),
  including normalized stroke-window fields.
- Manufacturer [streaming utilities](https://gitlab.com/sweettechas/platform/handyfeeling-utils/-/blob/190784911d0ef920a38decb52e1cf5594be6338e/ohdoki-sdk/src/hsp_utils.mjs),
  particularly zero-based absolute buffer indices.
- Manufacturer [legacy BLE client](https://gitlab.com/sweettechas/handy/handy-ble-client),
  which demonstrates firmware 3 Protocomm/handyplug. It is a different wire
  protocol from the firmware 4 RPC/HSP path and is not copied into that path.
- Buttplug v3 [enumeration](https://buttplug.io/docs/spec-v3/spec/enumeration/)
  and [watchdog/status](https://buttplug.io/docs/spec-v3/spec/status/) contracts,
  plus its [current Handy driver](https://github.com/buttplugio/buttplug/blob/93c4689dd944df12f47ad806299865badf6889d8/crates/buttplug_server/src/device/protocol_impl/thehandy_v3/mod.rs).

## Corrected defects

### Bluetooth wire units and shared sampler precision

The browser multiplied semantic HSP positions by ten. The manufacturer's
`Point.x` definition now explicitly states integer 0–100, with values above
100 clamped to the endpoint. A 25% target consequently became wire value 250,
which could turn most of a sweep into an endpoint hold. The codec now emits
integer percent, and the Go owner declares the actual 1% resolution to the
existing quantization-aware sampler. Semantic content and traces keep floats;
rounding precedes reverse mapping at the owner boundary.

Stroke-window floats are a separate contract: 0–1. The browser had emitted
semantic 0–100 directly. It now divides by 100 once, matching Cloud's existing
correct projection. Tests include a 1% lower bound to prevent unit guessing.
Cloud's existing HSP point scale was already correct.

### Shared HSP validation and indices

Both Handy owners now reject non-finite/out-of-range positions, non-increasing
append timestamps and values outside the firmware timestamp widths. Validation
happens before Cloud stream setup, so malformed input cannot reset the current
device buffer. The shared validation does not retime accepted points.

Cloud sent the point count as the tail index; the last point's zero-based index
is count minus one. Bluetooth omitted the absolute tail entirely. Both now
track the absolute index across appends and reset it for a new stream. Browser
splitting of 45 points ending at index 144 produces chunk tails 119, 139, 144.
This fixes progress/threshold bookkeeping without treating firmware's approximate
`current_point` as exact physical position.

### Bluetooth connection lifetime and clock behavior

React's persistent write queue could survive disconnect. A stalled native write
then blocked a replacement connection, while queued work could look up a new
characteristic through a mutable ref. `HandyBleSession` now owns an immutable
GATT pair and a queue for exactly one connection. Retirement aborts waiters and
removes the listener; delayed old promises cannot write to a replacement link.
A native write is bounded to one second. Failure fences motion and retains a
bounded last-resort Stop/disconnect path. A device chooser completing after
unmount cannot start a GATT connection.

All RPC requests now have distinct IDs. Streaming retains write-ack behavior
for firmware that omits optional replies; it does not wait for every firmware
completion. A correlated late rejection, including an error with only a numeric
code, retires the channel. Unknown late responses are ignored and tracking is
bounded to 64 recent submissions. Proto3 omitted zero-valued HSP fields decode
correctly, and unknown fixed64 fields no longer invalidate supported messages.

Clock synchronization uses the fastest of three samples instead of averaging
in a delayed sample. The three unconditional 60 ms sleeps were removed. If the
optional clock read is unavailable, Play omits `server_time`, as the protocol
allows, rather than using an unsynchronized UNIX timestamp. A synchronized Play
is stamped when it actually reaches the native writer. Cloud already omitted
an unmeasured clock; its owner now also clears caller-supplied timestamps before
applying its own measured offset.

### Intiface handshake and event ordering

The server watchdog begins at identification. Previously Ping began only after
RequestDeviceList completed, so slow enumeration could expire the session.
Ping now starts after ServerInfo. Oversized watchdog durations cannot overflow
Go's duration arithmetic, and connection failure during discovery cannot be
reported as successful setup.

The request goroutine could overwrite a newer hotplug event with the initial
device list, or set Scanning true after ScanningFinished. The reader now applies
matching response state before processing later events. An uncorrelated protocol
error retires the session instead of being silently dropped. Session I/O and
connection lifecycle are extracted into `intiface_session.go`; segment pacing
remains in `intiface_motion.go`.

### Cloud response and Stop handling

Cloud previously treated malformed HTTP 200 JSON as successful delivery. It now
rejects malformed/null/oversized responses and explicit `ok:false`; failed
appends cannot advance local buffer bookkeeping. Error bodies stay redacted.
Stop admission is rechecked between setup and append and between clock refresh
and Play, closing the window where a multistep request could issue motion after
Stop had already invalidated it. The existing serialization barrier and the
engine's cancellation path remain in place; ambiguously delivered motion is
never automatically retried.

### HTTPS cleanup race found by CI

The PR race run exposed an existing failure in the pending HTTPS base:
`TestPublicIPCertificateEndToEnd` could observe `ready` before deferred cleanup
released the temporary validation listener. A prompt restart could then contend
for that same port. Certificate preparation now returns from its listener-owning
helper before publishing success or failure. The strict success assertion stays
in place, with a failure-path release regression added. Five race-enabled
netaccess runs and 100 repetitions of each terminal path pass locally; no sleep,
retry or weakened assertion masks the ordering defect.

## Upstream Handy 2 issue

[Buttplug issue 893](https://github.com/buttplugio/buttplug/issues/893) reports
unequal rising/falling timing from `stop_on_target:true` in the Handy 2 HDSP
driver. Upstream changed it to false in
[commit 5a60f54](https://github.com/buttplugio/buttplug/commit/5a60f5492800dbe9bcb5e9443c4db0a04b7239e6)
on September 1, 2026. MagicHandy cannot alter that driver through standard v3
LinearCmd. Its installed Intiface/engine version must be recorded during
physical acceptance; this review does not claim a particular release includes
the upstream fix. No raw-driver bypass or direction compensation was added.

## Evidence and limits

All artifacts remain ignored under `.scratch/bluetooth-intiface-review/`:

- `atlas.json` and `atlas-plots/`: 204 shared-engine cases, 181 distinct plots,
  twelve overview sheets. All overviews were inspected, with detailed Full
  sweeps 10/43, Soft turnarounds 43 and free-roaming Creative 85 comparisons.
  The whole-percent output follows the compiled shape; expected piecewise
  velocity steps remain. No recipe, LLM contract, speed profile or selection
  changed, so no new model-selection trials were introduced.
- `transport-samples.json`, `simulation-traces.json`, `wire-metrics.json` and
  `bluetooth-wire-comparison.png`: six Full sweeps captures through the real
  engine with an in-memory fake, comparing old/new advertised resolution at
  10/25/43. An artificial 12-second prebuffer and one-hour dispatch tick capture
  the initial window before immediate Stop. They do not measure radio latency.
  Documented firmware clamping would affect 86/112, 143/189 and 213/287 old
  points; corrected points all fit 0–100. Counts fall to 84/141/228.
- Trace latency in those in-memory captures is the fake's declared zero delay.
  Separate local-WebSocket regressions hold an ACK beyond the next 80 ms segment
  and verify pacing proceeds independently; transport latency accounting also
  retains the synthetic 35 ms bridge-delay regression. None is real BLE timing.
- Full Go and race logs, focused transport/HTTP checks, final binary vulnerability
  scan, bundle budgets and real review-model readiness output.

The frontend passes 658 tests, TypeScript, localization and production build.
Go tests/race, vet, Windows/Linux lint, import boundaries and the pure-Go build
pass. The current review app uses an isolated simulated datastore with local
Ollama `huihui_ai/granite4.1-abliterated:3b`; a real generation returns `ready`.
See the [scorecard](goal-scorecard.md) for artifact/startup measurements.

Intentionally unchanged: motion sources, recipe geometry, speed limits, stroke
calibration, reverse semantics, immediate dispatch deadlines, the 50 ms Intiface
sampling floor, 1.5 s interactive / 5 s media BLE lead, and no silent transport
fallback. Physical feel, real adapter throughput and firmware-specific Stop
behavior still need an operator-controlled device comparison. Record device,
firmware and Intiface versions, transport mode, limits, latency, starvation
events and a sanitized trace for that comparison.
