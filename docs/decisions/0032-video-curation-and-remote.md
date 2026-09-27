# ADR 0032: Video curation, chat beside the video, and a phone remote

- Date: 2026-09-27
- Status: Implemented for review; no merge or release authorized
- Amends: the media-manager guardrail in [video-playback.md](../video-playback.md)
- Extends: ADRs 0002, 0026 and 0029

## Context

A design review of chat and video together split its proposals into what a
local model can carry now and what waits for a stronger one. For
Gemma 12B the direction was a chat beside the video, not a model that directs
playback. The direction also asked for two features now: managing video
metadata, and a remote for a phone that controls either the video or the chat
on the desktop.

The video guardrails said MagicHandy is "not a media manager" and ruled out
tagging and metadata editing. Library curation reverses that line. Transcoding
during playback, external players and codec bundling stay out.

The player had one component that owned the media element, the sync session,
paired-script loading and every control. A remote that reached into it would
have entrenched that shape, so the player was split first. Commands now pass
through one controller that on-screen controls, keyboard gestures and the
remote share.

## Decision

### Curation is library content

Each catalog video can carry a title, a rating from 1 to 5, notes and up to 32
tags. They live in SQLite schema v27 beside the scan output. The title is at
most 200 characters and the notes at most 2,000. Tags are 1–40 characters,
compared without case, and keep the library's first spelling. A rescan keeps
curation because a file keeps its catalog row, and a converted copy inherits
its source's curation. A moved or renamed file is a new row. Nothing is
written to media files or sidecars.

Curation needs control permission, not the controller lease. A phone can tag
a video while the desktop keeps control, and an observer sees curation but
cannot change it. Bulk tagging covers at most 500 videos per request. Renaming
a tag onto an existing one merges them.

### Chat beside the video

The player page can open the active conversation beside the picture. It is
the same backend session as the Chat page, so its replies are spoken by the
tab that holds control. Opening or closing the chat keeps the player mounted.
With the chat open, fullscreen takes the video and the chat together. A paired
script still moves the device; if the chat starts motion, it takes over like
any other source and the video pauses.

### A phone remote that asks the desktop

A signed-in phone controls the desktop tab that holds control. It does not
become a second controller. It asks; the desktop acts with its own controls,
its own controller authority and its own Stop fencing, so the remote adds no
motion path.

- **Presence.** The controller tab reports what it shows: its page, the open
  video (title, play intent, position, duration, volume, mute, speed, sync
  state, whether the script has loaded) and the open conversation (session,
  persona name, busy, ready). It reports on changes and every five seconds. A
  report needs the controller at its current generation. It is not a device
  command: it takes no command ticket, never waits behind live control, adds
  nothing to the audit log and does not renew login idle time. Presence lapses
  after 20 seconds. A hidden tab, a tab that loses control and a closing tab
  withdraw it.
- **Commands.** Video: play, pause, toggle, seek, seek by, volume, mute,
  speed, open and close. Chat: open and send. The phone needs control
  permission and the same account as the desktop. Commands wait at most ten
  seconds, at most 32 at a time; a late command is dropped, never run late.
  Emergency Stop drops every waiting command, and so does a change of desktop.
- **Delivery.** Commands reach the desktop as `remote_command` events on its
  existing motion stream. Only the tab that reported presence and still holds
  control receives them. The desktop already streams video, chat and speech,
  and on loopback HTTP/1.1 a browser allows six connections per host, so a
  dedicated command stream could have delayed the controller heartbeat.
- **Execution.** A video command calls the player's own command, which returns
  whether it was accepted; a paired script still arms, seeks and stops through
  media sync. Opening a video only opens it: playback starts when someone
  presses play. A chat message goes through the desktop composer, with the
  composer's Stop fence, and the desktop speaks the reply. Outcomes are
  canonical English sentences that the phone translates.
- **Audience.** Observers cannot read remote state. Another account learns only
  that a desktop is present. `internal/remote` imports none of motion,
  transport, chat, media, modes, voice, LLM or the HTTP edge.

The phone page (`#/remote`) switches between Video and Chat. Video shows the
transport, a position slider, volume, mute, speed and a searchable catalog to
open from. Chat shows the recent conversation and a composer. The persistent
Stop remains in the shell on every route.

## Alternatives not selected

- The phone as a second controller. It would need the lease, and a takeover
  stops the desktop's motion; two tabs would contend for one media clock.
- The phone driving `/api/media/sync` directly. The browser that plays the
  video owns the media clock; a phone cannot know when the picture is ready.
- A server-side playback state machine. It would duplicate the player's
  arming and Stop rules in a second place.
- A dedicated command stream, WebSockets or WebRTC. The motion stream already
  reaches the desktop; no new connection, protocol or dependency is needed.
- Opening a video with playback. With a paired script that would start motion
  from a list tap; play stays an explicit press.
- Streaming the video to the phone. It was not requested and would add a
  second media clock.
- Keyword rules for chat-driven playback. Letting the model act on the video
  is left for a later, stronger model.

## Consequences

- Five routes join the admission matrix: presence (control) and its
  withdrawal (control), state and events (shared admission, handler requires
  control) and commands (control). Curation adds four control routes.
- The desktop must stay visible. A hidden protected tab gives up control
  after 15 seconds, as any controller tab does.
- One desktop at a time: the tab that holds control. A second desktop tab
  that takes control replaces it and drops its waiting commands.
- The remote and curation are covered by Go hub and HTTP tests and by the
  frontend executor, provider and page tests. Physical acceptance of phone
  control over a paired video remains open with Phase 18 M3.
