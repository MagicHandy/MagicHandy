# Floating Playback Panel — Design Sketch

## Status

**Plan only — nothing here is built.** This proposes a floating settings panel
scoped to the video currently open in the player, styled after the connection
manager, holding the sync offset and a small set of funscript filters.

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

The panel is per-session UI, not per-video state: values are the global media
settings. Per-video overrides are a plausible later step — some scripts need
their own offset — but they need a storage decision (a `media_videos` column vs
a settings map) that should not be made implicitly by building the panel first.

## Contents

### 1. Sync offset — live

A slider over ±2000 ms with a numeric readout, matching
`config.MaxScriptOffsetMillis`. Positive delays the device against the picture.

**This should become a live control, which today it is not.** The shipped
implementation applies the offset by moving the slice point in
`Funscript.TimelineFrom`, so changing it stops the run and requires Play again
(`stopMediaForPolicyChange`). That is correct but it makes the slider useless
for calibration: you cannot feel a 40 ms change if every drag stops the device.

There is a way to apply it without touching buffered points. The video is not
locked to its own clock — it is locked to `expected_media_time_ms`, the engine's
transport-aligned projection. Adding the offset to *that projection* moves the
video relative to the device without rebuilding a single point:

```
expected_media_time_ms = anchor + offset + running × rate
```

The offset must be added to the drift comparison as well, or the shifted video
reads as constant drift and trips the soft breach. Both live in
`observeHeartbeatTiming`, so it is one consistent change, not two.

Two consequences worth accepting deliberately:

- Each adjustment nudges the video (a correction seek), so dragging is visibly
  steppy. Debouncing the write and only re-aligning on release keeps that to one
  seek per gesture.
- The device never moves for an offset change. That is the point, and it should
  be visible: the readout says the *video* shifted.

If the projection approach is rejected, the fallback is the current behavior
with honest labeling ("applies on next play") — but then the slider belongs in
Settings and this panel loses most of its reason to exist.

### 2. Script filters — applied when the run re-arms

Each filter changes the points the device receives, and accepted HSP points
cannot be rewritten. So every filter change stops an active run, the same way a
speed-policy change does. The panel says so once, at the group heading, rather
than on each control.

The existing `requires_reanchor` path can make that nearly seamless: stop, then
let the player's resync arm a fresh run at the current video time. The user sees
a brief motion gap, not a reset to a paused player. That reuse should be
confirmed early in implementation — if it does not hold, the group needs an
explicit "Apply" button instead of applying on change.

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

#### Soften direction changes

Media curves are linear by design — funscript segments interpolate linearly,
matching the authored format. Every reversal is therefore a perfect corner
demanding infinite acceleration, and what the device does with that is its own
business rather than something the script specified.

The pattern engine already solved this shape: `withBoundedLoopReversalGuides`
replaces a corner with a bounded trapezoid whose ramp length comes from an
acceleration budget (see the 2026-07-24 review). It is gated to `loop && !linear`
so media never sees it, and it should stay that way — media should get the same
*shape* emitted as real knots, keeping linear interpolation intact:

```
   authored:  A ─────────────────→ B(peak) ─────────────→ C
   softened:  A ──→ M1 ═══════════→ M2 ──→ B(peak) ──→ M3 ═══...
                    (ramp)  (faster cruise)   (ramp)
```

Non-negotiable properties, each of which is a way this can go wrong:

- **The peak is preserved exactly.** Clipping it would be the amplitude bug
  again, wearing a different name.
- **Timestamps are unchanged.** Only knots are inserted between them.
- **The ramp is bounded and cannot eat the stroke body.** A short leg gets a
  proportionally shorter ramp or none.
- **Cruise velocity rises.** The area under the velocity curve is fixed by the
  endpoints, so slowing at the ends means going faster in the middle. On fast
  scripts this can exceed what the device can physically do, which makes the
  interaction with the speed cap below the main thing to measure. If it cannot
  be bounded safely, this filter does not ship.

#### Limit speed to the motion maximum

Surfaces the existing `motion.apply_video_speed_limit`, unchanged: a causal
forward slew limiter that clips only over-limit segments and never touches
timestamps. It is already the one media filter with shipped behavior and tests,
and it belongs in the same group as the others rather than only in Settings.

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
- **Per-video filter profiles**, until the storage decision above is made.
- **Anything that edits the file.** Filters are playback transforms; the paired
  script on disk is never rewritten.

## Suggested slices

Each is one reviewable PR.

1. **Panel shell.** Trigger, floating panel, focus/dismiss behavior, read-only
   gating, and the offset slider bound to the existing settings field with
   today's re-arm behavior. Ships something usable and proves the shell.
2. **Live offset.** Move the offset from the slice point to the engine-clock
   projection and the drift baseline; make the slider live. Regression test that
   drift stays near zero with a non-zero offset.
3. **Smoothing.** Wire `StabilizePatternReversals` into media timeline
   construction behind the setting, plus the effect readout.
4. **Direction-change softening**, only if the velocity bound above can be
   demonstrated — with a catalog-style measurement over real scripts, not a
   plausibility argument.

Slices 1–3 are independently valuable; 4 is the one that can fail its own
evidence gate and should be the last thing built.

## Open questions

- Does `requires_reanchor` re-arm smoothly enough for filter changes, or does
  the group need an explicit Apply?
- Should the offset readout show the *measured* residual drift beside the
  configured value, so calibration has feedback rather than only a setting?
- Is per-video offset the common case? If most scripts from one source share a
  bias, a per-source default would beat both global and per-video.
