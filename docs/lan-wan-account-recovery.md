# Saved account recovery and password changes

This checkpoint advances LAN-03, LAN-16 and LAN-18 on the full
[LAN/WAN checklist](lan-wan-control-checklist.md). The primary deployment
remains self-hosted direct HTTPS or a trusted reverse proxy. Saved recovery
codes are available to any enabled account; they do not grant device control.

## User workflow

Open **Settings → Access → Security → Recovery codes**, confirm the current password,
and generate a set. Save the eight codes in a private password manager. The
application displays them only in this successful issuance response; reopening
the panel shows the remaining count, never the codes. Generating another set
immediately invalidates the old set. Removing a set also requires the current
password. These operations do not need controller ownership.

If the password is lost, choose the small **Forgot password?** action on the
sign-in screen to reveal the recovery-code form.
Supply the installation's username, a previously saved code and a confirmed
replacement password. Successful recovery changes the password and invalidates
**every existing login and every code in that set**. Sign in separately with
the replacement password and generate a new set. The application does not log
in, claim control or resume motion automatically.

A password change, administrator password reset or account disabling also
invalidates the whole saved set. A code cannot re-enable a disabled account.
Codes for one account cannot recover another. A stolen code permits password
recovery, so protect it like a password; recovery codes are not a second factor.

If a recovery response times out, its outcome is unknown. Try the new password
at sign-in before retrying recovery. For a timed-out issuance, refresh the
count, then generate another set if its codes were not received. The UI never
replays either mutation automatically. Closing the panel, going offline or
changing login clears displayed codes. Obsolete responses cannot restore them.
A refresh that sees another browser's replacement also hides the older set.

## Backend contract

| Route | Authority and result |
| --- | --- |
| `GET /api/auth/recovery-codes` | Current live login; returns only count, limit and issuance time for its own account |
| `POST /api/auth/recovery-codes` | Current live login plus current password; atomically replaces all codes and returns the new set once |
| `DELETE /api/auth/recovery-codes` | Same password and login proof; removes the entire set |
| `POST /api/auth/recover` | Username plus saved code; atomically resets the password and retires credentials, returning only an acknowledgement |

Codes contain 128 random bits, formatted as eight groups of four hexadecimal
characters. Case, hyphens and ASCII whitespace are normalized, with an 80-byte
input bound. Only a domain-separated SHA-256 digest is stored. A code is checked
with its account identity; plaintext codes and digests are absent from status,
audit and diagnostic responses. Issued secrets are never placed in URLs,
browser persistent storage, logs or the audit history.

Schema **24** adds one indexed, account-owned table. Database constraints reject
null/malformed digests, a ninth code and updates to stored rows. Account deletion
cascades to recovery codes. Migration and reapplication preserve existing
passwords, logins, lifetimes and previously issued codes. The migration is
forward-only; preserve a private backup before upgrading and do not run an
older binary against the upgraded database.

Login and recovery share the existing per-peer/per-username guessing limiter.
Recovery-code management and own-password confirmation also use that limiter.
The public recovery route uses the reserved login request budget (eight global,
two per peer), requires same-origin JSON, limits its body to 8 KiB and gives
uploads five seconds. Credential operations have a ten-second request budget.
Successful management responses are no-store, at most 32 KiB and use the
existing cancelable socket writer. Unknown accounts, disabled accounts and
invalid/consumed codes have the same public credential error. No account is
locked or changed merely because an invalid code was presented.

The implementation follows the random, securely stored, single-use credential,
session-invalidation and separate-sign-in principles in the
[OWASP password recovery guidance](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html).
These are pre-saved offline codes, not emailed reset links; they remain valid
until used, replaced or invalidated by a credential/account change.

## Races, retirement and audit

Password verification takes place outside SQLite's serialized writer. Its
private proof is rechecked against the enabled account's exact current hash in
the transaction that creates the login. Verification and login issuance can
no longer straddle a password reset. The legacy authentication helper also
conditions its final login update on that proof.

Own-password changes and code management recheck both the live session and
current-password proof at the mutation. Administrator resets recheck the
administrator's session and role inside that transaction. Recovery rechecks
the saved code after password hashing, so concurrent uses or replacement
cannot revive a spent set. Password replacement, all-session revocation,
all-code invalidation and the success audit record commit or roll back together.

After commit, the HTTP edge cancels affected active session work and retires
the controller/device gateway through the existing shared Stop lifecycle.
The request may finish a bounded acknowledgement without retaining authority.
That acknowledgement does not assert physical stop confirmation. Direct store
changes retain the existing watchdog fallback.

Audit events record code replacement/removal, successful recovery and rejected
or throttled credential checks. They contain reviewed identifiers/counts only;
submitted usernames, passwords, codes, bearer cookies and private session keys
are not recorded. Failed success-audit insertion rolls back the credential
mutation. Existing retention and runtime-drop reporting still apply.

## Evidence and remaining work

Regressions cover password-reset/login races, a password change admitted before
recovery, administrator revocation during a queued reset, concurrent use of
different codes in one set, rollback on audit failure, current-password and
account boundaries, code replacement/removal, migration and schema bounds.
HTTP tests cover JSON/origin/body rejection, shared throttling, audit privacy,
five-second unfinished uploads and immediate stream retirement over HTTP/1
and HTTP/2. Frontend tests cover confirmation, explicit sign-in after recovery,
one-time display, offline/unmount cancellation and ignored late responses.

Invitations and enrollment, stronger WAN authentication/step-up policy, recovery
when every code and password is lost, and the external/mobile/device acceptance
matrix remain required work. A local network-mode override only repairs network
configuration; it does not bypass account authentication. No numbered checklist
item is closed by this component checkpoint. Build and review evidence is
recorded in the [implementation log](lan-wan-implementation.md) and
[scorecard](goal-scorecard.md).

The September 19 settings follow-up places recovery under personal Security,
with separate bookmarked pages for Profile, Sessions, account administration,
remote access and access history. Pages mount on demand and discard credentials
on navigation or login changes. The [UI contract](ui-design.md) records the
reference patterns and loading behavior. This organization does not add or
change backend account roles, control permissions or network exposure.
