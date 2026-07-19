# Codebase Audit Ledger

This ledger tracks the systematic reliability, maintainability, and efficiency
review. A subsystem is complete only after its code paths, ownership and
lifecycle boundaries, tests, and relevant documentation have been reviewed.

Baseline: `origin/main` at `b81e9eb2` (2026-07-19).

| Subsystem | Status | Current evidence |
| --- | --- | --- |
| Configuration and persistence boundaries | Reviewed in dedicated pass | One process-owned SQLite pool serves six logical domains; writes share one transaction lock, schema v10 preserves invalid settings, physical corruption is quarantined, logical damage fails clearly, and schema/version/permission/lifecycle behavior has focused coverage. |
| Diagnostics and structured logging | Reviewed in first pass | Trace storage now overwrites in O(1) and returns independent snapshots. Logging volume and redaction need review with each provider/transport. |
| HTTP and process lifecycle | Reviewed in first pass | Oversized JSON is rejected, response encoding cannot panic after committing headers, browser requests are loopback same-origin, mutating leases require headers, and shutdown quiesces device work before closing stores. |
| Motion engine and transports | Reviewed in first pass | PR #87 serializes ownership and command admission, hardens transport teardown, and expands race and lifecycle coverage. Real-device behavior remains subject to the documented hardware validation matrix. |
| Chat, LLM, memory, modes, patterns, library, validation | Reviewed in first pass; Autopilot follow-up in PR #101 | Storage failures are explicit, mutations are transactional, mode lifecycle and stale-operation races are covered, provider/runtime limits are bounded, managed-model inventory is crash-safe, and validation exports only the active run. Chat Autopilot now consumes bounded canonical conversation context, preserves resolved custom patterns across hold/drift, cancels announcements with the mode, and falls back visibly without creating a second motion path. Live-model and long-session acceptance remain open. |
| Voice, workers, queues, and audio | Reviewed in first pass | Worker framing and deadlines, bounded request queues, cancellation, process-tree teardown, provider response limits, deterministic sampling, and reference-code validation have focused coverage. Representative listening, simultaneous GPU LLM/TTS load, and lower-VRAM acceptance remain open. |
| Frontend state, accessibility, and UI performance | Reviewed in dedicated pass; Autopilot follow-up in PR #101 | Route lifetime preserves settings drafts; failed reads remain distinct from valid empty state; quick writes flush across unmount; chat tail reads retry; and persistence/mode mutations serialize before React rerenders. The Import timeline uses one measured coordinate system for waveform, selection, and fixed 44 px trim targets; zoom cannot mutate content, wheel zoom is cursor-anchored, and a proportional pointer/keyboard scrollbar directly moves the viewport. Long loops preserve duration, impossible essential-knot counts fail before upload, and compact previews retain saved reversals. Chat's control sidebar owns the single Autopilot control, including Pause/Resume and model/fallback provenance; Preset Modes remains deterministic. Manual motion lives in Diagnostics and keys its active state from backend target provenance rather than the generic running flag. Autonomous lines use the canonical log and only newly observed speech IDs reach browser playback. Full-suite count and final bundle measurement are recorded in the current scorecard entry. |
| Install, update, packaging, and release paths | Installer/update reviewed in first pass | Installer state is closed and strongly typed, delegated relative paths remain stable, binary sets and pinned Parakeet assets replace atomically with rollback, session PATH entries survive dependency refresh, and generated launchers have an owned removal path. A real clean-machine bootstrap and Phase 16 packaging/release artifacts still require dedicated acceptance. |

## First-Pass Findings Closed

- Settings callers could mutate stored worker arguments through returned slices.
- Trace-ring overwrite shifted the full buffer for every row and returned data
  that could alias stored pointer fields.
- JSON decoders could accept a silently truncated body at the size limit, and
  response encoding could panic after headers were written.
- Browser query IDs could authorize mutating controller work, and browser
  requests had no global loopback same-origin boundary.
- Concurrent starts could create multiple idle engines; owner transitions and
  shutdown could leave delayed starts targeting cleared engines.
- Idle and completing engines were not consistently visible or stopped during
  teardown, and process shutdown closed shared resources before HTTP handlers
  drained.
- Windows checkout line endings could make unchanged Go files fail local
  formatting checks; the repository now declares LF for Go and module files.
- Chat cursors could advance beyond the log head and permanently skip later
  messages; cursor advancement now clamps to the current head transactionally.
- Chat, memory, prompt, pattern, and model-inventory storage failures could look
  like valid empty state. APIs now expose availability and return redacted
  storage errors instead of silently discarding the failure.
- Pattern and personalization capacity checks could race concurrent writers;
  limits and returned records now come from the committing transaction.
- Mode start, stop, retarget, and chat-generated updates could publish stale
  state after cancellation. Lifecycle transitions and operation generations are
  serialized and covered by repeated concurrency tests.
- LLM streams and managed runtimes could remain live or ambiguous after partial
  failures. URL validation, response limits, terminal markers, bounded unload,
  provider cache transitions, and managed-model recovery are now explicit.
- Validation could mix trace rows from earlier runs and swallow stop or export
  failures. Runs now have a single trace boundary and combine teardown errors.
- Production opened six independent SQLite pools and discarded every idle
  connection. `config.Store` now owns one bounded pool that every logical store
  borrows, with one warm idle connection and an explicit close boundary.
- A negative `user_version` could index the migration slice with `-1`, and
  known Rockfire shapes were reconciled only when v8 happened to be the last
  migration. Version bounds and the actual compatibility boundary are now
  explicit and covered at the current schema.
- Physical database corruption had no recovery path, malformed settings could
  be overwritten without durable evidence, and logical current-schema damage
  was not validated. Schema v10 adds bounded settings recovery; physical files
  are quarantined exactly while logical damage fails non-destructively.
- Settings-document migrations were reported but not written back, and legacy
  reads plus app writes had no common document-size bound. Migrations are now
  durable and both directions enforce 256 KiB without altering an oversized
  legacy file.
- Transaction callback panics relied on connection cleanup for rollback, and a
  chat-log delete bypassed the serialized writer. Deferred rollback and one
  `WithTx` mutation path now make both lifecycle guarantees explicit.
- Installer state accepted string booleans, unknown fields (including secret-like
  fields), and inconsistent runtime choices. Reads and writes now enforce one
  closed schema and its cross-field invariants.
- A later worker compile could leave the repository with a mixed-version binary
  set. All four binaries now build under an explicit Windows/pure-Go target and
  promote as one rollback-capable set, independent of the caller's directory or
  Go environment.
- Existing Parakeet files were trusted by presence after the outer archive was
  verified. The installer now verifies pinned inner runner and license hashes,
  restores an interrupted verified backup, and rolls back failed activation.
- Updater-relative state paths could change meaning when delegated to the
  installer; PATH refresh could discard session-only tools; disabling launcher
  creation left a generated launcher behind. Each lifecycle now has regression
  coverage and an explicit ownership boundary.
- Pattern-library reads could fail into a catalog that looked valid but empty,
  one route-wide busy ID could unlock or hide overlapping work, and immediate
  object-URL revocation could cancel exports. Reads now expose retryable errors,
  semantic action keys serialize only conflicting work, import responses update
  the catalog directly, and export cleanup is deferred until navigation starts.
- Authoring tabs discarded unsaved drafts, overlapping backend previews could
  apply out of order, changing a knot time remounted its input, and freehand
  drawing re-rendered React on every point. Panels now remain mounted, preview
  generations invalidate stale responses, knot rows retain identity, and the
  canvas renders pointer drafts directly before one committed state update.
- Keying the route error boundary by the full hash unmounted Settings between
  subsections and silently discarded unsaved fields. The boundary now follows
  the top-level workspace, with a focused route-lifetime regression test.
- Settings, chat history, memory, prompt sets, managed models, and voice runtime
  failures could render as endless loading, valid empty data, disabled workers,
  or an empty conversation. Each surface now has an explicit loading/error/data
  distinction; retry keeps the last coherent snapshot where one exists.
- Quick-setting edits inside the debounce window were discarded on navigation,
  including edits queued behind an in-flight write. Teardown now flushes both
  cases while mounted-only reconciliation and notifications remain isolated.
- Cross-tab chat tail failures did not retry unless the sequence changed again.
  One in-flight tail reader now retries against the next backend poll and
  reports delayed synchronization without blocking an already coherent log.
- Mobile navigation labels disappeared with their visual text, the manual speed
  slider had no programmatic name, voice provider fields were ambiguous, route
  titles stayed static, and populated library views skipped heading level two.
  Explicit names, document titles, and route-local headings now have focused
  tests plus rendered desktop/mobile evidence.
- PR #97 rendered the Import waveform and trim slider with separate coordinate
  systems, used scaled/clipped SVG hit targets, could strand boundaries outside
  the viewport without a discoverable recovery control, and captured ordinary
  vertical wheel input for zoom. The integrated timeline now uses direct 44 px
  trim handles, compact named icon controls, action snapping through the active
  viewport, explicit fit recovery, cursor-anchored wheel zoom, horizontal wheel
  panning, and a proportional viewport scrollbar. Desktop, mobile,
  zoomed-payload, and isolated backend persistence checks cover it.
- Memory, prompt-set, Freestyle, and style commands could overlap before a
  state-driven disabled control rendered. Ref-level admission guards serialize
  each mutation domain; mode-specific Stop now respects read-only ownership
  while global Emergency Stop remains unconditional.
- PR #101 originally presented LLM autonomy as a Preset Mode beside Freestyle,
  despite its conversational purpose, and the model received no conversation
  history. Autopilot now has one compact session control on Chat, uses a bounded
  canonical history tail, and leaves Preset Modes deterministic.
- Manual Motion treated every running engine as an active test, so starting
  Autopilot changed the diagnostic control to "Restart test" and enabled its
  local Stop. The React contract now reads `target.source`, and manual start
  stops the active run and drains the mode before taking ownership of the
  shared engine. This is runtime provenance and lifecycle state, not persisted
  data; no database migration is required.
- Holding an LLM-selected custom library pattern retained its ID but lost the
  resolved definition, so the engine could silently play the built-in fallback.
  Hold and intensity drift now retain the same resolved definition and trace the
  actual manager segment index.
- Autonomous assistant lines were enqueued for TTS without a browser delivery
  bridge, could race Stop after a segment was armed, and could deepen an already
  slow speech queue. They now enter the canonical log first, carry an ephemeral
  request ID only to newly observing controller tabs, cancel with the mode, and
  stay text-only while TTS is busy.

This document intentionally does not declare the repository audit complete.
