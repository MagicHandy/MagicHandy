# Alpha.84 release integration review — 2026-11-09

## Preparation status

The maintainer requested review of Claude's latest changes and another release.
Updates prepares release policy independently while the source reviewer owns
the source fixes and validation. The base is published alpha.83,
`4cc745d0dd44d147a66630635ad8009a6cfdc1fd`. Preparation uses
`codex/updates-alpha84`; source review uses `codex/release-alpha84`.

The exact accepted source commit or PR and its final validation evidence have
not yet been handed off. No source integration, merge, release tag or
publication is authorized at this stage. The working scope is Advanced chat
reply/motion consistency guidance, its evaluation harness and review records.
Frontend, engine and transport changes are not expected; the final diff must
establish the actual scope before release.

## Release policy

Only `0.1.0-alpha.84` is added to ReviewedUnsignedPublic. Alpha.65 remains
excluded and the future-version rejection test advances to alpha.85. The
active versioning/packaging documentation and ADR 0014 move with this proposed
version while earlier release amendments remain historical records.

The native x64 Inno Setup loader, non-solid ZIP, five-executable payload,
three public assets and installer contract remain. No CI workflow, compiler,
scanner, dependency or managed-runtime pin changes. Source and exact-binary
vulnerability scans, exact-public-artifact Defender, full installer lifecycle,
manifests/checksums, green CI and exact-current-main tag provenance remain
mandatory. The previous Microsoft determination does not approve new hashes.

## Validation and budget

Preparation checks cover the release-policy change on the alpha.83 base. They
do not validate the pending source or replace any final-head check:

- Full installer suites passed on Windows PowerShell 5.1 and PowerShell 7.
- Architecture checks passed.
- Source vulnerability preflight reports zero reachable vulnerabilities and
  three module-only advisories in code not called by the app.
- Diff whitespace checks passed; the final committed preparation is checked
  for tracked-content integrity before handoff.

The initial parallel installer runs contended for the same startup-fixture
port, 11600. Windows PowerShell 5.1 passed; the PowerShell 7 failure log was
retained and its suite passed when run sequentially. No source or assertion was
changed to bypass that collision. Further installer runs use one suite at a
time and never stop another session to free a port.

The accepted source must receive its own focused and full validation. Changes
to motion mapping require retained failed selections, every evaluated pattern
and accepted proposal rendered through the shared engine, overview/outlier
inspection and explicit limits on physical acceptance. Model observations are
not hard-coded targets or guarantees. No earlier release's model sample is
claimed as fresh evidence for this release.

The same-host alpha.83 baseline was rebuilt using Windows amd64 Go 1.26.10,
`CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags '-s -w'`: **24,490,496 B**
including the canonical UI. The UI is **2,885,533 B** across nine files; main
JS is **1,323,641 B**, gzip **355,646 B**, and CSS is **210,443 B**, gzip
**36,663 B**. These are baseline measurements, not a final candidate result.

The final source must be measured with the same flags. Shipping builds retain
symbols for exact vulnerability scans. The canonical UI and any changed
browser payload are measured too. The scorecard and public notes follow the
final accepted behavior and budget.

## Review app and final gates

Existing user and Claude review processes remain untouched. Reuse requires an
exact production-source match; otherwise review uses a parallel simulator with
isolated data and an available local model, without copying credentials. The
exact current build must complete real model generation, a non-empty text-only
app chat without repair/fallback/motion, and a visible stopped-state review
before handoff. A prepared but unstarted app is not handoff-ready.

After the accepted source handoff, Updates must review the integrated diff,
publish concise user-facing notes, obtain all required final-head PR checks,
merge by PR, and verify merged main before creating an unused immutable tag
on that exact main commit. Tag and publication gates must pass, followed by
independent verification of the actual public downloads and their exact source
identity. Preparation does not authorize those later operations yet.

No hardware connection, motion command, installed-session restart or session
replacement is part of preparation. Runtime, data, logs, captures and build
helpers remain ignored under `.scratch/`.
