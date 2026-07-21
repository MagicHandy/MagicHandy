# ADR 0010: Transport-Neutral Motion Frames And The Intiface Dispatch Owner

## Status

Accepted and implemented in Phase 14B. Matched live Handy runs validated the
pre-asynchronous-ACK Intiface and Cloud REST transport/trace paths. The deadline-driven
asynchronous-ACK pacer added in PR #67 still needs a matched `motion_trace.v3`
hardware run and subjective feel confirmation. Revises the dispatch-owner scope
of ADR 0006; everything ADR 0006 dropped stays dropped.

## Context

Intiface Central (buttplug.io) is the de-facto open device layer: it owns the
radios and drivers for a wide range of devices and exposes them to
applications over a local websocket speaking the Buttplug protocol (spec v3).
Supporting it broadens MagicHandy beyond the Handy to the linear-actuator subset
of the Buttplug-compatible ecosystem, and it is a defining capability of the LSO
codebase the project intends to merge with
(`docs/lso-merge-alternatives.md`, Decision 2).

ADR 0006 says "exactly one transport family — HSP — over two dispatch
owners" (Cloud REST, Browser Bluetooth). An Intiface owner speaks a different
wire protocol, so before adding it the question had to be answered: **does
the motion handling schema need modification to produce relatively
consistent output whether motion goes over browser Bluetooth, Intiface, or
the Handy v3 API?**

## Evaluation

What the schema guaranteed at decision time (verified against `main` commit
581e5833):

- **One engine, one frame.** Every motion source (chat, Freestyle, patterns,
  funscript programs) flows through the shared sampler into
  `transport.TimedPoint{position_percent, time_ms}` chunks
  (`internal/motion/dispatch.go`), plus four transport methods then named
  `SetStrokeWindow`, `AddHSP`, `PlayHSP`, and `Stop`
  (`internal/transport/types.go`). ADR
  0002 keeps intent semantic and pushes stroke window and reverse direction
  to the transport boundary; the engine emits the semantic 0–100 relative
  span and both current owners invert/reproject from the same settings
  exactly once.
- **The frame maps cleanly onto Buttplug.** The Buttplug linear-actuator
  primitive is `LinearCmd(Index, Duration, Position 0..1)`. Consecutive
  timed points `(p_i, t_i) → (p_(i+1), t_(i+1))` become
  `LinearCmd(position p_(i+1)/100, duration t_(i+1)−t_i)` issued at stream
  time `t_i`. `Stop` maps to `StopDeviceCmd`. Absolute-time,
  absolute-position point streams are the correct common denominator — the
  same stream drives all three paths.
- **The real differences are delivery semantics, not frame shape:**
  1. *Buffered vs immediate.* HSP hands a buffered timeline to the device
     (server-time sync, device-side starving reports). Buttplug is
     immediate-mode: the host must pace each command in wall time. Pacing is
     an owner-internal concern. Semantic `CommandResult.LatencyMillis` measures
     queue admission; the owner separately records each scheduled time, actual
     write completion, lateness, effective duration, and acknowledgement RTT.
  2. *Health reporting.* Buttplug has no starving signal. The Intiface owner
     must detect its own underrun/backpressure and report an honest
     `playback_state`; recovery follows the existing stop-and-report rule —
     never a silent fallback (ADR 0006).
  3. *Device-side limits.* The Handy clamps velocity in firmware; generic
     Buttplug devices vary widely. The June 2026 motion-feel budgets (PCHIP
     sampling, acceleration and reversal-gap budgets, cycle floors, dwell
     floors) are enforced generator/engine-side, so every transport inherits
     the same shaped motion. That stays load-bearing: transports must never
     resample or reshape.

## Decision

**1. No structural schema change.** The timed-point stream plus
stroke-window/play/stop control commands remain the canonical
transport-neutral frame. An Intiface owner fits behind the existing
`transport.Transport` interface without changing its shape.

**2. Two modest schema modifications landed with Phase 14B:**

- **Neutral naming.** The interface and command kinds said HSP (`AddHSP`,
  `PlayHSP`, `CommandKindHSPAdd`, …), which wrongly implied the frame itself was
  Handy-specific. Phase 14B renamed them to transport-neutral terms
  (`AppendPoints`, `Play`; kinds `points_add`, `points_play`) with a
  diagnostics/trace naming migration. HSP remains the name of the *Handy
  encoding* of the frame, applied inside the two Handy owners.
- **Float positions.** `MotionSample.PositionPercent` and
  `TimedPoint.PositionPercent` widened from `int` to `float64`. Pattern content
  and PCHIP sampling were already `float64` (`internal/motion/content.go`); the
  old sample boundary rounded to whole percent. Cloud REST API v3 still
  requires integer `PointPosition`, but firmware-v4 Bluetooth protobuf exposes
  a native 0..1000 point scale and Buttplug-side actuators may resolve still
  finer positions. Slow shallow strokes visibly stair-step when precision is
  discarded early. Each owner quantizes only at encode time: Cloud rounds to
  integer percent, Browser Bluetooth maps the semantic float to 0..1000, and
  Intiface divides by 100. The JSON field stays `position_percent` (a number),
  so traces, the UI visualizer, and stored content are unaffected.
  Cloud declares its 1% endpoint resolution to the shared engine, which may
  remove redundant rounded knots under a combined 0.8% wire-error bound. This
  keeps curve fitting in the one shared motion path rather than in the owner.

**3. Per-owner obligations.** Consistent output across owners is a contract,
not an accident. Every dispatch owner — Cloud REST, Browser Bluetooth,
Intiface — must satisfy the same tested invariant table:

| Obligation | Handy owners | Intiface owner |
| --- | --- | --- |
| Stroke-window projection, exactly once | device-side (stroke window command) | host-side scale before `LinearCmd` |
| Reverse mapping, exactly once, at boundary | `x → 100−x` at point encoding | same, snapshotted when points enter the owner |
| Stop preempts everything queued | HSP stop | `StopDeviceCmd` + pacer flush |
| Honest health, no silent fallback | device starving reports | pacer-detected underrun |
| No resampling/reshaping of the frame | already tested | same tests, parameterized |

The Intiface delivery policy does not create a second motion model. It uses
absolute monotonic deadlines for the neutral segments, establishes the first
point with one generation-guarded startup anchor, and preserves the segment endpoints.
Acknowledgements are correlated asynchronously so response latency cannot delay
the next deadline. A late segment is shortened only within a 25% bound; expired
segments are discarded instead of burst-replayed, and anything later becomes a
reported starvation followed by Stop. These are immediate-mode delivery and
initial-condition rules, not semantic interpolation or resampling.

`DeviceMessageTimingGap` is exposed as a transport capability. The shared engine
raises its neutral sample interval when a selected device requires a slower
message cadence, including a bounded scheduler margin. `StepCount` remains a
reported physical-resolution limit; MagicHandy keeps the neutral float position
and does not add a second quantizer.

The existing HSP invariant suite generalizes into an owner-agnostic contract
suite run against all owners (the fake transport, the Cloud builder, the
Bluetooth builder, and the implemented Intiface owner).

Implementation note: the durable/API field remains `hsp_dispatch_owner` for
settings compatibility. New UI and documentation call it "Dispatch owner";
changing the persisted field name would add migration risk without changing
the neutral transport contract.

**4. Intiface owner scope.** Connect to a user-run Intiface Central over a
local websocket (default `ws://127.0.0.1:12345`, address configurable),
Buttplug message spec v3, one selected linear-actuator device initially.
Vibration/rotation mapping (`ScalarCmd`) is deferred to a later slice.
Buttplug ping keepalive is honored — it is a safety feature: the server
stops devices when the client dies.

The pacer keeps at most 64 unsent segments and eight unacknowledged linear
commands. Every response has a transport-owned deadline. A missing or rejected
response invalidates that playback generation, prevents stale responses from
affecting a newer generation, attempts an acknowledged `StopDeviceCmd`, and
marks the owner stale if Stop cannot be confirmed. Stop/Close invalidate before
their wire barrier, so an old `LinearCmd` cannot follow the final Stop.

**5. Dependency.** A pure-Go websocket client is required
(`github.com/coder/websocket` — CGO-free, maintained). This is the first
non-SQLite runtime dependency; accepted as clearly better than hand-rolling
RFC 6455.

**6. LSO merge alignment.** This resolves `docs/lso-merge-alternatives.md`
Decision 2 as **option A — first-class dispatch owner** (the owner selector
already makes it opt-in per user). Phase 14B landed independently; future LSO
integration must adapt to this interface and owner rather than add a parallel
Buttplug implementation.

## Consequences

Positive:

- The linear-actuator subset of the Buttplug/Intiface device ecosystem becomes
  reachable while the motion engine, planners, patterns, and safety semantics
  stay untouched.
- The contract stops implying Handy-only semantics; the same frame is
  provably consistent across three delivery paths, and the Handy itself is
  reachable through all three — which gives a direct like-for-like
  consistency measurement during validation.
- Fine-resolution devices benefit from the float positions the sampler
  already computes internally.

Negative:

- A third owner in the safety matrix: Stop, owner-switch, goroutine
  lifecycle, and diagnostics coverage all extend to Intiface (risk R22).
- The neutral-naming migration touches diagnostics kinds and trace
  vocabulary; one mechanical refactor with a migration note.
- An immediate-mode pacer is genuinely new machinery with its own failure
  modes (timer drift, underrun) that buffered HSP never had; it needs its
  own soak evidence before default recommendation.
