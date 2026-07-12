# Controller And Dispatch Semantics

Phase 9B makes the browser UI a single-controller surface instead of a set of
independent tabs racing hardware commands.

## Controller Lease

- Browser clients identify themselves with `X-MagicHandy-Client-ID`. EventSource
  clients use `client_id` in the query string because browser `EventSource`
  cannot send custom headers.
- `GET /api/state`, `GET /api/controller`, and `GET /api/motion/events` claim or
  refresh the lease for the first client that appears.
- The active lease expires after 15 seconds without a refresh.
- A second client receives `controller.read_only=true` in state responses. It may
  watch state and use Stop, but mutating device paths return HTTP 409.
- Stop remains available to any client because safety takes priority over
  controller ownership.

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

## Motion State Stream

`GET /api/motion/events` emits `event: motion` Server-Sent Events containing the
same envelope as `GET /api/motion/state`. The browser UI consumes this stream for
the visualizer and keeps HTTP polling as a fallback and diagnostics source.
