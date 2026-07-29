# Media Tooling — Thumbnails, FFmpeg, and Format Conversion

## Status

**Built**, except the optional FFmpeg downloader (slice 3), which is deferred
for a stated reason in [What shipped](#what-shipped). This document was the
plan; the sections below are kept as the design record, with corrections marked
where implementation proved the plan wrong.

It answers a question the tiled video library raised (there is nowhere for a
thumbnail to come from) and, in doing so, resolves the open pre-transcode
decision in [video-playback.md](video-playback.md).

## The correction this plan needed

The plan asserted that an incompatible file **cannot appear in the library at
all**, because the scanner only indexed `.mp4 .m4v .webm .mov`. That was wrong,
and the error mattered: it is true of incompatible *containers* and false of
incompatible *codecs*.

An `.mp4` holding HEVC is indexed today. It is an ordinary catalog row with an
ordinary extension, and Firefox — which ships no HEVC decoder on most platforms
— will not play it. Nothing about the filename says so. That file was always
reachable, always broken, and invisible to every check the plan proposed.

Two things follow, and both are in the shipped code:

1. **An extension is never a playability claim.** Classifying by extension may
   report "unknown", never "playable". Only a real playback attempt produces a
   positive verdict.
2. **The observed failure is the primary signal, not the third one.** The plan
   listed it third, after extension and `ffprobe`. It is first: it is the only
   signal that catches a supported container holding an unsupported codec, and
   it is free, because the browser is already reporting it.

A second correction came from running the result: **the extension prior is
browser-specific**. Chrome opens MKV; Firefox does not. A server-side list
saying "`.mkv` cannot be played" puts a wrong badge on working files for every
Chrome user. So the server sends the container's MIME type and the client asks
its own engine via `canPlayType` before showing the badge. Verified live: the
same catalog reports one repairable file in Chromium and would report two in
Firefox.

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

One further decision is superseded below: the scanner's exclusion of `.mkv` and
other unplayable containers, justified as "honest absence beats a broken row".
That reasoning held while nothing could be done about those files. Once there is
a repair path, absence hides exactly the files the repair exists for.

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
- Bounded output: one JPEG at a 640 px max edge, encoded at quality 0.85 with
  the browser's high-quality smoothing hint.
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
- Uses spline downscaling and an explicit MJPEG quantizer of 3. This keeps batch
  quality deterministic while avoiding Lanczos ringing around hard edges.

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

The requested "automatic sync setup" uses the Stash task shape: related
checkboxes are consolidated and chosen *before* a scan rather than split across
unrelated settings groups.

- `Settings > Media` gains **Generate missing thumbnails after scanning**, off
  by default.
- It rides the same bounded scan whether that scan was started manually or by
  the user's explicit **Scan library when MagicHandy starts** preference.
  Startup scanning is off by default and there is still no timer or watcher.
- Bounded, cancellable, and reported in the same scan summary. A scan that would
  generate 4,000 covers must say so before it starts, not after.
- A cancelled or failed scan never launches thumbnail or conversion follow-up
  work. Startup and manual scans share this rule.

If FFmpeg is absent, the option is visible but disabled with the reason, rather
than hidden — otherwise the feature is undiscoverable for exactly the people who
would want it.

## 4. Conversion — repair only

Conversion exists to make an unplayable file playable. **A file that already
plays is never converted**, by any path — not by the button, not by a scan, not
to save space. That closes the archival question the first draft left open, and
it removes a whole class of ways this feature could waste hours of someone's CPU
on files that were fine.

Explicit, never automatic in the sense of running unasked, and never in the
playback path. The no-real-time-transcoding wall stands.

### What "incompatible" means

Three signals. The order below is the shipped order of authority, which is not
the order the first draft gave them:

| Signal | Cost | Catches |
| --- | --- | --- |
| **Observed failure** | Free, already happening | Anything this browser actually refused, including a supported container holding an unsupported codec. The only signal that reaches HEVC-in-MP4. |
| **Extension, checked against the engine** | Free, at scan | Containers `<video>` will not open — confirmed per browser with `canPlayType` before it is shown, because MKV support differs between Chrome and Firefox |
| **`ffprobe`** | Needs FFmpeg | Which codecs are actually inside, used to explain a verdict and to choose remux over re-encode |

The first is the important one, and it is nearly free: the player already
handles `onError`. What it needed was to read `MediaError.code` and separate
`MEDIA_ERR_DECODE` / `MEDIA_ERR_SRC_NOT_SUPPORTED` from a file that simply did
not arrive — a deleted file raises the same code in some browsers, and telling
someone to convert a video that no longer exists sends them to fix the wrong
thing. So the stream is range-probed before the codec is blamed.

The verdict is stored, so the offer survives a reload, and it is **reversible**:
a later successful play writes "playable" straight back over it. It has to be,
because the answer belongs to a browser rather than to the file.

### The library has to index incompatible files first

**This is the change that makes everything else possible, and it supersedes a
recorded decision.** The scanner currently indexes only `.mp4 .m4v .webm .mov`,
and `video-playback.md` justifies excluding `.mkv` because "the `<video>`
element cannot reliably play it, and honest absence beats a broken row."

That was right when nothing could be done about it. It is wrong now: absence
hides exactly the files this feature exists to fix. Someone with a library of
`.mkv` files would see an empty grid and no reason to think the app could help.

So the scanner gains a second extension set — **known video containers that
cannot be played here** — indexed and shown with a "needs conversion" state
rather than skipped. They are visibly not playable, they carry the conversion
affordance, and they still pair with their funscripts, so the pairing is already
known before conversion rather than discovered after.

Indexing them is useful even with no FFmpeg configured: "40 files here need
conversion, which needs FFmpeg" is actionable, while an empty grid is not.

### Where conversion is offered

- **On the video page**, when the selected file is known-incompatible by
  extension or has just failed to play. This is the contextual case and the one
  that matters: the user is looking at the thing that did not work.
- **As a task in `Settings > Media`**, for a library-wide sweep, and as a
  scan-attached option alongside thumbnail generation.

Both paths convert **only files established as incompatible**. A scan-attached
conversion that re-encoded a playable library would be the single worst thing
this feature could do, so the gate is the same in both places rather than
re-derived per entry point.

If FFmpeg is absent, both are visible and disabled with the reason. Hiding them
would leave the "needs conversion" badge unexplained.

### Remux when the container is the only problem — feasibility

**Practical, and the common case.** Most `.mkv` files hold H.264 or H.265 video
with AAC audio; both are legal MP4 payloads, so the streams copy across
untouched. A feature-length film remuxes in seconds with no quality change at
all, against tens of minutes to hours for a re-encode.

`ffprobe` reports the codecs, and the decision is mechanical:

| Video | Audio | Action |
| --- | --- | --- |
| MP4-legal | MP4-legal | **Remux** — `-c copy`, seconds, lossless |
| MP4-legal | not (Vorbis, FLAC, PCM…) | Copy video, encode audio to AAC — still fast |
| not (VP9 in MKV, MPEG-2, VC-1…) | either | Re-encode video; copy audio when already AAC |

Caveats that decide whether a remux actually succeeds, all of them known and
bounded:

- **Subtitles do not survive.** MKV's ASS/SSA tracks have no MP4 equivalent
  worth carrying. Dropping them is the honest default, and the result should say
  so rather than let someone discover it later. Converting them to `mov_text`
  loses styling and is not worth the complexity here.
- **`-movflags +faststart` is not optional.** It relocates the index to the
  front of the file so the browser can begin playing over a range request
  instead of fetching the whole thing first. Without it a remuxed file
  technically plays and practically feels broken over the app's own streaming
  path.
- **Odd timestamps** occasionally need regenerating. Detectable from the
  result, and a failed remux should fall back to a re-encode offer rather than
  reporting success.

The value here is large enough that remux is the *first* thing tried, not a
special case: skipping it would turn a ten-second job into an hour-long one and
degrade quality for no reason.

### H.265 is assumed playable, with a toggle

Default assumption: **HEVC plays here**, so an H.265 file in an unplayable
container needs only a remux. Most current setups can decode it, and assuming
otherwise would force a needless re-encode on the majority.

For setups where it does not play, `Settings > Media` gains **Convert H.265 for
wider compatibility** (off by default). Turning it on has two linked effects:

1. H.265 files become *candidates for conversion* rather than remux targets.
2. The re-encode target is **forced to H.264**.

That interlock matters. Without it, someone who just declared "H.265 does not
play here" could still have the re-encode produce H.265 — spending an hour to
arrive back at an unplayable file. The setting means one thing, and the code
enforces both halves of it.

The observed-failure signal covers the same ground automatically: a file that
fails to play *is* incompatible on this machine regardless of what any setting
says, and the conversion offer follows from that directly.

### Quality controls

These apply only when a re-encode is genuinely required, which after the remux
path is a minority of files. Code-owned enums, not free text:

- **Re-encode codec** — H.264 by default; H.265 available for smaller output,
  and forced to H.264 when the compatibility toggle above is on.
- **CRF** — the quality/size dial. Lower is better and larger, exposed over
  roughly 18–30, with the UI saying what the number means rather than assuming
  familiarity. **The default differs per codec** (around 23 for x264, around 28
  for x265): the two scales are not the same, so carrying one value across a
  codec change would silently alter quality in whichever direction the user did
  not ask for.
- **Preset** — the speed/efficiency dial (`ultrafast` … `veryslow`). It trades
  encoding time for file size at the same quality, and does **not** trade
  quality. Worth saying, because it is the most commonly misread encoder knob.
- **AAC bitrate** — 128 / 192 / 256 kbps, plus **copy when the source audio is
  already AAC**, which is both faster and lossless.

### The settings this adds

All under `media`, all with a working default, none required to use the app:

| Field | Default | Notes |
| --- | --- | --- |
| `auto_scan_on_startup` | false | One bounded background scan after catalog startup; never a timer |
| `remove_missing_on_scan` | true | Removes absent catalog rows only after a complete root scan; never source files |
| `ffmpeg_path` | empty | Empty means absent; features that need it say so |
| `convert_h265_for_compatibility` | false | On: H.265 counts as incompatible, and re-encodes target H.264 |
| `reencode_codec` | `h264` | Only used when a remux is impossible; forced to `h264` by the toggle above |
| `reencode_crf_h264` | 23 | Separate per codec, because the scales differ |
| `reencode_crf_h265` | 28 | |
| `reencode_preset` | `medium` | `ultrafast` … `veryslow` |
| `reencode_audio_kbps` | 192 | 96–576 kbps target in 16 kbps steps; existing AAC is copied; FFmpeg may clamp by source format |
| `generate_thumbnails_on_scan` | false | Rides a successful manual or opted-in startup scan |
| `convert_incompatible_on_scan` | false | Rides a successful manual or opted-in startup scan; incompatible files only |
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

Because conversion is repair-only, the hidden original is by definition a file
that could not be played here — so hiding it removes a dead entry rather than a
usable one, which is what made the archival case uncomfortable and this one not.

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

## What shipped

| Slice | State |
| --- | --- |
| 1. Browser-captured covers | Built |
| 2. FFmpeg as a validated dependency | Built |
| 3. FFmpeg download | **Not built** — see below |
| 4. Batch thumbnails | Built |
| 5. Incompatible files in the catalog | Built |
| 6. Conversion | Built |

**Slice 3 is deliberately absent.** Shipping a downloader means pinning a URL
and a SHA-256 for a third-party build. Publishing a checksum that has not been
verified against the artifact it claims to describe is worse than having no
download button at all: it looks like a safety guarantee and is not one. The
path field covers everyone who already has FFmpeg, and the absent state is
honest and actionable. This stays open until someone can pin a real checksum.

### Where the plan met reality

Two failures that only appeared when the code ran against a real encoder, both
now covered by tests:

- **The output muxer must be named.** Conversion writes to `<name>.partial` and
  renames on success, and FFmpeg selects its format from the file extension. It
  cannot resolve one from `.partial`, and fails with a bare "Error opening
  output files: Invalid argument". `-f mp4` is load-bearing. The same applies
  to thumbnail capture, which needs `-f image2 -c:v mjpeg`.
- **The thumbnail seek needs a real duration, not a fallback.** Rows have no
  duration until the browser reports one after playback — and batch thumbnails
  exist precisely for videos nobody has played. A fixed fallback offset seeks
  past the end of anything shorter and FFmpeg reports only that no packets
  arrived. `ffprobe` is asked instead, the answer is stored, and a failed
  capture retries at frame zero.

One design change came from the same pass: **Tier 1 never seeks.** The plan had
it capturing a frame a fraction of the way in, but that element is the one a
clock-locked run follows, and seeking would emit `seeking`/`seeked` into the
sync engine — moving the device to chase a thumbnail. It now takes whatever
frame is on screen once playback passes three seconds, which is all Tier 1 ever
promised: covers for videos someone actually opens.

### Measured

Against a generated fixture set, with FFmpeg 8.1.1:

| Case | Path | Result |
| --- | --- | --- |
| H.264 + AAC in `.mkv` | Remux | 357 ms, 182 KB → 184 KB, plays |
| MPEG-4 Part 2 in `.avi` | Re-encode to H.264 | Plays, 320×180 confirmed decoded |
| Paired script across a conversion | Suffix-strip fallback | 3 actions still resolve |
| Per-video sync offset across a conversion | Inherited | 175 ms carried |
| Originals after conversion | Hidden, not deleted | Both files still on disk |

## Non-goals

Recorded so they are not proposed later as small additions:

- Hover previews, scrubber sprites, animated thumbnails.
- Perceptual-hash deduplication, tagging, scrapers, metadata editing.
- Batch/queued conversion pipelines beyond the bounded library sweep. One file
  on request, or one explicit pass over files that cannot play; a general queue
  is how a utility becomes a media manager.
- Converting a file that already plays, for size or any other reason. Repair
  only.
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
5. **Incompatible files in the catalog.** Index the containers the browser
   cannot open, show them as needing conversion, and let them pair with their
   scripts. Useful on its own — it tells someone their library is not empty,
   just unplayable — and every conversion path depends on it.
6. **Conversion.** Probe, remux first, re-encode only what must be, the suffix
   and pairing rules, and the task integration.

1 is independently worth shipping. 2 is worth shipping even alone, because it
turns "this is impossible" into "this needs a thing you can install". 5 is small
and unblocks the rest. 6 is the largest and should be last.

## Open questions

- Should Tier 1 capture on every open, or once per video? Once is cheaper; every
  open lets a later frame replace an unlucky one.
- Is a conversion worth offering for files that already play, purely to shrink
  them? That is archival, not compatibility, and it edges toward media
  management.
- How wide should the "cannot be played here" extension set be? `.mkv` and
  `.avi` are obvious; a long tail of rare containers would add rows nobody can
  act on without also adding value.
- Should an observed playback failure mark the catalog row, so the "needs
  conversion" state survives a reload without re-probing? It is useful state and
  it is also a cache that can go stale when a browser gains codec support.
- Does the scan-attached conversion want a ceiling — convert at most N files, or
  stop after M minutes? An overnight sweep is fine; an accidental one is not.
