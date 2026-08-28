# ADR 0017: Authenticated, Explicit-Address LAN HTTPS

## Status

Accepted. The backend foundation and account GUI are implemented; automatic
certificate authority management and real mobile-browser acceptance remain
open follow-up work. [ADR 0018](0018-account-gui-and-control-context.md)
records the permanent login, installer, profile-image, and linked-session UI.

## Context

MagicHandy historically served one trusted local operator on
`http://127.0.0.1`. That is a secure browser context by special case and keeps
the API away from the LAN. It does not support a phone or another computer:
browser microphone and Web Bluetooth features require a secure context, while
exposing the existing device-control API without identity would let any LAN
client read private state or command hardware. A controller lease identifies a
browser tab; it is deliberately not authentication.

Earlier planning therefore rejected every non-loopback `-addr` until HTTPS,
certificate lifecycle, authentication, origin policy, and Stop authority were
designed together. The user-account backend is also needed before a later GUI
can offer login and account administration without inventing persistence or
credential rules in React.

The implementation follows the Go standard library's TLS server and hostname
verification contracts, uses Argon2id as recommended by the Go cryptography
package and OWASP, and treats cookie `Secure`, `HttpOnly`, and `SameSite` flags
as defense in depth rather than a substitute for server-side authorization:

- [Go `net/http` TLS server documentation](https://pkg.go.dev/net/http#Server.ListenAndServeTLS)
- [Go `x509.Certificate.VerifyHostname`](https://pkg.go.dev/crypto/x509#Certificate.VerifyHostname)
- [Go Argon2id package](https://pkg.go.dev/golang.org/x/crypto/argon2)
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)

## Decision

### Exposure modes

MagicHandy has two explicit modes:

1. **Local default:** loopback HTTP, no account required. Existing installs,
   launchers, setup, and local secure-context behavior remain unchanged.
2. **Authenticated LAN HTTPS:** an exact private or link-local IP address,
   operator-supplied certificate chain and private key, and at least one enabled
   account are all mandatory. TLS implies authentication on a non-loopback
   bind. `-require-auth` can opt a loopback listener into the same account wall.

Wildcard binds, arbitrary hostnames, globally routable addresses, plain HTTP
on a LAN address, and a LAN listener with no enabled account fail before the
server starts. MagicHandy is not an internet-facing service and this decision
does not authorize port forwarding, reverse-proxy exposure, or cloud hosting.

The certificate is loaded before serving, must currently be valid, and its leaf
must contain the exact listen IP (or `localhost`) in a Subject Alternative Name
accepted by Go's hostname verifier. TLS 1.2 is the explicit minimum; modern Go
defaults choose safe cipher suites and negotiate TLS 1.3 where supported. The
app does not provide an insecure certificate-verification bypass.

Certificate issuance and client trust remain operator-owned in this slice. A
certificate from an organization/private CA is supported once that CA is
trusted by each client. Automatically generating and installing a local CA,
rotating leaf certificates, and an Android certificate helper are deferred
until a real mobile acceptance pass can define a supportable lifecycle.

### Identity and persistence

Schema v18 adds two tables to the one process-owned SQLite database:

- `user_accounts`: opaque ID, case-insensitive unique username key, public
  username, `admin` or `operator` role, Argon2id password hash, disabled state,
  and bounded timestamps;
- `user_sessions`: only a SHA-256 digest of a 256-bit random token, account
  foreign key, last-seen timestamp, and absolute expiration.

Schema v19 adds bounded profile-image metadata, an optional per-session control
context, and directional account-link rows. Those additions do not partition
shared application data or grant controller authority; ADR 0018 defines their
scope.

Passwords use Argon2id PHC strings with unique 16-byte salts and the OWASP
minimum profile of 19 MiB memory, two passes, and one lane. Inputs require at
least 8 Unicode characters and are capped at 1024 UTF-8 bytes; the UI continues
to recommend a longer unique passphrase. Hash parameters are self-describing for
future upgrades but are bounded when parsed so a damaged or hostile database
cannot demand unbounded memory.
Missing, disabled, and wrong-password cases perform the same password-hash work
and return one generic error.

Sessions have a 12-hour absolute lifetime, a 30-minute idle limit, a maximum of
20 per account, and at-most-once-per-minute last-seen writes. Password changes
and account disabling revoke all of that account's sessions transactionally.
The last enabled administrator cannot be disabled.

### HTTP boundary

Routes provide one-time loopback bootstrap, login/logout, current-password
change, session status/control context, profile-image operations, and
administrator-only list/create/password/disable operations. The React app
consumes those contracts without receiving the opaque session token.
Bootstrap creates a temporary session so protected setup can finish after the
live wall closes. The backend revokes the current session and clears its cookie
on the first transition to completed setup, forcing the first ordinary login
without exposing the token to React or depending on page-local state.

An authenticated session uses a host-only, path-rooted, `HttpOnly`,
`SameSite=Strict` cookie; HTTPS uses the `__Host-` prefix and `Secure`. Login
attempts consume independent bounded per-address and per-username token buckets.
One cancelable global admission slot bounds concurrent Argon2id memory even if
many requests arrive together.
Authentication errors do not disclose whether a username exists or is disabled.
API responses are non-cacheable and HTTPS responses include HSTS plus narrow
frame, referrer, MIME-sniffing, and CSP protections.

The embedded SPA shell may load before authentication so React can render the
login boundary. Private APIs remain protected. JSON login is the only password
exchange and issues the opaque server-side session cookie; the transitional
HTTP Basic bridge is retired so logout cannot be defeated by a browser's
separate native credential cache.

Allowed browser origins remain exact scheme/host/port matches. In LAN mode the
Host must also match the configured listen IP. The native host-path picker
remains loopback-only even for an authenticated LAN administrator because it
opens UI and reveals paths on the server computer.

### Stop and controller authority

Accounts answer “who may enter the app.” The existing controller lease still
answers “which authenticated browser may command the device now.” They remain
separate layers.

`POST /api/motion/stop` deliberately remains callable when a session is missing
or has expired. Same-origin browser checks still precede it, but a direct LAN
client can only cause the fail-safe Stop operation, not start or retarget
motion. This preserves the invariant that a user who can see a stale/offline
UI is never locked out of Emergency Stop by authentication state.

## Consequences

Positive:

- LAN transport is fail-closed: TLS, identity, and exact origin arrive as one
  unit instead of weakening the old loopback guard in isolation.
- The local install path and its trusted-loopback behavior do not change.
- Account data uses the existing database lifecycle and serialized writer;
  there is no second store or frontend source of truth.
- Users can opt into loopback password protection during in-app setup or later
  in Settings without enabling LAN access.
- Passwords, raw session tokens, and TLS private-key contents never enter
  settings readback, logs, diagnostics, traces, or exports.
- Authentication cannot bypass the controller lease, shared motion engine,
  transport owner, or Stop fencing.

Negative and open:

- Operators must obtain and trust a certificate whose SAN matches the current
  LAN IP. DHCP address changes require a new certificate and launch address.
- There is no password recovery, MFA, email identity, account deletion, or
  per-user data partitioning. Linked-account invitations and grants are not
  implemented.
- Real phone/tablet secure-context behavior is not claimed until a trusted-CA
  run covers login, reconnect, microphone/Bluetooth capability, Stop, and
  certificate renewal.
- Adding `golang.org/x/crypto/argon2` increases the pure-Go dependency and core
  binary slightly; the affected binary budget is remeasured in the scorecard.
- An unauthenticated LAN peer can issue Stop as a denial of service. That is an
  accepted fail-safe trade-off; it cannot start or alter motion.

## Rejected Alternatives

- **Enable LAN HTTP and rely on the controller client ID.** Rejected: a client
  ID is self-asserted tab ownership, not authentication, and HTTP is not a
  secure browser context.
- **Bind `0.0.0.0` and trust the certificate.** Rejected: wildcard binding
  obscures the intended interface and breaks the exact Host/SAN policy.
- **Generate an untrusted self-signed leaf automatically.** Rejected: a warning
  click-through is not a dependable secure-context or mobile trust lifecycle.
- **Store bearer tokens in browser local storage.** Rejected: script-readable
  storage turns an XSS flaw into offline credential theft; host-only HttpOnly
  cookies keep the token outside JavaScript.
- **Use fast SHA-256 password hashes.** Rejected: fast general-purpose hashes
  make offline guessing cheap. SHA-256 is used only for already-random session
  token digests.
- **Make all API access account-scoped in one step.** Rejected for this slice:
  app settings, device ownership, chat, personas, and libraries are still one
  shared installation. Per-user data partitioning is a separate multi-tenant
  product decision, not implied by having login identities.

## Follow-up Gate

Certificate automation requires its own review covering key storage, renewal,
removal, mobile trust, and installer behavior. Any linked control context that
affects motion must pass ADR 0018's invitation, consent, revocation, controller,
Stop, and audit gate. Internet exposure or per-user data partitioning requires
another ADR and threat model.
