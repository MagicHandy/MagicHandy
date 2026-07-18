# Codebase Audit Ledger

This ledger tracks the systematic reliability, maintainability, and efficiency
review. A subsystem is complete only after its code paths, ownership and
lifecycle boundaries, tests, and relevant documentation have been reviewed.

Baseline: `origin/main` at `7071e37a` (2026-07-16).

| Subsystem | Status | Current evidence |
| --- | --- | --- |
| Configuration and persistence boundaries | Reviewed in dedicated pass | One process-owned SQLite pool serves six logical domains; writes share one transaction lock, schema v10 preserves invalid settings, physical corruption is quarantined, logical damage fails clearly, and schema/version/permission/lifecycle behavior has focused coverage. |
| Diagnostics and structured logging | Reviewed in first pass | Trace storage now overwrites in O(1) and returns independent snapshots. Logging volume and redaction need review with each provider/transport. |
| HTTP and process lifecycle | Reviewed in first pass | Oversized JSON is rejected, response encoding cannot panic after committing headers, browser requests are loopback same-origin, mutating leases require headers, and shutdown quiesces device work before closing stores. |
| Motion engine and transports | Reviewed in first pass | PR #87 serializes ownership and command admission, hardens transport teardown, and expands race and lifecycle coverage. Real-device behavior remains subject to the documented hardware validation matrix. |
| Chat, LLM, memory, modes, patterns, library, validation | Reviewed in first pass | Storage failures are explicit, mutations are transactional, mode lifecycle and stale-operation races are covered, provider/runtime limits are bounded, managed-model inventory is crash-safe, and validation exports only the active run. |
| Voice, workers, queues, and audio | Reviewed in first pass | Worker framing and deadlines, bounded request queues, cancellation, process-tree teardown, provider response limits, deterministic sampling, and reference-code validation have focused coverage. Representative listening, simultaneous GPU LLM/TTS load, and lower-VRAM acceptance remain open. |
| Frontend state, accessibility, and UI performance | Review in progress | Browser-owned Bluetooth Stop delivery and percent encoding, ordered eager TTS retrieval, voice-capture Stop epochs, serialized quick settings, reset/save races, status polling, and chat SSE framing have focused coverage. Changed chat, voice/model settings, and connection surfaces pass 1440x900 and 390x844 rendered checks; bundle growth is measured. Library/authoring mutation UX, remaining route accessibility, and a full-route desktop/mobile pass still require dedicated review. |
| Install, update, packaging, and release paths | Not started | Requires clean-machine dependency and choice-retention review. |

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

This document intentionally does not declare the repository audit complete.
