# Alpha.63 release review — 2026-10-16

The maintainer requested merging the open PR and publishing an update.
PR #326, reviewed at `cb1408dfc9ea3d020f15e08084972bbc1eed433b`, merged into
main as `4a9457cf59d8c14ba982abc0fc7bdd0f3d5ca02f`. Alpha.63 packages that
change above alpha.62; the release preparation adds documentation and an
exact-version unsigned policy amendment.

## Source review and evidence

The release review read the scheduler, prepared-choice lifecycle, routing,
prompt/schema changes, settings defaults, UI projection and regression tests.
It found no blocking issue at the reviewed head. The preparation stays in an
isolated worktree; the original checkout and its existing launcher edit are
preserved.

- Hosted motion auto timing is opt-in and bounded to 8–300 seconds. Missing
  or unusable timing falls back to the existing sampled window.
- Early preparation applies only to external planning routes. A pending choice
  stays behind its boundary and the existing mode/controller/Stop generation
  checks. Chat and settings discard it; pause shifts the established deadline.
- Speech and local planning retain their prior contracts. The new schema field
  is absent from local turns and spoken check-ins.
- The UI shows the controls from backend routing, saves through the existing
  settings path and keeps the countdown active while a decision is ready.
- No compiler, physical limit, sampler or transport implementation changes.
  The feature's transport-free evaluation uses the production parser/compiler;
  timing tests use a fake clock and engine. Existing curve evidence remains
  applicable; this is not new physical-device acceptance.

The reviewed PR's Go/race/lint/vulnerability, frontend, installer, voice,
optional Labs and Windows packaging checks passed. The release reviewer also
ran the new timing/preparation checks locally:

```text
CGO_ENABLED=0 go test ./internal/chat ./internal/config ./internal/modes ./internal/httpapi
  -run 'Test(Prepared|ChatDiscards|JustInTime|AutopilotModelHorizon|ModelHorizon|AutoMotionTiming|HostedCreativeV2PlannerChooses|LocalPlannerIgnores|NextCheck|TakeNextCheck|AutoTiming)' -count=1
```

All four packages passed. The full timing evidence, failed/control comparisons
and request-rate limitations are in
[the feature review](autopilot-auto-timing-review-2026-10-16.md) and
[ADR 0050](decisions/0050-hosted-autopilot-auto-timing.md).

The release-preparation patch also passed `go test ./internal/architecture
-count=1` with CGo disabled and `scripts/test-installer.ps1` under Windows
PowerShell 5.1, including the exact-version policy and future-version rejection
fixtures. Full CI and packaging are still required on its merged main commit.

## Packaging and carried limitations

ADR 0014 and the verifier admit only `0.1.0-alpha.63` in addition to the
previously reviewed versions. The rejection fixture advances to alpha.64;
the Microsoft case check and all integrity/security gates remain. README and
Getting Started download links advance to the new release.

There is no new executable, dependency, asset bundle or runtime behavior in
the release-preparation patch. Feature size measurements remain in the
October 16 scorecard entry. Release executables retain symbol tables for the
existing exact-binary vulnerability scans.

The tag must name the clean, merged main tip after its quality and package
checks pass. The tag workflow repeats quality, adapter, exact-artifact
Defender, manifest/checksum and installer-lifecycle verification before
publishing the setup EXE, portable ZIP and two-entry SHA256SUMS file. Published
tags remain immutable.

Hosted planning remains experimental. Simulated timing results do not establish
physical feel or long-session provider usage. ADR 0049's Intiface Stop issue,
unverified loaded COYOTE/vibrator behavior and native-worker crash recovery
limits remain in release notes; no upstream fix is bundled or claimed.
