# Alpha.83 release integration review — 2026-11-09

## Scope and source

The owner requested review of Claude's recent changes and actual publication,
using Updates for release integration. The base is released alpha.82
`98f2ea83c70f43d8999a4e6a56af15900eb2cea9`. Claude PR #363
(`08cc027672ac979af0a32d49c36563b9688ab437`) concerns Edge cancellation
messages; PR #364 (`120e40188080a4e32e62d52df6c863df06fcf448`) concerns
Advanced Layered Autopilot direction and recent-score observations.

The completed source-review handoff is
`f2819daf7ef88fe711c256e25481b1b40e94250f`, with both Claude commits preserved
through merge commits `7b796fb4` and `9a03f8c0`, followed by the review fixes.
Updates prepared release policy separately and integrated the accepted source
after that handoff. Original PRs are superseded only when their changes reach
main. See the [source review](autopilot-followthrough-review-2026-11-08.md) and
[ADR 0066](decisions/0066-hosted-planning-direction-and-recent-scores.md).

## Behavior and review corrections

Edge retains its existing cancellation/stale-work boundary and now supplies a
typed cause. Chat and the phone remote distinguish a reply interrupted before
save from a saved reply whose motion and speech were canceled. Emergency Stop
still takes precedence. Quick speed/stroke settings changes keep retiring
stale replies and their diagnostic cause now names model or motion settings.

Advanced Layered planning sees at most three distinct scores that finished
within three minutes, as observations without recall IDs or conversation text.
Prompt guidance retains a named direction and requested stroke qualities across
variations. Advanced live chat separates immediate action from a later cue and
uses counted phrases for ordered progressions. Advanced capability includes
Advanced local connections; Simple/Intermediate prompt bytes and Creative v2
planning remain unchanged. Auto timing remains 8–300 seconds. The compiler,
timing fit, point sampler, transports, limits and single motion path are intact.

Source review corrected the history window to expire a score three minutes
after it finishes, so a long Auto timing stretch remains recent immediately
after replacement. Boundary and five-minute-duration tests cover the case.
It also corrected the synthetic evaluator: any failed model call fails the
scenario instead of counting unchanged low pace as a successful easing/hold.
The scripted regression retains both failed turns and their errors. Capability
tests/docs now cover both compact tiers and Advanced local connections.

The first integrated branch CI race run exposed missing rejected-attempt
diagnostics after a failed automatic correction, although the duplicate PR
run and accepted source run passed. The original failure log is retained.
A controlled 750 ms SQLite writer hold reproduced the loss under the existing
500 ms diagnostic deadline. Completed correction attempts now receive the
same bounded two-second persistence allowance as manual repair offers. The
regression preserves both rejected responses and their failed outcome; blocked
writer cases still return within the request bound without another model call,
reply publication or motion. Three focused race repetitions passed after the
fix. No retention assertion or CI gate was weakened.

## Evidence and limits

Fresh production ChatGPT GPT-6.1 Terra / low reasoning passed all 17 synthetic
follow-through turns without provider or validation failures, plus a separate
initial chat smoke. The chat scenario changed 20 to 32%; a separate planner
scenario started at 20% and selected 28/35/70/82/92%. These are separate
synthetic starting states, not one continuous chat-to-Autopilot run. Easing
selected 18/22/16/21%. Six invented scores stayed at
100% with six geometries and no A/B returns. Ordered widening produced six
centered counted groups from 45–55 through 20–80. These are observed model
choices, not hard-coded targets or guarantees. Seventeen-turn median/mean/max
generation latency was 5.554/6.777/13.657 seconds.

All 18 captures were compiled through the shared engine into 17 distinct plots
and two overview sheets. Source review inspected both overviews and five
detailed outliers; the exporter retains every accepted case. Distinct widths,
anchored returns, ordered widening and the planned/wire overlay remain visible,
with no new steady-motion discontinuity concern. A 35-to-70% pace choice still
occurred; these plots do not establish physical transition feel. Original
default-suite before/after evidence, failures and unrun opt-in rows remain
recorded in the source review. No new hardware acceptance is claimed.

Full-package chat/chatapp/modes/httpapi tests, focused cancellation/history race
tests with LLVM-MinGW, the scripted evaluation-failure regression and the
CGO-free app build passed in source review. Updates requires all mandatory full
Go/race/lint/architecture/frontend/installer gates on the final release PR,
merged main and tag; focused evidence does not replace those gates.

The user-launched exact-source simulator at `http://127.0.0.1:50354/#/chat`
uses isolated data and local Gemma 12B. A real generation probe passed; app
chat returned non-empty text in 483 ms with one provider call and no
repair/fallback/motion. Browser testing verified Edge's explanation during a
streamed reply, then Emergency Stop left simulated playback idle. Updates
initially reused that process while production source matched the handoff.
The later diagnostic-persistence correction requires a parallel isolated
simulator at `http://127.0.0.1:50355/#/chat`, with local Gemma, LLM motion Off
and Autopilot Off. Updates prepares that app and requires real generation,
text-only app chat and visible idle-state verification before handoff; the
user-launched source simulator is preserved.

## Budget

Windows amd64 Go 1.26.10, `CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags
'-s -w'`: core plus embedded UI **24,478,720 -> 24,490,496 B** (+11,776 B).
Canonical UI **2,884,094 -> 2,885,533 B** (+1,439 B), still nine files. Main
JS grows 262 B (+43 B gzip); CSS is unchanged. No dependency, worker or inference
round is added. Shipping binaries retain symbols for exact scans. See the
[scorecard](goal-scorecard.md). Source review's post-test dev memory sample is
not a release RSS or cold-start improvement claim.

## Release preparation

Only alpha.83 joins the reviewed unsigned public policy; alpha.65 stays
excluded and the negative version test advances to alpha.84. The active
packaging/version documentation and ADR 0014 move with that version. This
does not change any CI workflow, vulnerability scanner pin, compiler/runtime
pin, native x64/non-solid packaging contract or mandatory safety gate.

Updates requires green checks on the final reviewed PR, merged main and immutable
tag, followed by independent verification of the three public downloads.
Source/five-binary vulnerability scans, exact-artifact Defender, full installer
lifecycle, manifests/checksums and exact-current-main provenance remain
mandatory. The old Microsoft case does not approve the new binary hashes.

Installed app/data and user-launched sessions are preserved. This release
task authorizes no hardware command, installed-session restart, credential
copying or repeat of the earlier private data recovery. Runtime and private
evaluation artifacts remain ignored under `.scratch/`.
