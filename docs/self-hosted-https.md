# Self-hosted HTTPS deployment

Implementation and acceptance are in progress on the LAN/WAN branch. This
guide describes the implemented direct-HTTPS and trusted-proxy boundaries;
it does not claim that the complete [LAN/WAN checklist](lan-wan-control-checklist.md)
has passed. Physical clients, external routing, fault/load limits, authentication
recovery and remaining command-delivery work still need acceptance.

## Provision locally first

Create the administrator on the app host at its ordinary loopback URL, using
Settings > Access. Keep the data directory private to the app's OS account.
Creating an account enables login on subsequent local launches as well.
Remote modes refuse startup without an enabled account. Existing operators
become observers: an administrator grants separate control permission, lasting
one minute to twelve hours. The UI offers common durations from 15 minutes.

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
| Direct HTTPS | Explicit IP and port; wildcard/public binds require this explicit mode | One advertised HTTPS origin and matching operator-provided certificate/key |
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
It does not establish client trust, install a CA or bypass a warning. Use an
operator-managed CA/ACME client for issuance and renewal. Restrict private-key
file access to the app's OS account.

New TLS handshakes check replacement files at most once per 30 seconds. A bad
replacement retains the last valid pair; an expired pair is refused. Access
settings show expiry, a 30-day renewal warning and a failed-reload indicator.
Existing connections keep their TLS session. Real-client trust enrollment,
private-key ACL verification and renewal acceptance remain checklist work.

If saved remote settings cannot start, use the same data directory locally:

```powershell
.\magichandy.exe -network-mode local -addr 127.0.0.1:49717
```

This overrides the saved boundary without deleting it or disabling accounts.
Repair it in Access settings, validate, save, then restart without the override.
An unavailable address/port or invalid certificate fails startup; there is no
silent insecure fallback. Stop the old listener before reusing its data
directory. A local recovery launch is not password recovery.

The one-click connection report contains version, network mode, authentication/
cookie state, lease/generation, Stop sequence, dispatch owner and certificate
expiry/reload status. It omits credentials, addresses, private paths, chat,
media and audio. Share it with the developer when reporting a problem. Client
RTT, jitter, stream age and full connection diagnosis remain unfinished; the
report cannot diagnose an unreachable server by itself.
