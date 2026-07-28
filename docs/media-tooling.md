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

### The compatibility target is H.264, and this matters

The stated goal is to make browser-incompatible files playable. **Encoding to
H.265 can defeat that goal.** Browser HEVC support is conditional on the
operating system, the browser, and often hardware decode support; it is not
something the app can rely on. H.264 in MP4, by contrast, plays essentially
everywhere.

So the two things asked for are offered as two different jobs, labeled as such:

| Preset | Codec | Purpose |
| --- | --- | --- |
| **Make it playable** (default) | H.264 + AAC in MP4 | Guaranteed to play in the app |
| **Archive smaller** | H.265 + AAC in MP4 | Roughly half the size; **may not play here** |

Choosing H.265 must warn that the result may not be playable in this app, and
the conversion should verify afterward rather than assume — the honest check is
whether the browser can actually decode the output.

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

- **CRF** — the quality/size dial. Lower is better and larger. Sensible exposed
  range is roughly 18–28 with a stated default; the UI should say what the
  number means ("lower is better quality and a bigger file") rather than
  assuming familiarity.
- **Preset** — the speed/efficiency dial (`ultrafast` … `veryslow`). It trades
  encoding time for file size at the same quality, and does **not** trade
  quality. Worth saying, because it is the most commonly misread encoder knob.
- **AAC bitrate** — 128 / 192 / 256 kbps, plus **copy when the source audio is
  already AAC**, which is both faster and lossless.

### Output, and the constraint no general tool has

Paired funscripts match by **exact basename in the same directory**. Convert
`Foo.mkv` and the existing `Foo.funscript` pairs with the result only if the
output is `Foo.mp4` in that same folder.

That dictates the default: **beside the source, same basename, new extension**,
with a name collision refused rather than resolved silently. Writing output to
the data directory or a chosen folder must warn that the pairing will not follow
— which is a real cost, not a preference, because the pairing is the entire
point of the library.

Other rules:

- **Never overwrite or delete the source.** A conversion that loses the original
  is not recoverable.
- Check free space against an estimate before starting, and fail early rather
  than at 90%.
- On success the new file is scanned in normally. An optional "hide the
  original" marks the source row excluded rather than deleting anything.
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
