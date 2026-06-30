# Motion Retargeting Spec

## Purpose

This spec defines how MagicHandy changes active motion while a device is already moving. It exists because hard resets, stationary bridge holds, stale buffer assumptions, and phase-zero restarts caused repeated motion regressions in StrokeGPT-ReVibed.

The goal is not to make every retarget invisible. The goal is to keep motion continuous, bounded, explainable, and recoverable.

## Terms

Semantic target:
The app-level motion request: speed intent, depth/focus, range, label, pattern, or anchors.

Plan:
A repeatable motion description that can be sampled over time.

Active stream:
The currently scheduled transport output and its associated semantic plan/target.

Handoff time:
The future playback time where replacement output becomes authoritative.

Lead time:
The minimum future buffer time required so a replacement reaches the device before it is needed.

Bridge points:
Short transition points added before or around handoff to avoid jumps. Bridge points must not become a long stationary hold.

## Active Stream Representation

The motion engine must track at least:

- active plan identity
- active semantic target
- active generation ID
- stream start monotonic time
- semantic phase offset
- transport stream offset
- last scheduled tail time
- last scheduled tail index
- recent command latency samples
- last known device playback state, if available
- whether the stream is healthy, paused, starved, rejected, or stale

The current semantic target is not the tail of a future HSP buffer. For active continuous playback, current target estimates come from the active plan clock and sampled phase.

## Retarget Inputs

A retarget can be triggered by:

- LLM motion update
- mode planner update
- quick settings speed-limit change
- quick settings stroke-range change
- reverse-direction change
- explicit user motion command
- transport recovery
- pattern or area-focus change

Every retarget must record a trace reason.

## Lead Time Policy

Replacement points must be scheduled far enough into the future to survive observed command latency.

The lead time should be derived from:

- rolling recent command latency
- fixed safety padding
- minimum lead floor
- maximum lead cap
- transport type
- current device playback state, when available

A replacement must not schedule its first required point only a few milliseconds ahead of estimated playback time. If the app cannot schedule enough lead, it should either continue the old stream briefly or enter a recovery path instead of sending expired points.

## Handoff Selection

For same-pattern retargets:

- preserve phase
- adjust speed/range/depth without restarting at phase zero
- avoid changing semantic phase just because physical settings changed

For new-pattern retargets:

- choose a candidate phase whose sampled position is close to the current sampled position
- prefer candidates that do not immediately oppose current travel direction
- avoid candidates at near-hold segments if they would feel like a stop
- use bounded bridge points only when needed

For area-focus retargets:

- treat regions as emphasis ranges, not hard lock points
- move into the new region smoothly
- do not command a fast jump to the exact center/start of the new region

## Bridge Point Rules

Bridge points are allowed when they reduce discontinuity. They must obey:

- bounded depth delta
- bounded range delta
- no long stationary hold unless the user explicitly requested a hold
- no reset to a stale endpoint
- exact point at replacement stream time when needed to prevent snap
- trace annotation marking bridge points

## Settings Retargets

Speed-limit changes:

- preserve semantic intent when possible
- retarget active physical output immediately
- do not feed physical velocity back into semantic target

Stroke-range changes:

- update the active transport envelope immediately
- preserve semantic depth/focus meaning
- do not pre-bake the new envelope into every HSP point

Reverse-direction changes:

- update transport-boundary mapping immediately
- preserve semantic phase
- do not reverse pattern phase unless the user explicitly requested phase reversal as a future feature

## Recovery Behavior

If device state reports paused, starved, rejected, or past buffered tail:

- do not keep appending points to a stream the device is not playing
- attempt resume/play only when the transport state supports it
- otherwise stop the active stream and report recovery failure
- preserve diagnostics with command path, status, error, and playback state

## Trace Requirements

Every retarget trace should include:

- source
- retarget reason
- previous plan identity
- next plan identity
- previous semantic target
- next semantic target
- estimated current sampled position
- selected handoff time
- lead time
- recent command latency summary
- whether phase was preserved
- whether bridge points were inserted
- transport command result

## Initial Implementation Allowance

The first version may use a conservative retarget algorithm, but it must be explicit, testable, and instrumented. If a case cannot be smoothed safely, the engine should choose a visible recovery path and diagnostics rather than pretending the transition is smooth.
