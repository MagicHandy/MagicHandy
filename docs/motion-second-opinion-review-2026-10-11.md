# Local motion with an asynchronous second opinion

Date: 2026-10-11. Branch: `codex/asynchronous-motion-review`.

## Result

Keep the local action immediate and use a direct, conservatively admitted cloud
correction. Do not ask the local model to interpret another hidden prompt.
The feature is optional, covers local Chat and Autopilot, and reuses existing
motion modes and the single engine. See [ADR 0040](decisions/0040-asynchronous-motion-second-opinion.md).

## Comparison

Reviewer: the existing signed-in ChatGPT account's `gpt-6-sol`, Low reasoning.
Local model: `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` on Ollama.
Inputs include retained production failures, correct production outputs, continuing
Autopilot pace/region requests, permitted development, requested holds, and a
motion-independent question. No physical device was connected or commanded.

| Candidate run, 33 cases | Direct correction | Feedback sent back to Gemma |
| --- | --- | --- |
| Material defects corrected | 5 / 5 | 3 / 5 |
| Acceptable choices left alone | 28 / 28 | Only the five findings were retried |
| Extra generations per finding | 1 cloud | 1 cloud plus 1 local |
| Cloud review latency | Median 3.070 s; maximum 6.359 s | Same review, then another generation |

Two compound corrections explain the difference: the local output stayed in a
tip-only band after a request for broad strokes plus shrinking base rebounds.
GPT restored the broad band and base focus. Gemma's feedback attempts restored
the range but retained a tip focus, while their replies claimed to have fixed it.
The third retained local defect was an omitted inertia change. The synthetic
Autopilot defects violated a continuing slow-pace request and a 75–100 region.

The first prototype is retained, including one provider error and four unwanted
seed-only interventions. Supplying the current seed, making the material-defect
criterion explicit, and rejecting seed-only corrections removed these in the
candidate run. Three initial fixture failures were mislabeled: the actual compiled
tip reach was valid even with broad global limits. The candidate fixture checks
compiled reach, rather than assuming one representation is the only valid one.
Thus the initial 25/33 fixture count is not an accuracy estimate. These small,
partly calibrated cases do not establish a general false-positive rate.

## Real app-path check

Two subsequent runs use real Gemma and ChatGPT completions through the app, with
the shared engine dispatching only to the fake transport. Each run includes
three chat requests, a scheduled Autopilot decision, and Stop. All six chat
choices were kept. In both runs Autopilot changed an explicitly retained pace of
18; the reviewer corrected it after 4.129 s and 3.845 s, respectively. The second
capture includes every local target before review as well as the accepted result.
This demonstrates an actual improvement over the local choice without putting
cloud latency in front of that choice. It does not demonstrate subjective comfort.

The model's evaluation input and the local prompt remain separate. The reviewer
can receive explicit user wording because it must interpret the request; a refusal
is an unavailable second opinion and does not invalidate the local reply or motion.
No new refusal bypass, transcript rewrite, persona transfer, or local repair loop
is introduced by this feature.

## Motion inspection

Artifacts are ignored under `.scratch/second-opinion/` in the review worktree:
`prototype.json`, `candidate.json`, `atlas-trials.json`, `live-app.json`,
`live-app-2.json`, and `atlas-final/index.html`. The atlas retains 157 records,
59 distinct steady-motion plots, four overview sheets, and two captured transport
timelines. Failed selections and feedback attempts remain visible.

The complete overview shows the corrected base rebounds reaching the base and
mixing longer excursions, while failed feedback remains concentrated at the tip.
The upper-quarter correction compiles inside 75–100. Pace corrections keep the
character while lengthening the timing. Detailed plots compare plan and quantized
wire points, phase/velocity, acceleration, and seams. The existing engine's
geometry/jerk/velocity limits still reduce effective pace for some choices; the
reviewer is explicitly told not to compensate for that with higher requested pace.
The fake dispatch timelines retain startup, retarget and Stop. Queued samples
beyond Stop are canceled, not evidence of later physical movement.

Reproduce the replay with `go run -tags magichandy_labs
./cmd/motion-second-opinion-review -data-dir <existing-account-profile>
-captures <retained-production-capture-directory> -feedback -output <ignored-json>`.
The profile is opened through the protected token source, not copied into test data.
The live app test is `go test -tags "liveeval magichandy_labs" ./internal/httpapi
-run TestMotionReviewLiveApp -count=1 -v`, with `MAGICHANDY_LIVE_MODEL`,
`MAGICHANDY_LOCAL_MODEL`, `MAGICHANDY_HOSTED_DATA_DIR`, and
`MAGICHANDY_EXPERIMENT_CAPTURE` set to the models/profile/ignored output above.
Render the captures with the standard [motion visual review](motion-visual-review.md) tooling.

## Admission and regression checks

Tests exercise immediate local completion while the reviewer is blocked; a
correction on the same stream; both chat and scheduled Autopilot; disabled review;
strict output parsing; cosmetic reseeds; and late results after Stop, pause,
replacement, new chat/session, settings or credential changes, and cancellation.
Existing controller, cloud-admission, engine, and goroutine-lifecycle gates remain
in place. Configuration preserves omitted fields for older clients and rejects a
missing assigned reviewer. UI tests cover opt-in, sharing copy, unchanged primary
routing, draft discard, and turning review off when its connection is removed.

The standalone review app uses an isolated data directory and a real local model.
Its readiness probe and a Motion-Off chat request both return real text without
repair/fallback or motion. Existing signed-in application sessions are preserved;
their credentials are not copied into this review profile.
