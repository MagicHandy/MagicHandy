# ADR 0038: Freestyle as a continuous generated stroke stream

Date: 2026-10-01

Status: Proposed for PR review

## Context

Freestyle still chose a library pattern every few seconds and sometimes changed
its speed midway through that segment. The intended behavior is fluid motion
that keeps developing without restarting, with useful control over its feel.
Changing between unrelated repeated patterns makes that intent difficult to
express and spends transport runway on handoffs. Style also coupled several
preferences into one choice.

## Decision

Freestyle produces a seeded semantic stroke stream in `internal/motion`, carried
as `FlowSpec.Freestyle`. The existing mode lifecycle supplies overlapping finite
windows of that stream. Each stroke depends on its index, seed and bounded
control keyframes. Overlapping windows reproduce the same strokes and fitted
timing, so the existing engine can continue at the same stroke and phase.

This is content for the shared compiler, sampler and sanitizer. Freestyle owns
no transport, device payload, playback goroutine or separate safety envelope.
Each window is finite and stops if planning disappears. The existing user
intent, cancellation, pause, recovery and global Stop mechanisms still apply.
The old segment planner remains available for Autopilot's existing fallback.

Pace, length, focus, roaming, variety and direction accent are durable backend
preferences. Gentle, Balanced and Intense name server-owned presets; editing a
control selects Custom. Pace is a tendency within the saved speed band,
including bounded variation, rather than a new physical limit. Focus uses the
existing depth frame: base 0, tip 100. All output is mapped through the saved
stroke range and selected Handy profile by the existing shared path.

Steady, Slow build, Waves, Edge and Cooldown are explicit session shapes. Their
clock counts running, unpaused time; changing shape starts its arc from zero.
Slow build and Cooldown accept 1–240 minutes. Their stroke-timed ramps are
approximate arcs, not precise wall-clock deadlines. Cooldown finishes with a
short finite wind-down. Once that wind-down is queued it completes; controls
are disabled during the visible ending state and later saved preferences never
revive it. An explicit Start begins a fresh stream.

Preference changes start after the engine's queued handoff and ease across
several unqueued strokes. Compaction retains the surrounding timing context
and the earlier controls read by seeded event blocks. Dropping history solely
because a ramp has settled before the playhead changes existing strokes and
is prohibited by a regression test.

The frontend renders backend settings and shape status. It holds only local
form edits and serialized pending writes. Queued edits merge their changed
fields onto the server's acknowledgement, including its authoritative preset
values. The stroke clock and generated windows stay out of browser snapshots.

## Alternatives and consequences

- Keeping the pattern/segment loop would retain abrupt changes between repeated
  shapes and provide less direct customization.
- A private native or browser motion loop would violate ADR 0002 and duplicate
  Stop, buffering and safety responsibilities.
- Generating an unbounded curve would make cancellation and resource use harder
  to bound. Windows cap stroke and keyframe counts and retain bounded history.
- Seeded variation is reproducible for diagnostics; explicit starts choose a
  fresh seed. It is independent of LLM inference and introduces no dependency.

Existing installs initialize the separate Freestyle feel from their saved
motion style. Chat motion contracts, transport policies, physical calibration
and hard safety constants do not change. Software plots and fake-transport
continuity are acceptance evidence for the implementation; real-device feel
and transport latency still require operator feedback.

See [the implementation review](../freestyle-stream-review-2026-10-01.md).
