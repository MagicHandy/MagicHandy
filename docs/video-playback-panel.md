# Floating Playback Panel — Design Sketch

## Status

**Built.** A floating settings panel scoped to the video currently open in the
player, styled after the connection manager, holding a per-video sync offset and
a small set of funscript filters. This document stays the design record; the
"What shipped" section reports what each part actually does.

Sketch: [video-playback-panel-sketch.svg](video-playback-panel-sketch.svg).
Sync behavior and its constraints: [video-playback.md](video-playback.md).

## Why a panel and not more Settings

The script offset shipped into `Settings > Media` because that is where media
settings live. It is the wrong place to *use* it: calibrating an offset means
adjusting it while watching and feeling the result, and Settings is two routes
away from the picture. The same is true of any filter — "does this script feel
better smoothed" is a question you answer against a playing video, not from a
form.

So the panel is not a second settings surface. It is the same values, reachable
without leaving the video, in the one context where their effect is observable.
`Settings > Media` keeps them for the case where someone wants to set a value
once and never think about it again.

## Scope

**In:** sync offset, script filters, filter effect readout, reset.

**Out:** library locations, transport/connection, speed and stroke limits (the
connection manager already owns those and is reachable on this route), anything
that is not observable while a video plays.

The offset is **per-video**, because that is how it is actually used. Filters
stay global for now; if per-video filters turn out to be the normal case too,
they follow the offset into the same row.

## Contents

### 1. Sync offset — per-video, live

A slider over ±2000 ms with a numeric readout, matching
`config.MaxScriptOffsetMillis`. Positive delays the device against the picture.

#### Two offsets, added, because they have two different causes

The total offset a setup needs comes from two independent sources, and they
change on different schedules:

- **Your setup** — display presentation latency and device actuation lag. Constant
  across every video, and it changes when you change hardware.
- **This script** — the author's sense of where the beat sits. Different for
  every file, and the reason the offset is normally adjusted per video.

So the effective offset is `setup + this video`, not one overriding the other.
Collapsing them into a single per-video number would mean re-calibrating every
video in the library after changing a monitor. Keeping them separate means the
setup value is measured once and the per-video value stays small and honest —
it is the script's bias and nothing else.

The panel shows both and the sum, so a surprising total is explainable:

```
This video   −70 ms          [────●─────────]
Setup        −150 ms                 Effective −220 ms
```

The panel's slider adjusts **this video**. The setup value is displayed as
context with a link to `Settings > Media`, where it belongs: it is a property of
the room, not of the file.

#### Storage

A `script_offset_ms INTEGER NOT NULL DEFAULT 0` column on `media_videos`, added
as schema v14. The step is a guarded Go hook rather than a bare `ALTER`,
because SQLite has no conditional `ADD COLUMN` and re-running a migration over
an already-current database is a normal recovery path here. It is bounded metadata about a catalog row, which
is what that table already holds — no new table, and a video that has never been
calibrated reads 0 and simply inherits the setup value.

Rows survive rescans the way the rest of the row does; a video that goes missing
and returns keeps its calibration. Removing the video removes the offset, which
is correct — it was a fact about that pairing.

#### Why it is live

The first implementation applied the offset by moving the slice point in
`Funscript.TimelineFrom`, so changing it stopped the run and required Play
again. That is correct but it made the slider useless for calibration: you
cannot feel a 40 ms change if every drag stops the device.

It is applied without touching buffered points instead. The video is not
locked to its own clock — it is locked to `expected_media_time_ms`, the engine's
transport-aligned projection. Adding the offset to *that projection* moves the
video relative to the device without rebuilding a single point:

```
expected_media_time_ms = anchor + offset + running × rate
```

The offset is added to the drift comparison as well, or the shifted video would
read as constant drift and trip the soft breach. Folding it into the runtime's
anchor does both at once.

Two consequences accepted deliberately:

- Each adjustment nudges the video (a correction seek), so dragging is visibly
  steppy. Debouncing the write and only re-aligning on release keeps that to one
  seek per gesture.
- The device never moves for an offset change. That is the point, and it should
  be visible: the readout says the *video* shifted.

The write is debounced by 180 ms so a drag is one request, and the response
carries the recomputed projection so the picture moves on release rather than up
to a heartbeat later.

### 2. Script filters — applied when the run re-arms

Each filter changes the points the device receives, and accepted HSP points
cannot be rewritten. So every filter change stops an active run, the same way a
speed-policy change does. The panel says so once, at the group heading, rather
than on each control.

The panel writes to `POST /api/media/playback`, which runs the same settings
transition a full save does, so an active run stops the way a speed-policy
change already stopped it. Whether the player's existing resync makes that gap
seamless enough, or whether the group eventually wants an explicit Apply, is a
question for real use rather than for review.

**Every filter is off by default and the defaults are authored-exact.** This is
not a style preference; it is a recorded lesson. A media amplitude transform was
shipped once, and it "collapsed ordinary subtle strokes toward the device's
resolution floor and made each true zero-velocity reversal feel like a dwell"
([motion pathway review](motion-pathway-review-2026-07-20.md), 2026-07-22). A
filter that quietly makes scripts worse is worse than no filter.

#### Smoothing (jitter removal)

Removes rapid, insignificant extrema — the noise in motion-tracked and
auto-generated scripts — while leaving deliberate detail alone.

`motion.StabilizePatternReversals(points, prominence)` already does exactly
this and is already exported: it drops an extremum only when its prominence is
below the threshold **and** its shorter flank is under 250 ms, so a slow
deliberate 2% excursion survives while a 2% spike does not. Pattern import uses
it at a fixed 2.0; the panel exposes the threshold over a small range (roughly
1–5 percentage points) with the readout in percentage points, not an abstract
"strength".

The honest framing in the UI is "removes jitter smaller than N%", because that
is precisely what it does.

#### Round peaks

Media curves are linear by design — funscript segments interpolate linearly,
matching the authored format. Every reversal is therefore a perfect corner.

That is right for a script tracking real motion on screen, where the corners are
what actually happened. It is often wrong for a script that is really a *pattern*
— a hand-written sawtooth or triangle, which reverses instantly at every peak in
a way no body does. This filter is for those: it rounds the vertex so the stroke
approaches its extreme and turns, moving a triangle toward a sine.

A slider sets the rounding window in milliseconds — how much time either side of
each direction change is smoothed. Small values barely soften the tip; large
values make the whole stroke sinusoidal. Each corner is capped at a fraction of
its shorter adjacent leg, so dense fast sections round proportionally less than
sparse slow ones without the user managing that.

The vertex is replaced by a quadratic fillet, emitted as ordinary knots so
linear interpolation stays intact:

```
   authored:  A ────────→ B(peak) ────────→ C     corner, instant reversal
   rounded:   A ──────→ ⌒⌒⌒⌒⌒⌒⌒ ──────→ C        peak approached and turned
                        └─ r ─┘
```

Properties, and the ways this one can go wrong:

- **Timestamps are unchanged.** Only knots are inserted between them.
- **Peak velocity does not increase.** The straight body keeps its authored
  slope and the fillet only reduces speed toward the turn, so the fastest
  moment of the stroke is no faster than before and acceleration becomes finite
  where it was unbounded. This filter asks *less* of the device, not more.
- **Peak position is reduced, and that is the real cost.** A corner cannot be
  rounded without cutting it; the fillet's apex sits short of the authored
  extreme by an amount proportional to the window and the approach slope. Bound
  it, and show it: the readout states the largest reduction the current setting
  produces, in percentage points. This is *not* the removed 2026-07-22 amplitude
  defect — that contracted every position toward the centre and flattened subtle
  strokes everywhere; this touches only the neighbourhood of a direction change
  and leaves the body of the stroke exact — but it is adjacent enough that the
  cost has to be visible rather than argued.
- **Point count grows.** Each rounded corner costs several knots instead of one.
  Bounded against `MaximumMediaTimelinePoints`; a script dense enough to exceed
  it gets fewer knots per corner, or the filter declines with a clear reason
  rather than silently degrading.
- **It makes tracked-motion scripts mushy.** Applying it where the corners were
  real is exactly the wrong use. Off by default, and the timeline overlay is how
  someone sees that before feeling it.

An earlier draft of this section proposed the opposite mechanism: the bounded
trapezoid `withBoundedLoopReversalGuides` uses for loop patterns, which
*preserves* the peak exactly and pays for it with faster mid-stroke travel. That
is the right tool for a pattern whose amplitude is the point, and the wrong one
here — it keeps the harshness the user is trying to remove, and it raises
velocity on exactly the fast scripts where that is least affordable. Recorded so
the two do not get confused: patterns ramp velocity to keep their reach; media
rounds position to lose the corner.

#### Limit speed to the motion maximum

Surfaces `motion.apply_video_speed_limit` and labels the actual configured
maximum rather than merely saying "on". The limiter clips each authored
segment's displacement to the semantic rate budget while preserving that
segment's timestamp and direction. It never adds travel the script did not
request. This replaces the earlier absolute-target chaser: after one clipped
rise, a small authored fall could still make the old output rise quickly toward
the stale target, which felt faster and contradicted the script's reversal.

Fixed video timing makes one tradeoff unavoidable: when an over-limit segment
cannot reach its authored endpoint, later positions can be compressed or
offset. The panel therefore says travel is capped without changing the video
clock. It does not imply that both the original range and original timing can be
preserved under a lower speed ceiling.

### 3. Effect readout

One line under the group: how many of the script's actions the current filters
change, e.g. `1,842 actions · smoothing removes 214`. Computed from the same
transform the engine will apply, not estimated.

This is the panel's honesty mechanism. A filter that reports "changes 0 actions"
tells the user it is doing nothing on this script; one that reports "changes
1,600 of 1,842" tells them it is rewriting the script, and they can decide
whether that is what they wanted. It also gives the timeline strip something to
draw against later — showing filtered vs authored on the same curve is the
obvious next step once the numbers exist.

## Placement and behavior

- **Trigger** in the funscript strip's header row, beside `Hide timeline`, so it
  sits with the other playback-scoped controls rather than in the app chrome.
  It carries the active offset as its label (`Sync −150 ms`) so the value is
  visible without opening anything.
- **Panel** anchored to the trigger, same visual treatment as
  `.connection-manager-panel`: `position: fixed`, `min(360px, 100vw − 16px)`,
  `--surface` on `--line-strong`, `--radius-sm`, `--shadow`, scrollable at
  `max-height`. Anchoring to the player rather than the viewport corner keeps it
  from covering the picture on short windows; on mobile it should become a
  bottom sheet, as the connection manager already does at its breakpoint.
- **Dismissal and focus** copy the connection manager exactly: Escape closes,
  outside click closes, focus moves into the panel on open and returns to the
  trigger on close, `aria-expanded` / `aria-controls` on the trigger.
- **Read-only tabs** see the panel with controls disabled and the existing
  visualization-only labeling. It must not become a second way for a
  non-controller tab to reach the device.
- **Emergency Stop** is unaffected and unobstructed. The panel never overlaps
  the pinned Stop, and no control inside it can start motion.

## Deliberate non-goals

- **A minimum-movement deadband.** ScriptPlayer replaces queued commands when
  transformed positions differ by less than 10; this project rejected that
  because "it can erase an entire subtle MagicHandy focus window". Recorded here
  so it is not proposed again as "smoothing".
- **Amplitude or range scaling.** The stroke window already does this correctly
  at the device envelope, for every source. A media-side amplitude transform is
  the removed 2026-07-22 defect.
- **Per-video filter profiles.** The offset is per-video because its dominant
  term is per-script; a filter choice is closer to a taste setting. Revisit if
  use says otherwise.
- **Anything that edits the file.** Filters are playback transforms; the paired
  script on disk is never rewritten.

## What shipped

All four slices landed together rather than as four PRs, because the panel is
not usable without the offset being live and the offset is not worth a panel
without something to calibrate against.

- **Panel** — trigger in the funscript strip carrying the effective offset as
  its label; a fixed overlay layer (`pointer-events: none`) with the panel
  inside it, so it covers the workspace without blocking anything it does not
  occupy. Measured 288x321 px against the sketch's 374x514. Escape and outside
  click close it, and a read-only tab sees every control disabled.
- **Per-video offset** — schema v14 column, `POST /api/media/script-offset`,
  written on a 180 ms debounce so a drag is one request. Verified live: dragging
  saved -180 for the file, the trigger label followed, and the panel reported
  `this video -180 ms · setup 0 ms`.
- **Live offset** — the calibration moved out of the slice point and into the
  sync runtime's anchor, so `expected_media_time_ms` and the drift comparison
  both carry it. Changing it during playback does not stop motion; the response
  recomputes the projection so the video moves on release rather than up to a
  heartbeat later.
- **Filters** — smoothing reuses `motion.StabilizePatternReversals`; peak
  rounding is the quadratic fillet described above. Both re-arm the run, both
  report their measured effect, and the zero value is authored-exact.

Peak rounding was measured rather than argued: tests pin that it never raises
peak velocity, that its reduction grows with the window and stays bounded, that
each corner is capped by its own shorter leg, and that a script too dense for
the fillets plays unrounded instead of being thinned to fit.

Not verified on hardware. How much rounding feels right, and whether smoothing
helps or flattens a given script, are subjective and need a device.

### Slice order as built

Each was one reviewable step; all landed in one PR.

1. **Panel shell and the per-video offset.** Trigger, floating panel,
   focus/dismiss behavior, read-only gating, schema v14 column, and the slider
   over `setup + this video` with today's re-arm behavior. Ships something
   usable and proves both the shell and the storage.
2. **Live offset.** Move the offset from the slice point to the engine-clock
   projection and the drift baseline; make the slider live. Regression test that
   drift stays near zero with a non-zero offset.
3. **Smoothing.** Wire `StabilizePatternReversals` into media timeline
   construction behind the setting, plus the effect readout.
4. **Peak rounding.** The fillet, its per-corner cap, the point-count bound, and
   the peak-reduction readout. Wants a measurement over real scripts — the
   largest peak reduction and the point growth at each end of the slider — but
   that is characterisation, not a gate it can fail.

All four are independently valuable. Slice 2 is what makes the panel worth
opening; slice 4 is the one whose cost has to stay visible in the UI.

## Open questions

- Does `requires_reanchor` re-arm smoothly enough for filter changes, or does
  the group need an explicit Apply?
- Should the offset readout show the *measured* residual drift beside the
  configured value, so calibration has feedback rather than only a setting?
- Should a script's offset be guessable? If most files from one source share a
  bias, offering the last-used value as the default for new videos from the same
  folder would save calibrating each one from zero.
- Does peak rounding want a shape control as well as an amount — how *much* of
  the corner is round versus how far the rounding reaches? One slider is the
  right starting point; a second only earns its place if the first cannot reach
  a setting people want.
