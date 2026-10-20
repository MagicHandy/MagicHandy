# ADR 0040: Local startup recovery and instance ownership

Status: Accepted for the first implementation, 2026-10-20.

## Context

Two Windows reports describe socket bind denial on the default local port.
Changing ports or resetting Host Network Service worked around the problem.
The error alone does not establish that HNS is responsible. Previously either
the app listener or the adjacent remote listener could abort startup, leaving
only a native socket error. Source launchers assumed the requested port was
the running port and could terminate a healthy app after a recovery.

## Decision

The executable owns port selection, instance identity and startup reports in a
small pure-Go `internal/startup` package, independent of the datastore, API and
motion. No dependency is added. HNS, WinNAT, firewall settings, other processes
and Windows port reservations are never changed automatically.

`-port-policy default` permits recovery only for the ordinary default-port,
literal-loopback HTTP launch with no explicit `-addr` or saved network policy.
Explicit addresses, nondefault saved ports and saved network policies remain
fixed. `-port-policy fixed` pins even the default launch. `-port-policy auto`
opts an explicit local HTTP preference into recovery; HTTPS, LAN and proxy
deployments stay fixed. Explicit network/TLS flags combined with `auto` are
rejected; a saved hosted policy stays fixed when a source launcher supplies
`auto` for an ordinary local install. Authentication is unchanged. Hosted
readiness records fail local discovery promptly with guidance to use the
configured URL; the local source launcher does not verify HTTPS/proxy readiness.

For automatic local startup, try a remembered working port for the same
configuration, otherwise the preference. A denied or occupied cached port can
fall back to the preference and then an OS-assigned port on the same loopback
address. Only permission denial and address-in-use errors permit fallback;
there is no unbounded scan or retry. The winning listener stays open through
serving. Bind both sockets before constructing the API and its workers. An
explicit remote port stays fixed and is reserved before selecting an automatic
main port. An automatic remote prefers the adjacent port and can independently
fall back. Both listeners remain required when remote is enabled; explicit
`-remote-port -1` disables it. A separate degraded-remote mode is deferred.

A per-profile OS file lock is acquired before opening the datastore or workers.
It is released after teardown, and by the OS on process death. The lock file is
never unlinked, avoiding replacement-inode races. Different profiles remain
independent; directory aliases reference the same locked file. Configuration-only
commands retain their existing behavior and do not start a runtime.

Publish the prepared actual app and remote URLs in private profile storage.
An existing local instance must answer a fresh random challenge using an HMAC
key held in that private record. The key itself is never sent. Identity checks
accept only literal loopback HTTP, disallow redirects and environment proxies,
and return no account, settings or device state. Windows ACLs restrict the
startup directory and generated files to the user, SYSTEM and administrators;
Unix permissions are 0700/0600. An acquired lock invalidates crash-stale
readiness. Successful duplicate launches do not open the datastore, recreate
workers, take control or resume motion. A mismatched fixed address is rejected.
Fixed duplicate launches also reject a recovered or unverifiable remote port,
even when the app port itself did not move.
Explicit authentication, TLS or hosted-network changes require closing the
existing instance; duplicate detection never silently discards those flags.
An explicit `-simulate-motion` duplicate is also rejected: opening an existing
instance must never silently substitute a live transport for requested simulation.

`-running-instance -data-dir ...` prints verified public JSON or `null` without
starting the app. The source launcher uses it and also checks the executable's
process and socket ownership. Default-port source launches explicitly opt into
automatic selection; custom ports remain fixed, with `-PortPolicy` available
on the shared launcher function. Browser opening, readiness and rebuild Stop
use the verified actual URL. A verification timeout leaves the process running
instead of killing an app whose state could be healthy or still initializing.
It identifies that state separately from a failed launch. Language/TTS callers
warn and preserve the process rather than retrying or claiming a failed restart.
Hosted listeners remain strict and are not rediscovered through an untrusted or
unverified HTTPS request. Older builds without this protocol cannot be reused
through it; the existing update/uninstall shutdown path remains applicable.
Language changes discover the actual port before querying the active profile;
language/TTS changes and rebuilds pass the running data directory to Stop.
Reconfiguration keeps the old profile for Stop if the new data directory differs.

## Retained reports

The private `startup` directory under the selected data directory contains
`current.json`, `last-failure.json` and `last-recovery.json`, each capped at
16 KiB and written atomically. Ordinary clean launches retain the previous
failure and recovery. Reports allowlist version/commit, platform, timestamps,
stage, policy, outcome, listener roles, port numbers and native error codes.
Raw error text, addresses, paths, environment, settings, chat and identity tokens
are excluded. No report is uploaded. A writable private temporary directory is
the fallback when profile storage cannot be written; the failure message names
the report so it remains accessible without the web server or database.
On Windows, atomic replacement retries sharing/access denial at most five times
over 150 ms before reporting failure. Private permissions remain required for
coordination storage: the app never publishes its identity token in a directory
that could not be secured. A permission failure advises a user-owned local data
directory and writes the credential-free report to private temporary storage.
`recovered` means a failed bind followed by automatic success, including a busy
cached port falling back to the preferred port; that event remains diagnosable.

The main app exposes reports to host administrators (or unprotected bootstrap
local sessions) without requiring controller ownership. Diagnostics provides a
localized status and report download. Normal authentication, host-private route
admission, no-store and nosniff protections apply. The identity record is never
included. Full Windows reservation inspection, a console retry menu and system
network repair are deferred; native code 10013 is evidence of denial, not an
automatic HNS diagnosis.

## Verification

Tests cover real occupied listeners, injected Windows 10013 denial, fixed-port
refusal, held winning sockets, profile lock ownership, stale records, unrelated
services, foreign hosts/peers, redirect rejection, duplicate launches before
datastore initialization, bounded reports, credential exclusion, retention,
temporary report fallback, role admission, frontend download and source launcher
discovery/reuse. Review uses isolated data and the simulator, never hardware.
