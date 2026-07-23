# ADR 0008: SQLite Persistence Datastore

## Status

Accepted for the rewrite plan. Implemented in Phase 11B; extended through
schema v13 by atomic chat-reply visibility.

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
    `messages` shared chat log and `client_cursors` per-client cursors (ADR
    0003); schema v12 scopes messages and cursors to `chat_sessions`, adds the
    singleton `chat_workspace` active-session record, and retains bounded,
    non-secret response provenance in `diagnostics_json`. Schema v13 adds a
    commit marker so canceled generated replies never become visible or consume
    the per-session history cap.
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

The 2026-07-18 persistence-boundary audit classifies the remaining state so a
new feature does not accidentally create a second datastore:

- SQLite is authoritative for app settings and settings recovery history,
  memories, user prompt sets, chat sessions/messages/per-session cursors,
  patterns/programs/feedback, and managed-model inventory/import lineage.
- Files are intentional for large or atomically activated artifacts: model and
  runner bytes, activation manifests, voice reference WAV/code artifacts, and
  validation exports. A model metadata sidecar exists only to recover an
  interrupted filesystem install; it does not supersede SQLite inventory.
- Installer choices remain a private file because the installer and updater
  need them before the app or its database can exist. Logs are append-only
  operational evidence, not application state.
- Motion/controller state, diagnostics rings, active jobs, worker queues, and
  audio leases remain process memory. Browser client identity remains local to
  that browser profile. Persisting any of these would change lifecycle or
  privacy semantics and requires an explicit design decision.

### Connection and durability settings

These encode the well-known SQLite-under-`database/sql` pitfalls so the
implementation does not rediscover them:

- `config.Store` owns the one process-lifetime `store.DB`. Memory, prompt-set,
  chat-log, pattern, and model-inventory stores borrow that database; they do
  not open independent pools or close the shared owner.
- `PRAGMA journal_mode=WAL` is selected once and verified so a read never
  blocks the single writer. The DSN applies `foreign_keys=ON`,
  `synchronous=NORMAL`, bounded `cache_size`, and `busy_timeout` to every
  pooled connection rather than assuming connection-local pragmas carry over.
- The pool allows at most four open connections and retains one idle
  connection. This permits bounded concurrent reads without reconnecting for
  every operation.
- Single-writer discipline: SQLite allows one writer at a time. One mutex is
  associated with the shared database and every logical store writes through
  `WithTx`; `busy_timeout` also protects standalone tools that open the same
  file. This prevents the app's own concurrency from surfacing `database is
  locked`.
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

Schema v10 appends `settings_recoveries`, a bounded history of invalid settings
documents. Invalid or oversized active settings are copied there in the same
transaction that removes the active row, then safe defaults are activated.
Only the latest 20 records are retained. This preserves exact evidence for
support without exposing it through public settings or diagnostics.
Successful settings-document migrations are rewritten immediately so a restart
does not repeat an in-memory-only migration. App and legacy-file reads are
bounded to the same 256 KiB document limit enforced before writes.

Schema v11 adds the explicit-scan video catalog described in
`docs/video-playback.md`. Schema v12 turns the single chat stream into a
backend-owned workspace. Existing messages migrate into one saved "Previous
conversation" session; sequence values remain stable. `chat_sessions` owns tab
metadata and manual-save state, `chat_workspace` owns the one active session,
and `chat_session_cursors` isolates each browser cursor by session. Saved tabs
are durable. At startup, settings either restore the previous working session
or create a new unsaved one, and transactionally remove any other unsaved
drafts. A clean shutdown discards the working draft immediately when retention
is off; startup repeats that reconciliation because crashes and forced shutdowns
cannot run an exit hook. Starting with a new chat always discards the prior
unsaved draft, so that policy cannot be combined with unsaved-draft retention.

Schema v13 adds `messages.committed`. User and deterministic rows are committed
when inserted. Interactive and autonomous generated assistant rows are staged
with `committed=0`, which excludes them from history, cursors, session
summaries, prompt mood/context, and cap pruning. Immediately before the commit,
the delivery path compares the request's Stop epoch with the current atomic
epoch. That comparison is the acceptance point: if Stop already won, the row is
deleted; if completion won, the row commits and prunes in one transaction even
if a later Stop begins while SQLite finishes. The later Stop still fences motion
and TTS. Stop publishes its epoch without taking the database path or waiting on
SQLite; only the bounded in-memory TTS submission shares a mutex with voice
invalidation. Startup removes any staged row left by a process interruption.
Existing rows migrate as committed.

`messages.diagnostics_json` stores only bounded run provenance needed by the
assistant-avatar tooltip and continuity: source, provider/model identifiers,
prompt-set ID, elapsed time, parser/fallback flags, semantic motion action, and
strict mood metadata. Prompt text, memories, raw model output, request bodies,
user text beyond the message itself, and credentials are excluded.

Opening a current schema validates the expected tables, columns, indexes,
foreign-key enforcement, and `foreign_key_check`. Negative and newer-than-
binary `user_version` values fail clearly without indexing the migration list
or changing the file. The schema-v8 Rockfire reconciliation is keyed to its
actual compatibility boundary and also repairs known current-version shapes;
it is not coupled to whichever migration happens to be last.

### Corruption recovery and ownership

On open, `PRAGMA quick_check(1)` distinguishes physical SQLite corruption from
logical schema damage. A physically corrupt database is closed and its exact
database, WAL, and SHM files are moved into a timestamped private directory
under `recovery/`. MagicHandy then creates a fresh current schema and reports
the backup path in structured startup logging and public load status. The
preserved contents are never returned.

Logical damage, such as a missing required table at the current schema version,
fails startup with `ErrInvalidSchema`; it is not replaced automatically. This
keeps a migration or application bug from being misclassified as disposable
user data. If any quarantine move fails, already-moved files are restored and
startup fails rather than creating a partial replacement.

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
document exactly as sensitively as `settings.json` stores it today: plaintext
at rest under the app data directory; the trust model is a single local
operator, not encryption-at-rest. The directory is restricted to `0700` and
the database, WAL, and SHM files to `0600` on POSIX. Windows relies on the
user-profile ACL because `os.Chmod` cannot express the equivalent ACL. The key
is never returned by settings reads, diagnostics, trace exports, recovery
status, or the redacted `Public()` view.

## Consequences

Positive:

- Mutations become row-scoped (add or toggle one memory; append one message)
  instead of rewriting a whole growing file. Queries (enabled memories, a
  client's unread messages, enabled patterns) become indexed lookups instead of
  load-everything-then-filter.
- One transactional store: atomic multi-row operations, one durability
  mechanism, one migration runner, one datastore to back up — replacing three
  bespoke atomic-write + version + recovery implementations.
- The chat workspace and Phase 14 library use the same transaction and
  migration substrate instead of inventing persistence per feature.
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
- Physical corruption now starts with a fresh database only after preserving
  the exact database and sidecars in a private recovery directory. Logical
  schema damage still fails clearly. Invalid settings documents are archived
  in bounded schema-v10 history before defaults become active. Recovery of a
  corrupt *legacy JSON* file during the one-time import is also preserved (it
  is recorded as `recovered` and defaults stay active).

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
