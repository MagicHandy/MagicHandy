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
share additional entries. The 6600 ms routine value is a minimum, not a maximum;
a coherent imported or authored loop may retain a longer cycle.

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

## Built-In Catalog

The built-in catalog contains 29 patterns: three established patterns
(`Stroke`, `Pulse`, and `Tease`), 18 accepted generated patterns, six new
experimental replacements, and two timing-preserved user-curated patterns.
`experimental` is now an active review state rather than a historical label:
retained patterns no longer carry the tag or the `Experimental:` description
prefix. Only `Deep, Medium, Short`, `Falling Crest`, `Three Deep, One Short`,
`Descending Ladder`, `Wandering Swell`, and `Rising Reach` are experimental.

The replacement pass used live library feedback from a connected device. The
six disabled patterns were `Deep Bookends`, `Lower Midrange Mix`, `Midrange
with Full Finish`, `Mid-to-Top Switch`, `One Deep, Three Shallow`, and
`Top-Anchored Depths`. Their shared weakness was not simply regularity. Each
repeated a nearly fixed endpoint; four mixed in 10-20% micro-strokes and the
other two repeated nearly identical 30-40% spans near the reversal floor. That
combination produced limited phrase variation and physically jittery or shaking
motion. Regular full-range motion remains a useful, deliberate behavior.

Replacement screening again reduced source action streams to reversal extrema
and considered only complete phrases whose final source travel closed onto the
first point. The six selected phrases come from six distinct source
fingerprints. Each generated replacement has at least 30% travel per stroke,
four amplitude bands, no endpoint band used more than twice, and no run longer
than two near-equal stroke amplitudes. Generator tests also retain the 450 ms
reversal-gap and 3000 relative-position/s2 acceleration budgets. Source
filenames played no role in selection, names, descriptions, or tags; source
paths, filenames, and payloads are not retained.

`Hard and Regular` and `playful jerk` are exact curves promoted from the live
user library. Their accepted timing is intentionally preserved instead of being
passed through the generated-pattern time fitter, which would change their
feel. They carry the `curated` tag, are bounded by the same persisted motion
envelope, and still play only through the shared engine. On an existing
database, seed reconciliation transfers enabled state and weight from an exact
name-and-curve match to the canonical built-in, then removes only that proven
duplicate. Similar names or edited curves are left untouched.

The seed also removes the six explicitly retired built-in IDs, including their
cascading feedback rows, and inserts the six replacement IDs. No SQLite schema
change is required. Existing names, enablement, and weights on retained
built-ins remain user-owned. The library's inline rename control changes the
display name for any pattern and persists it across restart; IDs and built-in
curve content remain immutable, so chat and playback keep a stable contract.

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
  playback. Compact pattern curves insert the backend-owned saved knots into
  those samples so long cycles cannot visually alias away reversals. React does
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
  Cycles shorter than 6600 ms are stretched to that floor; longer active timing
  remains intact. The UI rejects a selection with more than 255 essential
  reversal knots because loop closure and the stored 256-point limit make that
  shape impossible to preserve. This is a shape limit, not a duration limit.

Unknown schemas and unknown funscript targets are rejected. Imported bytes are
never sent to a transport or executed directly.

Funscript source time is normalized so the first action is zero. The Import tab
starts fitted to the complete source. Compact Earlier, Later, Zoom in, Zoom out,
Fit selection, and Fit all controls keep viewport changes discoverable without a
large editor toolbar. Vertical wheel input over the timeline zooms around the
cursor; horizontal or Shift-wheel input pans. At the fit-all and one-millisecond
zoom limits, outward wheel input is released to the page. A proportional
scrollbar below the plot supports direct dragging, track jumps, and standard
arrow/Page/Home/End keys. The focused timeline also supports `+`, `-`, `0`, and
arrow keys. The zoom viewport is independent of the trim selection and never
changes submitted content. Waveform, higher-contrast selection shading, and
fixed-size draggable action-snapped trim handles share one timeline coordinate
system. The visible selection length is therefore exactly the final selected
action time minus the first selected action time. Submission rebases that first
selected action to zero and preserves every selected program knot. MagicHandy
share files carry their own content kind and bypass this trim workflow.

## Curation And Feedback

The chat prompt receives only enabled `{id,name,description,tags,weight}` rows,
ordered by visible weight. The primary LLM motion vocabulary is
`{pattern_id,intensity}`. Parsing rejects IDs outside that supplied catalog;
dispatch resolves the ID again to protect against a concurrent disable. When no
pattern is enabled, the deterministic semantic speed-only contract remains
available.

Model permissions further narrow that catalog. Turning pattern selection off
removes pattern fields and skips the pattern-store read for the turn. Turning
experimental patterns off (the default) excludes the six replacement rows
while retaining all accepted and user-curated built-ins. Area focus is
independent of catalog storage. These permissions persist in the existing
versioned settings document in SQLite and therefore do not add a table or
schema migration.

Interactive chat receives the current engine target and a bounded tail of
recent chat-selected pattern ids from the runtime trace ring. Steady and
pacing-only requests preserve the current pattern. Explicit variation prefers
an enabled pattern outside the recent tail; one repair pass and a deterministic
fresh-pattern fallback prevent a weak model from returning a semantic no-op or
rapidly alternating the same two choices. The recent tail is runtime context,
not durable user data, so it is not migrated to SQLite.

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

Built-ins are seeded idempotently on open while preserving user names,
enablement, and weights. Explicit catalog retirement and exact-curve promotion
also run in that transaction. Library writes use the datastore transaction
helper. Runtime databases, WAL files, imports, and exports remain user data and
are never committed.

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
