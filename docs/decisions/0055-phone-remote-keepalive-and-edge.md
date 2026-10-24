# ADR 0055: The phone remote keeps its desktop's session alive and offers Edge

## Status

Accepted 2026-10-24. Amends the hidden-heartbeat rule in
[ADR 0029](0029-session-bound-network-control.md) and the phone remote in
[ADR 0032](0032-video-curation-and-remote.md). See the
[review](../remote-keepalive-edge-review-2026-10-24.md). Decisions 1 and 4 are
amended by [ADR 0056](0056-hidden-desktop-acts-for-its-phone.md): the
renewal now issues the hidden desktop's command tickets, and that desktop stays
awake to carry out video and chat.

## Context

The phone remote asks the desktop tab that holds control to act. That tab acts
only while it is in front:

- A hidden desktop withdraws its remote presence, so the phone showed "No
  desktop" the moment the desktop was tabbed out.
- A hidden protected desktop also pauses its controller heartbeat (ADR 0029).
  Fifteen seconds later the access watchdog fences the lease as abandoned and
  runs the global Stop, ending the session even while someone held the remote.
- Edge, the gentle hold, existed only in the desktop rail.

The lease is a dead-man's switch: only a responsive foreground document of the
controlling account may keep a protected session going. A phone remote that is
open and in front is such a document.

## Decision

1. **Phone keepalive.** While its page is in front, the remote posts
   `/api/remote/keepalive` every three seconds. The server renews the live
   lease of the controller tab when the phone has remote-control permission,
   an active login and grant, and the same desktop account as that tab. It
   never claims control, crosses a Stop or handoff, or revives an expired
   lease. (As first decided it issued no command tickets either; ADR 0056
   has it issue the hidden desktop's tickets.) The request is passive for login idle time.
   A hidden or locked phone sends nothing, so when neither the desktop tab nor
   the phone is in front, the lease expires and, on a protected install, the
   usual Stop runs within about 15 seconds. Unprotected local control retains
   its existing expiry policy; the backend's `session.stop_on_expiry` flag
   limits the phone's automatic-Stop notice to protected sessions.
2. **Session status outlives presence.** The remote state carries a `session`
   block for the phone's own account: whether a desktop of that account still
   holds live control, whether motion runs, and the Edge hold and its return
   countdown. It is withheld from other accounts. The remote stream re-reads it
   every two seconds.
3. **Edge on the phone.** `POST /api/remote/edge` and
   `/api/remote/edge/resume` apply the same Edge and Return as the desktop
   (shared server code) for the phone's desktop session. They need
   remote-control permission, the same desktop account as a live controller and
   the current Emergency Stop sequence. They act directly rather than through
   the desktop tab: like Stop, a gentle hold must not wait for a desktop that
   may be hidden or asleep. Each press is audited as a finished motion command
   under the phone's login. The phone's footer shows Edge beside Stop.
   Each request binds to the desktop owner's cancellation path before its
   authority is rechecked, so Stop or handoff also fences an in-flight press.
4. **Video and chat still ask the desktop.** As first decided, the hidden
   desktop stayed dormant: its state poll, motion stream and heartbeat
   stopped, it withdrew presence, and phone video and chat waited for the
   desktop tab to return to the front. Superseded by ADR 0056: a hidden
   desktop its phone holds stays awake and carries them out; one that is not
   taking commands (closed, frozen by the browser or offline) still leaves the
   phone with Edge and Stop, and the phone says so.

## Consequences

- Tabbing out, minimizing or locking the desktop no longer ends a session
  while its phone remote is open and in front.
- On a protected install, the phone becomes the dead-man's switch while the desktop is hidden: locking
  it, closing the remote or losing its network stops the session within about
  15 seconds. The remote states this.
- A desktop tab that closes while its phone keeps the lease alive leaves that
  lease held; a new desktop tab takes over through the existing stop-first
  takeover, or the phone stops the session.
- The remote listener gains three allowlisted routes. It still cannot claim
  control, configure the host or reach the main application's APIs.

## Validation

Go tests cover renewal only of a live, same-account lease (including the
desktop tickets added by ADR 0056),
refusal across accounts, handoffs and expiry, the lost-controller watchdog
leaving a phone-sustained session running past the lease and stopping it once
the phone goes quiet, the session block after a withdrawn presence and its
privacy, phone Edge and Return on a hidden desktop's stream, Stop-sequence and
account fencing, in-flight Edge/Return cancellation on Stop and handoff, the
protected-versus-local expiry notice, the remote listener route and the audit row. Frontend tests
cover the shared Edge control, the phone's Edge admission, the keepalive's
foreground-only cadence and the hidden-desktop status.
