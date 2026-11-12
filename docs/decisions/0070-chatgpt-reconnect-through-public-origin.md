# ADR 0070: Initiate ChatGPT reconnect through the configured public origin

## Status

Accepted for alpha.85 integration, 2026-11-12. Supersedes the unreleased
socket-derived local sign-in link in source PR #368.

## Context

The owner uses MagicHandy's public HTTPS address on the MagicHandy PC.
Hostname checks cannot establish browser locality, and NAT can make that
PC's actual peer address look remote. The initial fix offered a direct
socket-IP link. Review found that the network policy rejects that Host when
the public origin is configured, and the existing certificate need not cover
the IP. The installed account, certificate and network configuration were not
changed; certificate validation was never bypassed.

The account API already requires an authenticated administrator with
configure-host permission and current controller admission. Its request also
passes the configured Host/TLS or trusted-proxy policy and same-origin browser
protection. Initiating OAuth creates a pending attempt, not a connected account.

## Decision

- Permit that authorized administrator to initiate or reconnect through the
  configured origin, without guessing the browser's hostname, IP or location.
- Keep existing authentication, administrator, controller, command-admission,
  browser-origin, TLS and trusted-proxy gates. Do not add an IP-origin exception
  to the network policy or alter the configured certificate.
- Keep OAuth's server-side random state, nonce, PKCE, protected token storage
  and random **host-loopback-only** callback listener unchanged. An unrelated
  remote browser cannot finish that callback on the MagicHandy PC.
- Explain that sign-in must complete in a browser on the computer running
  MagicHandy. Account controls remain available during model discovery/checks;
  generation/signature fences discard stale results after an account change.

## Consequences and validation

A remote controlling administrator may create a pending attempt, cancel it,
or transfer its authorization URL to the host browser. Initiation is not proof
that the browser is local and does not bypass callback validation. Completing
the flow on another PC reaches the wrong loopback listener and cannot connect
the host account. The existing configured public URL and certificate work for
the host browser without a second local app origin.

Full-handler tests use the configured public HTTPS origin, authentication,
administrator and controller fences, and a local OAuth issuer fixture. They
reject signed-out, operator, read-only, cross-origin, unconfigured-Host,
plaintext and untrusted-forwarded requests before reaching the issuer. The
authorized controller reaches it even when NAT makes its peer look remote.
Existing OAuth tests validate the loopback redirect, state, nonce, PKCE and
cancellation. No real account was reauthorized in release review.
