# Windows branding integration review — 2026-11-14

## Scope and decision

Reviewed Claude's unpublished `claude/windows-branding` worktree, based on
`b4aeb9d6f1fb7e4362061f646ce1dbfeaa069a91` (alpha.87), and the relevant
follow-up in his local conversation. The owner's request is to carry the
approved branding into the Windows installer and installed executable, then
hand publication to Updates.

Accepted the product implementation without changing the images, aesthetic,
Go generator or installer directives. All seven artwork files under `assets/brand/`
are preserved byte for byte. The new SVG masters use exactly the existing web
mark's hand, streak and star geometry. The browser source and canonical UI
bundle are unchanged.

## Code review

- The generator uses only the Go standard library. It reads a local developer
  input; it is not an HTTP endpoint or part of the running motion path.
- ICO header, image count, file size and image spans are checked before
  slicing. The bounded counts and sizes fit the COFF fields. The resource
  directory has sorted type/ID entries, per-image language/data entries and
  image-relative relocations. Its timestamp is zero and output deterministic.
- `resource_windows_amd64.syso` is selected only for Windows amd64. A drift
  test compares the committed link input with fresh output from the icon.
  [ADR 0074](decisions/0074-windows-icon-and-installer-artwork.md) records the
  narrow exception for this generated resource input and its maintenance cost.
- Inno Setup references the canonical icon and both resolutions of each wizard
  image. Existing shortcuts and the uninstall entry already reference the core
  executable. The packaging workflow now runs for asset-only changes too.
- Worker binaries, dependency versions, browser assets, motion, transport,
  settings and credential behavior are unchanged. No hardware was connected
  or commanded during this review.

## Completed checks

- The complete `CGO_ENABLED=0 go test ./...` suite passed, including resource,
  architecture, motion and transport tests. Full lint reports zero issues;
  full vet and formatting checks passed.
- Pure-Go builds passed for Windows amd64, Linux amd64 and Windows arm64. The
  latter two confirm that the amd64 Windows object does not enter other builds.
- Both Windows PowerShell 5.1 and PowerShell 7 installer suites passed.
- The release builder compiled a local **test-only**, dirty-source
  `0.0.0-pr88` installer, portable ZIP and checksum file using Inno Setup 7.0.2.
  This is not a public release candidate and must not be uploaded.
- Read-only `Test-WindowsRelease.ps1` verification passed for that local package
  under `UnsignedCI` with the declared dirty source state: outer checksums,
  portable file manifest/provenance and all five exact-executable vulnerability
  scans. No reachable vulnerability was reported. Installer lifecycle acceptance
  remains a required clean-runner CI gate; it was not run against this machine's
  existing installed app.
- Windows `ExtractIconExW` reports zero icon groups in the alpha.87 baseline
  and one in both the branded core and test setup. `PrivateExtractIconsW`
  successfully loads each at 16, 20, 24, 32, 40, 48, 64, 96, 128 and 256 px.
  Native-rendered small and large icons and both installer images were visually
  inspected. All four PNG dimensions match the intended 1x/2x pairs.
- A local focused race-test attempt could not compile because this host has no
  `gcc` on PATH. This is a local toolchain limitation, not a passing race run.
  Full CI race checks remain required before merge/publication.

Further package, complete-suite and exact-app readiness results are recorded
on the review branch/PR and in the release handoff when they finish. A launch
of the local installer UI was rejected by automatic approval review before
execution; fresh page-by-page installer interaction is not claimed here.
The native resource checks and actual Inno compilation above did run.

## Budget and release requirements

Reproduced the same-toolchain measurement with Go 1.26.11, Windows amd64,
`CGO_ENABLED=0`, `-trimpath`, `-ldflags=-w` (symbols retained):
**26,790,912 → 26,966,528 bytes**, an increase of **175,616 bytes**.
The local test setup is 23,709,454 bytes; its version and source-state metadata
differ from Claude's earlier local build. Browser payload and dependencies are
unchanged. This does not establish a new idle-memory or cold-start benchmark.

The alpha.88 public notes and version-specific policy entries are prepared.
Only alpha.88 is added; alpha.89 and excluded alpha.65 remain rejected.
Updates must obtain green branch/PR/main checks,
then create an immutable tag at the verified current main commit. Public
publication still requires the scanner, vulnerability, provenance and installer
lifecycle gates, followed by independent verification of the downloaded setup,
portable ZIP and checksum file. Do not publish the local test package or reuse
the existing alpha.87 tag.

Ignored evidence is retained under `.scratch/windows-branding-review/` in the
review worktree, including the frozen Claude source manifest, resource checks,
native icon renders, test logs, local packages and budget/geometry comparison.
