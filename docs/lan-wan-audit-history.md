# Bounded access and control history

The LAN/WAN implementation adds administrator history under **Settings → Access
→ Access and control history**. It is part of LAN-16 and contributes a storage
contention regression to LAN-06. The complete network, hardware, resource and
security acceptance matrix remains open in the
[checklist](lan-wan-control-checklist.md).

## Recorded outcomes

Schema 23 adds `access_audit` to the existing SQLite datastore. Account creation,
enable/disable, password changes, session creation/revocation and control grants
write their event in the same transaction as the access change. An audit insert
failure rolls back that transaction; it cannot leave a successful access change
with a silently missing record. Grant events include their expiry. Password and
disable events also imply the existing session invalidation behavior.

Runtime records cover rejected/throttled JSON logins, initial control claims,
completed takeovers, controller loss, admitted protected command outcomes,
Emergency Stop results and core start/shutdown. A replayed command returns the
original receipt without adding another execution record. A completed HTTP
command records the handler result; it does not assert physical execution or
successful model generation. Interrupted handlers are marked unknown. Ordinary
unprotected local requests and every denied/malformed API request are not a
complete activity feed.

Attribution uses the authenticated account and its independent session
management ID, rather than the selected linked profile or a browser-supplied
client ID. Grant IDs and ownership generations link controller transitions.
Watchdog/lifetime losses are system actions linked to the retired generation.
Public Stop intentionally has an unauthenticated actor even if a caller sends
a cookie: its safety path performs no account/database admission.

Each runtime event has a process epoch and the latest trace sequence observed
when it is recorded. The stopped-run trace export now includes that epoch.
These establish a run and nearby trace boundary, not exact device timing.
Overlapping Emergency Stops retain their own invalidation sequence. Command
correlation is a domain-separated SHA-256 digest of the command ID, never the
original ID or request body. The expandable UI exposes these references for
support correlation.

## Storage and response bounds

The table retains at most **10,000 events**. An insertion trigger enforces the
row cap for both transactional and queued writers; schema validation checks
that the bound remains installed. Events older than **30 days** are excluded
from reads, deleted on runtime batches, and pruned hourly while idle. Deletion
is logical retention, not a promise of secure disk erasure or SQLite file
shrinkage.

Runtime logging has one worker, **256 waiting events**, and batches of at most
**64 events**. Enqueue never waits for the database. Writes get two cancelable
500 ms attempts, using unique event IDs so an uncertain first commit cannot
duplicate the retry. Queue overflow and failed writes increment explicit loss
counters. A subsequent successful write adds a history-gap event. The worker
drains for at most two seconds during orderly shutdown and is joined before
the shared datastore closes.

The writer status reports queue occupancy, storage availability, failed writes
and dropped events since startup. History and its download show this status;
the UI warns when history may be incomplete. A process crash or persistent
storage failure can lose queued events and their gap report. This is bounded
operational history, not a tamper-proof or lossless forensic log.

Administrator-only `GET /api/audit` and `GET /api/audit/export` return at most
**100 events / 256 KiB**, in descending sequence order. `before` requests older
pages. The database read transaction ends before any socket write, and the
request/write lifetime is bounded. Responses are not cacheable. Export is an
attachment with the same fresh administrator authorization as the page read.

The panel does not poll. It fetches only while expanded, retains one page,
aborts obsolete requests on disconnect/collapse/unmount, and bounds reads and
downloads to ten seconds. **Download this page** makes one export request using
the displayed page's upper sequence boundary, then downloads JSON with one
click. Retention may remove rows between display and download; the export is
a fresh retained view. All five supported locales include the new controls.

## Privacy boundary

An explicit event schema and reviewed classification codes have no fields for
passwords, tokens, token hashes, raw headers, request URLs/query strings,
command bodies, raw failures, chat, audio or private file paths. Identifiers
are validated non-authenticating references. Usernames are resolved for display
from the administrator's account list and are not persisted in audit records.
Exports use the same bounded event schema. Unknown/corrupt stored attribution
fails the read instead of manufacturing an identity or timestamp.

## Stop and trace persistence

The storage-contention test exposed existing synchronous trace persistence
after the engine had stopped. It could delay the public Stop response by up to
two seconds waiting for SQLite. Trace persistence now has one background worker,
one pending latest document and one in-flight document. The latest immutable,
sanitized trace remains immediately readable from memory. Superseded pending
runs coalesce, and an older callback cannot replace a newer trace.

Trace documents keep the existing **128-row / 1 MiB** bound. An orderly close
flushes the latest pending run within a two-second cancellation budget; a
storage failure leaves the in-memory trace available but may prevent recovery
after restart. This worker never issues motion commands. The shared engine,
sampler, transport dispatch, physical Stop and motion-teardown gates remain
the motion path.

## Verification

Regressions cover transaction rollback, migration/reapplication, retention,
pagination, malformed stored data, retry deduplication, overflow/gap reporting,
blocked storage and worker shutdown. HTTP tests cover actor/grant/generation
correlation, administrator-only bounded exports, replay deduplication, public
Stop under a held datastore writer and overlapping Stop sequence attribution.
Trace tests cover coalescing, immediate reads, redaction, shutdown durability
and cancellation. Browser tests cover on-demand reads, bounded pagination,
one-click export, blob cleanup, late responses, disconnect/collapse, timeouts
and explicit loss/unconfirmed outcomes.

These are deterministic application/simulator tests. They do not establish
physical stopping latency, WAN availability, flood resistance or real-client
acceptance. The [implementation log](lan-wan-implementation.md) records the
current built-app verification and remaining work.
