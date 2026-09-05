# Motion and LLM experiment review — September 4, 2026

Four experiments are available in the optional Labs workspace: correlated
drift, symmetric turn softness, cadence hold, and a relative/layer editing
contract. They address specific failures from the earlier reviews. Production
recipe defaults retain the existing zero/default values; the experiments need
physical feedback before being promoted.

## Evidence and selection

Reviewed the earlier **Improve motion control schema** conversations and the
motion redesign, library review, feature ideas, LLM control surface and visual
review documents. Recurring complaints were one-sided jerks from directional
timing, persistent spatial character despite scalar edits, repetitive phrases,
and promises in replies that did not match applied motion. The user specifically
rejected forced coverage and compulsory novelty.

| Idea and source | Decision and reason |
| --- | --- |
| Correlated procedural variation; [Perlin, Improving Noise (2002)](https://mrl.cs.nyu.edu/~perlin/paper445.pdf) discusses smooth interpolation and derivative artifacts | Implement bounded, seeded, two-scale value noise with a septic C3 interpolant. This is our motion hypothesis, not Perlin's implementation or evidence of human motion feel. It replaces the three-wave envelope without more texture names. |
| Turnaround shape independent of envelope and directional timing; local redesign and physical complaints | Implement a symmetric cosine/septic carrier blend. Longer turnarounds in both directions concentrate more travel in the middle; greater softness is not equivalent to lower jerk everywhere. |
| Separate cadence from travel speed; local clock and range review | Implement cadence hold. The widest requested span becomes the timing reference as hold increases. Short strokes receive less cadence compensation. Effective mean pace can fall. |
| Grounded API arguments and preserved state; [Mok et al., NAACL Industry 2024](https://aclanthology.org/2024.naacl-industry.36/) | Implement relative deltas and per-axis layer edits. The backend performs arithmetic and preserves omitted layers, reducing the model's need to rewrite an entire score. |
| Split action generation from conversational prose; local ideas and [Natural Language Tools (2025)](https://arxiv.org/abs/2510.14453) | Defer a second inference pass. Test one-call editing with exact state checks first. Continuing reply/action mismatches make grounding prose in the accepted diff a strong next experiment. |
| More named textures, forced variation, unconstrained pattern summation | Defer. More labels do not fix temporal structure; forced novelty contradicts feedback. Independent position-pattern summation obscures range and derivative bounds. |

Structured output is measured, not assumed to solve intent. The findings in
[Let Me Speak Freely?](https://arxiv.org/abs/2408.02442) and the contrasting
[.txt reproduction](https://blog.dottxt.ai/say-what-you-mean.html) motivate
comparing prompt/schema combinations on the same requests. They do not
establish a universal advantage for free text or constrained JSON.

## Motion implementation

`FlowSpec` adds three optional controls. Omitted values retain the original
serialized score and motion. No runtime dependency, device payload or private
playback loop was introduced.

- `variation_mode`: `waves` or `drift`. Drift uses hashed knots, smooth
  interpolation and two offset scales. `memory_cycles` controls trend length;
  the seed makes output reproducible. It still repeats at the score boundary,
  normally after 64 cycles. Existing layers and sections remain available.
- `turn_softness_percent`: 0–100. Zero retains cosine travel; 100 uses septic
  half-strokes with flat endpoint derivatives. The carrier is symmetric. The
  final compiled quintic plan is C2, not globally C3 device output. Local
  timing anticipates the steeper middle; existing exact extrema checks still
  enforce velocity, acceleration and jerk limits.
- `cadence_hold_percent`: 0–100. The clock blends current span toward widest
  requested span. At 100, changing reach no longer accelerates the beat to
  maintain mean travel. Pace variation/layers and device limits can still
  affect timing. The UI shows effective pace rather than hiding the cost.

Motion Lab exposes these controls only for Continuous flow. Historical
generators retain their applicable controls. **Compare motion experiments**
creates a saved sequence: reference, drift, softness 70, cadence hold 100,
then their combination. It preserves the selected band, seed, layers and
sections, resetting only the three experiment axes for comparison. The same
roster drives the atlas. Order is labeled and fixed; this is guided feedback,
not a blinded or statistically controlled preference study.

## Relative and layer edits

The LLM Lab's new interface accepts, for example:

```json
{
  "change_by": {"speed_percent": -5},
  "layers": [{"axis": "pace", "amount_percent": 20, "period_cycles": 12, "phase_percent": 0}],
  "reply": "Speed is five points lower, with a pace layer added."
}
```

`controls` sets absolute values. `change_by` applies signed percentage-point
or cycle deltas. A field cannot occur in both. Relative speed also adjusts
each existing section by that delta, preserving their differences. Global
range edits with sections remain rejected; Sequence owns those changes.
Invalid bounds/conflicts reject the entire proposal without mutating state.

`layers` upserts supplied axes. Omitted layers remain and `[]` changes nothing.
Each supplied layer is complete; retained period/phase must come from the
current score. `remove_layers` explicitly removes listed axes. The older
Layers interface still replaces a complete stack, for comparison. One provider
call produces a trial, without repair or semantic fallback. Accepted proposals
update only the preview until an explicit audition.

## Live evaluation and prompt iterations

Local Ollama models:

- `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
- `huihui_ai/granite4.1-abliterated:3b`

The tuning matrix uses 12 request types, two repetitions per model/interface:
relative speed/memory, each new control, removing base pace variation, a
question, adding/editing/removing a layer, and section-relative speed. An
intent pass requires the **entire resulting score** to match the expected
score and compile successfully, including every unrequested field. Valid
JSON or a plausible reply is insufficient.

| Editing iteration | Gemma 12B exact passes | Granite 3B exact passes |
| --- | ---: | ---: |
| v1: `adjust`, combined example | 24/24 | 14/24 |
| v2: separate examples, executable edit emphasis | 24/24 | 16/24 |
| v3: `change_by`, layer example, base/layer distinction | 24/24 | 18/24 |

In v3, matched schema-guided baseline interfaces passed 24/24 on Gemma and
8/24 on Granite (8/16 Single controls, 0/6 Layers, 0/2 Sequence). Plain Single
controls separately passed 16/16 and 4/16. Part of the improvement comes from
the schema; the edit contract adds preservation benefits beyond syntax.
Remaining Granite failures include removing a pace layer instead of base
pace variation, promising removal without emitting it, and emitting both
absolute and relative speed. The latter is rejected atomically.

After freezing v3, a separate six-turn conversation used new wording, values,
layer periods/phases and actual accepted history. **Both models passed 5/6.**
Both omitted turn softness when asked for drift and softness together. Gemma
also copied an unchanged layer. Granite's later summary misstated an earlier
memory value. Numeric score checks do not grade prose truthfulness; these
reply defects remain recorded. This is a small diagnostic set with mostly
repeatable outputs, not a statistical reliability estimate.

v3 edit latency on this host: Gemma median 616 ms, p95 1,354 ms; Granite
median 193 ms, p95 544 ms. Each trial made one provider call. Raw inputs,
prompts, outputs, limits, failures and scores are retained in ignored
`.scratch/motion-ideas-live-v1.json`, `-v2.json`, `-v3.json`, and
`.scratch/motion-ideas-holdout.json`. Opt-in live tests deliberately fail when
a model misses intent; the required deterministic suite is green.

## Visual and numerical review

The atlas renders all tuning/continuation outputs, including valid but wrong
selections and rejected responses. Eighteen reference/experiment cases cover
requested speeds 10, 25 and 43, including maximum softness at each speed.
The isolated review app's accepted reply is included. Generated artifacts:
`.scratch/motion-ideas-atlas.json` and `.scratch/motion-ideas-atlas/index.html`,
with a manifest, detailed figures and `contact-01.png`. Identical outputs
share a figure; every trial retains a card.

Final atlas: 383 steady cases, 36 distinct steady figures and five captured
timelines. The 364 tuning/holdout trials include 62 valid intent mismatches
and 38 rejected structures; none were omitted. The additional live review
reply also has its own record. The ready review sequence is
`#/labs/tests/2ORJ7GLRK6IPD3A6N66YMANP7W`, round 1 of 5, with no answers filled in.

Inspected the overview and all 18 detailed experiment figures, plus failed
layer replacement, unintended pace-layer, section-speed and two-control
outliers. Drift has longer irregular envelopes and smoother successive reach
changes for seed 17. The reference has frequent within-trend swings. Softness
broadens both turnarounds but raises middle-stroke speed and can raise jerk.
Cadence hold spaces strokes more evenly and lowers mean travel. Combined
output retains drift's envelope and more even spacing. Whole-percent samples
track the planned excerpts; stepwise wire velocity remains visible. These
are commanded estimates, not measured carriage motion.

Requested 25%, Original profile, band 5–95, floor 25, anchor 0, memory 8, seed 17:

| Case | Mean travel (%/s) | Peak acceleration (%/s²) | Peak finite-segment jerk (%/s³) |
| --- | ---: | ---: | ---: |
| Reference | 104.1 | 1,526 | 14,858 |
| Drift | 106.1 | 1,228 | 9,172 |
| Softness 70 | 100.1 | 1,387 | 21,575 |
| Cadence hold 100 | 66.9 | 605 | 2,291 |
| Combined | 73.4 | 789 | 6,340 |
| Combined, softness 100 | 73.3 | 942 | 7,938 |

All 45 profile/speed/roster combinations pass exact derivative bounds, C2
knots including the loop, range bounds, quantized fidelity and no stationary
wire edges. A stress case combines memory 2, softness/hold 100, all three
maximum layers and differing sections. An isolated cadence test removes
deliberate pace modulation and verifies at least an 80% reduction in cycle-time
spread. Existing motion, transition, Stop and leak gates remain.

The HTTP test captures all five frozen rounds through shared start, sampler
and fake transport, with five plays and explicit non-controller Stops. It
checks for no commands after Stop returns. Raw queued commands and traces
are in `.scratch/motion-ideas-captured.json`; five startup/Stop timelines show
canceled queued portions shaded. This checks dispatch/lifecycle, not sustained
physical feel. No physical hardware was connected or commanded in this review.
Production transport behavior, saved limits and device mapping were
intentionally unchanged.

## Handoff and next evidence

The current review app runs on `http://127.0.0.1:49845`, using isolated
`.scratch/motion-ideas-review-data`, Labs enabled and available Gemma 12B.
It has no copied credentials. Real production text chat completed in 360 ms,
one call, without repair/fallback or motion; the required readiness script
passed. A live Lab edit changed only speed, in 640 ms with one call. Normal
chat rendering, the editing interface, motion controls and the guided
comparison were checked in the browser.

Full Go tests and race tests, vet, lint, frontend type/localization checks,
471 frontend tests, frontend build and ordinary CGO-disabled builds pass.
Final affected-package race checks cover the last bounded-conversion change.
Budget measurements are in the goal scorecard; no new runtime or frontend
dependency was added. Existing app sessions were preserved.

Use the guided comparison to judge whether drift is less repetitive, softer
turns help or hesitate, and the steadier beat feels better or merely slower.
Ratings/comments save locally with the exact score, engine preview, limits
and build metadata; the page displays the database path. Export the sequence
to share that evidence. Feedback does not alter prompts or motion automatically.
Promotion remains dependent on physical feedback, especially for the
middle-stroke concentration of high softness.
