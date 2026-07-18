# Codebase Audit Ledger

This ledger tracks the systematic reliability, maintainability, and efficiency
review. A subsystem is complete only after its code paths, ownership and
lifecycle boundaries, tests, and relevant documentation have been reviewed.

Baseline: `origin/main` at `db8ca56d` (2026-07-16).

| Subsystem | Status | Current evidence |
| --- | --- | --- |
| Configuration and persistence boundaries | Reviewed in first pass | Settings snapshots and save results now own their slice data; aliasing regression test added. SQLite schema, locking, recovery, and filesystem permissions still require a dedicated pass. |
| Diagnostics and structured logging | Reviewed in first pass | Trace storage now overwrites in O(1) and returns independent snapshots. Logging volume and redaction need review with each provider/transport. |
| HTTP and process lifecycle | Reviewed in first pass | Oversized JSON is rejected, response encoding cannot panic after committing headers, browser requests are loopback same-origin, mutating leases require headers, and shutdown quiesces device work before closing stores. |
| Motion engine and transports | Reviewed in first pass | PR #87 serializes ownership and command admission, hardens transport teardown, and expands race and lifecycle coverage. Real-device behavior remains subject to the documented hardware validation matrix. |
| Chat, LLM, memory, modes, patterns, library, validation | Reviewed in first pass | Storage failures are explicit, mutations are transactional, mode lifecycle and stale-operation races are covered, provider/runtime limits are bounded, managed-model inventory is crash-safe, and validation exports only the active run. |
| Voice, workers, queues, and audio | Reviewed in first pass | Worker framing and deadlines, bounded request queues, cancellation, process-tree teardown, provider response limits, deterministic sampling, and reference-code validation have focused coverage. Representative listening, simultaneous GPU LLM/TTS load, and lower-VRAM acceptance remain open. |
| Frontend state, accessibility, and UI performance | Review in progress | Browser-owned Bluetooth Stop delivery and percent encoding, ordered eager TTS retrieval, voice-capture Stop epochs, serialized quick settings, reset/save races, status polling, chat SSE framing, and library/authoring lifecycle failures have focused coverage. Changed chat, voice/model settings, and connection surfaces pass 1440x900 and 390x844 rendered checks; bundle growth is measured. Remaining route accessibility and a full-route desktop/mobile pass still require dedicated review. |
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

This document intentionally does not declare the repository audit complete.
