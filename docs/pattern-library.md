# Pattern Library

## Purpose

Phase 14 adds reusable motion content without adding another playback engine.
`internal/patterns` owns durable catalog data, import/export, authoring
transforms, and preference feedback. `internal/motion` remains the only owner of
sampling, active playback, completion, Pause, and Stop.

## Content Types

**Patterns** are repeatable relative curves. Positions are semantic 0–100 and
are projected into the configured stroke window exactly once at the transport
boundary. A pattern is either `routine` (minimum 6600 ms cycle) or `burst`
(minimum 500 ms cycle). Built-ins are code-generated; users can author and
share additional entries.

**Programs** are finite, elapsed-time curves. A funscript imported as a program
keeps its action timing and relative spacing and does not loop. Playback may
uniformly time-scale the complete curve through the bounded intensity control;
the stored actions are unchanged. The current engine applies a 500 ms minimum
execution period, so shorter stored programs play at that floor. The engine
samples it through the same path as patterns and sends an explicit Stop at
completion. A new Start is rejected until that Stop returns. Patterns and
programs have different SQLite tables, share
schemas, API routes, and engine definitions so callers cannot accidentally treat
one as the other.

## Sampling And Authoring

- Curves use wall-time PCHIP/Fritsch-Carlson interpolation. Tests require C1
  continuity, no overshoot, and zero velocity at reversal knots.
- Generated patterns are time-stretched until they satisfy a 450 ms reversal-
  gap floor and 3000 relative-position/s² acceleration budget. Stretching never
  changes amplitude.
- Freehand input is validated on the backend, sorted/deduplicated by time, and
  simplified by vertical error while preserving every direction reversal.
  Raw input is capped at 4096 points and a saved pattern at 256 points including
  loop closure.
- Playback preview samples come from the same backend `motion.Curve` used by
  playback. The React authoring/training canvases render those samples; they do
  not implement playback interpolation. The import view's raw source-action
  plot is a file-inspection timeline, not a playback preview.
- Training's Original, Smooth, and Crisp choices produce temporary resolved
  definitions for audition. They never mutate stored points.

## Share And Import Contracts

One pattern exports as `.mhpattern.json` with schema
`magichandy.pattern.v1`. One finite program exports as `.mhprogram.json` with
schema `magichandy.program.v1`. Both contain a name and sparse `{time_ms,
position_percent}` points; patterns also contain kind, cycle, description, and
tags.

Standard funscript `{at,pos}` actions are accepted, including the standard
`inverted` flag. Source inspection is bounded to 8 MiB, 24 hours, and 20,480
actions; the selected payload and direct backend import are capped at 4096
actions. Positions must be finite 0–100 values and saved names are limited to 80
characters. The browser applies source bounds before rendering untrusted data;
the backend validates submitted content again. Malformed actions and metadata
are rejected rather than dropped, coerced, or clamped. The user chooses one of
two interpretations:

- **Program** preserves stored elapsed timing, relative spacing, and amplitude.
- **Pattern** compresses stationary gaps over five seconds to 500 ms, normalizes
  the usable span once to relative 0–100, simplifies it, and closes the loop.

Unknown schemas and unknown funscript targets are rejected. Imported bytes are
never sent to a transport or executed directly.

Funscript source time is normalized so the first action is zero. The Import tab
starts fitted to the complete source and provides keyboard-operable zoom, pan,
fit-selection, and fit-all controls. Its zoom viewport is independent of the
trim selection and never changes submitted content. Both trim bounds snap to
source actions, so the visible selection length is exactly the final selected
action time minus the first selected action time. Submission rebases that first
selected action to zero and preserves every selected program knot. MagicHandy
share files carry their own content kind and bypass this trim workflow.

## Curation And Feedback

The chat prompt receives only enabled `{id,name,description,tags,weight}` rows,
ordered by visible weight. The primary LLM motion vocabulary is
`{pattern_id,intensity}`. Parsing rejects IDs outside that supplied catalog;
dispatch resolves the ID again to protect against a concurrent disable. When no
pattern is enabled, the deterministic semantic speed/pattern contract remains
available.

A thumbs rating moves weight by 0.15 within 0.1–3.0 and records before/after
weight and enabled state in `pattern_feedback`. Undo restores the exact prior
state only when no later feedback or direct edit would be overwritten.
Auto-disable is off by default; when explicitly enabled, a negative rating may
disable an entry after its weight reaches 0.25 or lower.

## Persistence

SQLite schema v8 introduced (and later schema versions retain):

- `patterns` for built-in/generated/user loops and visible curation state
- `programs` for finite user/imported content
- `pattern_feedback` for the reversible rating ledger
- `app_kv['patterns.auto_disable']` for the opt-in preference

Built-ins are seeded idempotently on open while preserving user enablement and
weights. Library writes use the datastore transaction helper. Runtime databases,
WAL files, imports, and exports remain user data and are never committed.

Schema v8 also reconciles databases produced by the divergent `Rockfire`
branch. It preserves Rockfire-only motion-block, funscript, queue, persona, and
UI tables but does not interpret them as canonical patterns/programs. Mapping
those rows belongs to Phase 15/LSO migration, with a dry-run compatibility
report rather than implicit conversion at startup.

## HTTP Surface

- `GET /api/library`
- `POST /api/library/preview`
- `POST /api/library/import?filename=...&as=pattern|program`
- `POST /api/library/patterns`
- `PATCH|DELETE /api/library/patterns/{id}`
- `GET /api/library/patterns/{id}/export`
- `POST /api/library/patterns/{id}/play`
- `DELETE /api/library/programs/{id}`
- `GET /api/library/programs/{id}/export`
- `POST /api/library/programs/{id}/play`
- `POST /api/library/feedback`
- `POST /api/library/feedback/{id}/undo`
- `PUT /api/library/auto-disable`

Reads and downloads are available to read-only clients. Preview, import,
mutation, feedback, and playback require the active controller. Playback is
also bounded by the persisted speed/stroke settings; global Stop stays
available regardless of controller ownership or backend state.
