# Alpha.64 release review — 2026-10-18

The maintainer requested an update after the hosted Layered and constraint
lifetime review. This release packages PR #328, reviewed at
`aa313fc2d82984337ad3967adb5154f11ede2544`, above alpha.63. The release-preparation
patch adds concise public notes, download-link updates and an exact-version
unsigned packaging amendment. The separate cloud-authentication migration in
PR #329 is outside this release's scope.

## Source review and evidence

The release review inspected the hosted operation schema and translation,
composition and layer exclusivity, original-object validation before recall
extraction, interactive/Autopilot/reviewer routing, lifetime guidance, shared
geometry fit, backend history budgets and settings projection. No blocking
issue was found at the reviewed head.

- Structured operations translate into the existing atomic validator. They
  cannot bypass lifecycle authority, saved motion limits or the shared engine.
  Duplicate fields and conflicting operations are rejected before motion changes.
- Hosted guidance distinguishes temporary directions from lasting preferences
  through model interpretation. No keyword classifier, expiration timer,
  additional provider call or local prompt change is introduced.
- Named anchor operations retain narrower stroke widths and fit inherited
  widths to an explicitly narrowed outer band. Pace remains independent.
- History is bounded by each external connection's request budget. Omitted
  history settings preserve existing behavior; local settings and inference
  limits remain independent. The frontend saves through the backend settings
  path and does not maintain a separate planning state.
- The nullable provider-schema fix retains required discriminators while
  restoring omitted optional properties. Coverage includes recall, reviewer,
  lifecycle and prepared-decision paths.

The final live ChatGPT checks passed 54/54 matched chat/planning responses,
7/7 standing-hold decisions and 18/18 Layered regressions, without repair or
fallback. The final comparisons share real interactive acknowledgments and
carry accepted scores forward at simulated request ages. All earlier failures
and harness corrections remain documented. Twenty motion overview sheets and
detailed outliers were inspected; these are commanded trajectories, not new
physical-device acceptance.

Full local Go tests, race tests, vet, standard lint, pure-Go build, frontend
typecheck/build, five-locale audit and 915 frontend tests passed. The latest
technical prompt change received another affected-package and chat-race check.
See the [operation review](hosted-layered-operations-review-2026-10-18.md),
[lifetime/context review](constraint-lifetime-review-2026-10-18.md), and
[scorecard](goal-scorecard.md) for reproduction and measured budgets.

The release-preparation patch passed `CGO_ENABLED=0 go test
./internal/architecture -count=1` and `scripts/test-installer.ps1` under Windows
PowerShell, including the exact-version allowlist and future-version rejection.
Feature and release-preparation CI remain publication gates; the local results
do not substitute for those checks or the exact public-artifact workflow.

## Packaging and release boundaries

ADR 0014 and the Windows verifier admit only `0.1.0-alpha.64` in addition to
previously reviewed versions. The future-version rejection fixture advances to
alpha.65. The Microsoft case binding and all artifact-integrity, vulnerability,
Defender and installer-lifecycle gates remain mandatory.

The payload still contains five executables, with no new runtime dependency.
The native x64 installer, non-solid ZIP, three public assets and data-preserving
over-install contract remain. Release executables retain symbol tables for
exact-binary vulnerability scans. This preparation does not change runtime code
or the embedded UI; their cost measurements remain in the feature review.

Both the feature and release preparation must reach main through reviewed PRs
with green CI. Only the clean current main tip is tagged; the tag workflow
repeats quality and adapter tests, builds the exact source, scans and verifies
the exact assets, exercises the setup lifecycle, and then publishes setup,
portable ZIP and the two-entry SHA256SUMS file. Tags remain immutable.

ADR 0049's Intiface Stop limitation, unverified loaded COYOTE/vibrator behavior
and native-worker crash recovery limits remain in public release notes. No
upstream device fix is bundled or claimed. The review simulator uses a working
local model and does not copy host credentials or connect real hardware.
