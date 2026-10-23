# Immediate holding control

Edge sits above permanent Stop. It applies a backend-generated gentle semantic
target to the existing engine without inference or a second playback loop.
Return is separate: repeated Edge presses keep holding and cancel automatic
return. Stop retains its independent path and precedence.

The hold lowers pace inside saved limits, removes modulation and softens turns.
It uses the current working region's midpoint, independent of the click phase.
Creative v2 retains its working width: changing its grammar and narrowing the
range simultaneously produced an unwanted reversal during braking. Other live
targets use a narrower region. A high saved minimum can prevent a pace reduction.
Video and finite programs are excluded; Edge never starts idle/paused motion.

## Autopilot and lifecycle

If Autopilot was active, its existing loop makes one compact request after the
hold is applied. The model chooses 8–180 seconds, a bounded return pace, and
previous or gentle geometry for the next stretch. The technical request states
that this is an adult device, contains no conversation/persona, and uses the
saved planning provider. Normal prompts, local context and inference counts are
unchanged. Decisions API retains a manual hold because this contract is not yet
supported there; it never silently selects another provider.

Refusal, invalid output or timeout leaves a manual hold, without a repair loop.
Stop, pause, newer chat or settings revoke automatic return. Explicit chat motion
uses normal adoption. Disconnects clear the hold and latch recovery off. A
limits-only refresh adopts the new plan ID and cancels automatic return; another
target takes ownership. Return uses the current plan-ID guard and live limits.
Autopilot adopts its returned pace into normal history/cadence; manual sources
do not gain recovery keepalive by using Edge.

The countdown starts after the decision arrives, conservatively extending the
hold by inference time. A fake-transport API run accepted Edge in about **4 ms**
with about **2 seconds of points buffered**. This is immediate relative to
inference, not a zero-latency physical stop. A difficult fast turn may continue
its authored path for up to one extra second to find a bounded smooth join.
Stop remains the interruption control. No real hardware was connected or moved.

## Claude consultation

Claude Opus 5.5 at max effort reviewed the proposal via CLI. Its findings led to
separate Edge/Return actions, idempotence, post-apply reconciliation, nonblocking
automatic-return admission, preserved keepalive eligibility and proper return
ownership/history. The model chooses scalars, not a speculative curve. Invalid
numbers are rejected rather than silently clamped. A timed return ladder or
queued-point replacement remains a separate future evaluation. Ordinary Creative
edits retain their existing transition policy.

## Wording and motion evaluations

Three truthful descriptions—explicit “edging”, a gentle counterpoint and a gentle
backup—were tested at previous paces 15, 50 and 90 for each model.

| Model | Valid | Refusals | Response time | Durations |
| --- | ---: | ---: | --- | --- |
| ChatGPT `gpt-6-sol`, reasoning none | 9/9 | 0/9 | 2.02–3.95 s | 45–60 s |
| Local Gemma 4 12B Q4 | 9/9 | 0/9 | ~0.48–0.50 s warm; 4.14 s first call | 45–120 s |

All eighteen chose gentle geometry. ChatGPT return paces were 6–25, local Gemma
10–40, inside the trial bounds. Production uses the functional counterpoint
description for clarity. This small test established neither a refusal problem
with “edging” nor a statistically reliable wording latency advantage.

Five motion variants covered four families, three paces, four phases,
entry/return and all accepted model proposals: **168 cases per variant, 840
rendered paths**. Initial transitions had 120 bounded joins and one tiny extra
reversal. Extending the existing bounded join only for explicit holds helped,
but did not solve the fast Creative outlier. Retaining Creative's working width
removed the tiny reversal. Detailed plots showed the simpler version still
inserted a larger unwanted turn. Brief bounded continuation of the authored path
until a feasible join removed that artifact. The selected `creative-region`
variant used **164/168 bounded joins and no tiny extra reversals**. Remaining
cases retain the existing fallback; no acceleration or jerk bounds were relaxed.

All six comparison overview sheets and four outlier details were inspected.
Sixty steady entries compiled through the shared fitter/sampler to 26 distinct
plots and two overview sheets, including quantized wire points. Holds are soft,
stable repeating sweeps. Every model result is retained. These are commanded
estimates; physical feedback remains necessary before release.

Ignored evidence is under `.scratch/edge-hold/`: both wording reports, Claude's
review, five `*-handoffs.json`/`*-atlas.json` variants, `handoff-plots/`,
`selected-atlas/` and test logs. Reproduce with `TestEdgeWordingLive` (`liveeval`)
and `TestEdgeVisualReview` (`magichandy_labs`), using their environment variables.

## Validation

Regressions cover immediate application, manual/automatic return, idempotence,
late decisions after Stop/pause/chat/settings, rejected plans, disconnects,
changed plan IDs, source ownership, finite-media rejection, one shared stream
and Stop. UI tests keep Stop usable while Edge is pending/read-only/offline and
keep Autopilot Pause available during a hold. The isolated simulator exercised
real local inference, the return countdown and resumed normal planning.

Full Go tests, vet, lint, pure-Go build, frontend tests, typecheck/localization,
the review-model probe and a real text-only chat are checked. Windows race
execution needs GCC, unavailable on this host; Linux CI provides that gate.
