# Pattern Library

## Purpose

Phase 14 adds reusable motion content without adding another playback engine.
`internal/patterns` owns durable catalog data, import/export, authoring
transforms, and preference feedback. `internal/motion` remains the only owner of
sampling, active playback, completion, Pause, and Stop.

## Content Types

**Patterns** are repeatable relative curves. Positions are semantic 0–100 and
are projected into the configured stroke window exactly once at the transport
boundary. A stored pattern is either `routine` (minimum 6600 ms authored cycle)
or `burst` (minimum 500 ms authored cycle). Built-ins are code-generated; users
can author and share additional entries. The authoring floor is not a runtime
speed lock: playback retimes the complete curve while preserving its relative
rhythm.

**Programs** are finite, elapsed-time curves. A funscript imported as a program
keeps its action timing and relative spacing and does not loop. Playback may
uniformly time-scale the complete curve through the bounded speed control;
the stored actions are unchanged. The current engine applies a 500 ms minimum
execution period, so shorter stored programs play at that floor. The engine
samples it through the same path as patterns and sends an explicit Stop at
completion. A new Start is rejected until that Stop returns. Patterns and
programs have different SQLite tables, share
schemas, API routes, and engine definitions so callers cannot accidentally treat
one as the other.

Before adding, importing or retuning any pattern, read
[pattern-quality.md](pattern-quality.md). It is the plain-language account of why
two batches of patterns were rejected after being felt on hardware, the numeric
envelope that came out of it, and which script to reach for.

## Built-In Catalog

The built-in catalog contains 87 patterns: three established patterns
(`Stroke`, `Pulse`, and `Tease`), eight retained generated patterns, 15
velocity-authored replacements, two user-curated patterns, and 59 generated
Rockfire/Claude patterns that survived the quality pass. `experimental` is an
active review state rather than a historical label. Six designed patterns and
50 generated survivors carry it, leaving 31 patterns in the default model
catalog and all 87 available when the user opts into experimental patterns.

The 59 `curated-*` files under `internal/motion/builtinpatterns/curated` are the
survivors of a 171-file generated import. Nine clear the current source and
fitted-feel checks; 50 remain experimental. Generated curves receive no timing
or speed-limit exemption. The two promoted user curves (`Hard and Regular` and
`playful jerk`) preserve their selected geometry, but normal runtime speed
retiming and safety bounds still apply. `playful jerk` keeps its intentional
imported midpoint holds. `Hard and Regular` keeps the 0 -> 100 -> 74 -> 0 beat
with velocity-balanced leg timing; preserving its original timestamps made the
short return accent feel like a stop once global pace normalization was
introduced.

Generated filenames and database IDs remain stable because saved sessions,
enablement, weights, and feedback refer to them. Display names, descriptions,
and model-facing tags describe geometry and relative rhythm, not an implied
global pace. Model prompts receive opaque handles rather than the legacy
pace-biased IDs.

The source directory carries an explicit
`magichandy.generated-pattern-catalog.v3` manifest. Tests require every
manifest filename and display name to match the embedded source. The labelling
tool resolves the repository path, keeps that manifest synchronized, and does
not rename IDs or modify points. The Python sampler is offline-only and cannot
post to a running app. The sole bulk-import command is the Go utility, which
requires `-allow-experimental`. That acknowledgment does not promote or
hardware-approve a clip.

The 250-pattern persistence limit applies to user-authored, generated, and
imported rows only. Reconciled built-ins do not consume that capacity, so the
larger shipped catalog cannot reduce how many patterns an existing user may
save.

### The velocity-authored replacement pass

The user disabled 15 built-ins by hand, reporting that they lacked smooth
continuous motion on the device. Measuring them found two failure modes and one
shared cause.

**Five stalled.** The rendered curve spends a contiguous span under 30%/s:
`Cascade` 2.46 s of a 6.6 s loop, `Descending Ladder` 2.04 s, `Deep, Medium,
Short` 1.68 s, `Pendulum` 1.04 s, `Surge` 0.68 s. No retained pattern exceeds
0.15 s. **Ten never settled into a pace:** their slowest stroke averaged 33%/s
against 62%/s for the retained set, with a 3.4x internal speed spread against
2.3x, across 5.5 distinct stroke lengths against 3.0.

The shared cause is that `Positions` and `TravelMillis` were independent lists,
so stroke velocity -- the quantity the hand actually registers -- was never
designed. `Cascade` put a 14-unit stroke and an 84-unit stroke on nearly the same
duration, giving 18%/s next to 116%/s. A second contributor: `mustFitCatalog`
reaches `RoutineCycleFloorMillis` by scaling every timestamp, dividing every
velocity by the same factor, which slowed `Descending Ladder` 1.22x from its
authored 410-474 ms strokes.

Replacements are authored as a stroke velocity per travel, with the travel time
derived as amplitude divided by that velocity, and reach the cycle floor by
repeating their phrase rather than by stretching it. Cycle lengths run 6.7-12.4 s.

`scripts/pattern-designer.js` is that generator and is the place to add or retune
a built-in. It holds the turning positions and intended stroke velocity for each
entry, derives the travel times, checks the envelope, and reports any pair of
patterns too close in shape to be worth choosing between; `EMIT=1` prints
pasteable Go specs. Every shipped `TravelMillis` list reproduces from it exactly.
Its constants are a design aid and are deliberately tighter than the Go test,
which is the gate.

The prior screening rule asked for reach *variety* -- four amplitude bands, no
repeated endpoint, no run of two near-equal amplitudes. Measurement does not
support variety as the quality axis: the disabled patterns were the more varied
group. `TestCatalogPatternsHoldTheMeasuredSpeedEnvelope` replaces it with authoring bounds
taken from the retained patterns: every stroke at least 22% travel and 42%/s, at
most a 3.3x internal speed spread, at least 55%/s mean, and no more than 200 ms
under 30%/s. Runtime retiming preserves the ratios between legs while changing
their absolute rates. The 450 ms reversal-gap and 3000 relative-position/s2 acceleration
budgets are unchanged; together they imply the 22% minimum stroke, and they cap a
short stroke's speed at `amplitude / 0.45`, which is why long strokes are the fast
ones in every replacement.

These bounds admit all 13 retained patterns and reject 12 of the 15 retired ones.
They do not catch `Sway`, `Rolling`, or `Double Tap`, whose slowest strokes
(46, 54, 57%/s) sit inside the retained range; those three were the weakest part
of the case for removing them.

`Hard and Regular` and `playful jerk` are stored curves promoted from the live
user library. `playful jerk` preserves its exact knots and relative timing.
`Hard and Regular` preserves the accepted turning positions and 16-beat shape,
but its 26-point partial return now uses 70 ms of the authored 450 ms beat
instead of nearly the same time as the preceding 100-point stroke. They carry
the `curated` tag, use the same runtime speed semantics and persisted motion
envelope, and still play only through the shared engine. On an existing
database, seed reconciliation transfers enabled state and weight from an exact
name-and-curve match to the canonical built-in, then removes only that proven
duplicate. Similar names or edited curves are left untouched.

The seed also removes the six explicitly retired built-in IDs, including their
cascading feedback rows, and inserts the six replacement IDs. No SQLite schema
change is required. Seed reconciliation updates a built-in name only when it
still equals a known older default; a user rename remains user-owned. Existing
enablement and weights are also preserved. The library's inline rename control changes the
display name for any pattern and persists it across restart; IDs and built-in
curve content remain immutable, so chat and playback keep a stable contract.

## Sampling And Authoring

### Playback speed semantics

A pattern is a shape plus relative rhythm, not a fixed pace. At runtime the
planner measures total absolute travel in one loop and chooses a period that
maps the selectable 1–100 control through the selected Handy model's published
full travel and normal physical-speed range. For the Original Handy profile,
representative requests are 93.3%/s at 20%, 272.4%/s at 73%, and 363.6%/s at
100%; Handy 2 profiles account for their 125 mm travel and Standard/Pro maximum.
The same calibration applies regardless of whether the selected curve is
shallow, deep, simple, or highly articulated. The exact rendered curve is lengthened when it
would cross the runtime acceleration or reversal envelope; the configured user
maximum remains an independent cap. Area focus contracts travel and period
together where safety allows, so selecting a narrow region does not silently
make a pattern slower. The derivation and separate catalog/runtime envelopes are
recorded in [`motion-calibration.md`](motion-calibration.md).

Finite programs use direct clock scaling (`stored duration * 100 / speed`) and
retain every relative action interval. Video funscripts are media-clock-locked
and therefore do not use pattern normalization.

The default catalog is also sampled at 40% playback pace in tests. Except for a
pattern explicitly tagged with intentional holds, no non-experimental loop may
remain below 45% semantic travel per second for more than 250 ms. This catches
bad per-leg timing that a correct mean travel rate can otherwise hide.

- Curves use wall-time PCHIP/Fritsch-Carlson interpolation. Loop reversals use
  a trapezoidal velocity profile with at most 75 ms acceleration/deceleration
  guides on each side, preventing whole-stroke endpoint easing from feeling
  like a pause at slow speed. Tests require C1 continuity, no overshoot, exact
  authored extrema, zero velocity at the reversal instant, and a continuous
  cyclic derivative when a loop crosses its seam without reversing.
- Generated patterns are time-stretched until they satisfy a 450 ms reversal-
  gap floor and 3000 relative-position/s² acceleration budget. Stretching never
  changes amplitude.
- Freehand input is validated on the backend, sorted/deduplicated by time, and
  simplified by vertical error while preserving meaningful direction
  reversals. Rapid reversal chatter at or below 2% prominence is removed; a
  slow subtle excursion is retained. Raw input is capped at 4096 points and a
  saved pattern at 256 points including loop closure.
- Buffered playback combines authored knot times with 25 ms probes, then emits
  a 0.3%-error-bounded adaptive frame. This prevents fixed-grid aliasing of
  short reversals. Internal velocity guides shape interpolation but do not
  masquerade as authored wire knots. Cloud additionally declares its 1%
  endpoint resolution so the engine can remove redundant rounded plateaus
  under a combined 0.8% wire bound; Bluetooth and Intiface retain the finer
  semantic frame.
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
- **Pattern** compresses stationary gaps over five seconds to 500 ms, requires
  at least 5% source span, normalizes it once to relative 0–100, removes only
  rapid low-prominence reversal chatter, simplifies it, and closes the loop.
  Stored cycles shorter than 6600 ms are stretched to that authoring floor; longer active timing
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

The chat prompt receives enabled pattern metadata ordered by visible weight,
with geometry/rhythm names and tags plus a deterministic opaque handle. The
primary LLM motion vocabulary is `{pattern_id,speed_percent}`. Parsing resolves
the handle and rejects entries outside that supplied catalog; dispatch resolves
the resulting stable ID again to protect against a concurrent disable. Persisted
IDs, including legacy pace-biased generated IDs, never enter the prompt. When no
pattern is enabled, the semantic speed-only contract remains available.

Model permissions further narrow that catalog. Turning pattern selection off
removes pattern fields and skips the pattern-store read for the turn. Turning
experimental patterns off (the default) excludes 56 experimental rows while
retaining 31 accepted and user-curated built-ins. Area focus is
independent of catalog storage. These permissions persist in the existing
versioned settings document in SQLite and therefore do not add a table or
schema migration.

Interactive chat receives the current engine target and a bounded tail of
recent chat-selected patterns from the runtime trace ring, represented to the
model by the same opaque handles. Steady and
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

Built-ins are seeded idempotently on open while preserving user-renamed names,
enablement, and weights. Untouched legacy default names migrate to the current
shape-based labels. Explicit catalog retirement and exact-curve promotion also
run in that transaction. Library writes use the datastore transaction
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
available regardless of controller ownership or backend state. Pattern and
program play requests use `speed_percent`; the backend still accepts the old
`intensity` request field for compatibility and gives the canonical field
precedence when both are present.
