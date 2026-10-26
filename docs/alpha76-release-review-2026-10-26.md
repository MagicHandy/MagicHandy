# Alpha.76 release review — 2026-10-26

The maintainer requested publication of the prepared Layered follow-up in
PR #353. Alpha.75 (`37775586`) is the release baseline. The implementation and
retained alternatives are documented in the
[Layered follow-up](layered-review-followup-2026-10-26.md); measured costs are in
the [scorecard](goal-scorecard.md). This release preparation adds public notes
and the version-specific artifact policy without changing the reviewed motion
or UI implementation at `d504404f`.

## Source review and behavior

The integration review checks the authored-duration metadata through curve
fitting and pace summaries, per-leg cruise selection, plan-relative derivative
limits, buffered transition history, backend-owned UI estimates and the related
regressions. No blocking finding was identified. Motion still uses the shared
engine, fitter, sampler and transport interface. The patch adds no model call,
language rule, dependency, worker, timer or separate device-command loop.

The accepted derivative envelope uses 2x the two plans' measured peaks with
the existing gentle floor and global acceleration/jerk caps. The rejected 1x
alternative delayed fast reductions and remains in the evidence. The 2x
candidate reduces affected gentle Edge jerk by about 47–84% without another
fallback in 180 comparisons. Two short-25% entries finish the old turn for
30 ms before a longer gentler join; the total bridge is 368/367 ms rather than
39 ms. This is not a physical latency or comfort guarantee.

The mixed-rate change preserves feasible slow-leg shapes; its largest tested
loop grows by 20 ms (0.33%). The previously physically accepted Original Handy
Cloud full/short/counted fixtures retain 4116/1536/2660 ms six-cycle periods.
Those older physical results do not establish acceptance of the new joins.
The rapid 35→85→7% reproduction removes two unwanted direction changes while
retaining already-queued history.

Claude Opus 5.5 xhigh's completed alpha.75 static review identified the
follow-up hypotheses. It did not independently approve this patch. The
reproductions, source review and automated checks are recorded separately;
neither static review nor commanded plots are measured device behavior.

## Verification and footprint

At the prepared implementation head, full local Go/race suites, vet, lint
(zero issues), architecture/lifecycle tests, the pure-Go build, frontend
typecheck/localization, 1065 frontend tests and the canonical build pass.
All 13 original branch/PR checks, including Windows packaging, passed at
`d504404f`. The final release-preparation head must pass the same CI before
merge, and merged-main CI must pass before tagging.

Release preparation repeats the architecture and motion suites and the
installer tests on Windows PowerShell 5.1 and PowerShell 7; all pass. The
future-version rejection moves to alpha.77 while alpha.65 remains rejected.

The retained visual review includes 19 steady comparisons, all 180 transition
comparisons with the rejected envelope, the rapid-edit reproduction, and the
132-case release matrix (122 distinct plots). Additional regressions cover
108 maximum-to-gentle joins across three Handy profiles, interior-turn
preservation, 18 hard 50/110 ms timing-floor cases and explicit rejection of
an unsupported 200 ms floor. A 350 ms fake transport keeps one playback stream
and stops correctly. The 11.26 ms contended Stop result and 9.85 ms demanding
Edge benchmark are host measurements, not universal bounds or physical Stop
measurements. The complete evidence remains ignored under
`.scratch/layered-followup/`.

The matched stripped Windows core is 24,292,864 B (+4,608 B); canonical UI is
2,841,582 B (+734 B), with main JS +112 B gzip. No new dependency, background
loop or inference request is introduced. These stripped measurements are
budget comparisons; release binaries retain symbols for exact-binary scanning.

The isolated implementation review at `127.0.0.1:50304` uses simulated motion
and available Ollama `huihui_ai/granite4.1-abliterated:3b`, without copied
credentials. It passed real provider generation and a one-call text-only chat
in 106 ms, without repair, fallback or motion. LLM motion and Autopilot are
off and playback is stopped. Final release handoff requires a current-source
review build with a fresh real-generation probe and a retained stopped browser
view; existing review and user sessions are preserved.

The release-preparation pure-Go review build succeeds. Automatic approval
rejects starting its parallel instance at 50305 with only "blocked by policy";
that launch is not retried. The existing 50304 app is preserved instead:
`cmd/`, `internal/` and `web/` are identical to its tested implementation
source. A fresh provider generation succeeds, and a retained browser tab
confirms the existing nonempty text-only chat, Motion simulator, Idle,
Autopilot Off and LLM motion Off. Non-embedded release documents and scripts
do not change its application code. This is not a claim that a new process
was launched successfully.

## Remaining acceptance and publication policy

No hardware is connected or commanded during this release preparation.
New gentle Edge and rapid-edit feel awaits a matched physical comparison.
The installed-app versus matched-test full-range speed discrepancy remains
unresolved. A transport whose minimum interval exceeds a true stroke still
rejects it; automatic transport-aware retiming is separate work. Existing
Intiface Stop limitations remain disclosed.

Only `0.1.0-alpha.76` is added to ADR 0014's reviewed unsigned allowlist.
Alpha.65 and alpha.77 remain rejected. The payload remains five executables
and exactly setup/portable/checksum public assets. Source and exact-binary
vulnerability checks, Defender scanning, manifest/outer checksums, installer
lifecycle, clean source and current-main provenance remain mandatory.
Publication is through the immutable tag workflow after green review and CI;
Microsoft's prior case does not approve these new hashes. Trusted signing
remains the long-term requirement.
