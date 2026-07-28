# Media Tooling — Thumbnails, FFmpeg, and Format Conversion

## Status

**Plan only — nothing here is built.** This answers a question the tiled video
library raised (there is nowhere for a thumbnail to come from) and, in doing so,
resolves the shape of the open pre-transcode decision in
[video-playback.md](video-playback.md).

## The question

The library grid renders a placeholder icon per tile because a pure-Go core
cannot decode H.264. Thumbnails therefore looked like they had to wait on the
transcoding decision, since both seem to need FFmpeg.

**They partly do not.** One dependency has three uses, and only two of them
actually require it:

| Want | Needs FFmpeg? |
| --- | --- |
| A cover for a video you have played | **No** — the browser already decoded it |
| Covers for a whole library you have not opened | Yes |
| A cover for a file the browser cannot play at all | Yes |
| Converting that file so it plays | Yes |

So covers for played videos can ship with no new dependency and no decision,
while the rest waits on FFmpeg. That also raises the stakes on the pre-transcode
decision: it now has two dependents rather than one, which is an argument for
settling it rather than leaving it open.

## What this supersedes

`video-playback.md` records "**Accepted deferrals:** thumbnails/posters,
transcoding, codec bundling" and a no-transcoding scope wall. This document is
the deliberate revisit those notes asked for, not scope drift. The wall's
substance survives intact:

- MagicHandy is still **not a media manager**. No tagging, no scrapers, no
  metadata editing, no hover previews, no scrubbing sprites, no deduplication.
- Nothing heavy still happens **implicitly**. No decoding at startup, on a
  timer, or as a side effect of opening a page.
- FFmpeg is still **never bundled** and never a requirement to use the app.

What changes is that "we cannot do this" becomes "this is an explicit,
consented, bounded thing a user can turn on".

## Reference

[Stash](https://github.com/stashapp/stash) is the closest comparable and the
right shape to borrow from:

- It **requires FFmpeg and offers to fetch it**: "Stash requires FFmpeg. If you
  don't have it installed, Stash will prompt you to download a copy during
  setup."
- Generation is an **explicit task**, not a background behavior — its Tasks page
  "allows you to direct the stash server to perform a variety of tasks", and
  generation during a scan is opt-in via checkboxes chosen before the scan runs.
- It also has an advanced **transcode** task for unsupported formats, which is
  the same feature proposed below.

What is deliberately *not* borrowed: hover previews, scrubber sprites, marker
screenshots, perceptual-hash deduplication, and image galleries. Those are what
make Stash a library manager, which is the thing this project has repeatedly
decided not to become.

## 1. FFmpeg as a managed optional dependency

Three states, and the app is honest in all of them:

| State | Behavior |
| --- | --- |
| **Absent** | Everything that needs it is visibly unavailable with the reason and a way to fix it. Nothing silently degrades or half-works. |
| **User-provided** | A path field, validated on save. This is the default expectation for anyone who already has FFmpeg. |
| **App-downloaded** | An explicit button with size, source, license, and checksum shown before anything is fetched. |

Reuse rather than reinvent: the model manager already has curated-download
machinery with consent, checksum verification, and progress, and voice worker
paths already use `HostPathField` for guarded host-path browsing. This is the
same problem with a different payload.

**Validation before trust.** A configured path is run once as
`ffmpeg -version`, and its reported version and configuration line are stored
and shown. A binary that does not identify itself is rejected rather than
invoked later with real arguments. `ffprobe` is resolved the same way, since
format inspection needs it.

**Licensing has to be visible.** FFmpeg builds are LGPL or GPL depending on how
they were configured, and x265 is GPL — a build that can encode HEVC is
therefore a GPL build. MagicHandy never links FFmpeg: it invokes a separate
process and exchanges files, which is the ordinary arrangement for this. The
download UI must still show which build and which license, because "the app
downloaded it for me" is not consent unless the terms were visible.

**Invocation safety.** Arguments are passed as argv, never through a shell.
Every input and output path is resolved through the existing catalog jail before
it becomes an argument. No user-supplied text is ever interpolated into a
filter, codec, or metadata argument — the presets below are code-owned enums,
not free-form strings.

## 2. Thumbnails

### Tier 1 — browser-captured covers, no dependency

The browser has already decoded the video in order to play it. Drawing the
current frame into a canvas and posting the result is the same precedent as
`duration_ms`, which the browser already reports back after
`loadedmetadata`, and `video-playback.md` already anticipated this: it notes
that reported duration "also enables an optional client-captured poster frame in
a later slice."

- Captured a fraction of the way in rather than at frame zero, because the first
  frame of a video is very often black.
- Bounded output: one small JPEG or WebP at a fixed max edge, produced by the
  canvas rather than by the app.
- Only for videos the user actually opens, so it costs nothing for a library
  that is never browsed.
- Controller-gated and skipped for read-only tabs, like the duration write.

This alone makes the grid stop looking broken for the videos someone actually
uses, and it can ship before any FFmpeg decision is made.

### Tier 2 — FFmpeg batch generation

For everything Tier 1 cannot reach: videos never opened, and files the browser
cannot decode at all.

- A **Generate thumbnails** action in `Settings > Media`, run as a task with
  progress, cancel, and a completion summary — the media scan already has
  exactly this machinery (`running`, `cancellable`, `cancelled`, counters), and
  it should be the same runner rather than a second one.
- One at a time, like scans. Two long media jobs at once is a way to make the
  app feel broken.
- Skips videos that already have a cover unless explicitly asked to redo them.

### Storage

Generated assets live under the data directory, not beside the user's media —
writing into someone's video folders is a surprise, and it makes the feature
impossible to cleanly undo:

```
data/thumbnails/<video-id>.jpg
```

A `thumbnail_generated_at` column on `media_videos` records presence, so the
grid does not stat a file per tile and a missing asset is a knowable state
rather than a broken image. Budget: 1,000 videos at roughly 30 KB is about
30 MB, which is small next to a model but not nothing, and it belongs in the
data-directory usage view that `feature-ideas.md` already proposes.

Deleting a video's catalog row deletes its cover. A "clear generated
thumbnails" action makes the whole cost recoverable in one step.

### Where it renders

`.media-card-visual` in the library grid is currently a placeholder icon; the
cover replaces it and the icon becomes the fallback for videos without one. No
layout change — the slot already exists at the right size.

## 3. Generation attached to a scan

The requested "automatic sync setup" is the Stash shape: checkboxes chosen
*before* an explicit scan, not a background service.

- `Settings > Media` gains **Generate missing thumbnails after scanning**, off
  by default.
- It rides the scan the user already started, so the work is still
  user-initiated. The guardrail is unchanged: **never at startup, never on a
  timer**.
- Bounded, cancellable, and reported in the same scan summary. A scan that would
  generate 4,000 covers must say so before it starts, not after.

If FFmpeg is absent, the option is visible but disabled with the reason, rather
than hidden — otherwise the feature is undiscoverable for exactly the people who
would want it.

## 4. Format conversion

Explicit, per-file, never automatic, and never in the playback path. The
no-real-time-transcoding wall stands: this converts a file once, ahead of time.

### Codec is the user's choice, with the tradeoff stated

Both codecs are offered and neither is decided for the user. H.264 is the
default because it is the one that always plays; H.265 is there because halving
the file size is a real reason to want it.

| Setting | Size | Plays here |
| --- | --- | --- |
| **H.264** (default) | Baseline | Yes, everywhere |
| **H.265** | Roughly half | Depends on your OS, browser, and hardware decode |

The note in Settings has to say that plainly, because the failure is silent and
specific: an H.265 file that plays fine in VLC can still refuse to play in this
app, on this machine, for reasons that have nothing to do with the file. The
conversion therefore **verifies playability afterward rather than assuming it**
and reports the result — that check is cheap, and it is the difference between a
setting and a trap.

Whichever codec is selected applies to every conversion until it is changed.
There is no per-file override: the point of a default is that most people set it
once.

### Remux before re-encode

Most files that fail to play are not encoded wrongly; they are in the wrong
*container*. An `.mkv` holding H.264 video and AAC audio needs no encoding at
all — copying the streams into MP4 is near-instant and mathematically lossless.

This should be detected with `ffprobe` and offered first:

| Detected | Action | Cost |
| --- | --- | --- |
| Compatible streams, wrong container | **Remux** (`-c copy`) | Seconds, no quality loss |
| Incompatible video stream | Re-encode video, copy audio if AAC | Minutes to hours |
| Incompatible audio only | Copy video, encode audio | Fast |

Skipping this would make the common case take an hour instead of ten seconds,
and would degrade quality for no reason. It is the single most valuable part of
this feature.

### Quality controls

Code-owned enums, not free text:

- **CRF** — the quality/size dial. Lower is better and larger. Exposed over
  roughly 18–30, with the UI saying what the number means ("lower is better
  quality and a bigger file") rather than assuming familiarity.

  **The default has to differ per codec.** The two CRF scales are not the same:
  x265 needs a higher number for comparable perceived quality, so carrying one
  value across a codec change would silently alter quality in whichever
  direction the user did not ask for. Separate defaults — around 23 for x264 and
  around 28 for x265 — each remembered per codec.
- **Preset** — the speed/efficiency dial (`ultrafast` … `veryslow`). It trades
  encoding time for file size at the same quality, and does **not** trade
  quality. Worth saying, because it is the most commonly misread encoder knob.
- **AAC bitrate** — 128 / 192 / 256 kbps, plus **copy when the source audio is
  already AAC**, which is both faster and lossless.

### The settings this adds

All under `media`, all with a working default, none required to use the app:

| Field | Default | Notes |
| --- | --- | --- |
| `ffmpeg_path` | empty | Empty means absent; features that need it say so |
| `conversion_codec` | `h264` | `h264` or `h265`, with the tradeoff noted beside it |
| `conversion_crf_h264` | 23 | Separate per codec, because the scales differ |
| `conversion_crf_h265` | 28 | |
| `conversion_preset` | `medium` | `ultrafast` … `veryslow` |
| `conversion_audio_kbps` | 192 | 128 / 192 / 256, or copy when already AAC |
| `generate_thumbnails_on_scan` | false | Rides an explicit scan only |
| `show_superseded_originals` | false | Reveals files hidden by the suffix rule |

### Output naming, and the marker that ties it together

Output lands **beside the source** with a reserved suffix:

```
Holiday.mkv   ->   Holiday_MHConverted.mp4
```

The suffix does three jobs at once, which is why it earns a reserved token
rather than a plain rename:

1. **It is self-describing.** A file in someone's library a year later says
   where it came from without a database to consult.
2. **It cannot collide.** Writing `Holiday.mp4` beside `Holiday.mkv` fails when
   the user already has a `Holiday.mp4`; the suffixed name has no such problem,
   so the "refuse on collision" case mostly disappears.
3. **It is the signal to hide the original**, below.

#### Funscript pairing has to be taught about it

This is the part that breaks if it is not handled. Scripts pair by exact
basename, so `Holiday_MHConverted.mp4` would look for
`Holiday_MHConverted.funscript`, find nothing, and the conversion would silently
cost the user their script pairing — which is the entire point of the library.

The pair resolver gains one fallback: **strip the reserved suffix and try
again.** `Holiday_MHConverted.mp4` matches `Holiday_MHConverted.funscript` when
that exists, and otherwise `Holiday.funscript`. That is a small bounded change
to a rule the app fully owns, and it means nothing is copied, renamed, or
duplicated inside the user's folders.

#### Hiding the superseded original

After a conversion the library should show one entry, not two. The rule is
**derived at scan time, never stored**:

> When `NAME_MHConverted.<ext>` exists in a directory, any other file in that
> same directory whose basename is exactly `NAME` is indexed but hidden.

Derived rather than a database flag, because the filesystem is the source of
truth and people move and delete files outside the app. Delete the converted
file and the original reappears on the next scan, with no stale flag to clean
up. Hidden rather than removed, so the row and its per-video sync offset survive
and a "show superseded originals" toggle costs nothing.

The details that decide whether this is safe:

- **Matching is case-insensitive on Windows**, consistent with how library paths
  are already deduplicated there.
- **A file already carrying the suffix is never converted again**, so
  `NAME_MHConverted_MHConverted.mp4` cannot exist. The suffix is stripped once,
  not repeatedly.
- **The token is fixed, not configurable.** A configurable marker would orphan
  every previously converted file the moment someone changed it.
- **Accepted risk:** a user who independently owns both
  `Holiday_MHConverted.mp4` and `Holiday.mp4` would see the latter hidden. The
  token is distinctive enough that this is vanishingly unlikely, hiding is
  reversible, and nothing is deleted — so the honest response is to accept it
  and keep the toggle discoverable, not to add heuristics that would be wrong in
  subtler ways.

#### Carrying the calibration across

The converted file is a new catalog row, so its per-video sync offset would
start at zero. It should **inherit the source's offset** instead: conversion
moves no timestamp and the paired script is the same file, so the script's bias
is identical. Making someone re-calibrate a file they just converted is a small,
entirely avoidable annoyance.

#### Other rules

- **Never overwrite or delete the source.** A conversion that loses the original
  is not recoverable.
- Check free space against an estimate before starting, and fail early rather
  than at 90%.
- One conversion at a time, cancellable, with progress — a two-hour job with no
  cancel button is a bug.

## Non-goals

Recorded so they are not proposed later as small additions:

- Hover previews, scrubber sprites, animated thumbnails.
- Perceptual-hash deduplication, tagging, scrapers, metadata editing.
- Batch/queued conversion pipelines. One file on request; a queue is how a
  utility becomes a media manager.
- Any decode or conversion in the playback path.
- FFmpeg as a hard requirement. Every feature here degrades to today's behavior
  when it is absent.

## Slices

1. **Browser-captured covers.** No dependency, no decision, visible improvement
   to the grid. Storage, the `media_videos` column, the render slot, and purge.
2. **FFmpeg dependency.** Path field, validation, version readout, and the
   absent/present states across the UI. No features yet — just the honest
   dependency.
3. **FFmpeg download.** Consent, license, checksum, progress. Optional, and it
   can be skipped indefinitely by anyone who supplies a path.
4. **Batch thumbnails.** The generate task plus the scan-attached option.
5. **Conversion.** Probe, remux-first, the two presets, output rules, task
   integration.

1 is independently worth shipping. 2 is worth shipping even alone, because it
turns "this is impossible" into "this needs a thing you can install". 5 is the
largest and should be last.

## Open questions

- Should Tier 1 capture on every open, or once per video? Once is cheaper; every
  open lets a later frame replace an unlucky one.
- Is a conversion worth offering for files that already play, purely to shrink
  them? That is archival, not compatibility, and it edges toward media
  management.
- Should a failed HEVC playability check offer to redo the conversion as H.264
  automatically, or just report it? Automatic sounds helpful and quietly spends
  another hour of someone's CPU.
- Should the suffix rule hide an original whose extension is already
  playable? Converting `Holiday.mp4` to H.265 for size produces
  `Holiday_MHConverted.mp4`, and the rule hides the perfectly good original
  — correct as written, but worth confirming, since that case is archival
  rather than repair.
