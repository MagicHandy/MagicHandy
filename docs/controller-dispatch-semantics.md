# Controller And Dispatch Semantics

Phase 9B makes the browser UI a single-controller surface instead of a set of
independent tabs racing hardware commands.

## Controller Lease

- Browser clients identify themselves with a tab-scoped
  `X-MagicHandy-Client-ID` on mutating requests. The ID persists through a
  reload in `sessionStorage`, while a new or duplicated top-level navigation
  mints a fresh ID even if the browser copied session state. Controller
  identity must never use shared `localStorage`, which would silently grant
  every same-profile tab the same lease. EventSource clients use `client_id` in
  the query string because browser `EventSource` cannot send custom headers.
- `GET /api/state`, `GET /api/controller`, and `GET /api/motion/events` claim or
  refresh the lease for the first client that appears. These read paths may use
  the query-string client ID; mutating paths never do.
- The active lease expires after 15 seconds without a refresh.
- A second client receives `controller.read_only=true` in state responses. It may
  watch state and use Stop, but mutating device paths return HTTP 409.
- A read-only client can explicitly request control with
  `POST /api/controller/takeover`. The request requires
  `X-MagicHandy-Client-ID`; a query-string ID cannot authorize a handoff.
- Takeover is stop-first. The server temporarily marks every client read-only,
  runs the same global Emergency Stop path used by the shell control, and only
  then assigns the lease to the requesting client. This cancels motion,
  Autopilot, video synchronization, chat work, speech playback, and queued voice
  work before ownership changes.
- A takeover by the current owner is idempotent. A second takeover request while
  a handoff is pending returns HTTP 409 instead of racing it.
- A failed physical Stop does not leave ownership ambiguous: local work is still
  invalidated and the requesting client receives the lease with an explicit
  warning that physical Stop was not confirmed. The response includes the Stop
  sequence so browser clients discard work from before the handoff.
- Stop remains available to any client because safety takes priority over
  controller ownership.
- Every Stop activation attempts the configured transport, including idle and
  no-engine states. If the owner is unreachable, local state remains stopped
  and the response carries an explicit delivery error for the Stop toast.

## Mutating Paths

These paths require the active controller:

- `PUT /api/settings`
- `POST /api/chat/stream`, except deterministic stop messages
- `POST /api/motion/start`
- `POST /api/motion/target`
- `POST /api/motion/quick`
- Cloud and Browser Bluetooth stroke-window, HSP add, and HSP play endpoints
- Browser Bluetooth connect
- Intiface connect/disconnect, scan, and linear-actuator selection

Read-only diagnostic and state paths remain available. Browser Bluetooth
connection check is a bridge-readiness diagnostic and does not queue a device
command. Low-level Stop endpoints remain available.

## Browser Boundary

The embedded UI is a loopback application, not a cross-origin web API.
Requests carrying browser `Origin` or `Sec-Fetch-*` metadata are accepted only
when both the request Host and Origin are the same loopback HTTP(S) origin.
This blocks cross-site requests and DNS-rebinding-style browser access while
leaving non-browser localhost API clients compatible. MagicHandy does not emit
CORS permission headers.

A custom controller header is therefore required for every mutating path. A
query parameter cannot authorize motion or settings changes. Emergency Stop
remains deliberately outside this ownership check.

## Dispatch Owner Switching

Changing `settings.device.hsp_dispatch_owner` through `PUT /api/settings` is a
runtime boundary:

1. The current active motion engine is stopped with reason
   `dispatch_owner_changed`.
2. The engine pointer and recorded owner are cleared.
3. No fallback transport is attempted.
4. A process-owned Intiface session is stopped and closed before the switch
   completes. Changing its saved server address applies the same teardown.
5. The next motion start constructs a new engine for the selected dispatch
   owner.

If motion settings change without changing dispatch owner, active motion is
refreshed in place so quick controls continue to apply without a stop/start.

Engine creation and dispatch-owner replacement are serialized. Each new start
also carries the engine's Stop generation captured while the server still owns
that engine. A concurrent Stop, owner change, or shutdown invalidates the
admission instead of allowing delayed work to revive a cleared engine.

## Shutdown Ordering

Shutdown first quiesces controller work and cancels chat, then stops and clears
the motion engine before draining autonomous planners. This order lets engine
Stop cancel a mode start that is blocked inside transport setup. The Intiface
session closes next. Only then does the HTTP server drain active handlers and
release voice, LLM, model, chat, pattern, personalization, and SQLite resources.
Stop remains callable while the server is quiescing. `Server.Close` is
idempotent so startup failures and normal deferred cleanup share the same
ordering.

## Motion State Stream

`GET /api/motion/events` emits `event: motion` Server-Sent Events containing the
same envelope as `GET /api/motion/state`. The browser UI consumes this stream for
the visualizer and keeps HTTP polling as a fallback and diagnostics source.
