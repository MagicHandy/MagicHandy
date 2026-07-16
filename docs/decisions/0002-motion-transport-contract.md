# ADR 0002: Motion And Transport Contract

## Status

Accepted for the rewrite plan.

## Context

StrokeGPT-ReVibed motion regressions repeatedly came from mixing semantic intent, transport command shape, live sampled state, settings envelopes, and device recovery behavior. MagicHandy must make those boundaries explicit.

The motion engine must describe what motion should feel like. The transport
layer must describe how that motion is encoded for the selected device owner.
Settings such as stroke range and reverse direction are physical transport
constraints, not changes to the meaning of user or LLM intent.

## Decision

MagicHandy separates semantic motion intent from physical transport output.

Semantic motion intent includes:

- requested speed percent
- requested depth/focus region
- requested stroke range as an intent envelope
- motion label or pattern identity
- optional motion program/anchors
- mode or chat source metadata

Physical transport output includes:

- transport-neutral timed-point positions and timestamps
- stroke-window projection at the selected transport boundary
- local reverse-direction mapping
- owner-specific encoding (HSP for Handy owners, `LinearCmd` for Intiface)
- physical speed/velocity limits where the selected transport requires them
- transport command latency and recovery state

The motion engine owns target selection, plan sampling, active state, retargeting, and trace semantics. The transport layer owns command serialization, authentication, API calls, device state, and transport diagnostics.

## Contract Rules

- The LLM and mode planners can produce only semantic targets/plans, not raw Handy API commands.
- Semantic speed remains an intent percent until transport encoding.
- Speed is encoded through timed-point spacing and point deltas; each owner maps
  the neutral frame to its wire protocol without resampling it.
- Transport code may calculate physical duration, lead time, or safety budgets, but those values never feed back into semantic intent.
- Stroke range settings are physical envelope settings and are applied at the transport boundary.
- Reverse direction is a physical orientation setting and is applied at the transport boundary.
- Current semantic target is not inferred from the tail of a future HSP buffer.
- Active settings changes for speed, stroke range, or reverse direction must refresh active motion immediately.
- Emergency stop interrupts all motion loops, planners, and transports.

## Motion Engine Responsibilities

- Maintain the current semantic target.
- Maintain active motion state and plan identity.
- Sample plans into timed points or transport-neutral frames.
- Retarget active motion without hard resets when possible.
- Annotate trace rows with source, reason, target, sampled position, and transport result.
- Request transport recovery when device state reports paused, starved, rejected, or stale playback.
- Run every motion source (chat, Freestyle, modes, trained patterns, imported
  scripts) through one shared sampler/sanitizer; new sources produce semantic
  targets, not parallel motion paths. See `docs/motion-retargeting.md`.
- Never weaken hardware safety clamping (speed, range, step size) for convenience.

## Transport Responsibilities

- Validate credentials and firmware/API prerequisites.
- Serialize commands according to the selected transport.
- Send commands and capture safe diagnostics.
- Never log or export secrets.
- Report owner-specific prerequisites with a clear, actionable error; owners do
  not silently fall back to one another (see ADR 0006 and ADR 0010).
- Expose latest command status, latency, and device playback state.

## Emergency Stop Contract

Stop is global and explicit. It must:

- cancel active motion loop work
- stop mode planners
- clear pending retargets
- attempt the configured transport stop on every activation, including idle
  and no-engine states
- mark the engine stopped even if the transport stop fails
- surface stop failure in diagnostics
- advance the run/transport Stop epoch before waiting on in-flight work, reject
  commands admitted against an older epoch, and serialize the final transport
  Stop after every earlier mutating command has drained
- cancel both loop-originated and request-originated transport calls through
  the engine run context; a caller waiting behind Stop must recheck its run
  epoch before it can reach the transport

An append, stroke-window, or startup command can fail after the remote side has
accepted it. The engine therefore treats an unexplained mutating-command error
as an uncertain device state: it marks that run inactive and attempts an
explicit recovery Stop. It must not skip the failed chunk and continue the
timeline. Cloud REST and Browser Bluetooth also keep their own Stop admission
gate so direct transport users receive the same stale-command rejection as the
Intiface generation barrier.

This is a safety gate, not a convenience. It must be covered by goroutine-leak
and stop-teardown tests: after stop during an active retarget, zero motion
goroutines may remain commanding the device. See `docs/goals-and-guardrails.md`.

## Consequences

This contract prevents common regressions where:

- speed caps rewrite semantic speed
- reverse direction flips semantic phase
- stroke-range calibration is baked into HSP points
- active retargets restart at phase zero
- transport failures look like planner pauses
- modes bypass the shared motion path
- a queued append reaches the wire after Stop
- persisted settings report live success when their active transport refresh
  actually failed
