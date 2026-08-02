# Windows Release Packaging

MagicHandy produces two unsigned Windows x64 artifacts from one staged payload:

- `MagicHandy-<version>-windows-amd64-setup.exe`
- `MagicHandy-<version>-windows-amd64-portable.zip`

The setup EXE is a thin Inno Setup shell. The portable archive contains the
same app, workers, optional-module helper scripts, license, source notice, and
release manifest. Neither artifact bundles models, Python, CUDA, llama.cpp, or
Parakeet; those remain explicit setup choices.

No command in this document publishes a GitHub Release.

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
creates the portable ZIP, compiles Inno Setup, and writes
`MagicHandy-<version>-windows-amd64-SHA256SUMS.txt` under `artifacts\`.

Useful build-only options:

| Option | Purpose |
| --- | --- |
| `-OutputRoot PATH` | Write artifacts outside the default `artifacts\` folder |
| `-ISCCPath PATH` | Select a specific Inno compiler |
| `-SkipFrontendBuild` | Reuse the existing canonical `web/dist`; local debugging only |
| `-SkipInstaller` | Build only the portable ZIP |
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
- the project documentation tree, so relative links in the packaged README stay
  usable offline;
- `LICENSE`, `README.md`, and `SOURCE.txt`; and
- `release-manifest.json` with version, exact commit, source URL, file sizes,
  and per-file SHA-256 hashes.

The manifest is generated after all payload files except the manifest itself
are staged. The outer checksum file covers the portable ZIP and setup EXE.

## Local Smoke Test

Build with a disposable version, then verify both forms:

```powershell
$setup = Get-ChildItem .\artifacts\*-setup.exe | Select-Object -First 1
$installDir = Join-Path $env:TEMP 'MagicHandySetupSmoke'
$install = Start-Process -FilePath $setup.FullName -ArgumentList @(
  '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/NOICONS', ('/DIR="{0}"' -f $installDir)
) -Wait -PassThru
if ($install.ExitCode -ne 0) { throw "Setup failed with exit $($install.ExitCode)." }
& (Join-Path $installDir 'magichandy.exe') -version
$uninstaller = Get-ChildItem $installDir -Filter 'unins*.exe' | Select-Object -First 1
$uninstall = Start-Process -FilePath $uninstaller.FullName -ArgumentList @(
  '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART'
) -Wait -PassThru
if ($uninstall.ExitCode -ne 0) { throw "Uninstall failed with exit $($uninstall.ExitCode)." }
```

Also expand the portable ZIP, run `magichandy.exe -version`, verify every
manifest hash, and start the app with a disposable data directory. Silent smoke
tests must not launch a browser or create shortcuts.

Interactive uninstall removes program files and shortcuts but deliberately
retains `%APPDATA%\MagicHandy`, including settings, history, models, and voice
modules. It reports that path after uninstall. Silent uninstall suppresses the
message but follows the same data-retention policy.

## CI Workflow

`.github/workflows/package-windows.yml` runs on relevant pull requests and
manual dispatch. It:

1. builds unsigned artifacts;
2. verifies the portable payload, exact source provenance, and every manifest
   hash;
3. verifies the outer checksum file;
4. silently installs, checks the installed version, and silently uninstalls;
5. uploads a short-lived workflow artifact for review.

The workflow has `contents: read`, has no release trigger, and never invokes a
GitHub Release API. Artifact retention is seven days.

## Release Boundaries

- Artifacts are visibly unsigned until a real certificate, protected CI secret,
  timestamp service, and revocation process exist.
- Versioned builds can discover the latest stable GitHub Release and notify the
  user. The check is cached, can be set to manual-only, and never downloads or
  runs an installer. See [Release Checks And Update Handoff](update-checks.md).
- There is no silent in-app updater. Source users run `update.ps1`; packaged
  users install a reviewed newer package over the existing app.
- The app binds loopback by default. Do not port-forward it; authenticated HTTPS
  and LAN/mobile access are separate future work.
- Optional external runtimes are downloaded or built only after an explicit GUI
  action. Their multi-gigabyte files do not inflate the core release payload.

See [ADR 0013](decisions/0013-windows-distribution-boundaries.md) for the policy
behind these boundaries.
