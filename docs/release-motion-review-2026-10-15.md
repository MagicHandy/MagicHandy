# Release motion and model review — 2026-10-15

This review starts at `5938a0cb` on the native Handy/COYOTE integration stack.
The software fixes are isolated on `codex/release-motion-review`. Publication
remains pending the physical Cloud/Intiface comparison and release integration;
the software comparison below does not establish equal physical speed.

## Defects and corrections

1. **A valid Creative v2 target could fail strict compilation.** A tip-anchored
   target in the 75–100 band produced `100.00000000000001` through floating-point
   arithmetic. Seed 2947866601 reproduced the hosted failure. Generated landing
   endpoints are now bounded to their already-validated outer band. The strict
   downstream validator, velocity limits and transport rules are unchanged.
   Regression coverage includes 24 combinations of seeds, widths and anchors.
2. **Layered could promise full travel while retaining its old endpoints or
   shorter strokes.** `full_slider` explicitly sets semantic 0–100, fills that
   range, and removes spatial layers; `full_band` fills the existing band.
   Both preserve speed, seed and independent pace variation. Conflicting
   controls/widths/layers fail transactionally. These are model-selected edits,
   not interpretations of user words in application code.
3. **Examples fought constrained JSON generation.** A geometry-before-controls
   example encouraged Gemma to emit geometry while omitting the requested
   speed. Controls-first examples passed three repeated full-travel trials.
   The upstream [schema grammar implementation](https://github.com/ggml-org/llama.cpp/blob/master/common/json-schema-to-grammar.cpp)
   builds required and optional property sequences from schema order. This is
   consistent with the observed failure; it is not evidence that every
   compatible endpoint enforces that decoder. Interactive Autopilot examples
   now put the required standing-wish field after reply, preserve nested edit
   order, and correctly demonstrate an exact-repeat hold.
4. **Correct prose did not guarantee the requested controls.** Gemma sometimes
   repeated the existing inertia, used near-tip instead of anchored-tip
   placement, treated Layered's patch list as replacement, or disabled local
   reach while promising mixed rebounds. Revised contracts explain those
   dependencies and include a compact compound example. No language matcher,
   reply rewriting, hidden retry or extra inference call was added.
5. **Continuing playback could accidentally freeze Autopilot.** The prompt now
   distinguishes this turn's action from permission for future autonomous
   changes. It states whether the backend is already holding or composing.
   A standing hold is preserved across neutral feedback and can be released by
   the person's request. Example construction uses backend state and code-owned
   examples only; the model still interprets the person's language.
   Stopped Creative v2 also needs a valid compound start example: the final
   acceptance run exposed retained 5–95 endpoints despite a full-travel request.
   Adding the explicit range/focus/speed start example passed three repeated
   text/start/pace/Stop trials without a retry or fallback.
6. **The live prompt could exceed the minimum managed context.** The original
   budget test exercised the Lab contract rather than the larger interactive
   envelope. A new test covers stopped/running Layered and Creative v2 with a
   realistic request, 16,384 context and 256 output reserve. Repeated prose was
   shortened without raising context limits or weakening admission. Final live
   request sizes are Layered 10,936/11,568 B and Creative v2 11,229/11,610 B.
   This fixture does not promise room for arbitrary personas, memories, larger
   output reserves or Autopilot context at the minimum setting; normal managed
   context remains larger and existing admission remains authoritative.

## Real model evidence

All motion tests use the production service/compiler with an in-memory
transport, or compile proposals without constructing a transport. No real
device was commanded in this review. Local generation uses the installed
Gemma 4 12B Q4_0 model through Ollama. Hosted generation uses a signed-in
ChatGPT profile, GPT-6.1 Sol with Low reasoning, without copying credentials.

| Scenario | Result |
| --- | --- |
| Off, Pattern library, Creative, Layered and Creative v2 through HTTP | Compact matrix passes both providers. Final hosted matrix passes all five; local first four pass, then Creative v2 fails full-travel start and passes three repeated mode runs after the example fix. Normal turns use one call without repair/fallback; Stop uses none |
| Detailed Layered conversation | Four local edits, one generated continuation, one uninterrupted transport play, then Stop |
| Detailed Creative v2 conversation | Five edits, one generated continuation, one transport play, then Stop on local and hosted providers |
| 29 varied local requests | Initial 27/29 semantic passes; final 29/29, all structurally valid; quoted/conditional starts and paused requests do not move |
| Ten hold/release cases across both continuous modes | Final local 10/10 and hosted 10/10; earlier local false holds and mistaken release remain in evidence |
| Hosted long conversation | 222 stored synthetic messages; corrected facts recalled in one 3.249 s request, with no motion |
| Asynchronous second opinion | Real local generation and hosted review exercised through chat and scheduled Autopilot against fake motion |
| Refusal retry | Injected hosted refusal followed by real local text generation; no motion |
| Text-only language handling | Six technical/conversational inputs per provider, including profanity/adult vocabulary; no movement or repair |

These are bounded regression results, not general language-accuracy estimates.
The pre-existing varied-request suite informed the prompt changes, so its final
score is no longer an untouched holdout. The wording and model configuration
from the original user's Qwen report were not available. Real OpenRouter and
separately billed OpenAI API accounts were not exercised; their protocol/error
paths retain automated coverage. Technical vocabulary passing does not imply
that a hosted model will accept every explicit story request.

The compact five-mode run measured median completed-generation diagnostics of
977 ms locally and 2,878.5 ms hosted (maxima 3,111 and 5,610 ms). These are small
host-specific samples including warm/load variation, not a benchmark of all
models. They support retaining immediate local execution and optional delayed
review; they do not justify replacing the device clock with cloud inference.

## Intiface comparison and remaining physical work

`TestTransportParityReview` captures 64 shared-engine cases: eight authored or
generated shapes, speeds 10/25/43/75, and Cloud/Intiface sampling constraints.
It uses Original Handy limits, a full stroke window and the same 12-second
measurement interval. Across the 32 pairs, the greatest average travel-rate
difference is **0.0782%**. This is planned/quantized travel, not measured carriage
speed and not a live WebSocket or Bluetooth dispatch measurement.

The 50 ms Intiface interval approximates rapid turns more coarsely. At 75%,
maximum position error is 3.249 percentage points for Creative and 3.059 for
varied Creative v2; corresponding RMS errors are 1.341 and 1.032 points.
This is a plausible contributor to different feel, but it does not establish
the reported speed-mapping cause. The production pacer, sampler interval,
speed curve and safety limits were deliberately not altered on this evidence.

Intiface Central 3.2.2 is installed, and the operator started its WebSocket
server on port 12345 for the pending hardware comparison. The test must use the same
Handy, pattern, speed and stroke window, record the active firmware/driver,
capture paced-send lateness and acknowledgements, and obtain the operator's
speed judgment. Repeat both full and restricted windows to distinguish wire
scheduling from a retained firmware stroke envelope. A retained envelope is a
hypothesis, not an established defect. Follow the <=40% hardware-test ceiling
in [the Intiface guide](intiface.md).

Earlier native Handy/COYOTE tests are documented in the
[linked hardware review](native-linked-hardware-review-2026-10-15.md); its Windows
worker-crash recovery limitation is not resolved by this change.

## Visual and automated checks

Ignored evidence is under `.scratch/release-motion-review/`. Raw proposals,
failed prompt iterations, SSE streams, accepted targets, fake-transport commands
and traces are retained. `release-atlas/index.html` contains 435 records,
130 distinct plots and nine overview sheets, including rejected/non-moving
turns; `transport-parity-atlas/` contains the 64 transport cases. Additional
final acceptance captures contain 46 records, 17 distinct plots and two overview
sheets, including the failed Creative v2 start and the three successful repeats.
Both acceptance overviews and the final start's detailed plot were inspected.
The atlas renderer consumes
actual shared-engine output, not reply text.

The overview sheets show corrected Layered full travel, preserved tip anchoring,
base-local rebound blends and unchanged geometry under pace-only edits.
Rejected iterations visibly retain the wrong 5–95 range or miss the requested
anchor. Detailed views compare quantized turns, velocity asymmetry, acceleration,
seams and the captured retarget/Stop timeline. Intiface's coarser turns remain
visible, without a systematic mean-speed reduction in the tested full-window
fixtures. None of these charts measures physical braking or electrical output.

Validation covers Go tests, race tests, vet, golangci-lint, architecture and
goroutine lifecycle gates, CGO-disabled Windows/Linux builds, and frontend
typecheck/build plus 901 tests in 117 files. The canonical frontend bundle is
unchanged. Live evaluations are opt-in and never run against hardware in CI.

The isolated current-source app at `http://127.0.0.1:50242/#/chat` uses the motion
simulator, has no device credential, and leaves Autopilot and LLM motion off.
`check-review-llm.ps1` succeeds, and a real text-only chat returns a non-empty
answer in one 301 ms request without repair/fallback. Its browser tab is left
open with the answer and idle motion visible. Build cost is recorded in the
[scorecard](goal-scorecard.md).
