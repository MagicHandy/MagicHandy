# Windows Release Packaging

MagicHandy can build two Windows x64 artifacts from one staged payload:

- `MagicHandy-<version>-windows-amd64-setup.exe`
- `MagicHandy-<version>-windows-amd64-portable.zip`

The setup EXE is a thin Inno Setup shell with a native x64 loader and non-solid
`zip/9` payload compression. This avoids the 32-bit loader and opaque solid
ultra-LZMA stream used by the withdrawn alpha.6 package. Until trusted
Authenticode signing is provisioned, that unsigned EXE is **CI lifecycle
evidence only** and is not a GitHub Release asset; the packaging change reduces
heuristic risk but does not establish publisher identity. Public alphas
temporarily contain the portable ZIP and a checksum file covering that ZIP.
The portable archive contains the app, workers, optional-module helper scripts,
license, source notice, and release manifest. It does not bundle models,
Python, CUDA, llama.cpp, or Parakeet; those remain explicit setup choices.

Release versions and tag policy are defined in
[Versioning And Releases](versioning-and-releases.md). Local build and test
commands never publish. The tag-triggered workflow publishes only after its own
quality and install-lifecycle gates pass.

## Build Prerequisites

The release builder, unlike the installed app, needs:

- Go from `go.mod`;
- Node.js 24 and npm;
- Inno Setup 6 or 7; and
- Git metadata for clean-source provenance verification.

On this machine Inno Setup is normally discovered at:

```text
C:\Program Files\Inno Setup 7\ISCC.exe
```

## Build

From the repository root:

```powershell
$commit = git rev-parse HEAD
.\scripts\release\Build-WindowsRelease.ps1 `
  -Version 0.0.0-local `
  -Commit $commit
```

The script runs `npm ci` and the production frontend build, cross-builds the
core and three Go voice adapters with `CGO_ENABLED=0`, stages the payload,
creates the portable ZIP, optionally compiles a native-x64 unsigned Inno Setup
lifecycle candidate, and writes
`MagicHandy-<version>-windows-amd64-SHA256SUMS.txt` under `artifacts\`.

Useful build-only options:

| Option | Purpose |
| --- | --- |
| `-OutputRoot PATH` | Write artifacts outside the default `artifacts\` folder |
| `-ISCCPath PATH` | Select a specific Inno compiler |
| `-SkipFrontendBuild` | Reuse the existing canonical `web/dist`; local debugging only |
| `-SkipInstaller` | Build the public portable-only artifact set; required for unsigned public releases |
| `-KeepStaging` | Retain the temporary payload for inspection |
| `-AllowDirty` | Permit a local smoke build and mark its source state dirty/unverified; never publish it |

By default, packaging refuses a dirty worktree or a source tree whose Git
metadata cannot be verified. This prevents a binary built from local edits from
claiming that it corresponds exactly to `HEAD`.

## Payload Contract

The staged `MagicHandy` directory contains:

- `magichandy.exe`;
- `voice-parakeet-worker.exe`;
- `voice-openai-tts-worker.exe`;
- `voice-elevenlabs-worker.exe`;
- setup and TTS PowerShell/Python helpers under `scripts\`;
- the managed llama.cpp installer plus its pinned upstream MIT license under
  `scripts\`;
- the project documentation tree, so relative links in the packaged README stay
  usable offline;
- `LICENSE`, `README.md`, and `SOURCE.txt`; and
- `release-manifest.json` with version, exact commit, source URL, file sizes,
  and per-file SHA-256 hashes.

The manifest is generated after all payload files except the manifest itself
are staged. For an unsigned CI lifecycle package, the outer checksum file
covers the portable ZIP and setup EXE. For a public portable-only package, it
covers only the ZIP.

## Local Acceptance

Build with a disposable version, then verify the payload and isolated
current-user install lifecycle:

```powershell
$commit = (git rev-parse HEAD).Trim()
.\scripts\release\Test-WindowsRelease.ps1 `
  -Version 0.0.0-local `
  -Commit $commit `
  -ArtifactPolicy UnsignedCI `
  -ExerciseInstaller
```

The test expands the portable ZIP, verifies exact version/provenance and every
manifest/outer hash, installs to an isolated custom directory, verifies the
desktop and Start Menu choices plus Add/Remove Programs metadata, performs an
active-process over-install, confirms settings survive, and exercises explicit
data retention before cleaning up.

`-ExerciseDefaultInstall` additionally tests the real Program Files default and
`%APPDATA%\MagicHandy` purge/reinstall boundary. It requires an elevated clean
test host and refuses to run if an existing install, data directory, shortcut,
or uninstall entry would be touched. CI runs this form on a disposable Windows
runner.

The temporary public release shape is built and verified separately so a broad
artifact glob cannot accidentally publish the unsigned setup executable:

```powershell
$commit = (git rev-parse HEAD).Trim()
.\scripts\release\Build-WindowsRelease.ps1 `
  -Version 0.0.0-local `
  -Commit $commit `
  -OutputRoot artifacts\release `
  -SkipFrontendBuild `
  -SkipInstaller
.\scripts\release\Test-WindowsRelease.ps1 `
  -Version 0.0.0-local `
  -Commit $commit `
  -ArtifactsRoot artifacts\release `
  -ArtifactPolicy PortablePublic
```

`SignedPublic` is the fail-closed policy for restoring a setup release. It
requires valid, timestamped Authenticode on the setup executable and all four
payload executables, rejects self-signed certificates, and requires the
approved certificate's 40-character thumbprint through
`-ExpectedSignerThumbprint`. The current workflow does not select this policy
because no protected signing identity has been provisioned.

Interactive uninstall asks whether to remove `%APPDATA%\MagicHandy`, including
settings, history, imported managed models, managed runtimes, and voice modules.
Yes is the recommended clean-reinstall choice. Silent uninstall purges by
default. `/KEEPUSERDATA` preserves that tree and `/PURGEUSERDATA` states clean
removal explicitly. External Ollama models, media/funscript folders, source
checkouts, and custom `-data-dir` paths are outside this boundary. Standalone
managed-voice setup falls back to this same app-data root. Source bootstrap
metadata under `%LOCALAPPDATA%` is separate, is not read by the packaged app,
and is not removed with a package.

## CI Workflow

`.github/workflows/package-windows.yml` runs with read-only repository access on
relevant pull requests and manual dispatch. It:

1. builds unsigned CI-only artifacts;
2. verifies the setup loader and every payload executable are x64, then checks
   portable payload provenance and every manifest hash;
3. verifies the outer checksum file;
4. verifies custom and Program Files installs, shortcuts, ARP metadata,
   over-install, explicit retention, clean purge, and fresh reinstall state;
5. uploads a visibly named, short-lived `unsigned-ci` workflow artifact for
   review.

It has no release trigger and never invokes a GitHub Release API. Artifact
retention is seven days.

`.github/workflows/release-windows.yml` runs only for a supported SemVer tag.
It requires that the exact tagged commit matches the current `origin/main` tip,
that the checkout is clean, and that matching release notes exist. It reruns
Go, race, lint, pure-Go, frontend, installer, package, and full Windows
lifecycle gates. It then builds and independently verifies a dedicated public
portable-only directory and creates the GitHub Release from explicit ZIP and
checksum paths in that directory. The unsigned setup candidate remains a
seven-day workflow artifact and cannot enter the release through a wildcard.
Prerelease tags are marked as GitHub prereleases.

## Release Boundaries

- Public setup executables are blocked until a real certificate, protected
  signing identity, timestamp service, and revocation process exist. The
  temporary portable alpha remains visibly unsigned; do not bypass Defender or
  SmartScreen if it is classified as unsafe.
- Versioned builds can discover the latest stable GitHub Release and notify the
  user. The check is cached, can be set to manual-only, and never downloads or
  runs an installer. See [Release Checks And Update Handoff](update-checks.md).
- There is no silent in-app updater. Source users run `update.ps1`; packaged
  users install a reviewed newer package over the existing app.
- The app binds loopback by default. Do not port-forward it; authenticated HTTPS
  and LAN/mobile access are separate future work.
- Optional external runtimes are downloaded or installed only after an explicit
  GUI action. Managed llama.cpp uses official checksum-pinned CPU/CUDA bundles
  and no compiler toolchain; its files do not inflate the core release payload.

See [ADR 0013](decisions/0013-windows-distribution-boundaries.md) for the wider
distribution boundaries and
[ADR 0014](decisions/0014-public-windows-signing-gate.md) for the public signing
gate introduced after the withdrawn alpha.6 installer.
