# Protected command delivery

Implementation checkpoint: 2026-09-13, on `codex/lan-wan-control` / [PR 266](https://github.com/MagicHandy/MagicHandy/pull/266).
This extends [ADR 0029](decisions/0029-session-bound-network-control.md) and
LAN-01/03/04 of the [acceptance checklist](lan-wan-control-checklist.md).
The overall LAN/WAN goal and real-network acceptance remain open.

## Admission contract

Authenticated mutations whose handler requires controller ownership carry the
following metadata. The same requirements apply on loopback when accounts are
enabled. Unprotected trusted-local clients retain their compatibility contract;
the remote deployment modes require accounts.

| Value | Scope and rule |
| --- | --- |
| Session cookie | Private login credential, validated independently of every other field. |
| `X-MagicHandy-Client-ID` | Tab identifier, bound to that login session and its control grant. |
| `X-MagicHandy-Control-Epoch` | Random server boot identity, obtained from an authoritative snapshot. |
| `X-MagicHandy-Control-Generation` | Current ownership generation, rotated on Stop, takeover or ownership loss. |
| `X-MagicHandy-Command-Ticket` | Server-issued delivery ticket renewed by explicit foreground heartbeats; valid for at most ten seconds. |
| `X-MagicHandy-Command-ID` | Unique 8–120 character request identifier, retained with the originating session and tab. |
| `X-MagicHandy-Command-Sequence` | Positive monotonically increasing integer within the ownership scope, bounded to JavaScript's exact integer range. Gaps are allowed. |

The server reports the accepted sequence high-water mark with the active
controller snapshot. A reload resumes above it; a new ownership generation
starts a new sequence. A later response with an older generation cannot replace
the browser's current delivery metadata. The frontend still renders backend
authority; a locally generated ID or sequence cannot grant control.

Immediate control and setting changes enter a bounded, cancelable serial lane.
Admission checks ownership, ticket expiry and the sequence again after any wait.
An older sequence is rejected even when its original receipt has been evicted.
Admission retains at most 256 receipts, each with at most 16 KiB of JSON response,
a URL bounded to 2048 bytes and fixed-size identity metadata. Completed receipts
expire after five minutes; the watchdog also prunes them while the app is idle.
Requests exceeding the live receipt capacity fail before execution.

The ticket bounds delivery and queue age. It does not impose a ten-second
generation limit on a large LLM or TTS model. Inference, native dialogs and
worker jobs retain their own cancellation/resource lifetimes. Interactive chat
and live Lab replies release the control lane during inference, then reacquire
it before applying motion. A newer motion-affecting command supersedes an old
proposal; the text reply may still be retained. Voice playback acknowledgments
and unrelated file operations do not supersede proposed motion.

Ownership changes synchronously cancel registered controller requests. A
settings write waiting on a lock uses the request context for its transaction,
so canceled work cannot later commit. Once a transaction has committed, snapshot
publication and runtime reconciliation finish to keep durable and in-memory
settings consistent; engine cancellation and Stop generation checks remain in
force. Long-running modes and media retain their existing shared-engine and
runtime-generation protections.

## Duplicate delivery and uncertain outcomes

The backend hashes method, URL and body as the existing bounded decoder reads
the request. It does not retain a second copy of request payloads or uploads.
A duplicate in the same ownership generation can return the original completed
JSON result only when the method, URL and body match. It never invokes the
handler a second time. Reusing an ID for a different request or generation is
rejected. Concurrent duplicates with a pending original are rejected and can
query its receipt.

`GET /api/controller/commands/{id}` is readable only by the originating login
session and tab. That session may inspect an old receipt after giving up control.
Other logins of the same account, copied tab IDs on another session, observers
and administrators using a different session cannot retrieve it. Revoked
sessions cannot read receipts.

| Receipt state | Meaning |
| --- | --- |
| `pending` | Handler admitted and still active; no completed result is known. |
| `complete`, `replayable=true` | Normal handler return with a bounded JSON result; the original HTTP status and response can be reconciled. |
| `complete`, `replayable=false` | Handling ended, but the response was streamed, oversized, non-JSON, or the request body was incomplete. Read the relevant canonical state. |
| `unknown` | Handling was interrupted or produced no acknowledged result. Never advertise success from a partially written response followed by a panic. |
| No receipt | It was never admitted, expired, was evicted, belongs to another session/tab, or was lost in a server restart. Current state must be reconciled. |

A receipt describes HTTP handling. A successful job submission is still only
job admission, and a successful motion response is not independent proof of a
physical device's position or Stop. Receipts are process-local reconciliation
data, not a durable audit log. Requests, credentials, audio and raw intimate
content must not be added to future command audit exports.

After a JSON mutation loses its response, the browser makes one bounded
three-second receipt lookup, refreshes canonical app state when it recovers an
outcome, and returns the recorded result. It never automatically reissues the
mutation with a new ID. Missing, pending or non-replayable outcomes produce an
explicit unconfirmed-result message. Streaming chat continues to use canonical
chat history/cursors for content reconciliation; its receipt does not replay an
SSE conversation.

Emergency Stop at `/api/motion/stop` remains independent of authentication,
tickets, receipts and this lane. The transport-specific emergency-stop aliases
also bypass delivery tracking. An expired or offline browser retains the Stop
control, and the browser does not delay a Stop error with a receipt lookup.
The ordinary `/api/modes/stop` operation can preserve motion via
`stop_motion:false`, so it remains an ordered, controller-authorized command.

Server shutdown cancels handler work immediately but gives socket writes a
bounded five-second grace period to finish HTTP framing. An immediate write
deadline at this point could truncate a healthy event stream's chunk terminator.
Ordinary session/ownership revocation on a running server still interrupts socket
writes immediately; it does not receive the shutdown grace period.

## Evidence and limits

The regression suite exercises delayed starts/resumes/mode/media requests after
Stop, reordered and duplicate settings, response loss after application,
session/tab isolation of receipts, ticket expiry under a live controller lease,
Stop while a settings transaction waits, synchronous ownership cancellation,
receipt eviction/expiry, interrupted handlers and a delayed model response after
newer controls. The browser suite covers sequence recovery, stale response
metadata, safe receipt lookup, unknown outcomes and canonical-state refresh.
A deterministic regression forces the shutdown cancellation callback to run
before an HTTP/1 stream handler returns and verifies clean end-of-stream framing;
a separate test verifies immediate interruption on live access revocation.

These are deterministic process/simulator tests. They do not close the remaining
exhaustive route/permission inventory, real proxy deployment, sustained load,
media/voice network-fault behavior, mobile sleep or physical-device Stop tests.
Those remain tracked in the main checklist and implementation log.
