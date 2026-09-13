# ADR 0029: Session-bound LAN and self-hosted WAN control

## Status

Proposed; implementation in progress for the user-requested full LAN/WAN goal.
On 2026-09-12 the user selected self-hosted HTTPS, with direct access or an
explicitly trusted reverse proxy, as the primary WAN connection model.
This decision is not a declaration that the implementation or acceptance is
complete. ADR 0017's legacy restrictions remain the default; the implemented
new modes require a saved policy or explicit network-mode option and still
need review and release acceptance. No listener or firewall is changed by
writing this decision. See the [deployment contract](../self-hosted-https.md).

## Scope

The complete work is tracked by the
[LAN/WAN checklist](../lan-wan-control-checklist.md). It includes authority,
disconnect behavior, permissions, streams, deployment setup, certificate
lifecycle, reconnect, diagnostics, resource budgets, media/voice, device
location, sessions/auditing, authentication recovery, network fault tests and
real-client acceptance. The implementation must not redefine WAN as loopback or
LAN compatibility merely because that is easier to test.

## Decisions

### Identity and command ownership

An account grants access; an opaque login session identifies one login; a
browser tab ID identifies a document. Controller ownership binds the session
and tab together. A random process epoch rejects old requests across restart,
even if the numeric generation repeats. A generation changes when ownership changes
or is lost. Protected controller requests must present that generation, and
in-flight work is canceled when the ownership lifetime ends. Public tab IDs,
selected linked profiles and generations are not bearer credentials.

Read-only requests and server-sent telemetry do not establish or renew protected
control. A responsive foreground document sends explicit heartbeats. A fresh,
stopped server can accept the first heartbeat as its initial claim; after loss,
control requires the existing explicit stop-first takeover flow. Unprotected
trusted-loopback automation retains its compatibility contract.

Protected leases initially retain the existing 15-second TTL. A one-second
watchdog fences expired ownership and invokes the shared global Stop path,
including modes, media synchronization, speech and admitted motion work. No
new controller is admitted while that stop is pending. This is a host-side
detection bound, not a promise of physical stop within 16 seconds: transport
latency, unavailable links and already buffered motion require separate
measurement and honest UI reporting. Protected foreground sessions pause their
heartbeat while hidden; local unprotected unattended use remains distinct.

### Command ordering and reconciliation

Protected controller mutations additionally require a short-lived delivery
ticket, a unique request ID and a monotonically increasing sequence. Bounded
receipts prevent duplicate execution and permit the originating session/tab to
reconcile a lost JSON response without resending a mutation. Immediate control
changes serialize; deferred chat/Lab motion checks for a newer control intention
when applying through the same lane. Large-model inference does not hold that
lane or inherit its short delivery timeout. Ownership cancellation is synchronous
and queued settings writes bind their transaction to the request context.
Emergency Stop bypasses delivery admission. See the detailed
[command contract and evidence](../lan-wan-command-delivery.md).

### Observation ordering and reconnect

App/motion observations use monotonic capture revisions scoped to the process
epoch; controller reads have their own revision counter under the ownership
lock. A full-state envelope orders its initial settings capture, while its
embedded motion carries an independent capture stamp. It does not claim an
atomic cross-module snapshot. No capture lock spans network/database work.

The browser runs the lightweight controller channel independently of full-state
polling and motion SSE. Hidden documents stop all three channels. Return,
restart and lost freshness require a full resync; old responses/events cannot
restore obsolete control or motion, and recovery does not trigger takeover or
resume. JSON responses are not cacheable. The
[observation contract](../lan-wan-observations.md) defines ordering, deadlines,
backoff, visibility behavior and the remaining acceptance work.

### Authentication lifetime and streaming

Automatic browser POST acknowledgements also remain passive: chat cursors,
gateway status/ACK/disconnect, voice played, duration reporting and media
heartbeats do not renew login idle time. Explicit media intentions record
activity only after bounded decoding and validation. Neither path revives an
expired login.

Passive polling and heartbeats validate credentials without extending login
idle time. A bounded registry tracks active authenticated requests; a periodic
read-only session check cancels revoked/expired sessions and affected ownership.
It does not create a second identity database. Storage failure fails closed.
Enabling accounts also cancels pre-existing unprotected private streams.
Bootstrap transfers the creating tab through Stop into its temporary protected
setup session when that tab is identified.

Stream writes have individual deadlines that are cleared after flushing. A
healthy connection may wait longer for LLM/TTS generation; a slow receiver may
not retain a writer indefinitely. Logging wrappers preserve the standard Go
response-controller deadline and error interfaces. Request cancellation also
interrupts socket writes and body reads. Healthy concurrent requests share a
bounded per-session budget; Emergency Stop bypasses authentication storage and
ordinary admission limits.

During application shutdown, cancellation permits a bounded five-second socket
write grace so healthy responses can finish their HTTP framing. Session and
ownership revocation on a running server still interrupt writes immediately.

### Browser device gateway

Protected Bluetooth dispatch binds the authenticated session, declared tab,
Bluetooth client ID, process epoch and a per-connection gateway generation.
This authority is independent of the remote controller. Gateway maintenance
survives controller handoff, while session loss and a ten-second gateway lease
retire it. An internal transport identity changes on each connection so an old
poll cannot drain replacement work. No account/HTTP dependency enters transport.

Connecting requires a controlling host administrator; replacement cannot bypass
gateway teardown. Explicit disconnect retains shared global Stop. Gateway loss
invalidates shared motion when Bluetooth is selected, and the browser attempts
local Stop before bounded GATT teardown. Recovery is explicit. The
[gateway contract](../lan-wan-bluetooth-gateway.md) documents admission, login
activity, timing policies and the remaining physical/network acceptance.

### Target network modes

The implementation exposes explicit local, direct-HTTPS and trusted-proxy
modes. Local remains the default. Both remote modes require initialized
accounts, a configured canonical HTTPS origin and the remote permission model.
Direct HTTPS validates its operator-provided certificate chain against the
advertised hostname/IP and binds only the selected interface configuration.
Public or wildcard listening requires a deliberate remote configuration; it
must never result from a missing value or failed migration.

Proxy mode separates the actual peer from the forwarded client identity.
Forwarded scheme/host/address information is accepted only from configured
proxy peers and checked against the canonical external origin. An arbitrary
client cannot opt into trust with a header. Proxy traffic must not acquire
loopback-only bootstrap, native path-picker or recovery privileges. The
documented proxy configuration must preserve streaming and avoid caching
authenticated responses. Private inference/worker ports are not published.

Certificate import/renewal, DNS/address changes, firewall scope and startup
recovery are part of the setup contract. Configurations must fail closed and
retain a local repair path. Router forwarding, public DNS changes, certificate
authority trust installation and exposure of this review host are deployment
actions, not side effects of enabling an app setting in a test.

### Stop and network partitions

Emergency Stop remains mounted for read-only, signed-out and disconnected
clients, and the existing endpoint remains independent of login validity.
Browser origin checks remain mandatory. Public Stop permits fail-safe
interruption by a direct unauthenticated peer; self-hosted WAN documentation
must disclose that availability tradeoff and support a restricted network
perimeter. Rate/resource controls must not put ordinary work ahead of Stop.

No software can deliver a remote request over a severed link. The UI must
distinguish local cancellation, an acknowledged backend stop and a physically
confirmed result. The host watchdog and local stop path cover loss of remote
contact; they do not create a second motion implementation.

## Acceptance and open work

The [chat recovery contract](../lan-wan-chat-recovery.md) extends observation
ordering to the durable conversation log. Display sequence and committed
revision are distinct; a pending reply committed later must remain recoverable
even below an observed display head. Login-scoped read markers are separate
from control grants and command delivery. One database read snapshot supplies
rows and recovery metadata, while browser cancellation, full resync and bounded
retries prevent stale responses from replacing a newer conversation view.

History resets also obey the encoded response budget. Stateless continuations
anchor the committed head, pruning marker and first retained row; mutations
that invalidate that window restart a bounded reset. Long content has explicit
previews and authenticated attachment downloads, with no database transaction
held across network writes. Publication keeps message/speech visibility together
while allowing canceled history requests and autonomous runs to leave its queue.

Current evidence and outstanding requirements are recorded in the
[implementation log](../lan-wan-implementation.md). The new exposure modes,
permission/grant model, command deduplication and ordering, certificate/setup UI,
network diagnostics, audit/recovery features and full acceptance matrix remain
required work. All checklist items need implementation and evidence before the
full goal can be marked complete. Existing hard gates are preserved.
