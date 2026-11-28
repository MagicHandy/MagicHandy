# Edge responsiveness and feedback

Windows evaluation against v0.1.0-alpha.91, November 27, 2026. All device
measurements used simulators. The inference trials used real local models and
the signed-in ChatGPT provider.

## Motion and control

With a Handy and vibrator selected, alpha.91 could start the idle slider during
vibration-only playback. Both Edge actions now reject that session before
canceling chat, and the engine also guards the linear-start path. The desktop
and phone consume the same availability and playback-phase facts. Hold still
uses ordinary Pause. Return resumes that specific paused motion, subject to
the controller, Stop generation and current settings.

The ordinary moving hold still chooses a pace inside the saved limits. It uses
a shorter span only when the original motion's commanded travel and stroke
length leave room for a lower reversal rate. Creative retains its working span.
The shared fitter verifies peak speed and reversal rate with a 0.5% allowance
for timing quantization. If the minimum pace prevents a suitable moving hold,
the control offers Hold still. Fitting four identical steady cycles keeps the
editable score's usual loop length for later changes.

| Full-width steady input | Before Edge | Alpha.91 hold | Updated hold |
| --- | ---: | ---: | ---: |
| 20% starting pace | 56 turns/min | 112 turns/min | 56 turns/min |
| 35% starting pace | 86 turns/min | 112 turns/min | 56 turns/min |

On a deterministic due-refill press, alpha.91 added an extra second of the
prior pattern. The first editable point moved from 1,500 ms after the press
to 500 ms with the new order. The accepted or in-flight tail remains binding.
The UI distinguishes that queued interval from deceleration and steady holding.
The separate still action canceled a blocked Cloud append in the simulator.

Healthy Cloud simulations at 35%, 50% and 80% used 32 press times per pace.
With the shipping refill budget, median times to a sustained 25% reduction in
commanded travel were 2.68, 2.37 and 2.49 seconds. Alpha.91 measured 2.53,
2.37 and 2.49 seconds. At 35% the wider, slower-turning hold needs a longer
transition. Smaller refill trials sometimes improved the median. At 50% a
500 ms target increased active requests from 59 to 117 per minute. The update
retains the existing healthy Cloud refill budget and starvation checks.
Physical Cloud and Bluetooth onset still need measurement.

## Model qualification

The baseline used the same real mechanical state with the previous technical
planner request. The candidate supplied same-chat button facts and actual
human remarks on the next press. Normal chat kept its existing schema.

| Profile | Observed result | Shipped feedback |
| --- | --- | --- |
| Signed-in GPT-6.1 Terra, Advanced, Low reasoning | 24/24 planner cases passed across eight scenarios. Repeated simulator conversations preserved the current plan and returned the requested next hold of 76 seconds at 24%. | Enabled |
| Signed-in GPT-6.1 Sol, Advanced, Low reasoning | 24/24 planner cases passed. All six checks in the full simulator conversation passed, including the 20% read-back and a separate current edit to 42%. | Enabled |
| Catalog Gemma 4 12B, Intermediate | Prompt variants followed exact single requests, but could lose a newer correction or reuse an unrelated duration. A separate relevance call also missed cases. | Existing contract |
| Catalog Gemma 4 E4B, Simple | Some trials lost the exact next-hold request after intervening chat. One candidate conversation replaced the active motion before Return. | Existing contract |

Any model assigned to Advanced (with reasoning) is eligible. An integration
test inspects the request to a separately selected compatible model with a
Simple conversation model, then checks limited sharing, a lower-tier planner,
and a chat switch. Each role uses its own configured capability.

Manually assigning the catalog Gemma 4 12B to Advanced also enabled feedback.
On the native llama.cpp route it passed five of six conversation checks,
including the 76-second hold with a 24% return, compared with one of six in
the matched baseline. Its first feedback acknowledgment produced an invalid
motion edit which the app rejected. On the same 32K server using a compatible
connection, automatic structured output passed two of six checks. Prompt-only
output passed zero of six. The automatic run acknowledged the exact request
but never produced an accepted return plan. These runs explain why the catalog
continues to recommend Intermediate for 12B.

The fixtures cover implicit relief, exact requests, later corrections,
read-backs, unrelated numbers, and a present-tense request to set the current
pace. With the 76-second, 24% plan active, the only new human message in a
three-press trial was “That was too close.” Both Terra and Sol acknowledged
it without changing the running plan. After an early manual Return, both chose
110 seconds and a gentle 20% return on the next press. These were model choices
observed in this trial. Terra's feedback evaluation used one planner generation. A valid
answer is applied directly within the existing bounds. Returning early,
asking for the same duration, and choosing a different duration are all valid.
The model uses previous settings as evidence when the person comments on them.
[Model routing and retention](decisions/0076-edge-feedback-in-the-planning-role.md)

## Reproduction

Run the Edge, HoldingTarget and CloudRefill Go tests and the EdgeButton frontend
tests. For inference, select a signed-in ChatGPT model or loopback catalog Gemma using
the live-evaluation environment. Run `TestLiveEdgeFeedback` with the `liveeval`
tag and `TestLiveEdgeConversation` with `liveeval,magichandy_labs`. Set
`MAGICHANDY_EDGE_FEEDBACK_EVAL=1`. The baseline uses
`MAGICHANDY_EDGE_BASELINE=1`. Set `MAGICHANDY_EDGE_REQUIRE_QUALITY=1` to
assert the qualified behavior. `MAGICHANDY_EXPERIMENT_CAPTURE` selects a local
output file. For another signed-in model, set `MAGICHANDY_EDGE_HOSTED_MODEL`
for the conversation fixture and `MAGICHANDY_LIVE_MODEL` for the planner
fixture. Set `MAGICHANDY_EDGE_REPEAT_PRESS=1` to test a third press after
the user replies “That was too close.” The conversation fixture drives the
streaming chat endpoint, the real planning adapter and the shared engine
against a fake transport.

Across the prompt experiments, 423 decisions were retained, including three
rejected outputs. Grouping identical compiled geometry yielded 47 atlas entries
and 140 entry or return transitions. All 140 used bounded joins and the atlas
found zero extra reversals below one percentage point. The highest sampled
acceleration was a 90% stepped input entering the hold at 17% of the original
cycle. The signed velocity briefly reached 341 percentage points of stroke/s
near that turn before settling into the slower hold. The steady peak check
describes the fitted hold, and the entry plots show this transition separately.
The local review retained the overview and detailed curves under
`.scratch/edge/final-atlas/`, the transition samples in
`.scratch/edge/final-handoffs.json` and the comparison in
`.scratch/edge/edge-comparison.png`. Use `TestEdgeVisualReview` and the
motion atlas renderer to generate the waveforms.
[Visual review procedure](motion-visual-review.md)
