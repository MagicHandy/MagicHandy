# Phone remote keepalive and Edge — October 24, 2026

Decision: [ADR 0055](decisions/0055-phone-remote-keepalive-and-edge.md).
Base: `4d41718b`.

## Findings

The remote was usable only while the controlling desktop tab was in front:

- `RemoteExecutorProvider` made a tab the remote's desktop only while
  `document.visibilityState !== "hidden"`. Tabbing out withdrew its presence,
  and the phone switched at once to "No desktop is taking commands".
- On a protected install the hidden tab is also read-only, its motion stream
  closes, and `useControllerConnection` stops renewing the controller lease.
  The access watchdog then fences the lease after 15 seconds and runs the
  global Stop (`controller_heartbeat_expired`). Locking the desktop screen or
  switching windows ended the session, phone or no phone.
- Unprotected local installs never take that Stop path, but the phone still
  lost its desktop on tab-out.
- Edge was mounted only in the desktop `NavRail`. The phone had Stop but no
  gentle hold.

These were deliberate choices (ADR 0029: only a responsive foreground document
keeps a protected lease; ADR 0032: a hidden desktop withdraws presence). The
fix keeps the dead-man's switch and lets the phone hold it.

## Initial change

- The phone renews its desktop's lease every three seconds while its page is
  in front. Renewal needs the same desktop account and live control; it never
  claims control, issues tickets or revives an expired lease, and it does not
  extend login idle time.
- The phone sees its desktop session (alive, motion, Edge hold) even after the
  hidden desktop withdraws presence, and explains that video and chat return
  when the desktop tab is in front.
- Edge and Return are on the phone, beside Stop. They act on the server
  directly under remote-control admission and the current Stop sequence, and
  are audited under the phone's login. The desktop and the phone share one
  Edge control and one server path.

## Follow-up

In this initial version, video and chat commands still needed the desktop tab in front. The
[hidden-desktop review](hidden-desktop-remote-review-2026-10-24.md) and
[ADR 0056](decisions/0056-hidden-desktop-acts-for-its-phone.md) let a hidden
desktop held by its phone keep carrying them out. Renewal now issues that
desktop's tickets; the initial no-ticket restriction above is superseded.

## Limits

The phone must stay open and in front: on a protected install a locked phone
stops the session within about 15 seconds when the desktop tab is hidden.
Unprotected local control retains its existing expiry policy. Paired video scripts keep
their own media heartbeat, so a hidden desktop's script playback depends on
that tab staying awake (ADR 0056). No hardware was connected; tests use the
fake transport.
