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
| Voice, workers, queues, and audio | Not started | Existing provider and seed work is not treated as an audit result. |
| Frontend state, accessibility, and UI performance | Not started | Requires source review plus desktop/mobile browser verification. |
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

This document intentionally does not declare the repository audit complete.
