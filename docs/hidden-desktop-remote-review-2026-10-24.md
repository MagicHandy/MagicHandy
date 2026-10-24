# Hidden desktop and the phone remote — October 24, 2026

Decision: [ADR 0056](decisions/0056-hidden-desktop-acts-for-its-phone.md).
Builds on the [keepalive and Edge review](remote-keepalive-edge-review-2026-10-24.md)
([ADR 0055](decisions/0055-phone-remote-keepalive-and-edge.md)).

## Findings

With ADR 0055 the phone kept a hidden desktop's session alive and offered Edge
and Stop, but video and chat still needed the desktop tab in front. Hiding the
tab put every part of it to sleep:

- `RemoteExecutorProvider` required a visible document, so the tab withdrew
  its presence and the phone had nowhere to send commands.
- `AppStateProvider` stopped the state poll, closed the motion stream that
  carries phone commands, and marked the view stale. Stale means read-only.
- Read-only locked the player (`VideoRoute` passes `locked`), which paused a
  paired video and stopped its synchronized motion the moment the tab hid,
  phone or no phone.
- `ChatPanel` cancelled an in-flight reply on hide, and `useChatHistory`
  skipped reads and spoken replies while hidden. A chat the phone opened on a
  hidden desktop could never load, so it could never send.
- `useControllerConnection` stopped observing, and command tickets come only
  from renewals. A hidden tab had no ticket for the media heartbeat or a chat
  send, so even a listening tab could not have acted.

## Change

- The phone's keepalive renewal now issues the desktop tab's next command
  ticket, and the owner's controller snapshot carries `remote_held` while a
  phone renewed within the last 15 seconds.
- A hidden tab that holds control keeps observing in the background
  (`X-MagicHandy-Background`), which never renews a lease, protected or local.
  It never sends a heartbeat while hidden.
- While held, the hidden tab stays awake: poll, motion stream and presence
  continue, so it stays current and not read-only. The player keeps playing,
  the chat keeps streaming, reading and speaking, and phone commands run
  through the same controls and admission as before.
- The tab reports `background` in its presence. The phone reads "Connected to
  the desktop in the background", explains that this phone keeps the session
  going and, for protected sessions, that locking it ends the session within
  15 seconds, and the footer
  says the session is kept alive from this phone.
- When the phone goes quiet, tickets stop within 10 seconds and the hold and
  lease end at 15 seconds: the tab sleeps and, on a protected install, the
  lost-controller Stop runs. A phone that returns before then wakes the tab.
  A tab kept awake returns to the front without a stale, read-only period.

## Limits

- The phone remains the dead-man's switch while the desktop is hidden.
- Browser Bluetooth still disconnects when the desktop tab hides, because that
  tab runs the device link (by design, see the
  [gateway contract](lan-wan-bluetooth-gateway.md)).
- A browser may freeze or discard a background tab (sleeping tabs, memory
  saving). The tab then stops taking commands, its presence lapses after 20
  seconds, and the phone falls back to Edge and Stop until the tab is in front.
- After about five minutes in the background, browsers slow the timers of a
  silent page. A muted paired video's 1.5-second sync heartbeat can then lapse,
  and the server stops that motion after five seconds, safely. Playback with
  sound is not slowed.
- A browser may also refuse to start sound in a page nobody has clicked; the
  phone's existing "click the desktop page once" outcome covers that.

## Verification

Go: all packages pass, `golangci-lint` reports 0 issues, and the
`CGO_ENABLED=0` build succeeds. The race detector needs a C toolchain that this
host lacks, so `go test -race` runs only in CI. Web: 984 tests and the
localization check pass.

Review app (unprotected local install, `-simulate-motion`, Ollama Gemma 12B,
synthetic one-minute test video with motion source Off). The desktop tab was
forced hidden for more than two minutes, past both the 15-second lease and the
20-second presence window:

- the hidden tab kept its state poll, background controller observation and
  presence reports, sent no heartbeat, and kept its motion stream open;
- the phone read "Connected to the desktop in the background" and showed the
  hold notice;
- from the phone, open, play, forward 10 s, pause and open chat all ran on the
  hidden desktop, confirmed against its video element;
- a phone chat message became a words-only `POST /api/chat/stream` (200, 3.6 s)
  from the hidden tab, the reply appeared on the phone, and the engine stayed
  idle;
- returning the tab to the front opened no new stream, sampling every 10 ms
  found no read-only moment, and the phone went back to "Connected to the
  desktop".

The protected watchdog path (tickets lapsing, lease expiry, Stop) is covered by
Go tests, not by the unprotected review app.

## Independent release review

The new phone Edge endpoints initially rechecked authority before applying a
mode but did not bind the request to the desktop owner. A deterministic test
blocked mode application, retired that owner through Stop or handoff, then let
the request finish. All four Edge/Return cases incorrectly returned success
and changed the target. The endpoints now bind to the owner's synchronous
cancellation path before revalidation. All four cases return conflict and
leave the plan unchanged. Local Stop also cancels bound requests without
changing its existing controller-generation policy.

The phone's original 15-second Stop notice also overstated the unprotected
local policy. The remote session now reports `stop_on_expiry` from the backend;
only protected sessions show that promise. Go tests check both policies, and
UI tests check connected and presence-withdrawn local sessions.
