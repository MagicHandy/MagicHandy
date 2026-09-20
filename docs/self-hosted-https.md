# Self-hosted HTTPS deployment

This guide covers guided HTTPS setup, automatic certificates, and the existing
trusted-proxy boundary. Security/reliability release evidence is recorded in the
[LAN/WAN readiness review](lan-wan-release-readiness-2026-09-19.md). That evidence
does not prove reachability through a particular user's ISP, router, or firewall.

## Guided setup

Initial setup's **Access** step and **Settings > Access > Remote access** offer:

| Choice | Certificate and account | Required incoming port |
| --- | --- | --- |
| Local only | Loopback HTTP; account optional initially | None; no firewall exclusion or forwarding |
| LAN + local | Account plus automatic local certificate | Selected local TCP port, allowed on private networks in the host firewall; no router forwarding |
| Public | Account plus automatic public certificate for an IP or domain | Internet TCP **443**, forwarded to the displayed local IP/port, plus a host firewall exclusion for that local TCP port |

For example, if the selected interface is `192.168.1.20:49717`, Public setup needs
router TCP `443 → 192.168.1.20:49717` and a host firewall inbound exception for
MagicHandy on TCP `49717`. TCP 80 is unnecessary for this implementation. Keep
the forwarding rule available for renewal. LAN clients instead use
`https://192.168.1.20:49717`, with TCP 49717 allowed only on private networks.
Reserve the host's LAN address in DHCP so these rules continue to point to it.

Public setup detects the outgoing IPv4 using ipify and reads Let's Encrypt's
current terms. It contacts these services only after choosing automatic Public
setup or pressing Detect. You can replace the detected IP with your domain or
public IPv6 address. Detection is not an inbound port check: VPNs, CGNAT and ISP
port blocking can make the outgoing IP unsuitable. Domain DNS must point to the
correct public address; stale AAAA records can also prevent CA validation.
If needed, request a public address from your ISP or configure an existing HTTPS
reverse proxy under Advanced HTTPS options. A public URL used from inside the
same LAN may require router NAT loopback or split DNS; a successful outside
connection does not prove that the router supports this internal route.

Read and explicitly accept the CA's terms. The certificate's IP/domain becomes
public in certificate transparency logs. **Set up HTTPS and save** confirms the
administrator password, prepares the certificate, validates it, and saves for
restart. During public validation, a temporary listener serves only the CA's
TLS-ALPN challenge; the existing app stays at its current address. A failed
certificate request leaves saved access unchanged. Fix the reported routing or
service issue before retrying. Router and firewall changes remain user actions.

LAN setup creates a private per-installation CA and a matching local IP
certificate. Download the **local trust certificate** while the local setup page
is still available, and enroll it as a trusted root on each client, including the
host browser, before restarting. Transfer the public certificate over a trusted
channel. Its signing key is never downloaded. Do not bypass browser certificate
warnings. LAN scope rejects public client IPs at the application boundary too;
it does not intentionally create an Internet route.

Finish initial setup, restart, then sign in at the displayed HTTPS address.
Test using a second device; for Public access, test from a different network too.
Do not expose model or voice worker ports. Existing certificate files and trusted
reverse proxies remain advanced choices, with their existing validation rules.

## Provision locally first

Create the administrator on the app host at its ordinary loopback URL, using
Settings > Access. Keep the data directory private to the app's OS account.
Creating an account enables login on subsequent local launches as well.
Remote modes refuse startup without an enabled account. Existing operators
become observers: an administrator grants separate control permission, lasting
one minute to twelve hours, or **Permanent** until revoked or replaced. The UI
offers 15 minutes, one hour (the default), four hours, twelve hours and Permanent.
Schema 26 preserves existing timed deadlines; no account gains permanent control
during migration. The API requires an explicit `{"permanent":true}` choice,
exclusive of `duration_minutes`. Missing, zero or invalid durations are rejected.
A permanent grant has `expires_at: null` and an explicit permanent audit marker.
It does not extend login expiry or the foreground controller heartbeat lease.

Accounts share this installation's chat, personas, media and motion settings.
It is not a service for mutually untrusted tenants. Control permission covers
semantic motion, chat and synchronized playback. Installation, model/worker
management, device configuration and other host mutations remain
administrator-only. Selected linked profiles confer no control permission.
Revocation or replacement invalidates old ownership before the permission API
acknowledges the change; physical Stop is a separate result.

## Listener and public identity

Settings > Access > LAN and WAN access offers:

| Mode | Listener | Public identity / trust |
| --- | --- | --- |
| Local only | Explicit loopback IP and port | Local HTTP; existing account protection stays on |
| Direct HTTPS | Explicit IP and port; automatic setup selects one interface | One advertised HTTPS origin and a managed or operator-provided certificate/key |
| Trusted reverse proxy | One loopback or private IP and port | One external HTTPS origin; only configured immediate proxy peers accepted |

The public URL is an origin, such as `https://control.example.com:8443`.
Paths, credentials, fragments, queries, wildcards and scoped IPv6 are rejected.
International DNS names use ASCII/punycode. DNS, router configuration, firewall
rules and client certificate trust are operator responsibilities.

Validate checks syntax, the account prerequisite and direct-mode certificate
files. It does not establish second-device reachability or client trust. Save
requires the administrator's current password. Configuration is stored atomically
in the existing SQLite datastore and applies on restart. Active and saved
configuration are displayed separately; saving never changes the live socket.

CLI overrides remain available. For direct HTTPS:

```powershell
.\magichandy.exe -network-mode direct_https -addr 192.168.1.20:49717 `
  -public-url https://control.example.com:49717 `
  -tls-cert C:\private\control-chain.pem -tls-key C:\private\control-key.pem
```

The certificate covers the advertised hostname, which may differ from the bind
IP behind NAT. Remote Host and Origin must match that exact public identity.
The app does not publish inference or worker ports.

For a reverse proxy on the same host:

```powershell
.\magichandy.exe -network-mode trusted_proxy -addr 127.0.0.1:49717 `
  -public-url https://control.example.com -trusted-proxies 127.0.0.1
```

Trust accepts at most 16 IPs/CIDRs and rejects universal ranges. Use narrow peer
scopes. A private-network backend hop uses HTTP and needs a protected network;
loopback keeps that hop on one machine. Multi-proxy chains are not inferred.

## Reverse-proxy contract

The actual TCP peer must be trusted before the app accepts forwarding metadata:

- `Host` and `X-Forwarded-Host`: the configured public host/port;
- `X-Forwarded-Proto`: exactly `https`;
- `X-Forwarded-For`: exactly one client IP, replacing the incoming header;
- no alternative `Forwarded` or `X-Real-IP` header.

The app retains the actual peer and a separate forwarded-client identity.
Forwarded loopback addresses never acquire local bootstrap/file-dialog
privileges. Direct/local modes reject forwarding headers. Secure HttpOnly
host-only cookies and external-origin checks apply even on an HTTP backend hop.

An nginx location in an already configured HTTPS virtual host can implement
this contract. Adapt the literal hostname and upstream port together:

```nginx
location / {
    proxy_pass http://127.0.0.1:49717;
    proxy_http_version 1.1;
    proxy_set_header Host control.example.com;
    proxy_set_header X-Forwarded-Host control.example.com;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header Forwarded "";
    proxy_set_header X-Real-IP "";
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_cache off;
    proxy_next_upstream off;
    proxy_connect_timeout 5s;
    proxy_read_timeout 1h;
    proxy_send_timeout 30s;
    client_max_body_size 64m;
}
```

Disabling buffering preserves streaming; disabling retries avoids repeating
ambiguous mutations. Configure the HTTPS virtual host's certificate/key and TLS
policy separately. These directives follow the
[nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
and [HTTPS documentation](https://nginx.org/en/docs/http/configuring_https_servers.html).
This template has not yet been exercised against an installed nginx instance.
Proxy timeouts are transport limits, not motion leases: the app's watchdogs
still cancel abandoned work.

Emergency Stop remains available without a valid login. An unauthenticated peer
reaching the configured origin can interrupt motion, but cannot start it.
Restrict the perimeter to intended participants when this availability tradeoff
matters. Do not queue Stop behind authentication or ordinary upload/inference
limits. A severed network cannot deliver a remote request; the host watchdog and
local Stop are separate safeguards.

## Certificates and recovery

Direct HTTPS validates the key pair, leaf validity, advertised SAN, server-auth
usage and supplied intermediate signatures/order. It serves TLS 1.2 or newer.
It does not install trust in a client or bypass a warning. Automatic public
certificates are obtained from Let's Encrypt using its short-lived profile;
IP certificates last approximately 160 hours. Automatic certificates renew
at half their actual lifetime, checked every 15 minutes while the app is running.
Failures retain the last valid certificate and retry with exponential backoff up
to one hour. A host returning after expiry can renew a previously prepared
identity, but refuses ordinary TLS handshakes until a valid replacement exists.
Changed CA terms require explicit acceptance in setup; there is no silent consent.
Keep the address and incoming validation port working. If a dynamic public IP
changes, detect and save the new address; this is not a dynamic DNS service.

Managed keys live in the private `https-private` directory, with protected Windows
ACLs or Unix 0700/0600 permissions. They are excluded from settings, reports and
database exports. The local CA lasts five years and must be explicitly replaced
and re-enrolled before it expires. See [ADR 0030](decisions/0030-guided-https-certificates.md).
For manual PEM files, continue using an operator-managed CA/ACME client for
issuance and renewal, with private-key access restricted to the app's OS account.

New TLS handshakes check replacement files at most once per 30 seconds. A bad
replacement retains the last valid pair; an expired pair is refused. Access
settings show expiry, a 30-day warning for manual certificates, a lifetime-based
renewal indicator for managed certificates, and replacement/renewal failures.
Existing connections keep their TLS session. Real-client trust enrollment and
external routing still require deployment-specific checks.

If saved remote settings cannot start, use the same data directory locally:

```powershell
.\magichandy.exe -network-mode local -addr 127.0.0.1:49717
```

This overrides the saved boundary without deleting it or disabling accounts.
Repair it in Access settings, validate, save, then restart without the override.
An unavailable address/port or invalid certificate fails startup; there is no
silent insecure fallback. Stop the old listener before reusing its data
directory. A local recovery launch is not password recovery.

Save account recovery codes from **Settings → Access → Security → Recovery codes** before
losing the password. The sign-in screen's small **Forgot password?** action opens
the recovery-code form, which changes
the password and signs out every login for that account; sign in separately and
generate a new set afterward. Codes cannot be retrieved again or re-enable a
disabled account. See the [saved-code contract](lan-wan-account-recovery.md).
Recovery without a password or any previously saved code remains unfinished;
the network override above must not be treated as an authentication bypass.

The one-click connection report contains version, network mode, authentication/
cookie state, lease/generation, Stop sequence, dispatch owner and certificate
expiry/reload status. It omits credentials, addresses, private paths, chat,
media and audio. Share it with the developer when reporting a problem. Client
RTT, jitter, stream age and full connection diagnosis remain unfinished; the
report cannot diagnose an unreachable server by itself.
