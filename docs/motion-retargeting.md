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
The legacy trace name for a bounded continuity transition around handoff. The
current engine uses a path blend rather than one stationary bridge sample.

## Shared Sampling And Smoothing Protections

Every motion source -- plain chat targets, Freestyle, mode planners, trained
patterns, and imported scripts/programs -- runs through one shared sampler and
sanitizer. Per-source motion paths were a root cause of mode-specific motion
bugs in StrokeGPT-ReVibed: a protection added for one caller did not reach the
others.

The shared path must:

- apply the same generation/stop/pause boundary to every source, so any motion
  is interruptible and replaceable the same way
- cap velocity against the current user maximum-speed setting, not only against
  pattern-local speed
- split large depth jumps and protect against oversized single steps
- smooth turn apexes and direction reversals
- sample with wall-time-parameterized monotone interpolation (PCHIP /
  Fritsch-Carlson style) that yields an exact zero-velocity instant at reversal
  knots and a cyclic derivative through non-reversing loop seams; do not use an
  index/phase-parameterized Catmull-Rom spline, which reintroduces instantaneous
  velocity at reversals
- for buffered owners, merge authored knots with 25 ms probes and simplify the
  emitted frame to at most 0.3 percentage-point vertical error; reject more
  than 128 essential points in a nominal frame instead of flooding a transport
- when an owner declares a coarser endpoint resolution, run a second
  quantization-aware reduction in the shared engine. Cloud's 1% endpoint scale
  uses a combined 0.8% bound to remove dwell/catch-up plateaus; higher-resolution
  owners keep the semantic frame
- for immediate-mode owners, honor the selected device timing floor and do not
  inject authored knots that violate its minimum command interval
- never weaken hardware safety clamping (speed, range, step size) for
  convenience or smoothness

Mutating transport calls are serialized and tagged with the engine run epoch.
Stop first invalidates the epoch and cancels the run context, then waits for an
in-flight append/setup call to drain before issuing the final wire Stop. A
request-originated retarget waiting behind that barrier cannot attach itself to
a later run. If an append fails with an uncertain acceptance state, the engine
stops the run explicitly instead of advancing past the missing chunk.

A new motion source is added by producing a semantic target/plan for this path,
never by building a parallel sampler or transport path.

### Phase 14 content semantics

Phase 14 adds two content shapes to the shared path:

- A **pattern** is a repeatable curve authored over a semantic 0–100 relative
  span. The motion engine samples its wall-clock knots with PCHIP, then the
  transport maps that semantic result into the configured stroke window exactly
  once. The library never preprojects points into the physical window.
- A **program** is a finite curve that retains its original knot timing and
  relative spacing. The player uniformly scales that timeline through the same
  bounded intensity/speed control used by the engine; it does not rewrite or
  loop the imported actions. Reaching its final knot causes an engine-owned
  explicit Stop; the completed phase remains 1.0 for an honest readout, and a
  new Start is rejected until that Stop returns.

Routine patterns are stretched in time to a 6600 ms minimum cycle while burst
patterns retain a 500 ms floor. This changes time only, never amplitude. The
generated catalog is checked against wall-clock acceleration and reversal-gap
budgets. Stopped or paused playback freezes semantic phase instead of allowing
the UI estimate to advance while no motion is commanded.

## Route Policy Learned On Hardware

StrokeGPT-ReVibed's June 2026 real-device testing produced a route policy that
MagicHandy must not rediscover. The regression: routing routine chat retargets
(plain "faster"/"slower", numeric adjustments, active focus changes) through
flushed HSP stream replacements produced repeated stop/go, "brick wall" feel on
hardware even after buffering and cadence fixes. Two design hazards were
identified in replacement-based morphs:

- A spatial morph blends from the predicted old position into a new moving
  cyclic sample; depending on the selected phase, the blend can cancel part of
  the cycle and create near-hold segments.
- A replacement scheduled at a fixed future lead can land while the old stream
  is moving opposite to the new target, which feels like hitting an endpoint.

Policy for MagicHandy:

- **Prefer adjusting the active plan over replacing the active stream** for
  routine changes: speed, stroke range, direction, and area-focus emphasis
  should retune the running plan and transport envelope (stroke-window,
  timestamp spacing) rather than flush-and-replace the timed stream.
- **Reserve stream replacement for genuine pattern changes**, and when
  replacing, follow Invariant 11 sequencing (`docs/hsp-v4-invariants.md`) plus
  the handoff-selection rules below.
- **Localize wide semantic focus requests** before dispatch: `tip`, `shaft`,
  and `base` become bounded local transport windows, not raw broad targets.
  Trace both the requested and the transport-resolved focus window so a
  "motion left the requested region" report is diagnosable.
- **Ramp envelope changes at a capped transition speed**, then restore the
  requested speed once the new window is active; do not trade a morph stop for
  a sudden high-speed correction into a new focus area.
- **Keepalive/recovery restarts motion only when the transport is actually
  inactive** or the active stream reports stale playback — never because state
  from a previous, already-replaced stream looks starved.
- **Higher-level variation goes through a planner contract**, not per-turn
  stream replacement: a mode or LLM requests a bounded arrangement of named
  styles, focus regions, durations, and intensity drift, and deterministic
  code compiles it with explicit transition rules (see IMPLEMENTATION_PLAN.md,
  Phase 11). LLM-facing contracts never expose transport details such as HSP
  replacement, morph duration, or phase offsets.

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
- use the effective path (including a transition already in progress) for phase
  selection, then apply the bounded continuity transition only when paths differ

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

The current implementation crossfades the old effective path into the new plan
for 750 ms with a smootherstep weight. The weight has zero first derivative at
both boundaries, so the handoff does not introduce a position or velocity snap.
Because two moving curves can crossfade into a small extra turn, the final
transition frame removes rapid reversals at or below 2% prominence while
protecting its start and end. The old `bridge_points=true` trace annotation is
retained for compatibility.

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

## Phase 7 Validation Workflow

`go run ./cmd/retarget-validate` runs the hardware checklist against Cloud REST
HSP with an enforced automated speed cap of 40 percent. The default maximum is
35 percent. The private Handy connection key is read from
`MAGICHANDY_HANDY_CONNECTION_KEY` or stdin; the public API v3 application ID is
bundled and can be overridden with `-app-id`.

The runner exports cumulative trace JSON files for:

- area changes while already moving
- speed changes while already moving
- stroke range changes while already moving
- reverse direction changes while already moving
- same-pattern changes that preserve phase
- cross-pattern changes that choose a low-jump handoff
- emergency stop after retargeting

The exported traces are written to `traces/` by default and are intended as
review artifacts or future fixtures. They must not contain the private Handy
connection key.

## Current Limitations

- The main UI exposes motion controls (Phase 8) and the engine binds to the
  selected live dispatch owner. The Cloud REST and Browser Bluetooth browser
  UI/chat paths have both been validated on real hardware in Phase 9B. Browser
  Bluetooth still requires a real browser user gesture, an active tab, and a
  stable browser-owned GATT session.
- The Cloud REST transport now follows the live API v3 wire shape used by
  StrokeGPT-ReVibed, but MagicHandy's motion timing, phase selection, and
  retarget policy are independent Go implementations.
- Browser Bluetooth is still a dispatch-owner bridge, not the source of motion
  behavior. The experimental Python Bluetooth motion path from ReVibed is not
  treated as a reference implementation because its physical motion was poor.
- Pattern and program sampling now uses the Phase 14 PCHIP implementation and
  shared engine path. Automated curve, projection, completion, stop, and
  lifecycle checks pass. Authored-knot/adaptive-frame checks now cover subtle
  focus windows and loop seams; the 6.6 s routine floor still needs a capped
  real-device feel check because that threshold was hardware-derived.
- Stroke-window and reverse changes remain immediate owner-envelope operations.
  Their interaction with points already buffered by HSP needs matched hardware
  traces before introducing a separate ramped-envelope contract.
- Recovery currently stops and reports unhealthy playback states when the
  transport reports paused, starved, rejected, or stale playback. More nuanced
  resume/play recovery can be added after real-device traces prove the state
  transitions are reliable.
