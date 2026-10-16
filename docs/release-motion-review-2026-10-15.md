# Release motion and model review — 2026-10-15

This review starts at `5938a0cb` on the native Handy/COYOTE integration stack.
The software fixes are isolated on `codex/release-motion-review`. Publication
remains pending the Intiface timing/Stop findings and release integration. The
matched physical comparison reproduced hesitation and catch-up on Intiface;
equivalent planned travel does not establish equal physical speed.

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

The model evaluations use the production service/compiler with an in-memory
transport, or compile proposals without constructing a transport. They command
no real device; the separate physical runs below do. Local generation uses the installed
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

The first software matrix had 64 cases using a generic 50 ms Intiface floor;
its 32 Cloud/Intiface pairs differed in average quantized travel by at most
0.0782%. Discovery of the actual Handy exposed a 100 ms advertised limit, so
the review now covers 144 cases: eight shapes, speeds 10/20/25/35/43/75, and
Cloud, generic 50 ms Intiface, and the actual 110 ms interval including the
scheduler margin. Each uses Original Handy limits, full stroke and the same
12-second measurement interval. These are commands, not carriage telemetry.

### Physical comparison

Original Handy firmware 4.3.0 ran `flow-full-sweeps` at 20% and 35%, full stroke,
through Intiface Central 3.2.2+46 and Cloud. Runs lasted 10 seconds after startup
with a stopped pause; the device was unloaded and the test ceiling stayed 40%.
COYOTE stayed disconnected. The operator reported a slight speed difference
on Intiface, a clearer difference and smoother motion on Cloud, and repeated
Intiface slowdowns followed by brief catch-up. Repeating after Cloud reset the
firmware stroke window did not remove the problem. Restricted-window tests
remain open; the reset alone does not establish all projection semantics.

| Observation | Intiface | Cloud |
| --- | --- | --- |
| Start completion | 251–255 ms | 5.3–10.0 s including preparation |
| Stop acknowledgment | 2–3 ms | 306–447 ms |
| Shared-engine commanded mean at 20% / 35% | 93.20 / 143.06 percent-travel/s | Same shared plan |
| Local paced send lateness | Rounded maximum 0 ms | Buffered upload path |
| Rejected/timed-out/coalesced live commands or pacer failures | None recorded | No playback starvation recorded |

Intiface's mean command interval was approximately 137–153 ms. Sampled last ACKs
rounded to 0 ms; the final repeats retained a cumulative 1 ms maximum. These
establish server acceptance only. They do not measure Bluetooth
arrival, physical execution or motor Stop. The 35% half-cycle has only about
five commands, compared with roughly twenty buffered Cloud points. Large
encoded velocity steps and downstream timing variation are separate plausible
causes; neither has yet been isolated by synchronized device/video evidence.

### Source audit and Stop limitation

The official [3.2.2+46 lockfile](https://github.com/intiface/intiface-central/blob/5413bf0/rust/Cargo.lock)
pins intiface-engine 5.0.5 and buttplug_server 12.0.2. The published server crate
was checked against its lockfile SHA-256. `thehandy_v3` already sends
`stop_on_target:false`, preserves duration in milliseconds and maps integer
position to 0–1 once. A missing September stop-on-target fix is ruled out in
that release's source, without claiming the installed executable was rebuilt
or instrumented.

`device_handle.rs::as_device_message_info` advertises a hardcoded 100 ms; the
installed Handy driver configuration has a 50 ms message gap. `device_task.rs`
starts a batch deadline when a pending queue first becomes nonempty, merges
overlapping commands, and then performs GATT writes with responses. Normal
command ACKs precede these writes. This is not a periodic 50 ms clock; a
constant delay alone does not establish jitter or a beat-frequency problem.

The same server generates Stop output for scalar features but explicitly skips
position features. For this Handy's position-only feature, the Stop handler
returns `Ok` with an empty hardware-command list. It does not send a device-task
barrier or hardware Stop. Already accepted motion may therefore be dispatched
after the Stop ACK. MagicHandy's generation fence cancels its own producers but
cannot recall another process's queue. The shared v4 server path has this
behavior too; changing message-spec versions is not a fix. The firmware's
behavior after the final HDSP target has not been measured. No end-of-segment
or immediate physical Stop guarantee is established by these runs.

Release acceptance needs a real Stop command plus ordering that retires pending
motion for the selected feature and prevents pre-Stop writes after that command.
The upstream implementation must account for an already admitted GATT write and
propagate failure honestly. A guessed position hold in MagicHandy would command
new movement and is not an acceptable substitute. Cloud/native buffered HSP
remain separate paths with actual Stop commands, but their ACKs also do not
measure physical halt latency.

### Sampler experiments and independent consultation

Four sampler variants redistributed interior points while preserving the
shared plan, timing floor and velocity ceiling. The early versions improved
the 35% case but regressed other shapes at append boundaries. The final guarded
variant fixed both sides of each append partition and admitted only locally
non-worsening fits. It passed motion/transport/architecture tests and the 144
case comparison: Cloud output was identical; 78 cases had lower RMS error;
mean-travel changes were at most 0.128%. The regression screen checked maximum
position error (+0.05 pp tolerance), RMS (+0.02 pp) and strict worst velocity step.

On the failing 35% full sweep, however, worst velocity step stayed 172.20
percent/s and maximum error stayed 3.92 pp; RMS changed only 1.751 to 1.614 pp.
At 20%, RMS changed 0.563 to 0.548 pp. These are modest command-fit gains, not
evidence of improved physical feel. More commands can also add downstream load.
The prototype is **parked outside shipping source**, with source, tests and all
failed reports retained under the ignored `sampler-experiment-guarded/` folder.
The real 110 ms spacing regression test and expanded review fixture remain.

At the user's request, Gemini web was consulted with the visible **Argon High**
selection in the existing architecture conversation. Its recommendation was to
hold the sampler and inspect downstream timing/Stop first. Its initial claims
about a periodic batching beat, a one-command post-Stop horizon, imperceptible
error and instant Cloud/native Stop were challenged; they are not accepted
findings. Its follow-up corrected those claims and agreed that the server's
empty Stop bypasses the downstream queue. Even then, its proposed
end-of-segment UI guarantee and assertions of causation from timing correlation
remain unproven and are not adopted. The independent review informs the
investigation but is not a source of hardware measurements. The two exchanges
and screenshots are retained in the ignored evidence directory.

Next evidence should align the existing host trace with server batch/merge
events, actual GATT write timing and observed carriage motion. A simpler
endpoint-only pattern or wider cadence can help distinguish mechanisms, but
also changes velocity demands and does not prove congestion by itself. Preserve
the failing baseline and establish Stop ordering before another acceptance run;
do not change Intiface configuration, ignore its advertised limit, or add a
speed multiplier based on subjective speed alone.

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
The 144-case follow-up retains all five states (baseline plus four experiments)
in `cadence-comparison-atlas/`: all six overview sheets and six detailed
position/velocity comparisons were inspected. They show unchanged Cloud curves,
coarser fast Intiface turns and remaining velocity jumps despite lower average
fit error. Physical captures and corresponding `*-traces.json` include both
matched pairs and `intiface-*-observed-repeat-2`, the last operator-observed run.
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
