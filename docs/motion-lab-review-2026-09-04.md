# Natural motion review and Motion Lab

**Historical first experiment.** The user's subsequent physical feedback and
the independent generator/separate LLM Lab are covered in
[the redesign review](motion-lab-redesign-2026-09-04.md), which supersedes this
panel's four visible methods. Both labs now require a development build.

## Findings from prior complaints

This review follows the tasks **Improve motion control schema** and **Improve
motion control schema (2)**, release history through alpha.39, ADR 0015, the
calibration/retargeting specs, and the actual Creative compiler, parser and
provider harnesses. The strongest next step is a controlled comparison of
independent expressive axes. Another global smoothing pass would not address
all the reports.

| Report / previous attempt | What the history and code support | Remaining distinction |
| --- | --- | --- |
| Robotic, linear travel and sudden direction changes | Short reversal guides became whole-leg PCHIP, then C2 quintics with exact acceleration/jerk fitting. Continuous velocity alone did not guarantee continuous acceleration. | Smooth mathematics does not prove comfortable physical output. Endpoint quantization and carriage response remain relevant. |
| Stroke lengths too regular | Four-cycle variation became longer seeded envelopes, locally varying wander/contrast, then section phrases. | A long period and high global variance can coexist with locally repetitive strokes. Inspect the worst 12-second window. |
| Same pattern despite maximum change rate | Prompt context gained phrase age, holds, reconsiderations and compiled perceptual summaries. | Reconsideration is not an obligation to change. Faster calls cannot fix an inexpressive vocabulary. |
| Avoids full range, especially the base | Named base/tip anchors are inset at 8/92. Center/span can reach further, but variable spans contract around a center that may also drift. | A wide outer window does not mean returning to one endpoint on every stroke. That needs a separate control. |
| 73% or maximum feels slow | Calibration and local fitting corrected a peak-versus-mean mismatch and removed the 500 ms Creative loop floor. | Safety saturation still separates requested from achieved pace. Pace-priority fitting can also erase timing asymmetry. |
| Directions ignored or described without action | Authorization, coverage, omitted-field preservation and repair were strengthened. | Valid JSON is insufficient: the accepted command must compile into the requested change. Inherited span floors can still conflict with narrowed geometry. |
| Pause resumes or startup repeatedly retries | Earlier releases addressed separate lifecycle defects. | These are safety/control failures, not evidence for increasing variation or adding a transport bypass. |

Past successful matrices establish particular fixes, not that natural feel was
solved. Earlier user requests explicitly opposed forced full-range use,
automatic changes and accumulating phrase-specific prompt rules. This work
preserves those constraints.

The existing full-length coverage test accepts a window extending to at least
the 12/88 endpoint bands. A passing correction therefore does not prove literal
0–100 travel, nor repeated contact with an endpoint under a shrinking envelope.
The lab reports compiled reach instead of treating that language check as a
physical coverage measurement.

## Control hypotheses and iteration

**Range anchoring** separates where shorter strokes return from the outer
window and length envelope. For normalized outer window `[lo, hi]`, width `W`,
current span `s`, route fraction `u`, and anchor `a` in `[0,1]`, the position is:

`lo + a * (W - s) + u * s`

At zero the lower endpoint stays fixed, at one the upper endpoint stays fixed,
and at one-half the existing centered contraction is recovered. Existing slow
center drift is multiplied by `4*a*(1-a)` so it cannot undo endpoint anchoring.
This transforms semantic knots coherently; it adds no per-sample noise.

**Directional timing** separates outbound/return timing from geometry. An
outbound share `p` in `[0.25,0.75]` weights authored durations by `2*p` and
`2*(1-p)`. The initial prototype reused pace-priority fitting; a requested
35/65 split approached 50/50 at saturation. A second prototype stretched the
whole phrase, but one difficult stroke slowed every other stroke. The final
experiment stretches each complete two-leg stroke together, repeatedly checks
the existing exact C2 envelope, and accepts lower mean pace. Ordinary Creative
retains its local pace-priority policy.

Settings > Motion Lab compares Creative baseline, anchored range, directional
timing and combined controls with the same seed and saved limits. Both neutral
values are 50; they normalize to an absent experiment, and tests assert exact
baseline sample equality. Experimental fields survive only on `motion_lab`
targets. Chat and other sources strip them. Multi-section and named-route
composition are not exposed by this first experimental panel.

Further candidates worth evaluating after this comparison:

- Independent endpoint envelopes could express reach more directly than
  coupled center/span drift. Anchoring is a smaller test before adding two
  complete envelopes to the model vocabulary.
- Existing section phrases could gain an explicit rhythm priority, with
  carefully defined section-boundary and retarget behavior.
- Receding-horizon position/velocity/acceleration intent could make handoffs
  more expressive, but must remain behind the shared engine and bounded state.
- Feedback-calibrated timing could distinguish planned pace, wire
  approximation and physical response. A semantic preview cannot infer those
  measurements.

The [Flash and Hogan minimum-jerk study](https://pubmed.ncbi.nlm.nih.gov/4020415/)
motivates considering smooth derivatives in human movement. It studied human
arm movement and does not establish an optimal profile for this device.
[Ruckig's trajectory documentation](https://docs.ruckig.com/tutorial.html)
illustrates separate velocity, acceleration and jerk constraints and why an
arbitrary requested duration may be infeasible. These are design references;
no native library or second physical control loop was introduced.

## Preview, audition and export

`POST /api/motion/lab/preview` accepts bounded semantic controls and returns
four compiled plans, a sorted 12-second excerpt including exact reversals,
full-phrase diagnostics and saved motion settings. Read-only clients may use
it. Preview does not create an engine, claim control, change settings or contact
a device. The graph is an initial excerpt, not a compressed full phrase.

Position and estimated velocity are before transport fitting. Acceleration and
jerk extrema come from the compiled curve. Reach is semantic 0–100; the
existing stroke-window mapping occurs once at the transport boundary. The
panel labels these as planned estimates, excluding feedback and wire rounding.

Start selected test uses the existing `/api/motion/start` controller admission,
mode teardown, engine and Stop lifecycle. A settings fingerprint prevents
auditioning a preview made with different limits/calibration. Tests repeat until
Stop; changing preview controls alone does not retarget live output. Emergency
Stop stays permanently mounted, including for offline/read-only clients.

The export includes request, seed, settings, computed results, optional LLM
trial, notes and a labeled current latency observation. It contains no device
connection key, provider credential, personal transcript or physical-feedback
claim. Runtime exports are not source files.

## Prompt and harness evidence

The optional lab LLM trial uses an isolated three-field contract:
`range_anchor_percent`, `outbound_time_percent`, `explanation`. Fixed phrase
data and editable starting values are separated. The proposal changes only the
preview; it neither appends chat nor alters settings or motion. The backend
derives the method name from neutral/non-neutral controls.

The initial contract also asked the model for a method label. A combined case
chose suitable numbers but labeled them directional. Removing this redundant
field eliminated a needless model decision. Separating fixed/editable context
then corrected a case that preserved a base anchor despite a centered-range
request. No phrase recognizer or deterministic taste policy was added. The
endpoint makes one call; invalid proposals remain errors without repair or
fallback. Existing interactive-work registration cancels queued/generating
trials on Stop.

Harness corrections:

- `MAGICHANDY_LIVE_MODEL` explicitly selects and verifies a model instead of
  depending on discovery order.
- `MAGICHANDY_LIVE_PROVIDER=ollama` uses the native protocol with `think:false`.
  llama.cpp options sent to Ollama's compatibility endpoint exhausted the
  visible-output budget in reasoning. The review-readiness PowerShell probe
  now uses the native Ollama protocol too.
- Variable-span acceptance examines complete section phrases instead of
  falsely rejecting them for lacking a top-level envelope.

Production Creative examples now follow lifecycle state: running examples use
update, stopped examples omit update, paused examples omit starts/updates.
None and Stop remain available. A complete nested reply/motion envelope is
shown. This reduces conflicting examples without choosing motion for the model.

The final recorded model run used the available local
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` through native Ollama:

- **Lab: 15/15**, three repetitions each of base, tip, timing, combined and
  neutral controls. Each includes strict parsing and real curve compilation.
  Generation took roughly 0.7–0.9 seconds per case.
- The existing range-envelope, four section cases and three correction cases
  passed in the final recorded run. The general Creative matrix retained two
  failures: one negated request needed nesting repair; one narrow focus update
  conflicted with its inherited larger span floor and remained malformed.
  These findings are retained rather than weakening acceptance or forcing
  outputs to satisfy a score.
- The reproduced full-length correction produced an accepted update after the
  lifecycle-example change. Behavior remains stochastic; this is not a
  guarantee for every subsequent turn.

No managed llama.cpp endpoint was evaluated in this run. Ollama results do not
establish parity for all providers/models.

## Compiled comparison and physical acceptance

One deterministic comparison used Original Handy calibration, center 50, outer
span 90, floor 25, wander, variation 65, seed 17 and outbound share 35:

| Requested pace | Baseline effective | Directional effective | Directional outbound share |
| --- | --- | --- | --- |
| 10 | 10.0 | 10.0 | 35.0% |
| 35 | 35.0 | 31.9 | 34.7% |
| 75 | 64.4 | 33.9 | 34.6% |
| 100 | 64.4 | 34.0 | 34.6% |

These are simulation results, not speed recommendations. A strong asymmetric
rhythm costs mean travel near saturation. Easing and physical limits do not
permit every combination of pace and rhythm.

Checks cover neutral equivalence, fixed endpoints, direction ratios, sorted
deterministic samples, exact runtime envelopes, preview inertness, controller
gating, stale settings, Stop teardown/cancellation and frontend stale-response
handling. Required full-suite checks are separate from optional model-quality
evaluations.

The initial review used an isolated `--simulate-motion` app with native Ollama.
That validation did not connect hardware or issue physical motion. Physical acceptance stays
open: compare one changed axis at the same limits/seed; record device model,
transport, requested/effective pace, latency, `motion_trace.v3`, continuity,
endpoint reach and subjective regularity. Include narrow slow strokes, broad
reach, Stop and a second transport where available. Promote controls to the
production LLM vocabulary only after that evidence.

The final browser check generated a combined proposal through the app in
approximately 0.8 seconds, then completed a text-only chat reply in 811 ms
with one provider call, no repair/fallback and motion `none`. A subsequent
combined-control audition used the in-process simulator and returned to idle
through the permanent Emergency Stop. Its runtime trace was exported to the
ignored scratch directory: `motion_trace.v3`, 10 rows, `fake_handy`, nine
commands ending in Stop, and 0 ms reported simulated latency. Transport
implementation, calibration constants,
configured limits, catalog curves and production Creative fitting were left
unchanged.

## Follow-up: connected device with no motion

The user connected the Handy through Cloud REST and started the directional
test, but the review process still had `--simulate-motion`. Cloud readiness
was healthy while every engine command was recorded by `fake_handy`. The
connection readout alone hid this distinction. This was a review-runtime
setup error, not evidence of a failed directional trajectory or Cloud dispatch.

The backend now publishes `motion_simulated` from the motion runtime. A
persistent Motion simulator readout, notices in Motion Lab and the connection
manager, and the explicit Start simulated test label expose this mode before
an audition. Comparison exports retain the flag. Regression tests cover
production's diagnostic fake fallback separately from an actual motion
simulation override, plus the UI disclosure and start label.

The simulated run was stopped before replacing the agent-owned process.
The same review datastore and URL were retained, preserving the user's saved
Cloud key and motion limits without copying credentials. The final review
process omits `--simulate-motion`, stays stopped, and uses Cloud REST. Its
read-only HSP readiness probe returned HTTP 200 in 612 ms. Native Ollama
readiness passed; a chat request with LLM motion temporarily Off returned a
non-empty reply in 289 ms, with one provider call and no repair/fallback.
Creative mode was restored afterward. The trace export for this readiness
check is `motion_trace.v3` with no motion rows; physical movement remains for
the user's explicit Start. The preview is restored to directional timing at
25% speed, 50% center, 90% outer / 25% shortest span, Wander, 30% variation,
35% outbound time and seed 17. Saved speed limits remain 10–43% and stroke
limits 0–100%. No trajectory, transport, calibration or safety-limit behavior
changed in this follow-up.
