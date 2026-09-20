# Informational notices

Explanatory notices share one dismissal component, one catalog and one durable
preference store. The × action asks whether to hide the notice **Just this time**
or **Don't show again**. Cancel restores focus to the ×. A temporary dismissal
lasts for the current application visit and does not write to storage. A saved
dismissal is applied only after the backend acknowledges it; a failed or timed
out save keeps the choice visible and offers a status refresh.

## Ownership and storage

Saved choices follow the authenticated account across browsers. The sign-in
explanation always belongs to the current browser because it must work before
authentication. In an installation without accounts, choices are browser-owned.
The client declares its expected scope, so expiry cannot silently turn an
account operation into an anonymous preference write. Account changes retire
pending preference reads, writes and temporary dismissals.

Schema 25 adds `notice_preferences` to the existing `magichandy.db`, alongside
settings, account/session data and the other persisted application domains.
There is no new database, preference file or localStorage collection. Account
rows cascade on account deletion. Browser rows use only the digest of a random
256-bit HttpOnly, SameSite=Strict cookie; HTTPS uses the Secure `__Host-` cookie
variant, matching the existing trusted-loopback exception for local HTTP.
The identifier is not a login credential and grants no control or host access.

Account sessions are revalidated inside the same write transaction as a saved
choice. Changes patch one notice ID, so concurrent tabs cannot erase each
other's choices. Records contain fixed catalog IDs rather than arbitrary user
content. Anonymous storage is capped at 2,048 profiles; documents are bounded
to 2 KiB and 16 IDs. Browser records expire after a year without a preference
change and are removed during subsequent writes. Reads create no database rows.
The endpoint has a 1 KiB upload budget, a three-second upload deadline and a
five-second operation deadline, and remains behind same-origin and ordinary
request admission. Public access reveals only the requesting browser's choices.

`internal/notices/catalog.json` is embedded by Go and imported by the frontend
at build time. It is the single allowlist of dismissible information. It covers
sign-in safety explanation, setup protection/remote-access/device/before-motion
guidance, model generation guidance, firmware requirements and the control
profile explanation. Adding a notice requires a stable catalog ID and localized
copy; an arbitrary ID cannot hide an operational control or error.

## Restoration and presentation

Access → Your profile contains **Informational notices**, with per-notice
**Show again** and **Show all notices again** actions. General links to that
manager. Before password protection is enabled the manager remains available
on Access. Both dismissal choices leave no placeholder, link, count or other marker
outside Settings, including on sign-in. Restoration is available only through
the settings manager after gaining access to the app. The manager labels
temporary choices, account choices and browser choices separately.

Notices use a neutral surface and a thin outline on every side. Explanatory
notices and existing operational readouts no longer use a colored left accent
line. Dismissal confirmation is inline; it never overlays Emergency Stop or
intercepts its global Escape shortcut.
The permanent Stop control, errors, live connection state, installer failures
and workflow requirements remain driven by current backend state rather than
the explanatory-notice preference catalog.

Existing notification-category preferences already live in the same settings
datastore. The bell/toast history remains a bounded per-login transient cache;
it does not hold saved informational-notice dismissals. No periodic preference
poll is introduced: the UI reads on audience entry and on returning to a visible
document, and uses backend responses after explicit changes.

## Validation

Regression checks cover account/browser isolation, cross-browser account
preferences, restart persistence, concurrent updates, expiration, record caps,
schema migration, missing-bound detection, account deletion, unknown IDs,
cross-origin requests, revoked sessions and scope changes. UI checks cover
temporary versus saved dismissal, delayed/failed writes, timeout and late
responses, account-change cancellation, keyboard focus and restoring notices.
The full Go/race suites and frontend suite passed for this implementation;
current artifact measurements and live-review results belong in the scorecard.
