# Settings review — 2026-09-19

The main settings menu now has seven destinations. Chat combines the previously
separate Chat, Model and Prompts pages; only Chat and Access need a second level
of navigation. Both reuse the same URL-based navigation component and compact
responsive styling.

| Destination | Structure and result |
| --- | --- |
| General | One page for interface, Labs, updates, notifications and setup. The current theme is visible in a disclosure, with the full palette chooser available on demand. |
| Access | Six focused destinations: Profile, Security, Sessions, Accounts & permissions, Remote access and Access history. Personal and installation tasks remain separated. Only the selected area's panels load. |
| Device | One page for connection, local server and manual testing. Flat sections and compact actions clarify the hierarchy without moving safety controls or changing connection behavior. |
| Media library | One page for script playback, locations, scans, FFmpeg, thumbnails and conversion. Existing related groups remain together; shared dividers and compact actions replace extra visual weight. |
| Chat | A sidebar for Conversation, Model and Prompts & memory. Only the selected section mounts. Generation fields adapt to the available content width, and import actions wrap as complete buttons. |
| Voice | One page with worker, speech input and speech output sections. Provider-specific forms appear on selection; runtime actions retain their saved-configuration checks. |
| Diagnostics | One page for logging, prompt composition, status report, traces and reset, using the same flat sections and action sizing. |

## Navigation and editing contract

The canonical Chat routes are `#/settings/chat/conversation`,
`#/settings/chat/model` and `#/settings/chat/prompts`. `#/settings/chat` opens
Conversation. Existing `#/settings/model` and `#/settings/prompts` bookmarks
still open the corresponding Chat section and select Chat in the primary menu.
Unknown Chat children resolve to Conversation. The persona-default link uses
the canonical Prompts route.

The parent settings route owns one host-settings draft and one Save action.
Changes survive switching Chat sections and visiting Access. Prompt sets and
memory retain their own immediate APIs. Account/security drafts retain their
separate clearing rules, and backend permissions remain authoritative.

Desktop secondary navigation uses row separators, a neutral selected outline
and a divider between navigation and content. Below 1000px each navigation
level becomes one 32px horizontal strip with 28px links. Selection uses a neutral
filled surface, with no colored side marker or underline. Short visual labels
retain full accessible names. Overflow arrows scroll the strip without changing
the route, and entry/resizing keeps the selected link visible without scrolling
the page vertically. Settings actions and icon buttons are 32px in this layout.
The permanent Emergency Stop remains in the application shell.

## Validation and limits

- All 631 frontend tests in 83 files pass, including legacy/canonical routes,
  one draft across all three Chat sections, no secondary navigation on General,
  localized labels, role restrictions, overflow/reveal and observer cleanup.
- TypeScript, 2,199 translation keys across five locales, the production build,
  architecture tests and embedded-asset checks pass. The recovery backend also
  passed full Go/race suites, vet and zero-issue golangci-lint v2.12.2.
- Live Chromium review at 390px covers all seven destinations, each Chat child,
  General's theme disclosure and the optional Qwen TTS form. Every reviewed
  settings action is 32px; both navigation levels measure 32px. No horizontal
  document overflow was observed. Voice selection was restored without saving
  or starting workers. No hardware connection, scan, installation or motion
  test was initiated.
- Restoring the normal 1110px viewport restores the Chat sidebar. The legacy
  Model bookmark selects Chat → Model, with the provider ready. Mobile session
  Refresh, Name, Sign out and Sign out other sessions all measure 32px.
- The current review binary at `http://127.0.0.10:50007` serves the exact
  worktree bundle and passes the real LLM readiness probe. Its text-only app
  chat returned “Text chat is ready.” in 110 ms, with one provider call and no
  repair, fallback or motion. Voice and LLM motion remain off; the simulator
  is idle and Bluetooth is disconnected.

This is responsive browser validation on the review host, not physical mobile
or external WAN acceptance. Artifact sizes and the limits of the runtime sample
are recorded in the [scorecard](goal-scorecard.md). The complete LAN/WAN
acceptance checklist remains open.
