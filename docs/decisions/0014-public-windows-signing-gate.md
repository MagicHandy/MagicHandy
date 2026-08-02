# ADR 0014: Public Windows Signing Gate

## Status

Accepted. This supersedes ADR 0013 only where that ADR allowed unsigned setup
executables to be attached to public releases.

## Context

The unsigned `v0.1.0-alpha.6` Inno Setup executable was classified at launch by
Microsoft Defender as `Behavior:Win32/DefenseEvasion.A!ml`. Windows then blocked
launch with ShellExecuteEx error 225. The downloaded file matched the release
SHA-256 (`42b47976da6ed0431e8dea8bce63f15fcb48fbf9ae962e44a82a691d33cf85bd`),
the tagged source and GitHub Actions build passed all release gates, and a
separate static Defender scan did not report a threat. Those facts establish
provenance; they do not establish publisher identity or make a severe runtime
detection acceptable to ask users to bypass.

A point-in-time VirusTotal report for that exact hash showed 4 of 71 engines
flagging the file: Microsoft, DeepInstinct, SecureAge, and Skyhigh. Its sandbox
summary showed no direct malware detection, while its structural and behavior
metadata identified an unsigned Inno Setup executable with an overlay and the
generic `obfuscated` and `self-delete` tags. That mixed result neither proves
malicious behavior nor establishes a false positive. It does show that the
classification was not isolated to one local Defender installation, so the
artifact remains withdrawn while Microsoft analyzes it.

The GitHub release was withdrawn without moving or replacing its immutable tag.
The exact executable was submitted to Microsoft Security Intelligence as
submission `15c1e36d-fb35-4c5d-85de-83707169818a` for false-positive analysis.
Its status was `Submitted / Pending` when this decision was recorded.

A controlled same-payload packaging comparison also found avoidable structural
traits in the withdrawn package: its loader was 32-bit even though the payload
and artifact name were amd64, and the payload was one solid
`lzma2/ultra64` stream. Inno Setup documents its
[64-bit architecture modes](https://jrsoftware.org/ishelp/topic_64bit.htm) and
that the single-file [Setup loader](https://jrsoftware.org/ishelp/topic_setup_usesetupldr.htm)
extracts and runs Setup from a temporary directory. Those traits are consistent
with the report's PE32, opaque overlay, `obfuscated`, and `self-delete`
observations, but correlation is not proof that they alone caused the cloud
classification.

VirusTotal report:
<https://www.virustotal.com/gui/file/42b47976da6ed0431e8dea8bce63f15fcb48fbf9ae962e44a82a691d33cf85bd/detection>

## Decision

1. **Unsigned setup executables are CI-only.** Pull-request and tag workflows
   may build an unsigned Inno Setup executable to test custom/default install,
   upgrade, shortcut, uninstall, purge, and clean-reinstall behavior. That
   executable is retained only as a short-lived workflow artifact, is labeled
   `unsigned-ci`, and is never attached to a GitHub Release.
2. **CI setup packaging removes avoidable heuristic triggers.** The amd64 setup
   uses Inno's native x64 loader, `zip/9` compression, and non-solid streams.
   Acceptance reads the PE header and fails if either the setup loader or a
   payload executable is not x64. This is defense in depth, not a substitute
   for publisher identity or the public signing gate.
3. **Public releases are portable-only until signing exists.** The temporary
   public artifact set contains the manifest-verified portable ZIP and a
   checksum file covering only that ZIP. It is built into a dedicated release
   directory, and release verification fails if a setup executable appears
   there.
4. **A public setup executable requires trusted Authenticode.** Restoring setup
   publication requires a protected organizational signing identity, a trusted
   timestamp, and `Valid` Authenticode status on the setup executable and all
   shipped payload executables. Release verification pins the approved signer
   certificate thumbprint and rejects self-signed certificates. A self-signed
   or personal development certificate does not satisfy this gate.
5. **Signing is a release operation, not a source-build dependency.** Developer
   and CI lifecycle builds remain possible without signing credentials. The
   eventual signing service must expose credentials only to the protected tag
   workflow and must not make private key material available to pull requests.
6. **Warnings are not an installation step.** Documentation must not advise
   users to disable Defender, ignore a malware classification, or use a
   SmartScreen bypass. A newly detected public artifact is withdrawn and
   investigated before another version is published.
7. **Published tags remain immutable.** A withdrawn version keeps its tag and a
   source-tree notice explaining the withdrawal. A corrected release uses the
   next SemVer prerelease ordinal.

Microsoft Artifact Signing and SignPath Foundation are viable protected signing
services. Selecting and provisioning one is an infrastructure decision that
requires organization validation and credentials outside this repository; the
repository gate does not pretend those credentials exist.

## Consequences

Positive:

- users are no longer offered an unsigned installer that Windows may classify
  as malicious;
- install and uninstall behavior remains continuously tested on clean Windows
  runners;
- public and CI artifacts cannot be mixed by a broad release glob; and
- signed setup publication will fail closed on missing signatures or
  timestamps.

Negative:

- public alpha users temporarily extract and run the portable package instead
  of receiving Program Files, shortcut, and Add/Remove Programs integration;
- portable executables still have no publisher identity until signing is
  provisioned; and
- a trusted signing service and identity-validation process must be funded or
  approved before the setup package can return.

## Verification

- `Test-WindowsRelease.ps1 -ArtifactPolicy UnsignedCI` requires the full
  unsigned setup/portable/checksum set, verifies x64 PE machine headers, and
  supports installer lifecycle tests.
- `Test-WindowsRelease.ps1 -ArtifactPolicy PortablePublic` requires exactly a
  portable ZIP and one-entry checksum file and rejects any setup executable.
- `Test-WindowsRelease.ps1 -ArtifactPolicy SignedPublic` requires valid,
  timestamped Authenticode from the explicitly pinned signer on the setup
  executable and every payload EXE.
- The tag workflow publishes only explicit paths under `artifacts/release`.
- Installer integration tests statically reject reintroduction of an unsigned
  setup path into the GitHub Release asset list.
