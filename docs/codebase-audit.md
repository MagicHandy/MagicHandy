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
| Motion engine and transports | In progress | Engine ownership/admission races and idle-engine teardown are covered. Sampler, retargeting, Cloud, Bluetooth, and Intiface behavior remain separate audit items. |
| Chat, LLM, memory, modes, patterns, library, validation | Not started | Existing test coverage is not treated as an audit result. |
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

This document intentionally does not declare the repository audit complete.
