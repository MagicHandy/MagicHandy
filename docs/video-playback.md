# Video Library And Synced Funscript Playback — Design

Status: **M0 implemented (2026-07-19); M1-M3 remain planned**. M0 is the
catalog and plain-playback foundation only: it does not load a funscript for
playback and cannot command motion.

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

M0 fulfills requirements 1 and 4 for plain playback and records the pairing
metadata needed by requirement 2. Automatic funscript playback and the OSD in
requirements 2-3 remain M1-M2 work.

## Architecture overview

```text
web (`#/videos`: grid, search, player, OSD strip)
   │  /api/media/*  (edge only)
internal/httpapi ──────────► internal/media (NEW)
   │                           • locations scan (explicit, bounded)
   │                           • video/funscript pairing (exact basename)
   │                           • SQLite catalog (schema v11)
   │                           • funscript timeline loader (media bounds)
   │                           • sync session (browser clock → anchor)
   └───────► internal/motion  • engine plays the paired timeline as an
                                anchored finite curve; transport unchanged
```

Boundary rules (enforced by the existing `internal/architecture` tests, with
`media` added to the semantic-client rule): `media` never imports `transport`
or `httpapi`; `motion` never imports `media`. The sync session hands the
engine *semantic* anchored targets; the transport keeps owning stroke-window
projection, speed caps, quantization, and Stop.

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
- In M1, `GET /api/media/videos/{id}/funscript` serves the paired script (bounded
  read, `MaxMediaFunscriptBytes` 16 MiB) for the OSD strip; the same loader
  feeds the sync session server-side, so the strip and the device always
  render the same data.
- Same-origin/localhost rules as every other route; streaming is read-only
  and therefore not controller-gated, but every motion-affecting endpoint
  below is.

## Sync design — planned for M2

**The video element is the clock master.** The browser owns play/pause/seek;
the backend anchors motion to the reported media clock:

- The player posts `POST /api/media/sync` events: `{video_id, state:
  playing|paused|seeking|ended, media_time_ms, client_time_ms, playback_rate}`
  on play/pause/seek/rate-change plus a heartbeat every 2 s while playing.
- `internal/media` keeps one **sync session** (single-operator app): an
  anchor `(media_time_ms, server_wall_time, rate)` from which media time is
  extrapolated between events.
- The funscript loads through `motion.NewCurve(points, duration, loop=false)`
  — the existing PCHIP sampler is already time-parameterized, so the engine
  samples `Curve.Sample(mediaTimeNow + lead)` where `lead` is the transport's
  latency-aware lead the engine already computes. **Media playback has its
  own bounds** (≤ 100,000 actions / 16 MiB) and never passes through the
  pattern-library import caps — a feature-length script is not library
  content.
- Engine integration: a new target shape `MediaTimeline` (label, video id,
  curve) started via the normal engine `Start`, plus one new engine
  capability — **`Reanchor(offsetMillis)`** — that phase-jumps a running
  finite curve to a new offset using the same low-jump handoff as retargets.
  Play → Start at offset; seek → Reanchor; pause → engine Pause (existing,
  phase-preserving); resume → Resume + Reanchor to the current media time;
  ended → engine Stop with reason `media_ended`.
- **Drift correction**: each heartbeat compares extrapolated media time with
  the engine's playback position; drift beyond 250 ms → Reanchor; smaller
  drift is left alone (constant re-anchoring feels worse than 100 ms of
  offset — the STGPT-RV morph-thrash lesson applied to sync).
- **Heartbeat loss** (tab closed, browser crashed) for > 5 s → engine
  **Pause** with a trace note (not Stop: the user may reopen; Stop remains
  the user's explicit act and the Stop button/Esc still works from any tab).
- Cloud REST expectation: wire latency means the device tracks the video
  with the transport's measured lead; the trace gains `media_sync` rows
  (anchor, drift, reanchor reason) so hardware sessions can measure real
  alignment before any tuning. Bluetooth/Intiface ride the same anchor.

Safety inheritance (nothing new to invent): positions are relative 0–100 and
are projected into the user's stroke window at the transport; user speed
caps clamp over-fast scripts exactly as they clamp any content; Emergency
Stop and the controller lease behave identically to every other motion
source; read-only tabs can watch video but their sync events are ignored for
motion (they get a visible "read-only — motion stays with the controller"
note).

## OSD funscript strip — planned for M1

A hideable strip rendered under the video (canvas, not SVG — feature-length
scripts have 10⁴–10⁵ actions; the import timeline's min/max bucket
downsampling is reused at canvas resolution):

- **Intensity coloring** per bucket from local speed (|Δposition| / Δt,
  %/s), using a design-system-compliant ramp instead of the traditional
  rainbow: `--line-strong` (idle) → `--accent` (slow–moderate) → `--warn`
  (fast) → `--danger` (very fast). Thresholds start at 0 / 50 / 200 / 400 %/s
  as constants and get tuned against real scripts; the ramp reuses the
  existing status semantics (red = intense) without introducing new hues.
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
  M1 adds the OSD strip; M2 adds the compact sync/device/drift status. The
  persistent Stop button stays global as on every route.
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

## API surface

| Endpoint | Gate | Slice | Purpose |
| --- | --- | --- | --- |
| `GET /api/media/videos` | read | M0 | catalog list (id, name, badges, duration) |
| `GET /api/media/videos/{id}/stream` | read | M0 | Range-capable file streaming |
| `POST /api/media/scan` / `GET /api/media/scan` / `DELETE /api/media/scan` | controller / read / controller | M0 | start, poll, or cancel an explicit scan |
| `PUT /api/settings` (`media.library_paths`) | controller | M0 | manage locations |
| `POST /api/media/duration` | controller | M0 | browser-reported `duration_ms` backfill |
| `GET /api/media/videos/{id}/funscript` | read | M1 | paired script for the OSD |
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
- **M1 — paired-script reading + OSD**: bounded funscript endpoint, canvas
  strip with intensity ramp + playhead + click-seek + hide toggle.
  Still no motion. *Gate: strip matches the script for feature-length files;
  rendering stays smooth at 10⁵ actions; hide state persists.*
- **M2 — synced motion**: media timeline loader + engine `Reanchor`, sync
  session (anchor/drift/heartbeat), pause/seek/ended/heartbeat-loss
  semantics, Stop/controller/read-only integration, `media_sync` trace rows.
  *Gate: Go integration tests drive the real engine over the fake transport
  through play → seek → pause → resume → ended with exactly one play command
  and bounded drift after seeks; goleak/race clean.*
- **M3 — hardware acceptance + polish**: real-device alignment measurement
  via the trace rows, drift-threshold tuning, client-captured poster frames,
  resume-from-last-position, fullscreen strip overlay decision.
  *Gate: a scripted video session on the real Handy with recorded
  drift numbers; subjective alignment acceptable.*

## Risks and budgets

- **New risk R25 (media sync)** for the register: browser-clock anchoring
  over a ~wire-latency transport may feel misaligned on hardware; mitigation
  is the M2 trace evidence + M3 acceptance gate before the feature is called
  done; fallback is honest labeling ("device follows with ~Xms lead").
- Binary: stdlib-only (`http.ServeContent`, `filepath.WalkDir`) — no size
  impact beyond the new package. Embedded UI: grid + player + canvas strip,
  no new dependencies (the design system and bucket-downsampling code carry
  over); watch-list item 4 still applies to any imagery.
- RSS: streaming is constant-memory; an M0 scan holds bounded discovery
  metadata for at most 10,000 encountered files per root while it runs. The
  loaded sync curve is the one
  meaningful allocation (≤ ~3 MB for 100k points) and is released when the
  player closes in M2.
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
