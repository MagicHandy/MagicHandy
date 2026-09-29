# ADR 0032: Video curation, chat beside the video, and a phone remote

- Date: 2026-09-27
- Status: Implemented; reviewed and hardened on 2026-09-29
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

A follow-up asked for the largest possible picture, informed by how
livestream sites combine video and chat, and for the sketch's switch between
the script and the chat as what moves the device.

## Decision

### Curation is library content

Each catalog video can carry a title, a rating from 1 to 5, notes and up to 32
tags. They live in SQLite schema v28 beside the scan output. The title is at
most 200 characters and the notes at most 2,000. Tags are 1–40 characters,
compared using stored Unicode lowercase keys, and keep the library's first spelling.
The v28 migration merges case variants from v27 previews without losing titles,
ratings, notes or calibration. A rescan keeps
curation because a file keeps its catalog row, and a converted copy inherits
its source's curation. A moved or renamed file is a new row. Nothing is
written to media files or sidecars.

Curation needs control permission, not the controller lease. A phone can tag
a video while the desktop keeps control, and an observer sees curation but
cannot change it. Bulk tagging covers at most 500 videos per request. Renaming
a tag onto an existing one merges them. Login and control permission are
revalidated in the write transaction, including time spent waiting for the
writer. Bulk edits return their rows and tags in two bounded reads.

### Chat beside the video

The player page can open the active conversation beside the picture. It is
the same backend session as the Chat page, so its replies are spoken by the
tab that holds control. Opening or closing the chat keeps the player mounted.
With the chat open, fullscreen takes the video and the chat together. The
chat moves the device only when Chat is the motion source, below.

### A watch page that puts the picture first

Livestream pages keep the picture first and the chat in a column beside it
that can be collapsed; their theatre modes, and popular extensions to them,
give the picture more room and keep stream details reachable without leaving
it. The player page follows that shape:

- The video fills the workspace width and the height the status bar leaves,
  beside the full-height chat column when it is open. The route heading stays
  for screen readers only, and the page drops the content-width cap.
- One bar under the picture holds back, the title, the motion source, the chat
  toggle and editing. Details, curation and conversion notices fold below.
- Below 1000px the picture keeps its shape at the top and the chat stacks
  under it, as phone apps do. Fullscreen with the chat open keeps it beside the
  picture. The persistent Stop stays in the shell; nothing overlays it.

At 1280×800 with the chat closed, the picture grows from 966×368 to 994×591.

### One motion source at a time

A switch beside the picture chooses what moves the device while the video
plays: Script, Chat or Off.

- Script is the default when a script is paired, Off otherwise, and Script is
  unavailable without one. An existing background motion or Autopilot run is
  initially shown as Chat so opening a video cannot falsely label it Off.
  The choice lasts while the video is open.
- Switching pauses playback and waits for the old source to stop. The displayed
  choice changes only after acknowledgement. Leaving Script uses the player's
  ordinary media Stop, then teardown closes its session; leaving Chat uses the
  mode stop, which drains Autopilot and pending chat motion. A failed stop keeps
  the old choice and displays the error. Stop, controller loss and navigation
  invalidate a pending handoff. Press Play to start the newly selected script:
  finishing a delayed script load never automatically resumes motion.
- With Chat, the video plays as a plain one and the chat and Autopilot move the
  device, not synced to the picture.
- While Script or Off drives, a message sent from the chat beside the video is
  words only. The request names the owner (`motion_owner`), so the turn
  composes the chat-only contract with that reason and a closing "Video motion"
  note with one example of declining. Without the note, a persona reply
  narrated a faster, deeper pace the script never took. With it, a live
  Gemma 12B check through the phone remote declined 9 of 9 motion requests
  with the reason and a pointer to the switch, kept neutral questions
  conversational, and never moved the device.
- The phone remote shows and switches the source too. Its acknowledgement waits
  for the actual handoff rather than reporting an optimistic choice as success.

### A phone remote that asks the desktop

A signed-in phone controls the desktop tab that holds control. It does not
become a second controller. It asks; the desktop acts with its own controls,
its own controller authority and its own Stop fencing, so the remote adds no
motion path.

- **Presence.** The controller tab reports what it shows: its page, the open
  video (title, play intent, position, duration, volume, mute, speed, sync
  state, whether the script has loaded, the motion source, whether a script is
  paired) and the open conversation (session, persona name, busy, ready). It
  reports on changes and every five seconds. A
  report needs the controller at its current generation. It is not a device
  command: it takes no command ticket, never waits behind live control, adds
  nothing to the audit log and does not renew login idle time. Presence lapses
  after 20 seconds. A hidden tab, a tab that loses control and a closing tab
  withdraw it.
- **Commands.** Video: play, pause, toggle, seek, seek by, volume, mute,
  speed, motion source, open and close. Chat: open and send. The phone needs control
  permission and the same account as the desktop. Commands wait at most ten
  seconds, at most 32 at a time; a late command is dropped, never run late.
  Emergency Stop drops every waiting command, and so does a change of desktop.
- **Delivery.** Commands reach the desktop as `remote_command` events on its
  existing motion stream. Only the tab that reported presence and still holds
  control receives them. The desktop already streams video, chat and speech,
  and on loopback HTTP/1.1 a browser allows six connections per host, so a
  dedicated command stream could have delayed the controller heartbeat.
- **Execution.** Delivery is only a notification. Immediately before executing,
  the desktop claims the command once through the backend. The claim rechecks
  its deadline, originating login and grant, current desktop and original video
  or conversation. Claimed commands are not redelivered after reconnect/reload.
  A delayed claim response is rejected locally after its remaining lifetime,
  Stop or authority change. The original Stop sequence fences the actual player
  or chat action; it is never upgraded to a newer sequence. Commands execute
  serially. A video command calls the player's own command, which returns
  whether it was accepted; a paired script still arms, seeks and stops through
  media sync. Opening a video only opens it: playback starts when someone
  presses play. Browsers refuse to start sound on a page nobody has clicked,
  and for a paired video that refusal would come after motion armed. So a
  remote play on such a desktop is refused with that reason; a muted video
  may start. A chat message goes through the desktop composer, with the
  composer's Stop fence, and the desktop speaks the reply. Outcomes are
  canonical English sentences that the phone translates.
- **Audience.** Observers cannot read remote state. Another account learns only
  that a desktop is present; command history stays private after disconnection,
  expiry and account replacement too. The sender's login reference never appears
  in command JSON. `internal/remote` imports none of motion,
  transport, chat, media, modes, voice, LLM or the HTTP edge.

The phone page (`#/remote`) switches between Video and Chat. Video shows the
transport, a position slider, volume, mute, speed and a searchable catalog to
open from. Chat shows the recent conversation and a composer. The persistent
Stop remains in the shell on every route.

Remote controls disappear while their state is stale or offline. Reads started
before a newer stream observation cannot overwrite it. A pending scrub is
discarded when its video or Stop sequence changes. Plain videos use the same
app controls as paired videos; fullscreen includes the one global Stop through
a portal into the fullscreen root, with reserved space above the picture.

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
- Letting chat motion replace a running script, as before this change: the
  source that started last took over and the video paused. The switch makes
  the owner explicit instead.
- A chat overlaid on the picture, as some livestream extensions offer. The
  column keeps the picture unobstructed; an overlay can follow if wanted.

## Consequences

- Six routes join the admission matrix: presence (control) and its
  withdrawal (control), state and events (shared admission, handler requires
  control), commands and one-shot command claims (control). Curation adds four control routes.
- The chat stream accepts `motion_owner` (`script` or `off`); other values
  are rejected. Chat elsewhere is unchanged.
- The desktop must stay visible. A hidden protected tab gives up control
  after 15 seconds, as any controller tab does.
- One desktop at a time: the tab that holds control. A second desktop tab
  that takes control replaces it and drops its waiting commands.
- The remote and curation are covered by Go hub and HTTP tests and by the
  frontend executor, provider and page tests. Physical acceptance of phone
  control over a paired video remains open with Phase 18 M3.
- Review findings and validation are recorded in
  [the completion review](../claude-progress-review-2026-09-29.md).
