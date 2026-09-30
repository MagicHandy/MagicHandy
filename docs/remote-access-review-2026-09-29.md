# Remote interface and account review — 2026-09-29

This extends the Claude completion review with a separate remote listener,
restricted accounts and compact phone/desktop designs. All validation uses
synthetic accounts/media and the shared engine's in-process simulator.

## Standards and reference comparison

| Reference | Assessment and implementation |
| --- | --- |
| [OWASP authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) | Deny by default and least privilege: explicit remote route inventory, stored interface scope, no controller takeover, and grants bound to the issuing administrator's desktop. Recheck sessions/grants at dispatch and before returning protected display data. Scope changes revoke sessions and grants. |
| [OWASP session management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) | Existing opaque random sessions, server-side expiry, idle limits, HTTPS HttpOnly/SameSite cookies, independent management IDs and revocation remain. Added separate cookie names and stored session audiences because a browser cookie is not port-scoped. Ordinary remote logout no longer clears all cookies on the host. |
| [NIST SP 800-63B-4, password verifiers](https://pages.nist.gov/800-63-4/sp800-63b.html) | New passwords require 15 Unicode code points for password-only sign-in. Existing shorter passwords continue to authenticate. Long passwords, password-manager paste, no composition rules, salted Argon2id hashing and online throttling already exist. This is not a claim of full NIST conformance: breached/common-password blocklisting and MFA/passkeys remain absent. |
| [Jellyfin user permissions](https://jellyfin.org/docs/general/server/users/adding-managing-users/) | Separate remote-control authority from administration and file/content access. An account can sign in to its remote and manage its own security before receiving any control grant. Disabling an account and inspecting/revoking its own sessions remain explicit operations. |
| [WCAG 2.2 target size](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html) | Compact remote buttons retain 36px or larger targets, above the 24px minimum. Native labeled sliders, keyboard controls and named groups remain. Filled selected states use no left accent or underline. Stop stays outside the scroll area. Full accessibility conformance was not audited. |

Additional review fixes bind profile-image writes and control-identity changes to
a still-live session. Profile uploads revalidate inside the final metadata
transaction and remove staged files if the login was revoked. Administrator domain
writes also reject a login issued to the remote interface.

## Capability inventory

| Capability | Result |
| --- | --- |
| Open a catalog video; play/pause; seek/skip; volume/mute/rate; close | Supported through the desktop's existing player. Opening does not start playback. Catalog pages are bounded and searchable. |
| Script / Chat / Off source choice | Supported, with the existing awaited Stop handoff and shared motion engine. |
| Open desktop chat; send text; read recent committed replies | Supported for the displayed conversation. Added display-only API and message/revision refresh so fast replies/edits do not remain stale. |
| Emergency Stop | Public on both ports, remains visible during sign-in, no permission, account management and offline states. A Stop cancels queued commands. |
| Remote-only login and timed/permanent delegation | Supported. Grant is scoped to the issuing administrator's desktop; revocation, replacement and scope changes invalidate stale authority. |
| Own password, recovery codes, session names/revocation, notice preferences | Supported in the remote account view, with the same backend and database as the main application. Session rows identify their interface. |
| Reconnect / stale desktop / wrong account / failed command | Explicit status; stale controls hidden; bounded retry and one-shot command admission. |
| Desktop not in front, host asleep or app offline | Cannot play independently. Stop remains callable, but successful physical delivery requires the core and device connection. |
| Phone-local video/audio streaming, remote microphone/TTS playback, fullscreen request | Not implemented. The desktop owns media/audio; browsers require local gestures for some actions. |
| Playlist/next/previous, choose a different persona or historical conversation, direct mode/pattern selection, dedicated reply-cancel button | Not implemented. These are future features, not reasons to expose full-app APIs. Stop already cancels active work. |
| Pair/manage hardware, edit filters/calibration, configure workers or network | Intentionally restricted to the main application. |

## Design work

Two built-in image generation runs produced mobile and desktop reference concepts.
The implementation uses native React controls and existing SVG icons; no generated
bitmap ships in the bundle. The phone has compact Video/Chat choices and an account
icon, while desktop places the two control areas side by side. No decorative accent
lines, glow effects, card grid, or extra navigation tier was added. Mockup playlist
counts were not treated as implemented features.

The visual follow-up restores the concepts' structural borders: full-height
desktop pane separation, pane headings, a divided chat composer, outlined volume
controls and library rows with full outer borders and shared horizontal dividers.
Buttons have solid fills and crisp corners; the
remote overrides the main application's gradients and shadows. The desktop status
joins the header to leave more space for the catalog. General settings persists
the optional Remote sidebar shortcut in the existing settings database. It does
not disable the listener. Browser verification covered hiding, reloading and
restoring the shortcut, plus 390px phone video/chat and desktop layouts.

## Verification

- Full Go suite, Windows race suite, vet and golangci-lint pass with architecture,
  goroutine-lifecycle and admission gates enabled; no new dependency.
- Frontend typecheck, localization, 737 tests in 96 files and canonical build pass.
  The added UI preference passes config tests, race tests and vet; older clients
  cannot reset the saved shortcut preference by omitting the new field.
- Installer tests pass in PowerShell 7 and Windows PowerShell 5.1. Runtime fixtures
  reserve non-overlapping app/remote port pairs and verify both remote listeners;
  the multi-instance teardown refusal remains enforced.
- Schema-v28 migration tests preserve existing accounts/logins and are idempotent.
  Interface, grant replacement, cross-port cookie/origin, public Stop and protected
  display-API tests cover denial and valid scoped reads; conversation tests check
  40-row/2048-character bounds, pending-message exclusion and field redaction.
- Real browser executor accepted commands from a remote-only account on the second
  socket. Seek/source/chat acknowledgements were 117–227 ms. Simulator play, live
  seek, Script → Chat → Script, and Stop passed (138–217 ms acknowledgements).
  The engine remained stopped after Stop. These are loopback software measurements,
  not WAN timing or real-device acceptance claims.
- Review LLM: local Ollama `huihui_ai/granite4.1-abliterated:3b`, real generation plus
  a text-only application chat with no repair, fallback or motion.
- Final review URLs: main `http://127.0.0.14:50245`, remote
  `http://127.0.0.14:50246`. The final review app passed real LLM readiness and
  completed a text-only reply through the remote. Earlier live verification
  completed a 1.657s Ollama reply in one provider call without repair/fallback.
  Live revocation produced
  403 for remote reads while Stop returned 200; restoring the grant restored reads.
  At 390px, the header fits one row and the transport labels do not wrap.
  The desktop chat composer stays above Stop at short viewport heights.
- Local trace and machine-readable QA reports live under
  `.scratch/remote-review-20260929/` and remain excluded from Git. Installed-instance
  data and credentials were not used. The preview browser's native confirmation
  dialog stalled automation; its controller lease was established with the same
  authenticated API in the simulator, then the real browser executed remote commands.

The architecture is recorded in [ADR 0033](decisions/0033-remote-listener-and-account-scope.md).
