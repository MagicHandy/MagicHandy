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

All filters are off by default. With them off, the authored timeline stays exact.
Accepted buffered points cannot be rewritten, so the player freezes its clock,
stops the active run, serializes the settings write, and re-arms at the held
timestamp. A failed write leaves playback stopped and restores backend values.
Closing the panel preserves an already requested save; an old player's queued
save cannot manipulate the next player. Stop and controller loss still revoke
pending resumptions.

#### Smoothing (jitter removal)

`motion.SmoothMediaReversals` reuses the shared O(n log n) reversal bookkeeping
with a conservative media predicate: both sides of a removed excursion must be
at most the selected 1–5 percentage points, and both adjacent flanks must be at
most 250 ms. A tiny dip beside a major peak cannot remove that peak first. Slow
subtle excursions and long holds are preserved. The local time window no longer
depends on total remaining script duration. Playback rate scales flank duration;
seeking can still change the immediate slice boundary.

Pattern-library stabilization retains its existing predicate. Media smoothing
does not apply an amplitude floor, center contraction, or loop retiming.

#### Round peaks

The 0–200 ms window controls local interpolation around direct direction changes.
The shared engine uses its existing Hermite polynomial evaluator to join each
linear body with continuous position, velocity and acceleration, with zero
acceleration at both joins. This replaces the earlier five-point quadratic
approximation, whose straight-line segments still jumped in velocity.

Each symmetric window is limited to 40% of its shorter adjacent leg, retaining
at least 20% exact linear travel between neighboring windows. Windows below
4 ms are skipped and counted. Deliberate plateau/dwell shoulders remain exact;
they are not claimed to be smooth. The overall video clock and source points
do not change, and rounding cannot raise the incoming/outgoing peak rate.

Rounding reduces peak reach. Unequal incoming/outgoing slopes can shift the
local apex in time, so the readout reports both costs. The optional speed cap
applies to authored deltas first; rounding then joins the resulting limited
curve. This keeps the two controls independent, but the combined result can
reach less far than the previous cap-after-inserted-points implementation.

Sparse polynomial windows and fitter landmarks are compiled separately from the
bounded 100,000 source points. Dense scripts no longer silently lose rounding
because inserted source points would exceed that limit. Work remains linear for
rounding; sampling locates each window by binary search. This is still the one
engine/sampler/sanitizer/transport path, with no new background loop.

The continuous plan is a commanded estimate. Whole-percent buffered output can
flatten tiny tips and has piecewise-linear velocity. There is no promise that
arbitrary fast scripts meet a global acceleration/jerk envelope. Physical feel
and alignment remain hardware acceptance work. See
[ADR 0027](decisions/0027-media-filter-interpolation.md) and the
[numerical and visual review](funscript-filter-review-2026-09-06.md).

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

The backend reports the applied smoothing, rounding and speed-limit policy with
the active run's measurements: removed actions, rounded corners, peak reach
reduction in percentage points, peak timing shift in milliseconds, shortened
windows and skipped corners. Rounding measurements come from the compiled
speed-limited curve. Compact results survive engine snapshots without copying
the entire media timeline on every poll.

The panel distinguishes filters off, a pending settings write, waiting for a
matching active run, measured zero, and measured changes. Old measurements are
not attributed to newly displayed settings. Controls follow backend snapshots
and save responses; a late response cannot overwrite a newer draft.

The timeline strip continues to display authored content. An overlay of the
compiled filtered curve remains a separate follow-up, not an implied feature.

## Placement and behavior

- **Trigger** is a compact gear at the far right of the funscript title/action
  row. The timeline collapse chevron sits beside it; both use 30 px square
  buttons with localized accessible labels and tooltips. The gear tooltip
  retains the effective sync offset, and its active state marks an open panel.
  The controls stay together on narrow screens while the title and metadata wrap.
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

The floating panel, additive setup/per-video offset, independent opt-in filters
and backend effect report are implemented. The offset adjusts the video clock
without rewriting accepted points; filter changes use a coordinated restart.
The September 6 filter review replaces coarse point rounding with continuous
local interpolation, protects major authored reversals during smoothing, and
makes save/readback and measured-zero states explicit. See the linked review
for test, atlas, simulator and performance evidence.

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
