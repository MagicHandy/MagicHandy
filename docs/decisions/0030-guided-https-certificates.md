# ADR 0030: Guided access scope and application-owned certificates

Date: 2026-09-20

Status: Accepted

## Context

The network policy already enforces explicit HTTPS origins, accounts, proxy peers,
and fresh administrator confirmation. Requiring users to independently find a LAN
address, provision PEM files, arrange renewal, and translate ports makes otherwise
secure setup error-prone. Public IP certificates are available from Let's Encrypt
using its `shortlived` profile; Go's x/crypto ACME client does not expose profiles.

## Decision

The initial Access step and Access settings share three choices: Local only,
LAN + local, and Public. Remote choices require an administrator. Manual PEM and
existing reverse-proxy deployments remain available under advanced options.
Network configuration continues to apply only after restart, in the existing
password-confirmed SQLite transaction. There is no automatic firewall, router,
DNS, account-permission, or client trust-store mutation.

`internal/netaccess.Automation` owns preparation and renewal. Preparing a public
certificate temporarily binds the selected local address for TLS-ALPN validation
only. Ordinary HTTPS/application requests are refused on that socket, connections
are capped at 16 with five-second timeouts, and the listener closes on completion,
failure, cancellation or process shutdown. The existing local app remains usable.
Preparation has a four-minute deadline, one job at a time, and retry throttling.
It cannot save or enable network access. Saving separately verifies a usable
certificate and fresh account authority.

Public issuance uses external TCP 443, forwarded to the selected local port where
needed. It requires explicit acceptance of the exact CA terms URL, verified again
against the ACME directory before account registration. Certificates use one
fixed public CA, the `shortlived` profile, and either one IP or one DNS name.
The UI identifies publication in certificate transparency logs. It does not
request wildcard names, expose worker ports or dynamically issue on handshakes.

The dependency is `github.com/mholt/acmez/v3` v3.1.6, Apache-2.0, pure Go. It supplies
ACME order/challenge handling and profile/IP support. The application provides
the exact-identifier TLS solver, lifecycle, persistence and renewal scheduling.
This avoids importing a full web server or DNS-provider framework. x/net and
x/text support the client; no browser dependency or CGo runtime is introduced.

LAN preparation creates a per-installation ECDSA root and an IP leaf certificate.
Only the public root is downloadable by an administrator. Trust enrollment remains
an explicit action on each client. LAN policies require private/loopback client
addresses even when an accidental router forwarding rule reaches the listener.

Account keys, local signing keys and certificate/key pairs live in `https-private`
under the data directory, outside SQLite settings/exports. Unix permissions are
0700/0600; Windows protected ACLs allow only the service account, SYSTEM and
administrators. One atomic PEM replacement keeps certificate and key together.
Private contents and filesystem/provider error bodies never enter API reports.

Renewal runs independently of incoming requests at half the actual certificate
lifetime (80 hours for a 160-hour certificate), checked every 15 minutes. Failures
retry with exponential backoff up to an hour while retaining the last valid pair.
Expired certificates are never served. A previously prepared expired identity can
renew on startup after a long shutdown; a missing/invalid cache requires local
recovery. The app must run and its external validation port must remain reachable.

External detection contacts fixed ipify and Let's Encrypt endpoints only after
choosing automatic Public setup or requesting detection. Calls have time/size
limits and are coalesced/cached for a minute. Detection cannot prove inbound
reachability; VPN, CGNAT, changing addresses, client trust and NAT loopback remain
distinct conditions, explained in the setup checklist.

## Validation and limits

Local integration tests cover an ACME directory/account/order, real CSR signing,
IP certificate profiles, and a real TLS-ALPN handshake with reverse-IP SNI. They
also cover trust export, hostname checks, failed preparation, no save before a
valid certificate, fresh-password authorization, changed terms, bounded discovery,
LAN peer restrictions and teardown. UI tests exercise the setup decision tree,
exact port instructions, consent and failure behavior.

Tests do not enroll this machine with a public CA, change its firewall/router, or
install a root certificate. A deployment still needs a real external client test.

References: [Let's Encrypt IP availability](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability/),
[profiles](https://letsencrypt.org/docs/profiles/),
[ACMEz](https://github.com/mholt/acmez),
[TLS-ALPN validation](https://letsencrypt.org/docs/challenge-types/).
