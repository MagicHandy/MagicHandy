# Alpha.66 security release review — 2026-10-20

The requested update includes PR #329's cloud authentication and Windows DPAPI
changes and PR #332's local startup recovery. Their exact heads were merged in
main `da27d8ed`, with green candidate, merged-main, packaging and tag checks.
The [alpha.65 feature review](alpha65-release-review-2026-10-20.md) and original
feature records retain the source review, tests and acceptance limitations.

## Blocked alpha.65 publication

The final Windows artifact gate in
[run 39524520208](https://github.com/MagicHandy/MagicHandy/actions/runs/39524520208)
found GO-2026-6809 at `crypto/x509.Certificate.Verify` in the exact Go 1.26.9
core executable. Defender had passed; the vulnerability gate correctly stopped
before artifact retention or GitHub release creation. No alpha.65 release or
public artifact was published. Its annotated source tag remains unchanged.

The [official advisory](https://pkg.go.dev/vuln/GO-2026-6809) was published at
2026-10-21 00:27:29 UTC, during release validation. Go 1.26.10 adds a denylist
for compromised intermediate certificates in cases where the system trust
store may not have been updated. The
[Go release history](https://go.dev/doc/devel/release#go1.26.10) identifies that
patch. Updating the supported 1.26 patch line avoids a major toolchain migration.

## Security and packaging changes

- `go.mod` and the Windows source installer require Go 1.26.10. CI and the
  release builder select that version from `go.mod`; exact-binary verification
  continues to reject older compiled Go versions and reachable vulnerabilities.
- The active reviewed unsigned policy removes unpublished alpha.65 and admits
  only alpha.66 alongside the previously reviewed releases. Both alpha.65 and
  unreviewed alpha.67 are explicitly rejected by installer tests. No scanner,
  check, assertion, signing requirement or provenance gate is bypassed.
- The same five payload executables and setup/portable/checksum distribution
  remain. No application source, motion, transport, frontend behavior, Go module
  dependency or external runtime changes in this patch pass.
- Alpha.66 notes carry forward startup recovery, standard-report diagnostics,
  cloud authentication, DPAPI migration/downgrade limits, unsigned packaging,
  update instructions and existing physical-device acceptance limits. Alpha.65's
  notes become an incident notice in main; its tagged source is never rewritten.

The patched candidate reruns the full release gates. The final Windows workflow
must scan all five actual payload binaries, run Defender on the public files,
verify manifests/checksums and exercise default/custom install, over-install,
uninstall and fresh reinstall before publishing. Readiness uses an isolated
simulator and an available local model without copying user credentials; it
does not claim a live cloud account or physical-device acceptance run.

The local Go 1.26.10 pure-Go Windows core passes the unchanged actual-binary
`govulncheck@v1.8.0` gate with zero reachable vulnerabilities. Architecture and
release-note tests pass. Its comparable stripped core is 23,446,016 B, up
2,048 B from the Go 1.26.9 candidate; the canonical UI is unchanged. The
scorecard retains the measurement flags and separates this toolchain change
from the original feature costs. Full CI remains required before merge/tag.
