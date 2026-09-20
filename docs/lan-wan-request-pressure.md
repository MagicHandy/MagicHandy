# LAN/WAN request pressure and Stop delivery

This checkpoint advances LAN-06, LAN-07, LAN-14 and LAN-19 in the
[full checklist](lan-wan-control-checklist.md). It bounds application admission
before account lookup, preserves capacity for the current controller/device
gateway, and handles stalled request/response bodies. It does not establish a
public Internet flood limit or physical Stop latency.

## Admission and authentication

After configured network and browser-origin validation, requests enter without
a waiting queue. Independent concurrent budgets are:

| Work | Global | Per validated peer IP |
| --- | ---: | ---: |
| Ordinary API requests, including streams and downloads | 128 | 64 |
| Login and bootstrap | 8 | 2 |
| Public shell, assets and health | 16 | 8 |
| Existing controller heartbeat and device gateway delivery | 16 | 8 |
| Existing controller reconciliation and immediate motion controls | 16 | 8 |

Exhaustion returns 429 with `Retry-After: 1`. Unknown mutation results still
require reconciliation instead of automatically resending the action. Peer
entries disappear after their last admitted request. The connection report
includes occupancy, limits, peaks and rejection counters without peer identities
or session references.

Controller priority requires the current tab ID and a constant-time match
between the cookie digest and cached owner session. Gateway delivery requires
the equivalent existing gateway session match. This only selects a work budget:
live validity, permissions, gateway identity and control generation are checked
afterward. A copied tab ID or revoked cookie cannot authorize work. Deferred
chat/model requests stay ordinary. Authenticated session tracking separately
bounds ordinary requests to 32 and each reserved lane to 8; ordinary sessions
cannot consume all reserved session entries.

Session lookup gets a one-second child context. Temporary storage failure
returns 503 without clearing the cookie or manufacturing a logout. Invalid
sessions retain the existing 401/clear-cookie behavior. Signed-out account-status
queries are also bounded; the public shell skips account storage entirely.
The configured header limit drops from 1 MiB to 32 KiB. Go's protocol parsing
allowance means this is not an exact connection-memory cap. Existing five-second
header, 30-second read and two-minute idle timeouts remain.

## Stop under overlapping requests

Public Stop bypasses these budgets and session storage. Every intent first
invalidates old commands, chat, modes, media and voice work, retaining its own
Stop sequence/audit outcome. Only overlapping HTTP requests share one in-flight
shared-engine/transport Stop. Completion occurs under the existing engine
lifecycle lock. Completed results are never cached; a later intentional retry
makes a new transport attempt.

Invalidation must precede joining an operation. A regression holds a second
request before invalidation, completes the earlier Stop, starts a new simulated
run, then releases the delayed request. It must stop the new run instead of
returning confirmation from the earlier one.

At most 16 callers wait for the operation, for at most five seconds each.
Excess, canceled and timed-out waiters return 503 with `stop_pending: true`,
`stopped: false` and `transport_stop_confirmed: false`. The leader retains the
existing detached 15-second dispatch context. Internal watchdog/takeover Stops
keep their independent path. No background motion worker or sampler was added.
Public Stop/rejection replies have bounded socket writes. A transport result
with `OK: false` cannot yield a confirmed protected Stop response when its error
is nil.

An unfinished HTTP/1 body previously delayed the Stop reply while Go drained
it. Stop and early rejection now close that connection and expire its unused
body-read deadline, permitting a prompt reply. HTTP/2 retains independent stream
framing. The public Stop availability tradeoff remains in ADR 0029.

## Content writes

Videos, thumbnails, portraits, profile images, static assets, retained audio,
persona/library attachments and installer reports use content writes of at most
64 KiB with cancelable five-second deadlines. Each write flushes before clearing
the deadline, allowing a healthy transfer or producer gap to last longer than
five seconds. Stalled receivers release their file and request/session slots.
Short writes preserve the actual byte count and fail instead of truncating
silently. Existing `http.ServeContent` Range, HEAD and conditional-response
behavior, authorization and redaction remain in place.

No total download timeout, second media buffer or dependency was added. Resource
preparation, ordinary JSON handlers, upload/decompression budgets and connection
admission still need the broader endpoint audit.

## Evidence and limits

Tests cover global/per-peer/session exhaustion, reserved controller/gateway
delivery, revoked-session rejection, unfinished uploads, oversized headers,
held SQLite connections and cookie preservation. With a blocked fake transport
behind the real shared engine, 65 overlapping Stops produce one transport
attempt, 16 waiters and 48 prompt pending replies. Every intent invalidates work;
a later retry makes attempt two.

Real HTTP/1 and HTTP/2 clients stop reading a 64 MiB virtual file; the tests
verify handler and admission release within eight seconds. A healthy HTTP/2
response survives a 5.5-second producer gap. Range/HEAD, cancellation and short
writes have regression coverage.

A loopback TLS/HTTP/2 profile opens 20 spectator streams, concurrently reads 20
state snapshots, performs 20 Stops and revokes one spectator. One run transferred
142,864 bytes of state JSON, measured Stop p95 1.006 ms / maximum 1.016 ms, and
closed the revoked stream after 946.907 ms. Ordinary occupancy peaked at 40.
Windows timer resolution yielded zero for some short samples. This small fixture
measures the application and fake transport, not a supported WAN or physical
latency distribution.

Three 1 MiB in-memory sink benchmark runs on Go 1.26.4/Windows amd64/Ryzen
9950X3D measured `ServeContent` at 13.509–14.680 microseconds, 33,296 B and 9
allocations per response; bounded content measured 23.783–24.185 microseconds,
42,584 B and 175 allocations. The extra cancellation/deadline bookkeeping costs
about 10 microseconds and 9,288 allocated bytes per MiB in this fixture. This
CPU/allocation tradeoff buys finite stalled-write lifetimes; the sink benchmark
does not measure network throughput. Build sizes are in the [scorecard](goal-scorecard.md).

TLS/connection exhaustion, CPU/disk saturation, LLM/media contention, packet
loss/reordering, the full RTT/outage matrix, sustained load/overnight soak and
real WAN/mobile/device acceptance remain open. These bounds cannot deliver Stop
across a severed network and do not replace a controlled WAN perimeter or local
hardware-side stopping.
