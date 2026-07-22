# Video Library And Synced Funscript Playback — Design

Status: **M0-M2 implemented (2026-07-22); M3 remains planned**. Exact-name
funscripts now render below the video and play through the shared motion
engine. Real-device alignment and transport-specific tuning remain M3 work.

## Decision note — this supersedes a recorded non-goal

`docs/feature-ideas.md` previously recorded "video + funscript sync player" as
a deliberate non-goal ("ScriptPlayer territory"). That disposition is
**reversed by explicit direction (2026-07-19)**. The concerns that motivated
the non-goal do not disappear; they become this design's guardrails:

- MagicHandy stays a **chat-first controller with a media library**, not a
  media manager: no transcoding, no tagging/metadata editing, no external
  player integration, no codec bundling.
- The video feature adds **zero new motion pathways**: paired funscripts play
  through the one motion engine and the transport boundary like every other
  motion source (ADR 0002/0006).
- **Nothing heavy happens implicitly**: library scans are explicit user
  actions with visible progress; startup never scans, hashes, or probes media.

## Reference review

The implementation borrows proven interaction ideas, not whole architectures:

- [Syncopathy](https://github.com/ofs69/syncopathy) validates an offline media
  library, obvious filtering, exact video/script pairing, and a lightweight
  single-file workflow. MagicHandy adopts those concepts. It does not adopt
  Syncopathy's Flutter shell, FFmpeg thumbnail requirement, ObjectBox native
  persistence, custom `media_kit` fork, or direct BLE playback path; those
  conflict with the embedded web UI, pure-Go core, and one-motion-path rule.
- [EroDeck](https://gitlab.com/Archon-Dev/erodeck) validates keeping local
  video/script metadata in SQLite and staging authenticated remote imports for
  user review. Remote topic ingestion, uploaded-media copies, and handoff to an
  external IVE player are deliberately excluded from M0. A reviewed remote
  metadata queue is a possible later feature, separate from local playback.
- [ScriptCompiler](https://docs.scriptcompiler.com/getting-started/quick-start/)
  validates putting the video directly above its timeline and matching the
  `.funscript` basename to the media file. MagicHandy's import preview adopts
  that spatial relationship while retaining the existing bounded parser and
  trim controls; it does not become a second authoring or device-control path.

- [ScriptPlayer](https://github.com/FredTungsten/ScriptPlayer) separates the
  media time source from synchronized and action-based devices, resets script
  indexing on discontinuities, and resynchronizes devices from the media
  clock. MagicHandy adopts the clock ownership and discontinuity rules, not
  ScriptPlayer's device dispatch layer: every sample still belongs to the one
  Go motion engine.
- [c4l-funscript](https://github.com/c4llv07e/c4l_funscript) resets its script
  cursor on play/seek and stops devices on pause. Its polling loop is too
  coarse and its browser-to-device path would violate MagicHandy's transport
  boundary, but the explicit reset/stop behavior is the correct minimum for a
  media-clock player.

The resulting reusable unit is the native-video component used by both the
dedicated Videos page and the optional funscript-import preview. It accepts an opaque
catalog entry and playback callbacks, and knows nothing about motion.

## Requirements (from direction)

1. A first-class **Videos page with a searchable grid**; selecting a video
   plays it.
2. A funscript **with the exact same base name** as the video is
   **automatically played in time with** the selected video.
3. The video OSD has a **hideable funscript visualization below it,
   color-coded to intensity**.
4. **Settings where library locations can be added and scanned** for library
   entries.

M0 fulfills requirements 1 and 4. M1-M2 fulfill requirements 2-3 with one
bounded script document shared by the canvas and backend timeline loader.

## Architecture overview

```text
web (`#/videos`: grid, search, player, timeline)
   │  /api/media/* (edge only)
internal/httpapi ──────────► internal/media
   │  sync lifecycle           • explicit bounded scan + SQLite catalog
   │  controller/Stop fence    • exact-basename jailed funscript loader
   │                           • linear finite media timeline
   └───────► internal/motion   • one engine samples the finite timeline;
                                 every transport remains unchanged
```

Boundary rules (enforced by the existing `internal/architecture` tests):
`media` may construct motion-semantic finite timelines but never imports
`transport` or `httpapi`; `motion` never imports `media`. HTTP owns the browser
session lifecycle and hands only semantic targets to the engine. The transport
keeps owning stroke-window projection, quantization, buffering, and Stop.

## Data model

**Library locations** are user configuration, not content: a new additive
settings field (no settings-schema ceremony, mirroring the Ollama library
path precedent):

```jsonc
"media": {
  "library_paths": ["D:/Videos/sessions", "E:/archive"]   // non-secret
}
```

**Catalog entries** are scan output: SQLite **schema v11** adds

```text
media_videos(
  id TEXT PRIMARY KEY,          -- stable hash of location + relative path
  location_path TEXT NOT NULL,  -- the registered root it was found under
  relative_path TEXT NOT NULL,  -- path under that root (jail key)
  display_name TEXT NOT NULL,   -- basename without extension
  size_bytes INTEGER NOT NULL,
  modified_at TEXT NOT NULL,
  duration_ms INTEGER,          -- filled lazily by the browser (see below)
  funscript_relative_path TEXT, -- exact-basename pair, if present at scan
  missing INTEGER NOT NULL DEFAULT 0,
  scanned_at TEXT NOT NULL
)
```

No thumbnails or probed metadata at scan time: a pure-Go core cannot decode
H.264, and shelling out to ffmpeg is out of scope. `duration_ms` is reported
back by the browser after first successful playback (`loadedmetadata`), which
also enables an optional client-captured poster frame in a later slice.

## Scanning

- **Explicit action only** (Settings button and Videos-tab empty state);
  never on startup, never on a timer (goals-and-guardrails download/IO rule).
- Walks each registered root with bounds: max depth 6, max 10,000 files per
  root, symlinks not followed, hidden directories skipped. Extensions:
  `.mp4 .m4v .webm .mov` (browser-decodable set; `.mkv` deliberately excluded
  — the `<video>` element cannot reliably play it, and honest absence beats a
  broken row).
- Pairing: a sibling `NAME.funscript` for `NAME.mp4` in the **same
  directory** is recorded at scan (requirement 2 says exact same name;
  multi-axis variants like `NAME.roll.funscript` are ignored in v1).
- Rescan is idempotent: existing ids update in place, vanished files are
  marked `missing` (kept for one rescan cycle so a temporarily unplugged
  drive does not wipe the grid), and a scan summary (added/updated/missing/
  skipped) is returned and shown.
- Permission failures and file-limit truncation make a root explicitly
  partial. Discovered rows may still update, but unseen existing rows are not
  marked missing and Settings shows the preservation warning.
- Scans run server-side with progress polling like model imports
  (`/api/media/scan` returns a scan-state object; one scan at a time;
  cancellable; controller-gated).

## Serving video safely

- `GET /api/media/videos/{id}/stream` resolves the id through the catalog
  and serves the file with `http.ServeContent` (native Range support,
  constant memory). **The path jail is the catalog**: no endpoint accepts a
  filesystem path; only scan-recorded ids resolve, and resolution re-checks
  that the joined path stays under its registered root and that the root, file,
  and every intermediate component are not symlinks. The final open uses Go's
  rooted filesystem handle and verifies file identity, closing the path-swap
  race without depending on platform-wide ancestor resolution.
- `GET /api/media/videos/{id}/funscript` serves the paired script (bounded
  read, `MaxMediaFunscriptBytes` 16 MiB) for the OSD strip; the same loader
  feeds the sync session server-side, so the strip and the device always
  render the same data.
- Same-origin/localhost rules as every other route; streaming is read-only
  and therefore not controller-gated, but every motion-affecting endpoint
  below is.

## Sync design (M2 implemented)

**The video element is the clock master.** The browser owns play/pause/seek;
the backend anchors motion to the reported media clock:

- The player posts controller-gated `{video_id, session_id, event_sequence,
  state, event, media_time_ms, client_time_ms, playback_rate}` events on play,
  pause, seek, rate change, buffering/decode failure, end, and close, plus a
  heartbeat every 1.5 seconds while following.
- `internal/httpapi` owns one serialized sync runtime. It anchors the media
  time to server wall time only after the shared engine has successfully
  started. `internal/media` only opens, validates, and slices the paired
  script.
- Media playback has separate bounds (at most 100,000 actions and 16 MiB) and
  never passes through pattern-import caps. Funscript segments use **linear
  interpolation**, matching the authored format; pattern curves keep PCHIP.
- Play, resume, seek completion, and rate change each start a fresh finite
  timeline at the exact browser media time. Pause, seek start, waiting/stalled
  playback, decode failure, end, close, and heartbeat loss call engine
  **Stop**, not Pause. Replacement stops the previous source before reading
  the next script, so a slow or invalid file cannot leave old motion running.
- This deliberately does **not** add `Reanchor`. Buffered transports can
  already hold future points, and changing an engine phase cannot retract that
  queue. Stop/re-arm is the only honest discontinuity operation until every
  transport exposes a proven queue-flush contract.
- Each heartbeat compares browser time with the current anchor. Drift beyond
  250 ms or a rate mismatch stops motion and returns `requires_reanchor`; the
  browser then holds the video and explicitly re-arms. Sub-threshold drift is
  observed but not chased. A heartbeat can validate or stop a run, **never
  start one**.
- Heartbeat loss for more than five seconds stops the engine. A closed,
  crashed, suspended, or controller-lost player therefore cannot leave a
  resumable media target behind.
- Every arm carries the current Emergency Stop sequence. A concurrent Stop
  invalidates work before and after engine startup, and stale heartbeats are
  rejected. The global Stop path also invalidates sync status before stopping
  the engine.
- Every mounted player also has a random session id and increasing event
  sequence. The backend retains a bounded set of closed-session fences, so a
  delayed arm cannot restart an unmounted player and a delayed close cannot
  stop a newer session for the same video. Unmount cancels an in-flight arm
  and sends a keepalive close for any session that reached admission.
- Cloud REST expectation: wire latency means the device tracks the video
  through the engine's transport-aware buffer. Cloud declares a 10-second
  minimum accepted lead for clock-locked media and the engine batches sampler
  windows up to the existing 100-point `/hsp/add` cap. Runtime refill adds up
  to four seconds of headroom per request. Interactive chat motion keeps its
  1.5-second minimum so a media fix does not turn into multi-second retarget
  latency. Trace rows record anchors, drift, and stop/re-arm reasons so M3 can
  validate real alignment without creating a transport-specific media path.

Safety inheritance: authored timestamps stay locked to the video. The user's
configured maximum speed percentage contracts semantic position excursions
around the 50% midpoint before the normal stroke-window projection; it reduces
authored velocity without slowing the video, but is not a calibrated physical
velocity guarantee. Emergency Stop and the controller lease behave like every
other motion source. Read-only tabs can watch video and inspect the timeline,
but are visibly labeled visualization-only and emit no motion events.

## Funscript timeline (M1 implemented)

A hideable strip rendered under the video (canvas, not SVG — feature-length
scripts have 10⁴–10⁵ actions; the import timeline's min/max bucket
downsampling is reused at canvas resolution):

- **Intensity coloring** per bucket from local speed (|Δposition| / Δt,
  %/s), using a design-system-compliant ramp instead of the traditional
  rainbow: `--line-strong` (idle) → `--accent` (slow–moderate) → `--warn`
  (fast) → `--text` (very fast). Thresholds start at 0 / 50 / 200 / 400 %/s
  as constants and get tuned against real scripts; red remains reserved for
  Emergency Stop.
- A playhead line tracks `currentTime`; **click/drag seeks the video** (the
  strip never commands the device directly — seeking the video drives the
  sync session, one clock).
- The **hide toggle** collapses the strip to a 4 px progress sliver; the
  choice persists per browser (`localStorage`), since it is presentation, not
  app state. Reduced-motion: the playhead updates stepwise instead of
  animating.
- Fullscreen uses the native `<video>` controls; the custom strip is a
  windowed-mode surface in v1 (an overlay strip in fullscreen is a later
  polish slice).

## UI integration

- **Videos page** at `#/videos`, with its own permanent-sidebar link. Pattern
  Library retains Browse / Programs / Import / Author / Training and does not
  load the media catalog. The unframed video workspace uses the wide content
  width without nesting repeated video cards inside another decorative card.
- **Grid**: cards with display name, duration (once known), file size, a
  `script` badge when a funscript is paired, and a `missing` badge for
  unplugged locations. **Search** is a client-side filter over name or registered
  location (the catalog returns the full bounded list; personal libraries do
  not need server search). Sort: name / most recent.
- **Player view** (replaces the grid within the route; back button returns): M0
  ships `<video>` with native controls and browser-reported duration backfill.
  Leaving the Videos route unmounts the player so hidden audio never continues.
  Paired videos load the M1 canvas timeline before controls become available;
  M2 adds compact sync/device/drift status. The persistent Stop button stays
  global as on every route.
- Settings > **Library locations** (new Settings section): list of
  registered paths with per-path entry counts, Add via the existing
  controller-gated `POST /api/host/path-picker`, Remove (after confirmation,
  catalog rows for that root are deleted when Settings is saved), Scan-now with
  progress and the last scan summary. Startup reconciles rows to saved
  locations without scanning files, closing a crash window after settings save.
- **Funscript import preview** (M0): after a funscript is parsed, an optional
  modal uses the same player above the existing timeline. Exact-basename media
  is selected first when present, another catalog video can be chosen, and the
  timeline shares the import form's trim and viewport state. Playback remains
  preview-only and never starts motion.

### M0 UI/UX and video-handling review (2026-07-20)

- **Ownership and navigation:** Videos is a top-level workspace, not a Pattern
  Library tab. Route focus, document title, active navigation state, persistent
  Emergency Stop, and mobile icon navigation follow the shell contract.
- **Catalog operation:** Reloading the current SQLite snapshot is distinct from
  explicitly scanning the filesystem. Scan and Cancel remain available with a
  populated catalog; read-only clients can search and play but cannot mutate
  scan state or duration metadata.
- **Failure isolation:** catalog-load and scan-status errors are separate. A
  transient scan-status failure does not hide playable results, polling retries,
  partial-root issues remain visible, and overlapping catalog reads cannot let
  an older response replace a newer snapshot.
- **Catalog legibility:** cards include the registered location label, search
  matches name or location, missing entries remain keyboard-discoverable with
  `aria-disabled`, and unavailable selections return an explicit state instead
  of silently falling back to the grid.
- **Playback recovery:** decode or network failures expose a Retry command and
  clear after the native player can play again. Near-identical browser duration
  readings do not repeatedly write catalog metadata.
- **Streaming compatibility:** supported extensions receive deterministic
  `video/mp4`, `video/webm`, or `video/quicktime` response types on every host;
  byte ranges, `nosniff`, no-store caching, rooted file opens, and constant-memory
  serving remain unchanged.
- **Accepted deferrals:** thumbnails/posters, transcoding, codec bundling,
  per-video deep links, the funscript OSD, and synchronized motion remain outside
  this follow-up. OSD is M1 and motion is M2; neither may add a second media or
  motion pathway.

### M1-M2 implementation review (2026-07-22)

- **One document, two consumers:** the opaque video id resolves the same
  bounded exact-name script for the browser canvas and the backend timeline.
  Neither API returns a filesystem path.
- **Timeline legibility:** the 60 px overview keeps position and intensity as
  separate layers: a continuous high-contrast azure position trace above a
  smoothed three-pixel, single-hue activity rail. Extrema envelopes appear only
  when multiple authored actions collapse into one pixel; sparse segments no
  longer acquire false vertical bars from endpoint bucketing. A separate
  white-on-dark playhead canvas avoids rebuilding the feature-length curve on
  each `timeupdate`, and red remains Stop-only.
- **Reference review:** [ScriptPlayer's position editor](https://github.com/FredTungsten/ScriptPlayer/blob/master/ScriptPlayer/ScriptPlayer.Shared/Controls/PositionBar.cs)
  preserves a clear geometric trace and outlines it against the canvas, but its
  per-segment speed colors become busy when compressed to a whole-video
  overview. [OpenFunscripter's player](https://github.com/OpenFunscripter/OFS/blob/master/OFS-lib/UI/OFS_VideoplayerControls.cpp)
  instead keeps its whole-file heatmap separate from the editor's action
  geometry. The playback strip follows that separation while using the app's
  existing azure ramp rather than importing either tool's multi-hue heat
  palette.
- **Honest control:** play holds the video while the backend arms; pause, seek,
  rate change, buffering/decode failure, end, close, drift, and heartbeat loss
  have explicit status.
  Read-only tabs retain ordinary video and timeline access without pretending
  to control motion.
- **Buffered-owner safety:** discontinuities Stop and re-arm rather than phase
  jumping a queue whose future points cannot be retracted. An Emergency Stop
  sequence fences stale starts and heartbeats.
- **Failure isolation:** malformed, oversized, missing, or changed scripts
  leave ordinary video playback available with a visible no-motion warning.
  A script that ends before its video stops motion and allows the remaining
  video to continue. A likely script/video duration mismatch is visible beside
  the paired-script metadata without disabling playback.
- **Accepted deferrals:** poster frames, resume position, fullscreen timeline,
  and transport-specific alignment tuning remain M3. M1-M2 make no hardware
  alignment claim.

### Seek and startup reliability review (2026-07-22)

- **Arbitrary timestamps:** every fresh arm still slices the authored linear
  funscript at the exact browser time. Tests cover an action timestamp, a time
  between actions, playback-rate scaling, and the final millisecond before the
  script ends; seeking does not snap to the next authored action.
- **Buffered physical arrival:** the startup stream now includes a stationary
  target tail after its timed lead-in. This keeps HSP from pausing on starvation
  before the physical slider reaches the script's first position. Arrival is
  measured while that target remains active, then the shared engine issues Stop
  before starting media time. The tail adds buffer coverage, not another stroke.
- **Replacement cancellation:** the engine's startup phase follows request
  cancellation, so a newer seek can cancel obsolete positioning even while the
  backend sync lifecycle is serialized. After startup succeeds, the normal
  detached dispatch loop still outlives the completed HTTP request.
- **Scrub admission:** the overview playhead previews pointer movement locally
  and commits one video seek on release. It no longer creates a Stop/re-arm pair
  for every pointer-move event. Native video-control seeks retain their browser
  event semantics.
- **Failure visibility:** sync error responses include the backend status and
  its transport-safe cause. The player can distinguish failed physical arrival
  from a missing script or unavailable transport without exposing paths or
  credentials.
- **Natural completion:** when a shorter paired script reaches its finite end,
  the next heartbeat trusts the shared engine's completed phase, tolerates the
  small transport/browser start-clock offset, and leaves the remaining video
  playing without motion. It is not reported as competing motion.
- **Reference boundary:** the Handy v3 HSP contract documents explicit
  paused/starving states and a finite point buffer. Syncopathy maintains future
  buffered coverage and reboots when the playhead leaves it; ScriptPlayer resets
  its action cursor when time moves backward. MagicHandy adopts those lifecycle
  lessons while retaining one shared engine and fresh Stop/re-arm streams rather
  than copying either application's transport-specific motion loop.

Live Cloud HSP validation used the paired `Kishiri106 By Mouth` catalog entry
with the saved maximum speed temporarily capped from 53% to 30%:

- Direct arms at 01:20 and 01:17 reached `following` on their first request in
  6.08 s and 5.69 s. No arrival-tolerance retry was needed. Those direct API
  probes intentionally omitted heartbeats, and the watchdog stopped each run
  after its expected 5 s timeout.
- A browser-equivalent seek canceled an in-flight 01:20 startup after 3.00 s.
  The serialized seek Stop completed in 377 ms, the replacement 01:17 arm
  reached `following` in 5.75 s, and its next heartbeat remained `following`.
- During that cancellation/re-arm interval, 34 `/api/state` polls all returned
  HTTP 200; maximum and p95 handler time were both 2 ms. Long motion setup did
  not block the core snapshot path.
- Cleanup paused the media session, issued Emergency Stop, verified Cloud HSP
  `stopped`, restored the saved 53% maximum, and issued a final Stop.
- The shared 125 ms sampler, linear funscript interpolation, 5 s heartbeat
  watchdog, transport boundary mapping, and no-fallback Cloud owner policy were
  intentionally left unchanged. The safe trace export is retained only as
  ignored local QA output, not repository content.

## API surface

| Endpoint | Gate | Slice | Purpose |
| --- | --- | --- | --- |
| `GET /api/media/videos` | read | M0 | catalog list (id, name, badges, duration) |
| `GET /api/media/videos/{id}/stream` | read | M0 | Range-capable file streaming |
| `POST /api/media/scan` / `GET /api/media/scan` / `DELETE /api/media/scan` | controller / read / controller | M0 | start, poll, or cancel an explicit scan |
| `PUT /api/settings` (`media.library_paths`) | controller | M0 | manage locations |
| `POST /api/media/duration` | controller | M0 | browser-reported `duration_ms` backfill |
| `GET /api/media/videos/{id}/funscript` | read | M1 | bounded paired script for the timeline |
| `POST /api/media/sync` | controller | M2 | play/pause/seek/heartbeat anchor events |

## Slices (each is one reviewable PR with its own validation)

- **M0 — catalog foundation (implemented)**: settings field, schema v11,
  explicit scanner with
  bounds + summary, Settings section (add/remove/scan), dedicated Videos page +
  search, Range streaming, plain video playback with **no motion**, reusable
  video player, and optional video-above-timeline funscript import preview.
  Exact-basename pairing metadata is recorded now so the grid can label it;
  the script is not read or played. Automated gates cover explicit scan,
  bounds, missing-file retention, partial-scan preservation, catalog path
  jailing, byte-range responses, controller gates, frontend search/playback,
  and the import preview. The 2026-07-19 resumed acceptance scan covered two
  roots, four encountered files, three videos, and one exact-basename pair
  without issues. A complete 2 GiB sparse-file stream returned 200 in 0.829 s
  with a 1,495,040-byte peak RSS increase; a tail Range returned 206 and the
  requested 648 bytes. These close M0's multi-root and constant-memory manual
  checks before M1.
- **M1 — paired-script reading + timeline (implemented)**: one jailed,
  16 MiB/100,000-action loader feeds the read endpoint and backend; the canvas
  uses min/max buckets, an intensity ramp, a separate playhead, click/drag and
  keyboard seeking, and a persisted hide toggle.
- **M2 — synchronized motion (implemented)**: the serialized browser-clock
  session starts finite linear media targets through the shared engine and
  Stop/re-arms at discontinuities. Integration tests cover play, heartbeat,
  seek, pause, resume, media stalls, end, timeout, stale Stop generations,
  closed-session ordering, controller gates, one transport Play per explicit
  arm, and no heartbeat-triggered restart.
- **M3 — hardware acceptance + polish**: real-device alignment measurement
  via the trace rows, drift-threshold tuning, client-captured poster frames,
  resume-from-last-position, fullscreen strip overlay decision.
  *Gate: a scripted video session on the real Handy with recorded
  drift numbers; subjective alignment acceptable.*

## Risks and budgets

- **Risk R25 (media sync)** remains open: browser-clock anchoring
  over a ~wire-latency transport may feel misaligned on hardware; mitigation
  is the M2 trace evidence + M3 acceptance gate before the feature is called
  done; fallback is honest labeling ("device follows with ~Xms lead").
- Binary: stdlib-only (`http.ServeContent`, `filepath.WalkDir`) — no size
  impact beyond the new package. Embedded UI: grid + player + canvas strip,
  no new dependencies (the design system and bucket-downsampling code carry
  over); watch-list item 4 still applies to any imagery.
- RSS: streaming is constant-memory; an M0 scan holds bounded discovery
  metadata for at most 10,000 encountered files per root while it runs. The
  loaded sync curve is the one meaningful allocation (roughly 3 MB retained
  for 100k validated actions, with bounded temporary parse copies) and is
  released or replaced when the player ends, closes, or selects another video.
- The `.mkv` exclusion, no-transcoding stance, and localhost-only serving are
  deliberate scope walls; revisiting any of them is a new decision, not
  scope drift.

## Cross-references

- [feature-ideas.md](feature-ideas.md) — the reversed non-goal row.
- [pattern-library.md](pattern-library.md) — why media scripts bypass library
  import caps (a feature-length script is not curatable loop content).
- [motion-retargeting.md](motion-retargeting.md) — the sampler/lead model the
  anchor rides on; [decisions/0002](decisions/0002-motion-transport-contract.md),
  [decisions/0006](decisions/0006-drop-legacy-motion.md) — the boundaries.
- [ui-design-guidelines.md](ui-design-guidelines.md) — the intensity ramp
  stays inside the one-hue + status-color law.
- `internal/motion/content.go` (`Curve`, PCHIP sampling), `internal/store`
  (schema v11), `POST /api/host/path-picker` (location add).
