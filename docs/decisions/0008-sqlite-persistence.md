# ADR 0008: SQLite Persistence Datastore

## Status

Accepted for the rewrite plan. Implemented in Phase 11B; extended through
schema v9 by the LLM model-manager foundation.

## Context

MagicHandy persists three independent JSON files in the app data directory,
each with the same shape:

- `settings.json` (`internal/config`) — a versioned settings document with a
  redacted `Public()` view (the Handy connection key is never returned),
  migration hooks, and corrupt-file recovery to defaults.
- `memories.json` (`internal/memory`) — `{enabled switch, memories[]}`, capped
  at 200 items.
- `prompt_sets.json` (`internal/chat`) — user prompt sets only; built-in sets
  are code-defined and never written to disk. Capped at 100.

Every store does a whole-document read on open and a whole-document rewrite
(temp file + atomic rename) on **every** mutation, carries its own `version`
int with hand-written migration/normalization, and recovers a corrupt file to
safe defaults without failing startup. Three separate implementations of the
same durability, versioning, and recovery logic.

That pattern is fine at the current bounds but is the wrong shape for what is
coming. The shared chat message log with per-client cursors (ADR 0003, Phase 13 foundation)
and the Phase 14 pattern/program library with tags, enable/disable, and
feedback weights are append-heavy and query-shaped. Rewriting an entire growing
log on every append, and re-reading it wholesale to answer a cursor or filter
query, does not scale.

The pure-Go rule in `docs/goals-and-guardrails.md` already anticipated this:
*"if a datastore is ever needed, a pure-Go one, not a CGo SQLite driver."*
Adopting SQLite is therefore not a new direction — it is the datastore the
guardrails pre-approved, on the condition that the driver is CGO-free.
`modernc.org/sqlite` (cznic) is exactly that: a pure-Go SQLite that builds with
`CGO_ENABLED=0`, so it preserves the single-binary, free-cross-build, and
pure-Go-core guarantees. The CGo `mattn/go-sqlite3` is explicitly disqualified.

## Decision

Adopt a single embedded SQLite datastore, `magichandy.db`, in the resolved app
data directory, accessed through `modernc.org/sqlite` via `database/sql`. A new
`internal/store` package owns the connection, schema, and migrations. The
existing config/memory/chat stores keep their interfaces and method contracts;
only their durability substrate moves from JSON files to DB tables.

### Driver and build

- `modernc.org/sqlite` (pure Go). No CGo. The `CGO_ENABLED=0` build and free
  cross-compilation stay intact; depguard's `C` denial is unaffected.
- License: `modernc.org/sqlite` and `modernc.org/libc` are BSD-3-Clause,
  compatible with MagicHandy's GPL-3.0-only license.
- The release matrix (windows/amd64 primary; linux/amd64, darwin/amd64,
  darwin/arm64 best-effort) is within the driver's supported platforms; any new
  target arch is checked against the driver's support list before it is added.

### One database, tables by data shape

- **Relational tables** where per-item mutation and queries pay off:
  - `memories(id TEXT PRIMARY KEY, text, enabled, created_at)` plus the global
    injection switch as a settings/kv value.
  - `prompt_sets(id TEXT PRIMARY KEY, name, system, created_at)` — user sets
    only; built-ins remain code-defined and never enter the DB.
  - (landed with the Phase 13 delivery-ordering foundation, schema v2) a
    `messages` shared chat log and `client_cursors` per-client
    cursors (ADR 0003).
  - (landed in Phase 14, schema v3) `patterns`, `programs`, and
    `pattern_feedback`. Pattern points and tags are JSON payloads inside
    relational catalog rows; finite programs stay in a separate table so a
    media-timed script cannot be mistaken for a repeatable loop.
  - (landed with the model manager, schema v9) `llm_models`, a searchable
    inventory of managed model metadata and import lineage. Multi-gigabyte GGUF
    bytes remain ordinary files in the app data model store, never DB blobs.
- **Settings stays a versioned document**, stored as one row in a `settings`
  document/kv table rather than exploded into columns. This preserves the
  existing `Settings` struct, `NormalizeSettings`, the migration hooks, and the
  redacted `Public()` view unchanged — only the substrate changes (file → row).
  Normalizing settings into columns is a deliberate non-goal (see Alternatives).
- **Not persisted in SQLite**: the diagnostics trace ring (ephemeral,
  high-frequency, in-memory), motion/engine runtime state, and embedded web
  assets. Managed llama.cpp build jobs are also process-local. The active
  runtime manifest stays beside the versioned runner files because it must be
  activated atomically with that filesystem install; durable model inventory
  and selected model ID remain in SQLite-backed model/settings records.

### Connection and durability settings

These encode the well-known SQLite-under-`database/sql` pitfalls so the
implementation does not rediscover them:

- `PRAGMA journal_mode=WAL` so a read never blocks the single writer.
- `PRAGMA foreign_keys=ON`, `PRAGMA synchronous=NORMAL` (WAL-safe), a bounded
  `PRAGMA cache_size` to keep RSS predictable, and a `busy_timeout`.
- Single-writer discipline: SQLite allows one writer at a time. The store
  serializes writes (a dedicated writer connection / tuned `SetMaxOpenConns`
  plus `busy_timeout`) so `database is locked` cannot surface under the app's
  own concurrency. The app is a single local operator, so this is sufficient.
- Each logical mutation is one transaction. A settings reset that must leave
  memories and prompt sets untouched is a scoped transaction on the settings
  table only — the current "reset does not touch memory or prompt sets"
  contract is preserved exactly.

### Schema migrations

One forward-only migration runner keyed on `PRAGMA user_version`, replacing the
three separate hand-written per-file version ints. Migrations are ordered and
run inside a transaction at open. A schema newer than the binary is a clear,
non-destructive error — never a silent downgrade.

Phase 14 publishes schema v8 because the remote `Rockfire` branch had already
used versions 1–7 for a divergent LSO-oriented schema. Versions 4–7 are reserved
compatibility markers, and v8 is an idempotent reconciliation migration. It
creates any missing canonical tables, converts the Rockfire integer-ID settings
row to the canonical `id='current'` document row without losing its JSON or
timestamp, and repairs the older `prompt_sets` shape. Rockfire-only tables such
as motion blocks, funscript files, queues, personas, and UI layouts are left
untouched for the explicit Phase 15/LSO importer; schema reconciliation does
not guess at their semantics.

Schema v9 appends the `llm_models` inventory and unique SHA-256/path indexes.
It does not reinterpret any Rockfire-only table and does not move model bytes
through SQLite.

### One-time import from the JSON stores

On first open where `settings.json`, `memories.json`, or `prompt_sets.json`
exist, import each legacy store into its DB tables inside a SQLite transaction,
then rename that JSON file `*.migrated` (rather than deleting it) only after the
commit so a bad import is recoverable. The import is non-destructive, logged in
the `legacy_imports` table, and settings import is reported in load status. This
mirrors the R8 user-migration discipline for the app's own files, and it is the
same relational target the Phase 15 StrokeGPT-ReVibed importer will write into.

### Redaction and at-rest sensitivity

The connection key stays a private credential. It is stored in the settings
document exactly as sensitively as `settings.json` stores it today — plaintext
at rest under the app data directory with `0700` perms; the trust model is a
single local operator, not encryption-at-rest. It is never returned by settings
reads, diagnostics, trace exports, or the redacted `Public()` view. The `.db`
file inherits the same file sensitivity `settings.json` had.

## Consequences

Positive:

- Mutations become row-scoped (add or toggle one memory; append one message)
  instead of rewriting a whole growing file. Queries (enabled memories, a
  client's unread messages, enabled patterns) become indexed lookups instead of
  load-everything-then-filter.
- One transactional store: atomic multi-row operations, one durability
  mechanism, one migration runner, one file to back up — replacing three
  bespoke atomic-write + version + recovery implementations.
- The chat log and Phase 14 library use the same transaction and migration
  substrate instead of inventing persistence per feature.
- Still fully embedded and offline; still `CGO_ENABLED=0`; still one binary;
  cross-builds stay free.

Negative / deliberate trade-offs:

- Binary size grows: `modernc.org/sqlite` + `modernc.org/libc` are large
  transpiled packages. Phase 11B measured 17.92 MB plain / 12.32 MB stripped,
  still under the < 30 MB stripped budget.
- Core RSS grows materially. Phase 11B measured 54.13 MB idle after `/healthz`
  and 54.36 MB after DB-backed API reads, exceeding the original < 40 MB idle
  budget. This is recorded as a Phase 11B waiver in `docs/goal-scorecard.md`,
  not silently relaxed.
- New dependency surface: this is the first substantial third-party runtime
  dependency in the core (`modernc.org/sqlite` and its transitive `modernc.org`
  packages, all pure-Go and permissively licensed). Accepted for what it buys.
- A schema is now a migration surface — mitigated by forward-only `user_version`
  migrations, transactional open, and the non-destructive one-time import.
- A divergent development branch consumed schema versions before merge. The v8
  compatibility migration preserves its rows but intentionally does not expose
  them as canonical library content until Phase 15 can produce a dry-run report
  and explicit field mapping.
- Pure-Go SQLite is slower than the C build. For this workload (small
  settings/memory/prompt data and a local chat log) it is far more than
  adequate; the app is a single local operator, not a high-QPS service.
- Startup on a *corrupted store* changes behavior. The JSON stores recovered a
  corrupt or unreadable file to safe defaults and never failed startup (a
  hard-won StrokeGPT lesson). A corrupt/unreadable `magichandy.db` currently
  fails at open instead, because `store.Open` propagates the error. WAL plus
  `synchronous=NORMAL` make corruption unlikely, and failing clearly beats
  silently discarding a user's data — but restoring "never fail startup" (back up
  the bad DB, start fresh, report it in load status) is a tracked follow-up, not
  yet implemented. Recovery of a corrupt *legacy JSON* file during the one-time
  import is preserved (it is recorded as `recovered` and defaults stay active).

## Alternatives considered

- **Keep the JSON files.** Simple and working, but does not scale to the chat
  log or pattern library and keeps three separate atomic-write/version/recovery
  implementations. Rejected for the growth path.
- **A pure-Go key-value store (e.g., `bbolt`).** Also CGO-free, embedded, and
  lighter than SQLite. But the coming data is relational and query-shaped
  (cursors, filters, joins across patterns and feedback), which SQL expresses
  directly; a KV store pushes that logic back into Go. SQLite is the better fit,
  and the guardrails named SQLite specifically.
- **Normalize settings into columns.** More relationally "correct," but discards
  the working versioned-document + migration + redaction machinery for a single
  small document that is always mutated as a whole. Rejected; settings stays a
  document row.
- **CGo SQLite (`mattn/go-sqlite3`).** Faster, but requires CGo — breaks
  `CGO_ENABLED=0`, free cross-builds, and the pure-Go rule. Explicitly
  disqualified by `docs/goals-and-guardrails.md`.

## Relationship to other docs

- `docs/goals-and-guardrails.md`: satisfies the pure-Go datastore clause; its
  binary/RSS budgets govern the size and memory cost, tracked in the scorecard.
- ADR 0001 (Go-first core): keeps the single-binary, pure-Go promise.
- ADR 0003 (voice worker boundary / message-and-audio delivery ordering): the
  shared chat message log with per-client cursors becomes a table in this store.
- `IMPLEMENTATION_PLAN.md`: introduced in Phase 11B; consumed by the Phase 13 foundation (chat
  log) and Phase 14 (pattern library); Phase 15 (StrokeGPT-ReVibed import)
  targets the same schema.
- `docs/risk-register.md`: R19 (datastore migration and budget) tracks migration
  correctness and budget impact; relates to R8 (user migration) and R11 (goals
  unmeasured).
