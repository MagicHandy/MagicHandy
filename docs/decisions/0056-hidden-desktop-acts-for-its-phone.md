# ADR 0056: A hidden desktop stays awake for its phone remote

## Status

Accepted 2026-10-24. Amends the hidden-document rules in
[ADR 0029](0029-session-bound-network-control.md), the presence rule in
[ADR 0032](0032-video-curation-and-remote.md), the visible-desktop scope of
[ADR 0033](0033-remote-listener-and-account-scope.md), and decisions 1 and 4
of [ADR 0055](0055-phone-remote-keepalive-and-edge.md). See the
[review](../hidden-desktop-remote-review-2026-10-24.md).

## Context

ADR 0055 let a phone remote in front keep a hidden desktop's session alive and
offer Edge, but video and chat commands still waited for the desktop tab. A
hidden tab slept:

- it stopped its state poll, motion stream and heartbeat, and withdrew its
  remote presence, so the phone could not send it commands;
- going stale made it read-only, which paused a paired video and stopped its
  synchronized motion;
- it cancelled an in-flight chat reply and stopped reading chat history and
  queueing spoken replies;
- with its heartbeat paused it received no command tickets, so it could not
  have acted even if it were listening.

The phone carries out video and chat through the desktop tab because the video
plays in that tab's player and the conversation runs through its composer.

## Decision

1. **The phone's renewal issues the tickets.** A keepalive that renews its
   desktop's live lease also issues that tab's next command ticket. The
   owner's controller snapshot reports `remote_held` while a phone renewed the
   lease within the last lease period (15 seconds). Renewal still never claims
   control, crosses a Stop or handoff, or revives an expired lease.
2. **A hidden owner keeps watching, never renewing.** A tab that hides while
   it holds control keeps observing its controller snapshot in the background
   instead of sending heartbeats. Background observations carry
   `X-MagicHandy-Background` and never renew a lease, not even an unprotected
   local one. The tab stops watching once its snapshot says it no longer holds
   control, or when no observation has succeeded for one lease period.
3. **Held means awake.** While its snapshot says the phone holds the session,
   the hidden tab stays awake: its state poll, motion stream and remote
   presence continue, so it stays current and not read-only, and it carries
   out phone commands with the same controls and admission as before. Its chat
   keeps streaming, reads history and speaks replies. Its presence says it is
   in the background, and the phone shows that this phone keeps the session
   going and, for a protected session, that locking it ends the session within
   15 seconds. The backend reports whether expiry enforces Stop; unprotected
   local sessions keep their existing policy and do not show that promise.
4. **The hold ends with the phone.** Without a phone renewal, no new ticket is
   issued, so the hidden tab's control requests fail within one ticket
   lifetime (10 seconds). The hold and the lease lapse 15 seconds after the
   last renewal; the tab then sleeps as before, and on a protected install the
   lost-controller Stop runs. If the phone renews again while the tab still
   holds control, the tab wakes. A tab kept awake returns to the front without
   rediscovering; any other tab still rediscovers before acting.

## Consequences

- With the phone open and in front, tabbing out, minimizing or locking the
  desktop keeps the whole remote working: video, chat, Edge and Stop.
- The phone is the dead-man's switch while the desktop is hidden, exactly as
  in ADR 0055. Nothing in the hidden tab keeps control alive on its own.
- A hidden tab held by its phone costs the same requests as a tab in front.
  A tab that hides without a phone watches its lease for at most 15 seconds.
- Browser behavior still bounds what a hidden tab can do. Browser Bluetooth
  still disconnects on hide, because that tab runs the device link
  ([gateway contract](../lan-wan-bluetooth-gateway.md)). A browser
  that freezes or discards a background tab stops it from executing; presence
  then lapses and the phone falls back to Edge and Stop. Browsers slow the
  timers of a silent page after a few minutes in the background, so a muted
  paired video's synchronization can lapse and stop its motion, safely.

## Validation

Go tests cover a hidden desktop acting with a ticket it learned in the
background after a phone renewal, that ticket lapsing once the phone is quiet,
the lease then ending with Stop, the hold ending with the lease and never
passing to a new controller, background observation never renewing a local
lease, and the background flag reaching only the desktop's account. Frontend
tests cover a held hidden tab staying awake without a heartbeat, sleeping when
the hold ends and waking when it returns, returning to the front without a
read-only gap, the executor carrying out commands while held and withdrawing
without a hold, a reply that keeps streaming, chat history that loads and
speaks while held, and the phone's background readout.
