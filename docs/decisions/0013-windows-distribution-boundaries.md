# ADR 0013: Windows Distribution Boundaries

## Status

Accepted, with the public unsigned-artifact clause superseded by
[ADR 0014](0014-public-windows-signing-gate.md). ADR 0014 now permits a narrow
reviewed unsigned-alpha exception after Microsoft's completed false-positive
determination; trusted Authenticode remains the production target.

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

1. **Unsigned setup publication is an explicit reviewed exception.**
   Development and pull-request artifacts are labeled unsigned and remain
   private to CI. ADR 0014 defines the completed Microsoft review and dedicated
   `ReviewedUnsignedPublic` gate for public alpha setup. Production signing
   still requires publisher identity, protected certificate access,
   timestamping, rotation/revocation, and release-workflow verification; a
   placeholder or personal certificate is not production signing.
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
6. **Uninstall makes data disposition explicit.** Program files, shortcuts, and
   Add/Remove Programs metadata are always removed. Interactive uninstall asks
   whether to purge `%APPDATA%\MagicHandy`, recommends purge for a clean
   reinstall, and permits cancellation. Silent uninstall purges by default;
   `/KEEPUSERDATA` is the explicit retention override. Purge is constrained to
   the packaged app's default data root and never follows external Ollama,
   media, funscript, source-checkout, or custom `-data-dir` paths.

## Consequences

Positive:

- pull-request packaging can be tested without pretending to be a release;
- optional multi-gigabyte runtimes do not bloat or contaminate the pure-Go core;
- upgrades cannot replace an active hardware controller silently;
- update discovery is visible and opt-out without becoming an execution path;
- browser and source installs share one setup and settings implementation; and
- packaging does not expand the default network trust boundary.

Negative:

- portable alpha artifacts remain unsigned and may produce Windows reputation
  warnings until the signing service is provisioned;
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

- the pull-request packaging workflow has read-only repository permissions and
  no release publication step; a separate tag-only workflow can publish after
  validating SemVer, exact source provenance, the current `main` tip, and all
  gates;
- release checks use the canonical repository, stable semantic tags, bounded
  responses, conditional requests, and no user credentials;
- artifact manifests identify the exact source commit and GPL-3.0-only license;
- setup custom/default install, upgrade, shortcut, retention, purge, and clean
  reinstall tests run in CI;
- optional runtime files are absent from the core payload while their helper
  scripts are present; and
- the shipped defaults continue to validate loopback addresses only.
