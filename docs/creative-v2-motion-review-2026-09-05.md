# Creative v2 motion and model review — 2026-09-05

## Result and scope

Creative v2 is a separate main-chat mode and LLM Lab contract for native uneven
travel, local excursions, shrinking rebounds and inertia. It occupies the
second row's first cell in the six-cell selector. Creative, Pattern library,
Layered and Off remain available, with one blank reserved. No default switches
on update. This work updates draft PR #250; it is not a merge or release.

The user's earlier feedback favored relative/layer edits, found ordered
sequences robotic, and found turn softness too weak to justify its prominence.
The new request needs travel character and intentional reversals as independent
parameters. Original Creative's four range textures cannot express those
directly. Adding more texture names would still leave the model guessing how a
name maps to the requested action. Layers remain useful for broad continuous
variation, but adding position oscillators can create unwanted reversals.

Creative v2 instead generates stroke destinations and travel shapes from a
small parameter vocabulary. The nine combinations in the review matrix are
fixtures, not named runtime patterns. Original Creative receives a focused
optimization: newly authored textured phrases use fresh realizations; pace-only
and horizon-only edits retain the active phrase. Its existing prompt is unchanged.

## Native controls and fitting

The model edits range, focus, sweep and rebound groups, plus pace, inertia,
variation and `evolve`. Each group supplies its paired fields. Unmentioned
groups persist. An order-independent array applies all edits atomically; its
order is not a motion sequence. Duplicate groups, unknown fields, partial
groups, nulls and invalid bounds fail the transaction. The generator is carried
by `FlowSpec.Gesture` through the existing shared-engine path, without a second
motion loop. See [ADR 0024](decisions/0024-native-creative-strokes.md).

Focus determines a local anchor and width. Mixed mode places groups of one to
three local primary cycles among full strokes, with at most six consecutive
local primary cycles before broad travel. Each primary cycle can add up to four
shrinking rebounds. Tails below 10 percentage points are omitted. Thus mix is
not an exact proportion of time, and dense rebound settings can dominate a run.
Variation changes correlated pace and local width; it never moves the anchor
or adds noise to individual samples.

Inertia time-warps a quintic travel primitive so the velocity crest builds
later. Endpoints have zero velocity and acceleration. It is an artistic travel
control, not a force command or a collision model. The primitive was informed
by the [Flash and Hogan minimum-jerk paper](https://pubmed.ncbi.nlm.nih.gov/4020415/).
Research on [the temporal structure of motor variability](https://www.nature.com/articles/nn.3616)
also motivates investigating correlated variation. Neither paper validates this
device's comfort or the perceptual effect of the implemented generator.

The first timing candidate fitted each stroke independently using historical
Flow's quieter 2,400 %/s² and 24,000 %/s³ authoring budgets. At ordinary and high
requested speeds, both directions saturated to the same duration, erasing the
requested fast-sweep/slow-return contrast. Final timing uses Creative's existing
runtime envelope, 7,500 %/s² and 150,000 %/s³, with profile-derived velocity and
100 ms minimum reversal bounds. A slow direction's floor remains proportional
to the fast direction's floor. No shared safety constant was raised, and
historical Flow fitting remains unchanged.

Local fitting prevents a short rebound from globally slowing every broad
stroke. The resulting interpolant is checked for exact velocity extrema,
unintended reversals and finite coefficients. The usual prepared plan,
sampler, sanitizer, startup, retargeting and Stop remain authoritative.

## Live model comparison

Host: Windows, local Ollama at loopback port 11434. Models:

- Gemma: `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`.
- Granite: `huihui_ai/granite4.1-abliterated:3b`.

The development set has nine sequential and seven independent requests per
model. It checks tip/base direction, compound numeric controls, rebound removal,
return to full strokes, question-only holds, fresh realization, automatic
continuation, exact holds, gentler pace and narrow-band edits. Each request uses
one real generation. Parsed validity and actual resulting-control intent are
scored separately. These are iterative development trials, not held-out
statistical estimates of reliability.

| Candidate | Contract change | Gemma valid / intent | Granite valid / intent |
| --- | --- | --- | --- |
| v1 | Flat partial object | 16 / 11 | 15 / 9 |
| v2 | Complete paired groups in an object | 16 / 12 | 16 / 10 |
| v3 | Explicit property-order guidance | 16 / 14 | 16 / 9 |
| v4 | Order-independent array of one-control items | 16 / 16 | 16 / 14 |
| v5 | Explicit evolution and mixed-middle examples | 16 / 16 | 16 / 16 |
| v6, selected | Brief reply and immediate JSON closing | 16 / 16 | 16 / 16 |

All counts are out of 16. Reports `.scratch/creative-v2-v1-live.json` through
`creative-v2-v6-live.json` retain prompts, raw replies, exact before/after
scores, limits, durations and failed selections. No Granite-only prompt is
adopted. Gemma improved across the retained candidates; the final change did
not reduce its development-set score. Original Creative's prompt was not changed.

The array form addresses an observed constrained-generation weakness: optional
properties in an object can have a grammar-imposed order, making a later choice
prevent the model from returning to an earlier group. This interpretation is
supported by [llama.cpp's JSON-schema grammar tests](https://github.com/ggml-org/llama.cpp/blob/master/tests/test-json-schema-to-grammar.cpp).
It does not imply all schema providers use that ordering. The app still validates
actual output independently of the provider's schema constraint.

For selected v6 Lab requests, observed total generation durations were
380–1,312 ms for Gemma and 123–1,261 ms for Granite. These local warmed-model
measurements are not device/network latency. Production uses the saved output
budget (256 in the review app); the Lab comparison allows 1,536 output tokens.

### Production conversations and retained failures

The production HTTP harness starts local tip sweeps, adds inertia, requests
mixed base rebounds, removes rebounds, and requests fresh variation. It captures
the resulting shared-engine targets and fake-transport commands. One production
Autopilot continuation is generated and compiled but not scheduled by this
fixture. Successful runs have five manual edits, one transport Play and a
literal Stop that leaves the engine stopped. There is no repair or fallback.

- Initial array prompt: Gemma completed; Granite held the old variation amount
  instead of refreshing the realization on turn five.
- v5: both models completed all five turns, the Autopilot generation and Stop.
- Final prompt/timing: Gemma completed again, including a final verification
  run in 14.41 seconds. Granite emitted only rebound settings on turn three,
  retaining the tip-only focus while claiming base rebounds and full strokes.
- The latter mismatch was accepted before a coverage guard was added. Its
  actual accepted Flow and limits were recovered from the captured SSE motion
  event and rendered, not replaced with an imagined correct target. A repeated
  final Granite run produced the same omission and was rejected by the new
  guard. The previous target stayed active. This is a remaining model limitation,
  not a successful production mapping result.

Artifacts use `.scratch/creative-v2-production-{gemma,granite}` with the original,
`-v5`, `-final` and `-verified` JSON/log variants. The recovered accepted failure
is `.scratch/creative-v2-recovered-target.json`. Later harness code captures
accepted motion before checking intent and stops before exporting even a failed
run. Earlier partial failure traces lack a Stop record and are labeled as such;
they do not prove the complete Stop scenario. Ordinary unit/integration tests
and the successful Gemma production traces supply that evidence.

Two earlier text-only explanation probes produced a plausible Gemma sentence
followed by repeated newline tokens until the 256-token budget was exhausted.
The app rejected the incomplete JSON without motion. The final prompt explicitly
closes the JSON after the brief reply. The exact final review app passed the
working-model probe and a non-empty production text-only chat with no repair,
fallback or motion. Failed SSE is retained in
`.scratch/creative-v2-review-chat.sse` and
`creative-v2-review-chat-second-failure.sse`; final successful SSE is
`creative-v2-review-chat-verified.sse`.

### Scheduled Lab continuation

On the final current-source review app, each model completed a manual preview
request and three actual scheduled preview-only Autopilot turns. All six
continuations changed the realization seed while preserving every gesture
control, speed and outer band. Both sessions stopped. No live motion was enabled.
The earlier v6 run also completed this check and is retained separately.
Artifacts: `.scratch/creative-v2-scheduled-preview.json`,
`creative-v2-scheduled-preview-v6.json`, `creative-v2-reviewed-probe.log`.

Realization changes are chosen at initialization or accepted evolution, not at
sample time. Autopilot uses the existing inference/retarget path. A captured
score remains reproducible and eventually loops (32 primary cycles by default).
Automatic continuation is necessary for indefinite fresh realizations. Exact
repetition requests deliberately suppress it.

## Visual inspection and numerical results

The retained atlas is `.scratch/creative-v2-atlas/index.html`, with manifest
`manifest.json`: **508 steady-case records, 373 distinct output plots, 24
overview sheets and four captured timelines**. Identical sampled outputs share
an image. The JSON inputs, plots, traces and runtime data are ignored and are
not embedded or committed. The tracked exporter can reproduce the final native
matrix via `-catalog=false -creative-v2`; live harnesses and reproduction commands
are documented in [the visual process](motion-visual-review.md).

The initial candidate's 108-case matrix, all six model iterations, rejected
selections, successful and failed production traces, final 108-case matrix and
final review-app continuations are retained. Every overview sheet was inspected,
including the final additions. Detailed inspection covered every new parameter
combination at Original low/middle/high speed, device-profile outliers, original
Creative realizations, failed model selections and captured transitions.

Final matrix manifest references and findings:

| Combination | Original 10 / 45 / 85 case indices | Visual finding |
| --- | --- | --- |
| Even full travel | 334 / 346 / 358 | Symmetric broad strokes; intentional regular baseline |
| Irregular mixed excursions | 335 / 347 / 359 | Changing groups of local and broad travel; no incidental extra reversals |
| Fast tip sweeps | 336 / 348 / 360 | Clear unequal direction timing; fast crest about 192.5 %/s versus return about 78 at 45 |
| Fast base sweeps | 337 / 349 / 361 | Directional asymmetry mirrors the tip case |
| Middle work among full strokes | 338 / 350 / 362 | Centered local groups return to both outer endpoints |
| Base rebounds and full strokes | 339 / 351 / 363 | Shrinking excursions at the lower anchor, interspersed broad travel |
| Tip rebounds and full strokes | 340 / 352 / 364 | Shrinking excursions toward the upper anchor with broad returns |
| Narrow band / truncated tails | 341 / 353 / 365 | Remains inside 30–60; sub-10 tails disappear instead of becoming repeated tiny reversals |
| Maximum combined character | 342 / 354 / 366 | Dense local work and strongly unequal travel; about a 250-second loop, an extreme rather than a recommended default |

Some middle/high cases share identical sampled output because the local timing
floor has saturated; their shared detailed plot was inspected once. Standard
and Pro maximum-character outliers were inspected separately (402 and 438).
Original Creative detail at 343/355/367 and the other high-rate realization at
369 shows broad textured variation but lacks a native rebound/direction control.
Rejected-model details 190 and 232 show wrong location or only local travel
despite a mixed-motion request. Recovered failure 500 stays entirely near the
tip, visibly contradicting the base/full request. Final Gemma cases 491–493
show the uneven sweep, changed inertia crest and mixed base rebounds. All four
captured timelines were inspected, with canceled queued samples distinguished
from executed time and partial traces identified explicitly.

The final gesture matrix's largest analytic acceleration was 3,947.3 %/s² and
jerk 149,105.7 %/s³, within the existing 7,500/150,000 runtime envelope. Including
the original Creative comparisons, maximum acceleration was 7,462.8 %/s².
The largest reported knot acceleration jump for final entries was about
0.000204 %/s², consistent with the C2 construction and numerical precision.
Exact bound and reversal tests remain the acceptance gate; plotted phase
portraits are downsampled over long loops and are not an extrema proof.

At requested speed 45 on Original, local fitting increased mean travel for even
full strokes from 147.5 to 177.2 %/s, irregular mixed excursions from 75.8 to
127.7, and mixed base rebounds from 54.9 to 99.2. The primary purpose of the
timing change was preserving directional contrast, not maximizing speed. Short
local strokes still saturate, and increasing inertia can lower effective pace
even when the requested pace setting is preserved. Commanded pace estimates
remain visible; higher requested percentage does not always change physical
timing after the bounds are reached.

Local transfers can meet another same-direction leg at zero velocity, producing
a brief slowdown without an extra reversal. Maximum inertia and rebound density
concentrate travel and local dwell. Those tradeoffs need physical feedback; a
rounded position curve alone cannot establish a natural feel.

## Validation, budgets and handoff

Passed: full Go tests, `go vet`, full race suite, goleak/Stop lifecycle gates,
import-boundary checks, optional Lab package tests, lint (zero issues), frontend
typecheck, localization, 473 frontend tests, and canonical production UI build.
Meaningful new coverage includes transactional parser rejection, question/paused/
refusal authority, explicit focus mismatch, mode-change fencing, deep-cloned
targets, native envelope/monotonicity/C2 checks, directional contrast under
saturation, original phrase replay and one-stream Stop integration.

Final ordinary CGO-disabled binary is 26,241,024 bytes; stripped/trimpath binary
is 19,033,088. Initial JS is 750.72 kB (207.61 gzip), lazy Labs JS 51.89 kB
(14.54 gzip). No new runtime/frontend dependencies. Three fresh-data launch
samples reached the listener in 132.6–183.0 ms and health in 134.7–212.5 ms;
working set was 21.91–22.46 MiB, private memory 53.87–55.39 MiB. These startup
snapshots exclude browser/model memory and do not close existing steady-idle,
active-hardware or soak budgets. See [the scorecard](goal-scorecard.md).

Review app: `http://127.0.0.1:49850/?review=creative-v2#/chat`, isolated data
`.scratch/creative-v2-review-data`, working local Gemma, Creative v2 selected,
text-only reply visible, Autopilot off and engine stopped. It contains no copied
Handy connection key or provider credentials. Existing user review sessions
were preserved. LLM Lab offers the same contract in its title-bar mode selector.

Transport scenario was fake captured dispatch or preview-only Lab, with zero
physical commands and zero fake transport latency. No real Handy latency or
carriage telemetry was measured. Physical window mapping, reverse handling,
transport implementations, global Stop semantics, the legacy library retirement,
user-created content and saved default mode were intentionally unchanged.
Actual comfort, tracking and whether the variety remains interesting over a
long session require the user's physical evaluation. Granite's compound
omissions also remain an explicit review limitation.
