# HSP v4 Invariants

## Purpose

These invariants preserve lessons learned from StrokeGPT-ReVibed's Handy firmware v4/API v3 work. They should become executable tests before live transport behavior is built on top of MagicHandy.

## Invariant 1: HSP Position Units

The shared engine and Cloud REST API use semantic `0..100` position units. API
v3 Cloud `PointPosition` is an integer, so the Cloud owner rounds only while
building its request. Firmware-v4 Bluetooth protobuf uses a native `0..1000`
integer field; the browser owner rounds to its corresponding 0.1% semantic step
at the bridge boundary before the codec maps it to that wire scale. The
`0..1000` value must never leak back into the engine, stored content, HTTP
bridge body, or diagnostics.

Test expectation:

- generated HSP point payloads never contain `x` outside `0..100` for normal sampled motion
- Cloud payloads quantize to whole percent, while Browser Bluetooth quantizes
  to 0.1% and maps that value to the native 0..1000 protobuf field
- Cloud's owner-declared 1% resolution may reduce redundant knots only in the
  shared engine and only under the combined 0.8% wire-error bound
- quantize before reverse mapping so forward and reversed output are exact
  mirrors in native endpoint steps
- one Cloud `/hsp/add` contains at most the API v3 limit of 100 points
- intentional invalid data is rejected before transport dispatch

## Invariant 2: Stroke Range Is A Transport Envelope

Local stroke-depth calibration and user stroke range should not be pre-applied to every HSP point.

HSP point positions describe the sampled motion in HSP units. Physical stroke window/range is applied through the relevant stroke-window transport command.

Test expectation:

- changing stroke range emits/updates a stroke-window command
- sampled HSP points remain in semantic `0..100` units

## Invariant 3: Reverse Direction Is Transport-Boundary Mapping

Reverse direction is a physical mounting/orientation setting.

It must invert outgoing physical positions at the transport boundary without changing semantic target meaning or pattern phase.

Test expectation:

- semantic target remains unchanged after reverse is toggled
- outgoing transport positions are inverted
- sampled phase is preserved for same-pattern active streams

## Invariant 4: Semantic Speed Is Not Physical Velocity Feedback

LLM/user speed percent is intent. HSP timed-point spacing and point deltas encode motion speed for HSP streams. Physical velocity calculations must not overwrite semantic speed and then feed back into the motion engine.

Test expectation:

- semantic target speed remains the requested/clamped intent speed
- transport-specific physical velocity fields do not alter stored semantic state

## Invariant 5: HSP Timestamp Spacing Is The Speed Contract

HSP stream timing must preserve authored or sampled timing. Do not stretch HSP timestamps through point-to-point velocity budgets intended for direct-position transports.

Test expectation:

- HSP point timestamps follow the sampled plan timing
- buffered frames include authored knots and a bounded adaptive approximation,
  rather than relying on an unrelated fixed tick to happen near each reversal
- speed limits do not flatten HSP point slopes into a fixed direct-position velocity budget

## Invariant 6: Same-Pattern Updates Preserve Phase

A same-pattern update, including speed changes and settings refreshes, should preserve semantic phase.

Test expectation:

- same-pattern update does not restart at phase zero
- trace marks phase preservation

## Invariant 7: New-Pattern Retargets Choose Low-Jump Handoff

A new-pattern replacement may change shape, but it should not blindly start at phase zero. It should choose a handoff phase that minimizes jump and avoids immediate opposing-direction turns when practical.

Test expectation:

- replacement phase selection considers current sampled position
- generated 750 ms continuity transitions do not snap to a stale endpoint or
  phase-zero position without an explicit recovery reason

## Invariant 8: HSP Unavailable Is A Clear Error, Not A Fallback

If firmware v4/API v3/HSP prerequisites fail, the app reports HSP unavailable with a clear, actionable error and does not move the device. There is no legacy fallback transport (see ADR 0006); the user fixes settings rather than being silently downgraded.

A successful HTTP status alone is not an HSP connection check. The response
must contain a recognized positive availability, success, or playback-state
signal; empty, malformed, and unrelated JSON bodies are unavailable. Errors
from the SSE endpoint are sanitized before diagnostics because its required
query string contains the private connection key.

The API v3 Application ID and the Handy connection key are distinct. The app may
ship a public Application ID, but diagnostics still need to distinguish a missing,
invalid, revoked, or overridden Application ID from a malformed user connection
key. An Application ID failure is not a connection-key problem.

Test expectation:

- missing/invalid API v3 Application ID reports HSP unavailable and dispatches no motion (there is no HDSP path)
- malformed connection key reports a specific validation error
- auth failure marks HSP unavailable

## Invariant 9: Active Settings Changes Refresh Motion Immediately

Active speed-limit, stroke-range, and reverse-direction changes must update active motion without waiting for a later LLM or mode retarget.

Test expectation:

- changing speed limits while active emits a refresh/replacement or active transport update
- changing stroke range while active emits a stroke-window update/replacement
- changing reverse direction while active refreshes outgoing transport mapping

## Invariant 10: Diagnostics Must Preserve Transport Truth

Transport diagnostics should report command path, status, elapsed time, safe body fields, error text, and device playback state when available. Speed-only visualizer fields are not transport truth.

Test expectation:

- diagnostics expose safe transport metadata
- secrets are redacted
- trace rows distinguish planner waits from transport/API rejection

## Invariant 11: HSP Stream Replacement Sequencing

Replacing the active stream (pattern swap, retarget) must reuse the active HSP
session, not tear it down. Real Cloud HSP sessions reported stop/go playback
after morphs when this was done wrong, and it recurred repeatedly.

- bridge into an exact point at the active stream's replacement time
- append replacement points at the future handoff and update the cumulative
  tail threshold; only the first add of a newly set-up HSP stream uses `flush`
- do not replay play while firmware reports a healthy active stream
- keep flushed replacement indexes and thresholds local to the newly added
  buffer; do not carry old stream indexes into a flushed replacement
- schedule the first replacement point far enough ahead to cover recent command
  latency, or a healthy firmware clock skips the bridge points before they arrive

Test expectation:

- a replacement does not issue a new play on a stream firmware reports healthy
- replacement indexes/threshold are buffer-local, not inherited from the old stream
- the first replacement point's lead is at least the recent command-latency estimate

## Invariant 12: Playback Time Starts At Accepted Play

Setup, initial buffering, browser bridge work, and Intiface anchoring are not
elapsed playback. Each owner reports its best monotonic stream origin after
Play is accepted. The engine uses that origin before starting lead management
or selecting retarget handoffs.

Test expectation:

- setup/add latency does not advance semantic phase before playback starts
- Cloud and Browser Bluetooth use the successful Play request/acknowledgement
  midpoint; Intiface uses its completed startup anchor
- API v3 numeric `play_state` values are mapped to named health states
- stopped/not-initialized is tolerated only while starting; it triggers
  recovery Stop during an established run
