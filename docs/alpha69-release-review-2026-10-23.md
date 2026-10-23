# Alpha.69 release review — 2026-10-23

Dates use America/New_York. The maintainer requested publication after the
Layered range and counted-pattern review. PR #341's exact head `7b53561e`
passed every branch and PR check and merged as `74ffdcd9`. Changes since
alpha.68 are that PR and this release preparation.

## Source and acceptance review

The [Layered range review](layered-range-review-2026-10-23.md) records the
installed-session evidence, completed Claude Opus 5.5 Max consultation,
addressed review findings, real-model comparisons and inspected motion atlas.
The [accepted design](decisions/0052-counted-layered-phrases.md) adds counted
phrases inside the existing shared Layered fitter, sampler, sanitizer,
transport and Stop path. No second engine or independent hardware loop is
introduced. Reach edits preserve count phase and bound joins by the actual
incoming and destination motion; severe narrowing can finish the previous
stroke pair before joining instead of forcing a catch-up burst.

Final real GPT-6 Sol evaluations passed 12/12 cases with None reasoning and
12/12 with Low, each without repair or fallback. Median response times were
5.284 s and 5.235 s. Six-turn continuation and production HTTP orchestration
with a fake transport also passed. Failed intermediate model selections remain
in the ignored evidence; these finite runs do not establish universal language
compliance or statistical superiority of one reasoning setting.

Numerical coverage includes 60 profile/speed/reach combinations and 288 severe
narrowing checks, selected-speed bounds, count/order fidelity, endpoint visits,
mirrored slow sweeps, continuity, transactional rejection and Stop. All 47
existing non-phrase comparison records exactly match alpha.68, including wire
timing. The 205 evaluation records produced 140 distinct motion plots and nine
inspected overview sheets. No deterministic user-language rules, preferred
speed or fixed regional width were added. Local-only prompts and schemas are
unchanged.

The full Go suite, vet, lint, pure-Go build, architecture checks, Linux race
suite, frontend checks, optional Labs, both voice-adapter versions, installer
tests and Windows package lifecycle passed on the final feature head. The
duplicate push installer job passed after a retry of a Windows port-denial
failure; no gate was changed. The isolated simulator completed a real local
Gemma generation and text-only app chat without repair, fallback or motion.

No real hardware was connected or commanded during this review. Physical
Original Handy / Cloud comparison remains outstanding, including endpoint
reach, broader regional coverage and live retargets. The maintainer's later
release request authorizes alpha publication with that limitation disclosed;
it does not establish that the reported physical shortfall is resolved. The
installed app and its data remain untouched.

## Release preparation and boundaries

Only `0.1.0-alpha.69` is added to ADR 0014's reviewed unsigned policy. Alpha.70
and unpublished alpha.65 remain rejected. This version-specific amendment
preserves source and exact-binary vulnerability scanning, Defender scanning,
manifests, checksums, x64 packaging, installer lifecycle, green CI and
current-main provenance. Microsoft's prior case does not approve these new
hashes. Trusted Authenticode remains the long-term requirement.

The five payload executables, native x64 setup, non-solid ZIP and three public
assets are unchanged. No dependency or runtime worker is added. The goal
scorecard records the comparable core/UI and compiler costs. This preparation
changes only policy, assertions and documentation. Public notes disclose
unsigned packaging, update instructions, pending physical acceptance and
existing device limitations. Publication remains conditional on merged-main
and tag checks, including the five exact Windows executable scans and the exact
public setup lifecycle on a disposable CI host.
