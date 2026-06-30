# ADR 0004: Frontend Strategy

## Status

Accepted for the rewrite plan.

## Context

The rewrite's primary goal is maintainability. The Go core only owns the
backend, so the frontend does not improve unless it is addressed deliberately.

StrokeGPT-ReVibed's frontend is roughly 13,000 lines of hand-rolled vanilla JS,
including a large shared `state`/`el` registry in `context.js`, an ~1,900-line
`motion-control.js`, and an ~1,900-line motion training editor. That registry
and the duplicated client-side motion state are themselves a meaningful part of
the maintainability debt the rewrite is meant to escape. Porting it verbatim
would carry the debt straight across and defeat the goal for half the codebase.

The current app also has a deliberate no-heavy-build ethos (no `npm install`, no
bundler at runtime), which keeps distribution simple and pairs well with Go
embedding assets into a single binary.

## Decision

Rebuild the frontend fresh, minimal-first, and backend-state-driven. Do not port
the StrokeGPT-ReVibed JavaScript wholesale. Use it as a reference for UX, markup,
and proven behaviors, not as a code base to copy.

- The visualizer and motion UI read motion-engine state from the backend. Do not
  recreate a duplicated client-side motion state model.
- UI state is derived from a backend state stream plus local pending/error state
  for in-flight commands. Optimistic display is allowed only when it is visibly
  marked pending and reconciled with the next backend state update.
- Commands flow through typed HTTP/WebSocket actions into backend APIs. The
  frontend never constructs raw Handy transport payloads.
- Quick settings (speed limit, stroke range, direction) apply immediately to
  active motion.
- Build the minimal core flows first (connection, manual motion, quick settings,
  emergency stop, diagnostics, trace export, visualizer) per Phase 8, then grow.

Detailed layout, components, feedback, accessibility, and the specific
StrokeGPT-ReVibed flaws this UI avoids live in `docs/ui-design.md`.

## Build And Tooling

- Keep the no-heavy-build ethos: assets are embedded into the Go binary; running
  the app must not require a Node runtime or a bundler server.
- A framework is optional but must produce static, embeddable output with no
  runtime build step. Bias strongly to minimal.
- Default: small modular ES modules with clear ownership and no global mutable
  god-registry. The exact framework question (if any) is decided concretely at
  Phase 8, defaulting to no framework unless a specific need is shown.
- The maintainability norms in `docs/goals-and-guardrails.md` (file-size limits,
  no god-module) apply to `web/`.

## State And Command Model

Recommended default:

- backend-to-UI state uses SSE or WebSocket snapshots/events
- UI-to-backend commands use small typed POST/WebSocket messages
- every command has an ID so success/failure can reconcile pending UI state
- one active controller lease owns motion commands; read-only clients can watch
  state and trigger emergency stop only
- stale backend state is a first-class UI state, not guessed by client timers

## Scope And Deferral

- The heavy authoring UI (pattern studio / training editor, Phase 14) is the
  largest frontend risk. It is explicitly deferred and may remain partial at the
  Phase 17 parity review.
- Feature-parity gaps in the UI are expected and acceptable until the motion core
  and core flows are proven.

## Consequences

Positive:

- Frontend maintainability actually improves instead of being inherited.
- No god-registry; client reads backend truth instead of guessing state.
- Distribution stays simple (embedded static assets, single binary).

Negative:

- More upfront UI work than copy-pasting the existing JS.
- UX parity gaps until later phases, especially pattern authoring.
- A framework decision is deferred, so an early choice could still be revisited.

## Revisit Criteria

Reconsider a framework or a larger frontend investment only if:

- the minimal modular approach starts reproducing god-module size in `web/`
- pattern authoring (Phase 14) cannot be delivered maintainably without one
- a packaging or offline requirement makes the current approach insufficient
