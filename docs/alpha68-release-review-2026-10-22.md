# Alpha.68 release review — 2026-10-22

Dates use America/New_York. The maintainer requested an update after review of
the WAN session, hosted motion and Layered smoothness changes. PR #339's exact
head `f6d0e246` passed every check and was merged as `f5165dab`. The changes since
alpha.67 are that PR and this release preparation.

## Source and acceptance review

The [WAN session review](wan-session-review-2026-10-22.md) separates the observed
30-minute idle sign-out from controller heartbeat loss and provider latency.
The new eight-hour default and configurable idle policy preserve absolute
expiry, revocation and controller safety fences. Active-controller retries no
longer back off beyond their lease; temporary observation staleness no longer
cancels an otherwise admitted chat request. Regression coverage preserves Stop,
hidden-document, ownership and authorization cancellation boundaries.

The [hosted pace review](hosted-pace-review-2026-10-22.md) records real ChatGPT
evaluations of pace descriptions, opposite-end tempo contrast and released
versus lasting constraints. Compact local contracts and existing speed limits
remain intact. No deterministic language rules or preferred speed band were
added. The successful final production HTTP fixture used one call per turn,
without repair or semantic fallback; failed experimental selections and provider
timeouts remain in the evaluation evidence.

The [Layered smoothness review](layered-smoothness-review-2026-10-22.md) records
the completed Claude Opus 5.5 Max consultation, resulting implementation,
numerical comparisons and inspected engine-rendered motion atlas. True combined
field turns, rounded wire crossing times and bounded pace-only splices address
identified defects without introducing another motion path. General geometry
edits retain the existing crossfade. A further focused Claude pass did not
produce a completed report and is not counted as validation.

The full Go suite, vet, lint, pure-Go build, frontend checks, Linux race suite,
installer tests, optional Labs, both voice-adapter versions and Windows package
lifecycle passed on the final PR head. The isolated simulator completed a real
local Gemma generation and a text-only app chat without repair, fallback or
motion. Real GPT-6 Sol production HTTP evaluation used a fake transport and
confirmed one playback stream across retargets and no work after Stop.
The vibrator Stop snapshot now clones mutable motor state under its lock,
with a regression for the shutdown race discovered by CI.

No real hardware was connected or moved during this review. Real WAN repetition
and physical Handy Cloud comparison remain outstanding; numerical and visual
improvements do not establish the user's physical smoothness issue is resolved.
The installed app and its data remain untouched.

## Release preparation and boundaries

Only `0.1.0-alpha.68` is added to ADR 0014's reviewed unsigned policy. Alpha.69
and unpublished alpha.65 remain rejected. This version-specific amendment keeps
source and exact-binary vulnerability scanning, Defender scanning, manifests,
checksums, x64 packaging, installer lifecycle, green CI and current-main
provenance mandatory. Microsoft's prior case does not approve these new hashes.
Trusted Authenticode remains the long-term requirement.

The five payload executables, native x64 setup, non-solid ZIP and three public
assets are unchanged. No dependency or runtime worker is added. The goal
scorecard records compiler/frame costs and the comparable core/UI budgets.
Release preparation changes only policy, assertions and documentation. Public
notes disclose unsigned packaging, upgrade instructions, pending physical
acceptance and the existing device limitations. Publication remains conditional
on the merged-main and tag checks, including all five exact Windows executable
scans and the exact public setup lifecycle on a disposable CI host.
