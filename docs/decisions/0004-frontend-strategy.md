# ADR 0004: Frontend Strategy

## Status

Accepted for the rewrite plan. Amended by
[ADR 0009](0009-react-frontend.md): React is now the selected frontend
implementation path.

## Context

The rewrite's primary goal is maintainability. The Go core only owns the
backend, so the frontend does not improve unless it is addressed deliberately.

StrokeGPT-ReVibed's frontend is roughly 13,000 lines of hand-rolled vanilla JS,
including a large shared `state`/`el` registry in `context.js`, an ~1,900-line
`motion-control.js`, and an ~1,900-line motion training editor. That registry
and the duplicated client-side motion state are themselves a meaningful part of
the maintainability debt the rewrite is meant to escape. Porting it verbatim
would carry the debt straight across and defeat the goal for half the codebase.

The current app also has a deliberate no-runtime-build ethos: the app must serve
static assets embedded in the Go binary, and running MagicHandy must not require
Node or a bundler server. ADR 0009 keeps that release property while allowing a
build-time React toolchain.

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
- React is an implementation tool, not a state-model change: the backend remains
  authoritative for motion, controller ownership, settings, prompt sets, memory,
  and diagnostics.

Detailed layout, components, feedback, accessibility, and the specific
StrokeGPT-ReVibed flaws this UI avoids live in `docs/ui-design.md`.

## Build And Tooling

- Keep the no-runtime-build ethos: assets are embedded into the Go binary;
  running the app must not require a Node runtime or a bundler server.
- Use React as decided in ADR 0009. It must produce static, embeddable output.
- Node is a development and CI build dependency only. Release artifacts remain a
  Go binary plus embedded static files.
- Preserve clear ownership and avoid a new global mutable god-registry. React
  contexts/hooks must stay narrow and backend-derived.
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
  largest frontend risk. Choosing React now is intended to reduce the later
  maintainability cost of that work, not to move Phase 14 into the shell PR.
- Feature-parity gaps in the UI are expected and acceptable until the motion core
  and core flows are proven.

## Consequences

Positive:

- Frontend maintainability actually improves instead of being inherited.
- React component ownership replaces ad-hoc DOM registries; client reads backend
  truth instead of guessing state.
- Distribution stays simple (embedded static assets, single binary).

Negative:

- More upfront UI work than copy-pasting or incrementally patching the existing
  vanilla JS.
- Adds a Node build step in development and CI, though not at runtime.
- UX parity gaps until later phases, especially pattern authoring.

## Revisit Criteria

Reconsider React only if:

- the static build cannot stay embedded/offline without runtime Node
- React state starts duplicating backend motion/controller truth
- bundle size or startup measurably violates the project budgets and cannot be
  fixed by ordinary code splitting or dependency trimming
