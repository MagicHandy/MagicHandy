# HSP v4 Invariants

## Purpose

These invariants preserve lessons learned from StrokeGPT-ReVibed's Handy firmware v4/API v3 work. They should become executable tests before live transport behavior is built on top of MagicHandy.

## Invariant 1: HSP Position Units

HSP timed points use Handy's current `0..100` position units.

Do not scale HSP `x` values to `0..1000` unless upstream Handy documentation and real-device traces prove the schema changed.

Test expectation:

- generated HSP point payloads never contain `x` outside `0..100` for normal sampled motion
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
- generated bridge/handoff points do not snap to stale endpoint or phase-zero position without explicit recovery reason

## Invariant 8: HSP Unavailable Is A Clear Error, Not A Fallback

If firmware v4/API v3/HSP prerequisites fail, the app reports HSP unavailable with a clear, actionable error and does not move the device. There is no legacy fallback transport (see ADR 0006); the user fixes settings rather than being silently downgraded.

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
- flush replacement points through the add path and update the tail threshold
- do not replay play while firmware reports a healthy active stream
- keep flushed replacement indexes and thresholds local to the newly added
  buffer; do not carry old stream indexes into a flushed replacement
- schedule the first replacement point far enough ahead to cover recent command
  latency, or a healthy firmware clock skips the bridge points before they arrive

Test expectation:

- a replacement does not issue a new play on a stream firmware reports healthy
- replacement indexes/threshold are buffer-local, not inherited from the old stream
- the first replacement point's lead is at least the recent command-latency estimate
