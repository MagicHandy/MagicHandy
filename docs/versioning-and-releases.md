# Versioning And Releases

MagicHandy uses [Semantic Versioning 2.0.0](https://semver.org/) for source
tags, release names, application version output, and package filenames. The
project is pre-1.0 and starts with `v0.1.0-alpha.1`.

## Canonical Version

- Git tags use `vMAJOR.MINOR.PATCH[-PRERELEASE]`, for example
  `v0.1.0-alpha.1`.
- The app and release manifest omit the tag's leading `v`, so
  `magichandy.exe -version` reports `magichandy 0.1.0-alpha.1 (<commit>)`.
- A release tag is immutable. Never move or replace a published tag; publish a
  new prerelease or patch instead.
- Release binaries come only from the exact tagged commit after it is merged
  into `main`. Local or dirty-tree builds are never publishable.

`main` between releases is development state. CI pull-request packages use a
clearly non-release `0.0.0-pr<N>` version and expire as workflow artifacts.

## Prerelease Stages

| Stage | Meaning | Compatibility expectation |
| --- | --- | --- |
| `alpha.N` | Feature-complete enough for hands-on testing, with known gaps and an explicitly documented Windows artifact policy | APIs, schema, setup, and UX can still change; data migrations must remain tested and non-destructive |
| `beta.N` | Intended feature set is substantially complete and release blockers are narrowed to defects and acceptance | Breaking changes require explicit release notes and migration coverage |
| `rc.N` | Candidate for the next non-prerelease build | Only release-blocking fixes should change behavior |
| no suffix | Published non-prerelease | Supported update source for the in-app stable release checker |

The prerelease counter starts at 1 and increments for every published build of
the same base version. A rejected or broken release is followed by a new tag.
Its tag is not moved or reused; unsafe assets may be withdrawn and the source
release notes remain as an incident notice.

Before `v1.0.0`, a minor version may contain deliberate breaking changes. Patch
versions remain reserved for compatible fixes. Beginning with `v1.0.0`, normal
SemVer compatibility applies: breaking changes increment major, compatible
features increment minor, and compatible fixes increment patch.

## Windows Version Mapping

Windows file metadata requires four numeric components while SemVer carries
prerelease text. Release packaging maps the fourth component as follows so
Windows metadata remains ordered within one base version:

- `alpha.N` -> `N` (`1` through `9999`)
- `beta.N` -> `10000 + N`
- `rc.N` -> `20000 + N`
- stable -> `65535`

The SemVer major, minor, and patch components must each fit Windows' unsigned
16-bit component limit (`0` through `65535`). Prerelease ordinals are limited
to `1` through `9999`.

The full SemVer value remains authoritative in Add/Remove Programs, filenames,
the release manifest, `SOURCE.txt`, and `magichandy.exe -version`.

## Release Artifacts

The reviewed unsigned Windows alpha.13 release contains exactly these
downloadable artifacts:

- `MagicHandy-<version>-windows-amd64-setup.exe`
- `MagicHandy-<version>-windows-amd64-portable.zip`
- `MagicHandy-<version>-windows-amd64-SHA256SUMS.txt`

The portable payload records the exact source commit, GPL-3.0-only license,
source URL, file sizes, and per-file SHA-256 hashes in
`release-manifest.json`. The checksum file covers the setup EXE and ZIP.
Alpha.12's portable-only GitHub Release was withdrawn; its immutable source tag
remains for provenance.

Pull-request workflows continue to retain setup only as a short-lived
`unsigned-ci` artifact and exercise its full lifecycle. The tag workflow uses
`ReviewedUnsignedPublic`, limited to alpha.8 through alpha.11, alpha.13, and alpha.14,
scans the exact public directory with Defender, verifies the setup/ZIP manifests
and two-entry outer checksum, exercises the exact setup lifecycle, and publishes
three explicit assets.

When trusted signing is provisioned, setup publication can resume under
`SignedPublic`, restoring the signed setup/ZIP/checksum set. That gate requires
valid, timestamped Authenticode from one explicitly pinned signer on the setup
EXE and every shipped payload EXE and rejects self-signed certificates. See
[ADR 0014](decisions/0014-public-windows-signing-gate.md).

## Release Gate

A release tag is created only after all of the following are true on the merged
`main` commit:

1. Go, race, lint, architecture, pure-Go, frontend, and installer suites pass.
2. The package workflow verifies the portable manifest, outer checksums, and
   unsigned installer lifecycle without release permission; the tag workflow
   repeats those checks against the exact reviewed public setup.
3. The installer acceptance test covers Program Files default placement,
   custom destination selection, optional desktop and Start Menu shortcuts,
   Add/Remove Programs metadata, active-process over-install, explicit data
   retention, clean uninstall, and fresh state after reinstall.
4. Release notes identify known limitations, the active artifact/signing
   policy, install/update instructions, and the exact source tag.
5. The release workflow verifies that the tag is valid SemVer and that its
   commit exactly matches the current `origin/main` tip before publishing a
   GitHub Release.

Tags containing a prerelease suffix produce a GitHub prerelease. Stable tags
produce a normal release. The in-app checker selects the highest published
semantic version compatible with the running channel. Stable builds ignore
prereleases; alpha can advance to alpha/beta/RC/stable, beta to
beta/RC/stable, and RC to RC/stable.

## Data And Uninstall Compatibility

Over-install upgrades preserve the app-owned data directory and migrate its
SQLite schema in place. Back up or export irreplaceable personas, chats, and
patterns before testing prereleases.

Interactive uninstall asks whether to remove `%APPDATA%\MagicHandy`; **Yes** is
the recommended clean-reinstall choice. Silent uninstall removes it by default.
Automation can pass `/KEEPUSERDATA` to preserve that tree or `/PURGEUSERDATA`
to state clean-removal intent explicitly. Purging includes the database,
diagnostics, imported managed models, managed llama.cpp runtimes, Parakeet, and
managed TTS modules. It never removes external Ollama models, user media or
funscript folders, source checkouts, or a custom data directory supplied outside
the packaged app's default path. Standalone managed-voice installation uses the
same default app-data root when no source-installer state or explicit data path
exists. The source bootstrap's separate
`%LOCALAPPDATA%\MagicHandy\install-state.json` records only source-install
automation choices; the packaged app does not read it, so it cannot change a
clean packaged reinstall.
