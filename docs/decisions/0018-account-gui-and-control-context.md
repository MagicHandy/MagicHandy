# ADR 0018: Account GUI And Session-Scoped Control Context

## Status

Accepted and implemented on the account-GUI feature branch.

## Context

ADR 0017 established accounts, opaque sessions, and authenticated LAN HTTPS,
but intentionally stopped at a browser-native Basic-auth bridge. MagicHandy
needs a permanent login experience, installer-time opt-in, profile images, and
account administration without moving passwords or bearer tokens into React.

Future multi-session and remote-control work also needs to represent a linked
account in the shell. That concept is easy to confuse with either signing in as
someone else or taking the single controller lease. Collapsing those concepts
would let a display selector become an accidental authorization path into
physical motion.

The thin Windows installer creates another boundary: collecting a password in
Inno Setup would put sensitive state into a second UI stack and create risks in
installer logs, command lines, response files, and upgrade state.

## Decision

### One backend-owned authentication UI

The embedded React application owns initial-account setup, JSON login/logout,
password changes, and administrator account management. The native Basic-auth
bridge is removed. Static SPA assets remain public so the login view can load;
all private API reads and mutations remain protected. The session token stays
in a host-only HttpOnly cookie and is never returned to or modeled by React.

An existing account makes authentication required on loopback after every
restart. Creating the first administrator switches the running middleware to
required immediately and returns an authenticated session. API `401` responses
invalidate the frontend's current account view and suspend private-state
polling until login succeeds again.

Emergency Stop and the permanent shell retain ADR 0017's exception: Stop stays
callable without a valid session, while private state, controller takeover, and
all non-fail-safe commands stay protected.

### Installer integration

The Access decision is step 2 of the in-app `#/setup` wizard. **Only this
computer** preserves the no-login loopback default. **Require an account and
password** creates the first local administrator through the loopback JSON API.
Inno Setup remains a thin package/shortcut/uninstall shell and never handles a
password. Account creation does not enable LAN listening or provision a
certificate.

### Session-scoped control context

Schema v19 adds an optional `user_sessions.control_account_id` foreign key and
directional `user_account_links` rows with `pending`, `active`, or `revoked`
status. The top-bar selector receives only Self plus enabled targets connected
by an active link. A new session selects Self. Changing the value updates that
single opaque login session.

Control context is attribution metadata reserved for later linked-session
work. It does not change the authenticated account, role, authorization checks,
controller lease, motion ownership, or transport path. No current motion API
uses it. The current slice has no link-creation endpoint or invitation UI, so
the migration creates no implicit relationships.

### Profile images

Schema v19 stores only `user_accounts.profile_updated_at`. Browser-normalized
JPEG files live in an app-owned `account-profiles` directory. The server bounds,
decodes, dimension-checks, atomically replaces, reconciles, and serves them only
to the owner, an administrator, or an actively linked viewer. Generated
monograms remain the no-file fallback. This mirrors the existing persona image
boundary and introduces no Go image-scaling or CGo dependency.

## Consequences

Positive:

- Passwords and session tokens have one backend contract and never enter
  installer persistence, local storage, settings, diagnostics, or exports.
- Loopback users can opt into password protection without configuring LAN; LAN
  remains exact-address, trusted-certificate, and explicit.
- The shell can show Self and future linked accounts now without granting an
  impersonation or motion-control primitive.
- Per-session selection is ready for concurrent logins and does not leak from
  one browser session to another.
- Profile images reuse a tested, bounded pure-Go/browser split and keep the
  binary free of a scaling dependency.

Negative and open:

- Application data remains shared; accounts are not tenant boundaries.
- Link rows are schema preparation only until a separately reviewed invitation
  and grant protocol exists.
- Administrator password reset is intentionally powerful and needs a future
  durable audit surface before internet-facing scenarios can be considered.
- App-owned image files add another filesystem/SQLite consistency boundary,
  mitigated by atomic replacement and startup reconciliation.
- Automatic certificate trust, recovery, MFA/passkeys, and real mobile-browser
  acceptance remain open.

## Rejected Alternatives

- **Collect the administrator password in Inno Setup.** Rejected because it
  duplicates UI/validation and risks password persistence outside the app.
- **Keep browser Basic authentication beside the React login.** Rejected
  because cached native credentials can silently reauthenticate after logout
  and create two user-visible session models.
- **Treat the selected linked account as the authenticated principal.**
  Rejected because a shell selector must not become impersonation or inherit a
  role.
- **Reuse the controller lease as account identity.** Rejected because the
  lease is tab command ownership, not durable authentication.
- **Put profile images in SQLite blobs or scale them in Go.** Rejected because
  large binary bytes do not belong in the relational store and server scaling
  would add avoidable runtime weight.

## Follow-up Gate

Any feature that creates account links or lets a linked context affect motion
must define invitation/consent, grant scope, revocation latency, attribution,
controller interaction, Stop authority, audit evidence, and remote threat model
before implementation. Internet exposure or per-user data partitioning still
requires its own ADR.
