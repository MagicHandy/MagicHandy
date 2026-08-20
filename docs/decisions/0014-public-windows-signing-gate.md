# ADR 0014: Public Windows Signing Gate

## Status

Accepted; amended 2026-08-03 after Microsoft completed the false-positive
review, for the alpha.9 installer correction, alpha.10's runtime readiness
corrections, alpha.11's update-discovery and clean-machine voice correction, and
alpha.13's restored setup distribution after alpha.12 was withdrawn, and the
reviewed alpha.14 through alpha.27 package-preserving releases.
This supersedes ADR 0013 where that ADR defines public unsigned setup
publication.

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
malicious behavior nor independently establishes a false positive.

The GitHub release was withdrawn without moving or replacing its immutable tag.
The exact executable was submitted to Microsoft Security Intelligence as
submission `15c1e36d-fb35-4c5d-85de-83707169818a` for false-positive analysis.
On 2026-08-03 Microsoft completed that case with final determination
`Not malware`, reported no current cloud or client detection, and stated that
the detection had been removed. That resolves the Defender classification for
the exact alpha.6 artifact. It does not give future hashes publisher identity
or pre-clear a differently packaged installer.

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

1. **Pull-request setup executables are CI-only.** Pull-request workflows build
   an unsigned Inno Setup executable to test custom/default install, upgrade,
   shortcut, uninstall, purge, and clean-reinstall behavior. It remains a
   short-lived workflow artifact labeled `unsigned-ci`; pull requests have no
   release permission.
2. **Setup packaging removes avoidable heuristic triggers.** The amd64 setup
   uses Inno's native x64 loader, `zip/9` compression, and non-solid streams.
   Acceptance reads the PE header and fails if either the setup loader or a
   payload executable is not x64. These constraints remain mandatory for both
   CI and public setup builds.
3. **Alpha.8 through alpha.11 and alpha.13 through alpha.27 reviewed unsigned
   setup are explicit exceptions.** The tag workflow may publish only those
   listed unsigned setup versions with the
   `ReviewedUnsignedPublic` verification policy and the completed Microsoft
   case ID above. Alpha.9 corrects installer-script argument handling without
   changing the hardened Inno packaging shape. Alpha.10 retains that shape and
   corrects managed llama.cpp cold-load readiness, managed TTS verification,
   and worker process-tree cleanup. Alpha.11 retains the hardened package shape,
   restores prerelease update discovery, and materializes Faster Qwen model
   files outside Hugging Face snapshot links for clean Windows installs.
   Alpha.13 retains the hardened package shape and ships the Ollama GGUF and
   repaired-chat corrections after alpha.12's portable release was withdrawn.
   Alpha.18 through alpha.21 retain the hardened package shape while shipping
   reviewed Autopilot and pattern-pacing corrections. Alpha.22 retains that
   package shape and corrects normalized pattern continuity. Alpha.23 retains
   the same package shape while adding selectable Dynamic LLM motion and the
   update-panel layout correction. Alpha.24 retains that package shape while
   adding calibrated Handy model speed, longer organic Dynamic motion, and the
   compact Chat control redesign. Alpha.25 retains the same package shape and
   corrects a Creative-plan endpoint rounding panic without changing installer
   behavior or payload composition. Alpha.26 retains it while adding explicit
   Creative stroke-length envelopes. Alpha.27 retains it while improving
   Creative reversal easing, short-window variability, and conversational
   correction validation; none changes installer behavior or payload
   composition.
   The tag workflow scans each exact candidate directory with Microsoft
   Defender before lifecycle verification. The
   verifier rejects every other version, so a later unsigned setup requires a
   new reviewed policy change. It builds setup, portable ZIP, and two-entry
   checksum into one dedicated `artifacts/release` directory, runs the full
   lifecycle against that exact setup, and publishes only the three explicit
   paths. An ordinary `UnsignedCI` build cannot enter a GitHub Release.
4. **Alpha.12 remains withdrawn and immutable.** Its portable-only GitHub
   Release was removed, its source tag is not moved or reused, and the corrected
   distribution uses the next prerelease ordinal, alpha.13.
5. **Trusted Authenticode remains the production target.** `SignedPublic`
   requires a protected organizational signing identity, trusted timestamp,
   and `Valid` Authenticode status on the setup executable and every shipped
   payload executable. Verification pins the approved signer thumbprint and
   rejects self-signed certificates. The reviewed unsigned exception does not
   satisfy this long-term publisher-identity requirement.
6. **Signing is a release operation, not a source-build dependency.** Developer
   and CI lifecycle builds remain possible without signing credentials. The
   eventual signing service must expose credentials only to the protected tag
   workflow and must not make private key material available to pull requests.
7. **Warnings are not an installation step.** Documentation must not advise
   users to disable Defender, ignore a malware classification, or use a
   SmartScreen bypass. A newly detected public artifact is withdrawn and
   investigated before another version is published.
8. **Published tags remain immutable.** A withdrawn version keeps its tag and a
   source-tree notice explaining the withdrawal. A corrected release uses the
   next SemVer prerelease ordinal.

Microsoft Artifact Signing and SignPath Foundation are viable protected signing
services. Selecting and provisioning one is an infrastructure decision that
requires organization validation and credentials outside this repository; the
repository gate does not pretend those credentials exist.

## Consequences

Positive:

- Microsoft has adjudicated the original severe Defender result instead of the
  project asking users to override it;
- the exact public setup receives the complete clean-runner lifecycle test;
- public and CI policies remain distinct and cannot be mixed by a broad glob;
  and
- signed setup publication will fail closed on missing signatures or
  timestamps.

Negative:

- public alpha executables still have no publisher identity and may show
  reputation warnings until signing is provisioned;
- Microsoft's determination covers the submitted alpha.6 hash, not alpha.8,
  alpha.9, alpha.10, alpha.11, alpha.13, or any later package; the later exceptions therefore add
  an exact pre-publication Defender scan but still do not establish publisher
  identity;
  and
- a trusted signing service and identity-validation process are still needed
  to retire reviewed unsigned setup exceptions.

## Verification

- `Test-WindowsRelease.ps1 -ArtifactPolicy UnsignedCI` requires the full
  unsigned setup/portable/checksum set, verifies x64 PE machine headers, and
  supports installer lifecycle tests.
- `Test-WindowsRelease.ps1 -ArtifactPolicy PortablePublic` requires exactly a
  portable ZIP and one-entry checksum file and rejects any setup executable.
- `Test-WindowsRelease.ps1 -ArtifactPolicy ReviewedUnsignedPublic` requires an
  alpha.8 through alpha.11 or alpha.13 through alpha.27 version, the recorded
  Microsoft case ID, the
  setup/portable/checksum set, x64 PE headers, unsigned status, exact hashes,
  and supports the complete installer lifecycle.
- Alpha.9 through alpha.11 and alpha.13 through alpha.27 reviewed setup
  workflows run Microsoft Defender against the exact public artifact directory
  before verification or release creation.
- `Test-WindowsRelease.ps1 -ArtifactPolicy SignedPublic` requires valid,
  timestamped Authenticode from the explicitly pinned signer on the setup
  executable and every payload EXE.
- The tag workflow publishes only the explicit setup, ZIP, and checksum paths
  under `artifacts/release`.
- Installer integration tests require the reviewed policy, case ID, exact setup
  path, and full lifecycle switches in the tag workflow.
