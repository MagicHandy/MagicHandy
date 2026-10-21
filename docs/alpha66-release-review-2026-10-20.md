# Alpha.66 security release review — 2026-10-20

Dates use America/New_York (EDT); explicit incident timestamps use UTC.

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
  The source installer separately rejects unpatched Go 1.27.0 through 1.27.2;
  that release line requires 1.27.3 or newer. Numeric ordering against the older
  supported line alone does not establish the security fix.
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

Claude Opus 5.5 reviewed the alpha.66 update through Claude Code CLI at medium
effort and reported no blocking issue. Its Low finding about accepting older
1.27 patches was adopted in a follow-up with boundary tests and installer-plan
wording in all five locales. The focused medium-effort re-review confirmed the
correction with no remaining blocking finding. Active manual setup/security docs were updated; historical
measurements retain their actual compiler versions. All workflows already read
`go.mod`, and source updates follow the branch/main ref rather than choosing
the failed alpha.65 tag. The README's pre-publication release link follows the
existing reviewed-PR/tag workflow.

The new installer test binds the exact source module returned at import time;
fixture modules may share its name later in the full suite. An initial
name-only lookup was ambiguous and failed under PowerShell 5.1. This test
scope correction retains every version-boundary assertion.

## Merged-main installer failure

The PowerShell 7 installer job in
[run 39528808037](https://github.com/MagicHandy/MagicHandy/actions/runs/39528808037/job/118989670777)
failed during verified relaunch after rebuilding the fixture. The final error
was access denied while deleting its still-running `magichandy.exe`; runner
cleanup subsequently killed that process. The roughly 30-second gap matches
the launcher's verification timeout, but the fixture's final folder deletion
masked the original error. The runner retained no underlying startup report,
so those logs alone do not establish why verification failed. PowerShell 5.1
passed in the same job, and both shells passed on the candidate.

Test teardown now finds and stops only executables beneath its
validated, unique temporary fixture directory before removing it. A secondary
cleanup error cannot replace the original test failure. Failure logging prints
the stack and bounded, credential-free startup reports; it excludes private
instance records, settings and general runtime logs. A forced verification
timeout regression checks the original error/process ID, the launcher's
intentional decision to leave an unverified app running, test-owned teardown,
and release of the executable file. Readiness assertions, production startup
timeouts, fixed-port behavior and motion Stop gates remain required. No HNS
reset or application behavior change is part of this harness correction.

Both complete local installer suites pass sequentially under Windows
PowerShell 5.1 and PowerShell 7, including the forced timeout and invalid-root
guard. Architecture checks pass. An additional concurrent local run exposed a
separate ephemeral-port collision during release startup acceptance; the new
logging retained its actual bind error and reports. CI runs the two shells
sequentially, as do the final local acceptance runs.

Claude Opus 5.5 reviewed the harness fix via the CLI at medium effort. Its
PowerShell 5.1 native-stderr finding was adopted: cleanup temporarily allows
`taskkill` stderr and checks actual process exit, so an already-exited process
does not abort teardown. Its suggestions to include orphaned fixture workers
and briefly retry the file-unlock assertion were also adopted. The report
schema was inspected directly: it retains only bounded version, platform,
timestamp, stage, policy and bind-result facts, without instance tokens,
paths, raw errors or settings. The forced regression covers timeout and
teardown; secondary-error preservation is also supported by code review and
the separately observed startup-failure logging, not a forced cleanup-error
test.
