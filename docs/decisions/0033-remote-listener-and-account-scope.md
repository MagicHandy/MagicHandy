# ADR 0033: Separate remote listener and scoped account access

Status: accepted for implementation, 2026-09-29. Extends ADR 0032.

## Decision

Run the remote on a second socket in the existing process. Both listeners share
one account store, controller, remote hub, motion engine and transport graph.
There is no second application backend or motion path. The canonical embedded UI
selects its shell from a server-injected interface marker. The remote shell never
mounts the main app's state/controller connection, even before sign-in.

The default remote port is the main listen port plus one (normally 49718).
Automatic selection skips a collision with the main public origin. `-remote-port`
overrides the saved choice; zero selects automatically and -1 disables it.
`-remote-public-url` supplies a separate HTTPS origin for a trusted proxy or
external port. Both sockets must bind before startup is announced. The console
shows App and Remote addresses with independent open/copy actions (R/L for remote).

The remote inherits the main bind IP, authentication, TLS and trusted proxy peers.
Direct HTTPS reuses the main certificate and therefore requires the same hostname.
Automatic public certificate issuance/renewal stays on main public TCP 443.
Operators must separately forward/allow the remote's displayed port, or configure
a second HTTPS proxy origin. The app does not silently change a firewall/router.
Local-only still binds only loopback. There is no cross-origin browser API access.

## Authority and session boundary

Schema v29 adds account `interface_access` (full/remote) and session `interface`.
Existing rows migrate to full; hashes, public session IDs and expiry survive.
Remote-only is an operator scope, not another role. Administrators retain full
access. The remote's cookie name differs from the main cookie and the session's
interface is checked in storage: cookies alone cannot isolate two ports. Renaming
a remote cookie cannot turn it into a main-app session. Ordinary logout clears
only the relevant cookie and revokes that login.

Each remote listener has an explicit route allowlist: shell, health, public Stop,
sign-in/logout/password/recovery, own sessions/notices, remote state/events/commands,
and display-only video catalog/current conversation reads. Full APIs, controller
claims, presence publishing, command claims, host configuration and file delivery
are absent, including for administrators signed into the remote.

An existing full-access account can remote-control its own desktop. A remote-only
account needs an explicit timed/permanent control grant and can control only the
granting administrator's desktop. The grant shares video catalog titles and the
currently displayed conversation; the UI explains that scope. Another account's
desktop and unrelated conversation IDs remain inaccessible. Replacing a grant
invalidates old queued commands, even if the replacement is also valid. Changing
interface access revokes all account sessions and its grant. Administrators must
grant permission again after a scope change. Disable, expiry, password changes,
controller loss and Stop retain the common cancellation/fencing rules.

## Display and resource bounds

Phone controls use compact filled segmented choices; desktop presents video and
chat side by side. Emergency Stop is outside scrolling content and available
before sign-in, without a grant and when the core is offline. Own account controls
include password, recovery codes, sessions and dismissed notices, never host settings.

The remote catalog returns at most 60 display-only rows per page: ID, title,
duration and script availability. Search handles Unicode names and indexed tag
keys without per-result metadata queries. It does not return file paths, notes,
scan state or download URLs. The chat read requires the exact currently shared
conversation, returns only the last 40 committed messages, and truncates each in
SQLite at 2048 Unicode characters. Diagnostics and browser attribution are absent.
A desktop-reported message/revision cursor refreshes fast replies and edits even
when the busy transition falls between presence reports.

The hub still has 32 pending commands, ten-second command expiry and one-shot
claims. The existing motion stream delivers commands to the desktop. No dependency,
transcoding, remote audio worker, independent device owner or sampler is introduced.

## Alternatives and limits

Keeping `#/remote` as a full-app route would not satisfy the requested separate
network boundary. It now offers a link to the dedicated origin (or disabled state).
A separate process/store would duplicate authority and complicate Stop. A shared
session cookie with a hidden navigation menu would provide no API isolation.

This remains a remote for a visible desktop browser, not unattended server playback.
The desktop controls the picture/audio clock and retains browser autoplay and
visibility requirements. The capability and standards review records deliberate
limits and possible follow-up work in [the review](../remote-access-review-2026-09-29.md).

## Amendment (2026-09-30): opening the Remote from the main app

The main app's Remote link never worked in alpha.51 or alpha.52. A link from the
main port to the next port is a cross-origin, same-site page load, so browsers
send `Sec-Fetch-Site: same-site`. The browser boundary accepted only `same-origin`
or `none` and answered "browser requests must use an allowed MagicHandy origin".
Reviews opened the Remote by typed address, which sends `none`, so it went unseen.

Both listeners now also accept a page load that meets all of these conditions:
- a top-level page load (`GET` or `HEAD`, `Sec-Fetch-Mode: navigate`,
  `Sec-Fetch-Dest: document`);
- marked `same-site` by the browser;
- sent with no `Origin` header;
- for a path outside `/api/`.

Only the app shell and `/healthz` live outside `/api/`. The browser never hands
the page to the one that linked it. API requests, frames, form posts and page
loads from other sites keep the same-origin rule, and the Host allowlist still
rejects rebound hostnames. This narrows the exact-origin rule in
[ADR 0017](0017-authenticated-lan-https.md) for page loads only. There is still
no cross-origin browser API access.
