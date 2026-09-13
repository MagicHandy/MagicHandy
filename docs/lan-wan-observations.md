# LAN/WAN observation ordering and recovery

Implementation checkpoint: 2026-09-13 on `codex/lan-wan-control`, following the
[command-delivery checkpoint](lan-wan-command-delivery.md). This implements
part of LAN-02 and LAN-12 in the [full checklist](lan-wan-control-checklist.md).
Real WAN, mobile, load and physical-device acceptance remain open.

## Backend ordering

The process epoch identifies one running server. App and motion observations
carry that epoch, a positive monotonic revision and a UTC `observed_at` value.
Controller observations carry the same epoch and their own monotonic revision,
alongside the existing ownership generation. Revisions order observations;
ownership generations and authenticated delivery checks authorize commands.
Neither a revision nor an epoch is a credential.

| Observation | Meaning of its revision |
| --- | --- |
| `/api/state` envelope | Collection begins with the in-memory public settings/status snapshot. Its stamp orders those captures before collecting other modules. |
| Embedded motion, `/api/motion/state`, motion SSE | Each motion value gets its own stamp after capture. All three use the same motion capture lock and app/motion sequence. |
| Controller state and heartbeat | A separate counter increments under the controller lock. It orders reads within an ownership generation as well as across ownership changes. |

The full-state envelope is **not an atomic snapshot of every module**. For
example, a slow transport diagnostic can finish after an independent motion
read. The embedded motion then has a later revision than both that independent
read and the enclosing settings snapshot. Consumers compare the motion stamps
when reconciling live motion, rather than using the envelope or HTTP completion
time to order it. Controller revisions are a separate namespace and must not
be compared numerically with app/motion revisions.

Capture locks cover in-memory reads only. No network call, database query or
response write retains them. UTC timestamps are diagnostic information; clock
skew and response arrival times do not decide which observation wins. JSON
responses and replayed JSON command results set `Cache-Control: no-store`.

## Independent browser channels

Each enabled foreground tab has one full-state request, one lightweight
controller request, and one motion event source. Each channel owns its own
timer and cancellation lifecycle:

| Channel | Timing and failure behavior |
| --- | --- |
| Full state | Poll again two seconds after completion; abort after eight seconds. Failed or timed-out reads make the UI stale/read-only. |
| Controller | Discover with `GET /api/controller`; protected sessions renew explicitly with a heartbeat. Repeat two seconds after success, with a three-second request deadline. Failure retries use four then eight seconds, with ±20% jitter. |
| Motion events | On error, close the old EventSource before scheduling one retry. Retry delays grow from one second to a maximum of ten seconds, with jitter. An accepted motion packet resets the backoff. |

Controller discovery and its first heartbeat share the same three-second
deadline. A slow full-state request cannot delay heartbeat scheduling. The
heartbeat deadline does not apply to LLM generation or speech. A successful
heartbeat alone cannot enable motion controls: a fresh full snapshot and
backend-authorized ownership are also required.

The API client retains newer controller metadata by backend revision, even
when an earlier-issued HTTP request observed the server later. A canceled
response cannot replace cached generation, ticket or sequence information.
Protected controller packets need valid booleans, epoch, generation and
revision; malformed packets cannot enable controls. Legacy unstamped
observations are accepted only for unprotected compatibility fixtures.

## Disconnect, restart and visibility

- Same-epoch full snapshots and motion packets cannot overwrite a newer
  observation with a lower or equal revision. Motion is reconciled separately
  from the slower full-state context.
- A stream error marks freshness lost but retains the newest known motion
  value. It does not fall back to an older full snapshot. A full-state request
  that began before the error cannot clear that stale marker; a fresh request
  is required.
- An epoch change triggers a full resync and invalidates the old stream and
  pending poll. Callback identity checks reject queued events from a retired
  source, including events delivered after a replacement source connects.
- Hiding the document aborts polling/controller requests, closes the stream
  and stops heartbeat renewal. Visibility return or `pageshow` invalidates the
  prior view and immediately rediscovers state and controller status.
- Returning to a page does not invoke takeover, Start or Resume. If the backend
  lease expired, the page remains an observer until the existing explicit
  stop-first takeover flow succeeds. Emergency Stop stays mounted.

This retains backend authority: the UI stores and selects backend observations,
not a parallel motion or ownership model. Browser visibility emulation is
useful regression coverage, but does not establish how a physical phone,
browser or operating system schedules timers during sleep.

## Evidence and remaining work

Backend tests block a full-state diagnostic while an independent motion read
completes, then verify the capture order and no-store headers. A second test
orders controller reads and renewal within one ownership generation.

Browser tests cover a twenty-second blocked state request with continuing
heartbeats, delayed motion packets, a pre-failure poll, server restart, old
callbacks after reconnect, one bounded heartbeat/reconnect loop, malformed
controller metadata, and hiding/resuming a phone-style tab after lease expiry.
API tests cover controller response reordering and an aborted delayed response.
Existing state lifecycle and subscription tests remain in place.

The implementation log records final build, test and review evidence. This
checkpoint does not implement durable chat cursor recovery, shared encoded
telemetry, adaptive spectator rates, RTT/jitter diagnostics or the complete
network fault/load matrix. The new lightweight controller reads and observation
fields have a wire cost; no telemetry speedup is claimed without LAN-13's
bytes/client/minute, CPU, allocations, RSS and state-age measurements.
