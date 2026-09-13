# Durable chat recovery across LAN/WAN clients

Implementation checkpoint: 2026-09-13 on `codex/lan-wan-control`. This advances
LAN-03, LAN-05, LAN-07 and LAN-12 in the [full checklist](lan-wan-control-checklist.md).
It does not close external/mobile acceptance or the complete resource/fault matrix.

## Why display sequence alone was insufficient

A generated assistant reply receives its display sequence while it is pending.
It becomes visible only after the existing commit barrier accepts it. A newer
message can commit first. Reading only `seq > highest_seen_seq` then misses the
older reply when it eventually commits. Separately, reading rows and then the
log head in different database snapshots could advertise a head beyond the rows
actually delivered; the browser previously acknowledged that head.

The implementation separates display order from committed recovery order:

| Event | Display sequence | Committed revision | Recovery behavior |
| --- | --- | --- | --- |
| First message commits | 10 | 10 | Delivered normally. |
| Assistant reply is staged | 11 | 0 | Not present in durable reads. |
| Newer message commits | 12 | 11 | Reader can advance through revision 11. |
| Staged reply commits | 11 | 12 | A read after revision 11 includes display row 11. |

The browser renders rows in display order and recovers by committed revision.
SSE status/message events can update a visible placeholder's sequence, but
cannot advance the durable read cursor. At stream completion, the browser
reconciles all committed changes since its last accepted page, merging matching
placeholders without duplicating the user's message or assistant reply.

## Persistence and upgrades

Schema v21 adds a committed revision to each message, conversation revision and
reset/pruning markers, and a revision position beside each legacy read cursor.
The revision advances in the same transaction as a visible append or pending
reply commit. Discarding an uncommitted draft does not publish a revision.
Deleting a committed row advances the conversation and requests a full reset;
ordinary pruning records the highest removed revision so a reader can detect
that unseen history left the retained window.

The existing per-conversation limit of **200 committed messages** remains.
Migration seeds existing committed revisions from their display sequences,
leaves pending revisions at zero, and preserves later revisions if a migration
is reapplied to an already-upgraded schema. Existing read markers survive within
the storage cap; migration does not expire them merely because an old fixture
or installation has an old timestamp. Normal reads and writes apply expiry.
No conversation content is rewritten by this migration.

## Read and acknowledgement protocol

`GET /api/chat/messages?session_id=…&after_revision=…` reads the conversation
using one short read transaction. Message rows, retained bounds, conversation
head and cursor therefore refer to the same committed database snapshot.
The transaction ends before any HTTP response write. It does not acquire the
application's serialized write gate.

| Field | Meaning |
| --- | --- |
| `latest_seq` | Highest retained display sequence; informational. |
| `revision` | Current committed conversation head. |
| `next_revision` | Recovery position through the changes delivered by this page. |
| `first_seq` | Earliest retained display row; old cached rows below it are removed. |
| `has_more` | More committed changes remain after this page. |
| `reset` | Replace the cached conversation with the supplied retained snapshot. |
| `history_gap` | Some changes after the requested cursor are no longer retained. |
| `server_epoch` | Process identity used to reject stale responses and acknowledgements. |
| `cursor`, `cursor_revision` | This caller's saved display/recovery read positions. |

Normal delta reads honor a positive `limit`, capped at 200. A reset returns the
complete retained window, even if a smaller limit was requested: using a partial
reset cursor below the deletion marker would otherwise repeatedly restart the
same snapshot. Initial reads, a future cursor, explicit deletion and a missed
retention window can require a reset. The response makes retention loss visible;
it does not promise recovery of data already removed by the existing log cap.

The legacy `after` sequence query remains supported and cannot be combined with
`after_revision`. It does not acquire the new revision protocol's late-commit
guarantee. The shipped browser uses committed revisions. Snapshot byte budgets
and pagination for exceptionally large retained messages remain part of
LAN-07/LAN-13; a row count alone is not a response-byte bound.

`POST /api/chat/cursor` accepts `seq`, `revision`, `server_epoch` and the
conversation ID. A revision acknowledgement from an old process is rejected.
Both positions are clamped to committed bounds and advance monotonically within
those bounds. The legacy sequence-only form remains available. Cursor requests
are passive login activity, require no controller lease, and do not consume
control-command sequence numbers or attempt command-receipt recovery.

## Cursor identity and resource bounds

A protected cursor key derives from the authenticated login session and the
declared browser ID using a domain-separated SHA-256 digest. The stored cursor
key includes neither the bearer token nor the private session identity. Reads
and writes use the same derived key. A different login of the same account, or
another account copying the public browser ID, cannot change that cursor.
Distinct declared tabs keep distinct positions, while same-login tabs still
share their session security boundary. Browser IDs are not credentials.

Unprotected loopback clients retain their ID-based compatibility. Enabling
account protection starts protected cursor positions independently of the
old unbound markers. Shared conversation visibility remains the installation's
existing account policy; cursor isolation does not create separate tenants.

Read-marker storage is capped at **4,096 rows** during migration and advancement.
Markers expire logically after **24 hours**; an advancing write removes expired
rows and evicts the oldest excess markers. Repeated unchanged acknowledgements
avoid a database rewrite and do not refresh marker retention. Eviction loses
only a read marker, never a conversation or authority grant. Request contexts
reach active-session lookup, cursor reads, the writer queue and SQL commit, so
a canceled waiter cannot later advance a cursor.

## Browser lifecycle and speech

The focused `useChatHistory` hook owns the rendered cache and read lifecycle.
It retains at most one history read and one cursor acknowledgement, with
**15-second** deadlines. It consumes at most **four delta pages** in a catch-up
burst, continuing on later app-state polls if needed. The first failure may retry
on the next poll; repeated failures back off to at most 30 seconds. Successful
delivery resets that backoff.

Offline transitions, a new server epoch, visibility changes and unmount cancel
obsolete reads and acknowledgements. Request-generation checks also reject a
late response after cancellation. A returning browser obtains a fresh retained
snapshot without invoking takeover, Start or Resume. Chat streams abort on
backgrounding, backend loss, observer status or process change, and the existing
Emergency Stop sequence handling remains in place.

Only normal live tail delivery queues new autonomous speech. Initial history,
resets, visibility/reconnect recovery and post-stream reconciliation do not
replay old audio. Live SSE and tail delivery share a bounded set of 256 speech
request IDs, and permission/visibility are rechecked before queueing. These
changes do not add an audio lease or motion path.

## Evidence and remaining acceptance

- [Storage regressions](../internal/chat/log_recovery_test.go) cover late commits
  below the display head, restart recovery, concurrent snapshot consistency,
  partial pages, deletion/future-cursor resets, retention gaps, canceled queued
  cursor writes, marker expiry and the storage cap.
- [Migration regressions](../internal/store/schema_chat_recovery_test.go) retain
  v20 content/read positions, seed committed revisions, preserve an existing
  revision on reapplication and bound excess markers. The earlier v11 migration
  preservation assertion remains intact.
- [HTTP regressions](../internal/httpapi/chat_recovery_test.go) exercise copied
  IDs across logins/accounts, separate declared tabs, old epochs, malformed
  queries and late committed replies through the real handlers.
- [Browser regressions](../web/src/components/useChatHistory.test.tsx) exercise
  ordered merging, partial pages, aborted old responses, observer changes,
  background/return, retention gaps, invalid metadata, deadlines and backoff.
  ChatPanel tests reconcile missing messages after SSE without duplicates or
  speech replay. API tests ensure read tracking stays outside command delivery.

Full verification, the current-source review and measured artifact sizes are in
the [implementation log](lan-wan-implementation.md) and [scorecard](goal-scorecard.md).
Real WAN/client scheduling, response-byte budgets, broader route/permission
coverage and the network fault/load/soak matrix remain open. The full LAN/WAN
goal remains active.
