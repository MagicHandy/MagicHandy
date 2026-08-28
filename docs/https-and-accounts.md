# HTTPS And Backend User Accounts

MagicHandy still starts as a local app at `http://127.0.0.1:49717`. HTTPS and
accounts are opt-in so existing launchers and single-computer installs do not
acquire a certificate or login requirement unexpectedly.

This document describes the backend foundation. There is intentionally no
account GUI yet. LAN mode uses the browser's native credential prompt as a
temporary bridge, and every account operation is also available through JSON
HTTP endpoints.

Architecture and threat-model decisions are in
[ADR 0017](decisions/0017-authenticated-lan-https.md). Internet exposure,
port forwarding, reverse-proxy hosting, automatic CA installation, and
per-user partitioning of settings/chat/library data are not supported.

## Modes and startup rules

| Mode | Listen address | TLS | Account wall |
| --- | --- | --- | --- |
| Default local | `127.0.0.1:49717` or another loopback address | Optional | Off unless `-require-auth` |
| LAN | One exact private/link-local IP, such as `192.168.1.20:49717` | Required | Always on; at least one enabled account must already exist |

MagicHandy refuses to start when any LAN invariant is missing. It also rejects
`0.0.0.0`, `[::]`, arbitrary hostnames, and globally routable addresses. Bind
the exact interface that clients will use.

HTTPS flags:

```text
-tls-cert PATH    PEM leaf certificate followed by any intermediates
-tls-key PATH     matching PEM private key
-require-auth     require login even on loopback
```

The certificate must be currently valid and cover the exact listen IP in an IP
Subject Alternative Name. A `localhost` listener needs a `localhost` DNS SAN.
The issuing CA must already be trusted by every browser/device; MagicHandy does
not weaken certificate verification or install trust roots. TLS 1.2 is the
minimum and TLS 1.3 is negotiated where supported.

## Create the first administrator

Bootstrap is a one-time API available only from the computer running
MagicHandy. Start the app on its ordinary loopback address and use a unique
passphrase of at least 12 bytes. The password is present only in PowerShell
process memory and the loopback request body; it is not placed in shell history,
an argument, a file, or MagicHandy logs.

```powershell
$securePassword = Read-Host 'New MagicHandy administrator password' -AsSecureString
$credential = [pscredential]::new('owner', $securePassword)
$bootstrapBody = @{
  username = $credential.UserName
  password = $credential.GetNetworkCredential().Password
} | ConvertTo-Json
try {
  Invoke-RestMethod `
    -Method Post `
    -Uri 'http://127.0.0.1:49717/api/auth/bootstrap' `
    -ContentType 'application/json' `
    -Body $bootstrapBody
} finally {
  $bootstrapBody = $null
  $credential = $null
  $securePassword = $null
}
```

Only an empty account database accepts bootstrap. The first account is an
administrator. Repeating the call returns `409 Conflict` and cannot replace the
existing administrator.

## Start authenticated LAN HTTPS

After provisioning an account and obtaining a trusted certificate for the
machine's exact private address:

```powershell
go run ./cmd/magichandy `
  -addr 192.168.1.20:49717 `
  -tls-cert C:\private\magichandy-chain.pem `
  -tls-key C:\private\magichandy-key.pem
```

Then open `https://192.168.1.20:49717/` from a device that trusts the issuing
CA. The browser's native Basic-authentication dialog is the no-GUI bridge for
this phase. On successful authentication the server creates a normal opaque
session cookie, so password hashing is not repeated for UI polling.
Closing the browser is the reliable way to clear its native Basic credential
cache; the logout API revokes the opaque session, but a browser that still
remembers Basic credentials may immediately authenticate again. The later
account GUI will use only the JSON login/session contract and remove that UX
limitation.

Do not add a router port-forward, expose this port through a tunnel, or bind it
to a public address. Account identities protect entry to one shared
installation; they do not make MagicHandy a multi-tenant internet service.

## Backend API

All request and response bodies are JSON. Account-management routes require an
authenticated administrator. Usernames are case-insensitively unique, contain
3–64 ASCII letters/numbers/period/underscore/hyphen, and preserve their chosen
display case. Roles are `admin` and `operator`; both can enter the shared app,
while only administrators can manage accounts.

| Method and route | Purpose | Authorization |
| --- | --- | --- |
| `GET /api/auth/status` | Initialization, login-required, and current-session state | Public |
| `POST /api/auth/bootstrap` | Create the first administrator | Loopback only; empty account table |
| `POST /api/auth/login` | Exchange username/password for an opaque session | Public, throttled |
| `POST /api/auth/logout` | Revoke the current session | Authenticated |
| `GET /api/accounts` | List non-secret account metadata | Administrator |
| `POST /api/accounts` | Create an administrator or operator | Administrator |
| `PUT /api/accounts/{id}/password` | Replace password and revoke all sessions | Administrator |
| `PUT /api/accounts/{id}/disabled` | Enable/disable; disabling revokes sessions | Administrator |

Create an operator after logging in (the example reuses the session cookie
returned by `/api/auth/login`):

```powershell
$session = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
$login = @{ username = 'owner'; password = 'your passphrase' } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri 'https://192.168.1.20:49717/api/auth/login' `
  -ContentType 'application/json' -Body $login -WebSession $session
$newUser = @{
  username = 'operator'
  password = 'another unique passphrase'
  role = 'operator'
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri 'https://192.168.1.20:49717/api/accounts' `
  -ContentType 'application/json' -Body $newUser -WebSession $session
```

Do not use `-SkipCertificateCheck`; install and trust the issuing CA instead.
Avoid literal passwords in reusable scripts or terminal history—the compact
example above illustrates endpoint shape, while interactive tools should use a
secure prompt as in bootstrap.

## Security behavior

- Passwords are stored only as salted Argon2id hashes. The API never returns a
  hash or distinguishes missing, disabled, and wrong-password accounts.
- Raw 256-bit session tokens exist only in host-only HttpOnly cookies; SQLite
  stores SHA-256 token digests. HTTPS cookies use `Secure`, `SameSite=Strict`,
  and the `__Host-` prefix.
- Sessions expire after 12 hours or 30 idle minutes, are capped at 20 per
  account, and are revoked by password changes or disabling.
- Independent per-address and per-username login buckets throttle automated
  guessing, and one cancelable hash slot bounds concurrent Argon2id memory.
  Throttled requests return a generic `429`.
- Browser requests require an exact same origin. LAN browser Host values must
  match the configured listen IP. Native host-path dialogs remain loopback-only.
- Account authentication does not replace the single active-controller lease.
  A logged-in second tab remains read-only until a stop-first takeover.
- Emergency Stop remains available if a login session expires. An
  unauthenticated LAN peer can stop motion but cannot start, retarget, change
  settings, read private state, or claim controller ownership.

## Deliberately not implemented in the GUI

This slice adds no React components, routes, fields, local-storage credentials,
or parallel account state. A later GUI should call the backend contracts above,
show expired-session state without masking Stop, and keep session tokens
HttpOnly. Password recovery, MFA, account deletion, profile data, and per-user
settings/history are separate product decisions.

## Acceptance still open

Automated tests cover certificate/address validation, TLS configuration,
schema migration, account lifecycle, hash parsing bounds, generic login
failure, throttling, session expiry/revocation, exact browser origins, roles,
and Stop without authentication. Before claiming phone/tablet support, complete
one real trusted-CA mobile run that records:

- exact IP SAN and trust installation method;
- browser and OS versions;
- native login → opaque session → reconnect behavior;
- microphone and/or Web Bluetooth secure-context capability;
- read-only second client, stop-first takeover, session-expiry Stop;
- certificate renewal and removal behavior.
