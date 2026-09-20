# Login management and route admission

Implementation checkpoint for LAN-03, LAN-05 and LAN-16, 2026-09-13. This
advances the [full checklist](lan-wan-control-checklist.md); it does not close
WAN deployment, physical-device or comprehensive handler authorization acceptance.

## Account-owned logins

Access settings lists the current account's active logins, including the login
shared by this browser's tabs. Each row has an optional device name, coarse
browser/platform hints, creation and last-activity times, idle and absolute
expiry, and backend-derived controller/device-gateway indicators. Observers can
manage their own logins without claiming control. Administrators use the same
account boundary for this self-service collection.

Schema v22 adds an independent random 128-bit management ID and optional name
and client hints to each login. A management ID cannot authenticate a request.
Neither bearer cookies nor their stored digests enter the management response.
The migration backfills IDs in batches of 256, preserving existing login
lifetimes and any previously assigned management metadata. Client hints are
allowlisted browser/platform codes derived from at most 512 user-agent bytes;
raw user agents and client IPs are not retained for this feature. Names and
hints are informational and never authorize control.

| API | Result | Additional scope check |
| --- | --- | --- |
| `GET /api/auth/sessions` | At most 20 active logins, current ID and collection limit | Current live login; one consistent read snapshot |
| `PATCH /api/auth/sessions/{id}` | Rename a login; an empty name restores its default display label | Current live login and same-account target, checked in the write transaction |
| `DELETE /api/auth/sessions/{id}` | Revoke one login, including the caller | Same transaction-level account and lifetime checks |
| `DELETE /api/auth/sessions` | Revoke the account's other logins, preserving the caller | Live caller rechecked in the write transaction |

Names are valid UTF-8, at most 80 Unicode code points, with no control characters.
Reads do not renew idle time. Requests have a ten-second operation budget;
encoded responses are capped at 32 KiB and use a cancelable five-second socket
write deadline. The existing 20-login retention cap now explicitly keeps the
newly issued token, even when multiple creation timestamps tie. An old policy
could immediately evict that token because its digest sorted below its peers.

After a successful durable revocation, affected active work loses its session
context immediately. Controller/gateway retirement uses the existing shared
Stop lifecycle. A bounded acknowledgement may finish after self-revocation;
this grants no new authority. Client cancellation and server shutdown remain
attached. Logout uses the same retirement path and clears the browser cookie.
HTTP password changes and [saved-code recovery](lan-wan-account-recovery.md)
now retire affected work immediately after commit. Disabling and external
store changes retain the watchdog fallback. Network acknowledgement is not proof that a
physical device has stopped.

The panel cancels obsolete reads, refreshes after changes and never replays a
mutation after a lost response. A lost self-revocation response triggers a fresh
authentication check. Reads run on entry, visibility return, explicit refresh
and action reconciliation; the panel adds no periodic poll. Offline/unmounted
requests are canceled. Account/session changes remount the panel to discard
the previous login's view. Rename and sign-out controls use ordinary backend
APIs and require no controller grant.

## Maintained admission inventory

The committed [route table](../internal/httpapi/testdata/route_admission.tsv)
classifies all **216** current ServeMux registrations, including saved-code
recovery, audit and caller-owned notice-preference endpoints. A source-inventory test
requires an entry for every registration and rejects stale, duplicate or
unrecognized registration shapes. The role test covers each method, every
implicit `HEAD` for a `GET` registration, and anonymous, observer, granted
operator, administrator and revoked-administrator callers.

| Admission policy | Registrations | Meaning |
| --- | ---: | --- |
| Public | 10 | Shell, health, authentication/recovery entry points, caller-owned notice choices and global Stop |
| Shared observation | 34 | Any enabled login may enter; installation data remains shared |
| Self-service / caller-scoped | 17 | Any enabled login may enter; the handler checks the affected identity/resource |
| Gateway maintenance | 4 | Login admission is independent of controller status; gateway ownership is checked separately |
| Semantic control | 30 | Administrator or operator with a current timed or permanent control grant |
| Host administration | 121 | Administrator admission; applicable controller/local-origin checks still apply |

The table verifies authentication and role **admission**, not successful
execution of each route. Origin checks, command tickets/generations, gateway
leases, ownership, payload redaction, apply-time cancellation and local-only
dialogs remain additional contracts. Existing and new behavioral tests cover
specific contracts; the table is not a substitute for their broader audit.
In particular, admission of a shared read does not prove all its fields are
appropriate for remote observation.

The [observer privacy follow-up](lan-wan-observer-privacy.md) adds explicit
response projections and restricts raw diagnostics/Labs to administrators.
The counts above include that follow-up; runtime/resource checks remain separate.

The audit fixed four mismatches:

- Exact `GET /api/setup` now requires host-administration access, matching its
  children. It includes host setup paths and details.
- Account subresource reads require administrator admission except the exact
  profile-image shape, whose handler checks visibility. Control-grant reads
  already had a handler administrator check; the middleware now agrees.
- Granted controllers can select the LLM motion mode through its narrow
  semantic endpoint, without gaining access to general settings writes.
- Granted controllers can undo pattern feedback as well as submit it. Both
  operations now pass request cancellation into the transaction so work waiting
  for the database cannot apply after its request has lost authority.

## Evidence and remaining scope

Regressions cover foreign-account IDs in both directions, management IDs that
cannot log in, hidden private credentials, expiry without idle renewal, queued
mutations after actor revocation, bulk revocation isolation, response-loss
reconciliation, offline return, timeouts and canceled feedback persistence.
Real loopback HTTP tests open motion SSE, then revoke the caller or log out:
the acknowledgement completes, the cookie clears, the old login stops
authenticating and the existing stream closes. A simulated peer gateway
revocation also preserves the caller's acknowledgement. Migration exercises
300 existing rows across the batch boundary and metadata-preserving reapplication.

The [implementation log](lan-wan-implementation.md) records final build and UI
evidence; [artifact budgets](goal-scorecard.md) include the added session panel.
Bounded audit history, invitations/consent, stronger WAN enrollment/recovery,
the full handler/UI capability matrix and external/mobile/device acceptance
remain required work. No numbered checklist acceptance is closed here.
