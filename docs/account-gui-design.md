# Account Access And Control-Context UI

Status 2026-08-28: implemented on the account-GUI feature branch. This document
is the product and integration specification for setup, login, account
administration, profile images, and the top-bar control-profile selector. HTTPS
exposure and the credential/session threat model remain governed by
[ADR 0017](decisions/0017-authenticated-lan-https.md); the GUI and future-session
seams are recorded by [ADR 0018](decisions/0018-account-gui-and-control-context.md).

## The Three Identities Must Stay Separate

MagicHandy has three related but deliberately independent concepts:

| Question | Backend owner | Current UI |
| --- | --- | --- |
| Who signed in? | Account + opaque HttpOnly session | Login, profile, account administration |
| Whose linked control context is this session using? | `user_sessions.control_account_id` constrained by active directional links | Compact top-bar selector; **Self** by default |
| Which browser may command the device now? | Existing single-controller lease | Controller/read-only status and stop-first **Take control** |

Selecting a linked control profile does not impersonate that account, inherit
its role, authenticate as it, or transfer the device-controller lease. The
selector only records backend-authorized attribution context for the current
login session. No current motion endpoint consumes that context. This gives a
later multi-session or invitation-based remote-control feature a stable seam
without inventing permission semantics prematurely.

Application settings, chat, personas, media metadata, and libraries remain
shared installation data. Accounts do not imply multi-tenant isolation.

## Entry Flows

### Installer and first run

The thin Inno Setup program still only installs files, shortcuts, and uninstall
metadata, then launches the embedded `#/setup` experience. It never asks for,
receives, logs, or transports an account password.

The in-app wizard now has an **Access** step immediately after Welcome:

1. **Only this computer** (recommended) keeps loopback-only access with no
   sign-in. Password protection can be enabled later in Settings > Access.
2. **Require an account and password** asks for the initial administrator and
   confirmation. Continue sends the password directly in the local JSON API
   request; it never enters installer arguments, logs, response files, registry
   values, or settings.

Creating the first account is the durable opt-in. The backend turns on the
authentication wall immediately, creates the initial browser session, and
requires login on later launches even if the process is restarted without
`-require-auth`. This choice alone does not bind a LAN address or configure a
certificate. Remote login still requires an explicit private listen IP and a
trusted matching HTTPS certificate.

### Login and session expiry

The embedded SPA shell is public so React can render the branded login screen;
private APIs remain behind authentication. The browser-native Basic-auth bridge
is retired. Login uses only the JSON endpoint and a host-only HttpOnly cookie,
so neither the token nor a parallel authentication model enters React.

When an API reports `401`, the shared auth provider refreshes backend status,
disables app-state polling, and returns the workspace to login. The permanent
shell remains mounted. Navigation and private state are hidden, while Emergency
Stop remains present and continues to call the intentionally public fail-safe
Stop endpoint.

If authentication is required but no account exists, the first administrator
can be created only from the server computer. A remote browser receives a
specific **Local setup required** state rather than a form that can never
succeed.

## Authenticated Surfaces

### Top-bar control-profile selector

The selector is a shell-owned, non-modal disclosure beside notifications and
the connection manager. Only one of those three disclosures may be open. Its
compact trigger contains a profile image or generated monogram, the label
**Control profile**, and the current selection.

- **Self** is always first and selected for a new login session.
- Only enabled accounts reached by an `active` directional link are returned.
- Pending, revoked, disabled, unlinked, and fabricated account IDs never appear
  and are rejected if posted directly.
- Selection is stored on the opaque server session, not in local storage. A
  second login session starts at Self and does not inherit another browser's
  selection.
- The panel states that selection does not sign in as another account or
  transfer device control, and links to Settings > Access.

The database reserves directional link states (`pending`, `active`, `revoked`)
so a later invitation/acceptance protocol can be added without redefining what
an active relationship means. This slice deliberately provides no UI or API to
create those links; no relationship is inferred from sharing an installation.

### Settings > Access

An unprotected installation shows one action: create the first administrator
and enable password protection. An authenticated account sees:

- its profile image, role, and username;
- replace/remove image actions and monogram fallback;
- password change with current-password confirmation; changing it revokes all
  sessions and returns the user to login;
- linked control profiles currently authorized for this account.

Administrators additionally see installation accounts with role, enabled
state, and last sign-in, plus create, enable/disable, and password-reset actions.
Operators never receive this list or its endpoints. The last enabled
administrator cannot be disabled. Account deletion, role mutation, recovery,
MFA, and an audit-log UI are deferred.

## Profile Images

Profile images reuse the persona portrait pipeline rather than adding an image
runtime to the Go core:

1. the browser decodes, center-fits, and resizes the selected source to a
   640-by-640 JPEG;
2. the server caps the request at 2 MiB, decodes it as JPEG, rejects dimensions
   above 1024 pixels, and never trusts the declared MIME type alone;
3. the file is written atomically under the app-owned data directory and the
   SQLite row stores only a cache timestamp;
4. image reads require the owner, an administrator, or an active directional
   link, and respond as private content;
5. startup reconciliation removes interrupted/orphaned files and clears a
   stale database stamp if a file is missing.

No image bytes are stored in SQLite or embedded in the frontend bundle. A
Unicode-safe first-character monogram is the zero-asset fallback.

## Visual And Interaction Rules

The UI uses the existing graphite surfaces, compact steel-azure interaction
hue, square-small radii, border hierarchy, and typography. Profile imagery is
content, not a new decorative palette. There is no purple, blue-green accent,
glow, oversized round pill, or animated identity treatment. Green remains
running/healthy state; solid red remains Emergency Stop.

The login screen is one quiet bounded panel inside the permanent shell, not a
marketing hero. Settings follows the existing section/group grammar. The
top-bar trigger stays compact and collapses gracefully at narrow widths. Every
form uses native labels and password autocomplete semantics; disclosures move
focus to Close and restore the trigger when closed. No identity animation is
required, including under `prefers-reduced-motion`.

## Current Acceptance

- local bootstrap flips the live and restart authentication walls;
- the static login shell loads while every private API stays protected;
- login, logout, password change, admin actions, expiry, and `401` recovery use
  backend-owned session state only;
- Emergency Stop remains mounted and callable before login and after expiry;
- Self is the default per-session control context; active links are session
  scoped and an unlinked target is rejected;
- profile upload/read/delete validates bytes, enforces visibility, and keeps
  monogram fallback;
- the eight-step setup flow never writes a password to installer-owned state;
- all five locale catalogs have key and placeholder parity;
- desktop rendered review shows no disclosure overlap or hidden Stop control;
  narrow-width acceptance remains part of the later real mobile-browser matrix.

## Deliberately Deferred

- link invitations, acceptance, revocation UI, and remote-control grants;
- concurrent multi-controller behavior or per-linked-account motion queues;
- internet exposure, reverse proxies, public DNS, or cloud identity;
- per-user partitioning of chat/settings/library/media data;
- password recovery, MFA/passkeys, email identity, account deletion, role
  changes, and durable audit logs;
- automatic certificate issuance, client trust installation, and renewal;
- claims of phone/tablet support before the real trusted-CA matrix in R29 is
  complete.
