# ADR 0013: Windows Distribution Boundaries

## Status

Accepted for the initial unsigned packaging workflow.

## Context

ADR 0011 selects a thin Inno Setup shell and an app-owned setup flow. Shipping
that shell also forces explicit decisions about signing, updates, optional
runtime bundling, browser presentation, and network exposure. Leaving those
implicit would let a packaging convenience silently expand the app's trust or
attack boundary.

MagicHandy controls physical hardware, stores private credentials, and serves a
controller-authoritative local UI. An installer must not weaken those
properties merely to imitate a conventional desktop application.

## Decision

1. **Unsigned until a signing process exists.** Development and pull-request
   artifacts are labeled unsigned. Production signing requires a publisher
   identity, protected certificate access, timestamping, rotation/revocation,
   and verification in the release workflow. A placeholder or personal
   certificate is not treated as production signing.
2. **Discovery without silent auto-update.** Versioned builds may perform a
   cached, unauthenticated read of the canonical GitHub latest-stable-release
   endpoint and notify the user when its semantic version is newer. The user
   can disable automatic checks. The checker neither downloads nor executes an
   artifact. The initial package supports explicit over-install upgrades, and
   source checkouts retain the fast-forward-only `update.ps1` path. An in-app
   updater is deferred until signed metadata, rollback behavior, active-motion
   shutdown, database compatibility, and interrupted-update recovery are
   designed and tested.
3. **Optional runtimes are not bundled into the core payload.** The release
   ships pinned helper scripts and metadata. Managed llama.cpp, Parakeet, local
   TTS Python environments, CUDA, and models are separate explicit actions with
   their own size/license/verification information. Prebuilt llama.cpp bundles
   may be added as separately checksummed setup assets later.
4. **Default-browser presentation remains acceptable.** The EXE starts the
   loopback server and opens `#/setup`. A WebView2 shell may later improve
   presentation, but it must remain a view over the same server and must not
   duplicate settings, setup, controller, or motion logic.
5. **Loopback only.** Packaged defaults bind to `127.0.0.1`. Documentation says
   not to port-forward the app. LAN/mobile access is out of scope until there is
   authenticated HTTPS, certificate lifecycle, origin policy, and an explicit
   multi-client/controller threat model.
6. **User data survives uninstall.** Program files and shortcuts are removed;
   `%APPDATA%\MagicHandy` is retained and disclosed to interactive users. A
   future purge action must be separate, explicit, and credential-aware.

## Consequences

Positive:

- pull-request packaging can be tested without pretending to be a release;
- optional multi-gigabyte runtimes do not bloat or contaminate the pure-Go core;
- upgrades cannot replace an active hardware controller silently;
- update discovery is visible and opt-out without becoming an execution path;
- browser and source installs share one setup and settings implementation; and
- packaging does not expand the default network trust boundary.

Negative:

- unsigned artifacts produce Windows reputation warnings;
- packaged users perform explicit over-install upgrades;
- managed llama.cpp currently requires compiler tooling unless the user chooses
  existing Ollama, an external server, or no chat setup; and
- localhost remains the supported UI origin for microphone and Web Bluetooth.

## Revisit Triggers

- a project-owned code-signing certificate and protected release process;
- a signed update manifest with rollback and motion-stop acceptance tests;
- published prebuilt llama.cpp CPU/CUDA bundles with licenses and checksums;
- a WebView2 shell that adds no second application model; or
- an approved LAN/mobile HTTPS architecture.

## Verification

- the packaging workflow has read-only repository permissions and no release
  publication step;
- release checks use the canonical repository, stable semantic tags, bounded
  responses, conditional requests, and no user credentials;
- artifact manifests identify the exact source commit and GPL-3.0-only license;
- setup silent-install and uninstall smoke tests run in CI;
- optional runtime files are absent from the core payload while their helper
  scripts are present; and
- the shipped defaults continue to validate loopback addresses only.
