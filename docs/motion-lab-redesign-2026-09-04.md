# Continuous motion redesign and separate LLM Lab

Follow-up: [the continuous library review](motion-library-review-2026-09-04.md)
deprecates the legacy catalog, promotes ten recipes into the public app, and
records the final stricter LLM checks and visual atlas. Results below are the
earlier generator/interface phase, not the final library acceptance data.

This supersedes the first panel described in
[the initial review](motion-lab-review-2026-09-04.md). The user physically
tested that panel and reported that directional timing and combined controls
jerked toward one direction, while anchoring was only slightly smoother than
Creative. The new Motion Lab compares Creative baseline, anchored range and
an independent continuous generator. Directional and combined experiments
are removed from the visible panel; their old developer API remains available
for reproducing the earlier experiment.

## What the development conversations reveal

The review read the tasks **Improve motion control schema** and **Improve
motion control schema (2)**, including the earlier complaints and attempted
fixes. The second task contains substantial forked history; it is not
independent acceptance evidence.

| Prior approach | What it addressed | Why it was insufficient |
| --- | --- | --- |
| Four span profiles and correlated span variation | Repeated stroke lengths | More profiles still operate on the same route/center/span model. A long global period does not ensure interesting local variation. |
| PCHIP easing, then C2 quintics | Abrupt endpoint shape | Smooth derivatives in the planned curve do not establish the shape of rounded device commands or physical response. |
| Coverage and local-window requirements | Plateaus and avoidance of broad/base motion | These can force variety that the user explicitly did not request. The user opposed mandatory full-range use and forced changes. |
| Two to four Creative sections | Prearranged changes in one response | Sections still use the same generator and its timing constraints. Repeated schema/prompt additions increase the burden on small models. |
| Local interval fitting instead of worst-interval whole-phrase scaling | High requested speed feeling slow | A necessary correction, but pace priority can suppress timing distinctions. Conversely, preserving strong directional asymmetry concentrates peak speed. |
| Phrase age, reconsideration, novelty and repeated contract reminders | Models holding or narrating changes | These are useful diagnostics, but cannot supply expressive controls missing from the motion representation. |

Four named “range textures” are not a fundamental upper bound on natural
variation. Adding labels alone would leave the coupling between endpoints,
stroke shape and timing intact. This experiment replaces that authoring
framework with continuously varying axes. Named presets can remain useful as
starting points over that space, rather than being the space itself.

## Physical evidence from the first panel

The retained `motion_trace.v3` ring contains 512 rows, with older rows dropped.
The user used Cloud REST with Original Handy calibration, saved speed limits
10–43 and stroke window 0–100. These are commanded estimates, not measured
carriage velocities. Groups cover different runs/durations, not a randomized
matched trial.

| Method | Retained rows | Requested speed | Maximum commanded peak velocity (%/s) |
| --- | ---: | --- | ---: |
| Creative | 172 | 24 | 220.19 |
| Anchored | 106 | 24 | 218.26 |
| Directional | 107 | 16–24 | 363.90 |
| Combined | 127 | 16–24 | 363.78 |

The directional peaks approach the Original Handy absolute calibrated ceiling
(about 363.64%/s, with the existing numerical tolerance). This supports the
reported concentration into one direction; it does not prove all perceived
jerk came from the curve. The ring also includes transport failures.

| Method | Transport results | Mean / p95 / max latency (ms) | Failed results |
| --- | ---: | --- | ---: |
| Creative | 168 | 347 / 395 / 739 | 2 |
| Anchored | 102 | 337 / 360 / 396 | 0 |
| Directional | 104 | 360 / 390 / 1457 | 0 |
| Combined | 124 | 347 / 393 / 701 | 0 |

The two Creative failures were `points_add` network errors at 20:38:16 and
20:40:51 UTC. Raw traces remain ignored local artifacts:
`.scratch/motion-lab-user-all-traces.json` and
`.scratch/motion-lab-user-feedback-trace.json`. The first has top-level rows;
the second is an archive with nested `trace.rows`. Neither is committed.

The earlier “running but no movement” review was launched with
`--simulate-motion`; a connected Cloud device did not change that dispatch
owner. The corrected review uses real dispatch. Backend state, the status
bar, connection manager and lab exports now expose simulation explicitly.

## Independent continuous motion

`FlowSpec` combines a smooth cyclic carrier with slow, seeded spectral
variation. Position is `lo + anchor*(width-span) + span*(1-cos(2πu))/2`.
The span field blends three smooth frequencies instead of choosing among
four textures or introducing new noise at every sample. Memory controls its
time scale. Range, anchor and pace are independent controls.

The cycle-to-time clock compensates for span: short strokes do not inherently
become slow strokes. It anticipates acceleration/jerk requirements smoothly;
the compiled plan then checks exact polynomial extrema against the device
velocity ceiling, experimental acceleration 2400%/s² and jerk 24000%/s³,
and the unchanged reversal/lifecycle gates. The hard runtime limits remain
7500%/s² and 150000%/s³. These lower experimental values are tunable hypotheses,
not established comfort thresholds.

The first prototype incorrectly treated requested pace as a peak limit and
used very restrictive 400/2400 acceleration/jerk values. That flattened the
speed control. The delivered version uses the existing mean-travel calibration
and a smooth local clock, with exact scaling as a final guard. It still
saturates at high pace; the panel reports achieved pace rather than hiding it.

For the reproducible default (5–95 band, floor 25, base anchor, memory 8,
pace variation 10, seed 17, Original Handy), compiled mean/peak velocity is:

| Requested speed | Mean (%/s) | Peak (%/s) | Peak jerk (%/s³) |
| ---: | ---: | ---: | ---: |
| 10 | 56.70 | 99.09 | 2976 |
| 25 | 104.12 | 180.94 | 14858 |
| 43 | 144.33 | 260.33 | 19534 |
| 85 | 156.46 | 306.79 | 19810 |

The 85 case is an automated wide-limit fixture; the review retains the user's
43 maximum. The default stroke-length coefficient of variation is about .246.
Neither that statistic nor lower peaks establish better physical feel.

Sections specify complete bands, speeds and cycle counts. They blend from the
previous section over the first 0.8 cycle and hold for the remainder. Up to
three simultaneous layers modulate range, center and pace. They are bounded
parameter envelopes over one carrier; arbitrary position patterns are not
added and clipped. The motion compiler produces immutable kinematic content,
which the ordinary plan, transition, sampler, sanitizer and transport consume.

Whole-percent sampler tests found that removing stationary duplicate points
could discard easing near a reversal. The shared sampler now checks its final
rounded segments against the original probes and restores distinct positions
where needed. It preserves prior append tails, intentional holds and mandatory
reversals; the existing point-cap gate remains in force. Both Creative and
prepared continuous content receive this correction.

Limitations remain explicit:

- A sinusoidal carrier can itself feel repetitive. The new degrees of freedom
  are an experiment, not proof that this is the final control representation.
- Scores repeat after 64 cycles, or the sum of section cycles. Layer periods
  are rounded to a whole number of waves in that loop (a requested 12-cycle
  layer in a 64-cycle score becomes 12.8). This avoids a discontinuous seam.
- An outer band permits reach; it does not force the span field to visit both
  extremes. Setting the floor to the band width gives full-width sweeps.
- Section changes are blends, not discontinuous exact switches. Aggressive
  modulation can require global safety slowing after local clock fitting.
- Preview curves are before transport rounding and physical feedback. Wire
  approximation is tested separately. No new Flow physical run was initiated
  by the agent in this review.

## LLM Lab and model experiments

Settings > LLM Lab is a separate backend-owned, bounded in-memory conversation.
It exposes three interfaces: partial controls, ordered sections, and concurrent
parameter layers. Prompt text and model are editable; model selection is local
to a trial. Schema guidance is selectable. Replies update only a validated
preview, never production chat or motion. Audition is a separate controller-
gated action with the saved-settings fingerprint. Stop cancels pending trials.

Trials retain raw text, changed fields, model, exact prompt, interface, schema
setting, call count, latency, validity and before/after score. Only explicit
fields apply. Unsupported keys, incomplete sections/layers, out-of-limit values
and malformed JSON are rejected without repair or fallback. “Valid structure”
deliberately does not mean “understood the request.” Export is explicit and may
contain the user's lab conversation; it excludes device/provider credentials.

Native Ollama tested the installed Gemma 12B
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` and Granite 3B
`huihui_ai/granite4.1-abliterated:3b`. Nine intent cases were repeated twice for
each model: five control cases, two sequence cases and two layer cases. Each
check includes strict parsing, intended field values and real curve compilation.
This is a small focused sample, not a statistical reliability guarantee.

| Experiment | Gemma 12B | Granite 3B | Combined |
| --- | ---: | ---: | ---: |
| Initial plain JSON, all interfaces | 16/18 | 11/18 | 27/36 |
| Corrected schema support, initial prompt | 16/18 | 15/18 | 31/36 |
| Final general field-application reminder; plain controls, schema sections/layers | 18/18 | 16/18 | 34/36 |
| Same final prompt; schemas for every interface | 16/18 | 16/18 | 32/36 |

An intervening all-schema run failed at grammar construction, not model intent:
the installed Ollama grammar rejected a bounded string rule. Removing string
length constraints from the grammar fixed it; the parser still enforces reply
length. Numeric bounds, required item fields and array limits remain. Both
Ollama and llama.cpp schema forwarding have HTTP payload tests; only Ollama
has live evidence here.

Common failures were nested layers in the wrong object, misspelled keys,
incorrect cycle counts and a reply claiming full-band motion while omitting
`max_percent`. The final mixed configuration's two failures were Granite
emitting `memory_ cycles` and omitting `reply`. Applying schemas universally
fixed syntax but worsened full-band intent, so it is not the default.

Separate continuation checks used three turns for each interface/model:
preserve an anchor while changing pace, preserve sections while changing an
anchor, add a pace layer without dropping a range layer, then preserve the
score when asked to hold it. **18/18 passed** using the selected mixed defaults.
This supports bounded sequencing/layering as a useful experiment for these
models. It does not establish arbitrary pattern composition or long-chat
reliability. No phrase-specific rewrite rules were added to force these scores.

The live harness is opt-in (`liveeval`), exports all trial failures, and does
not weaken CI to treat failed intent checks as successes. Relevant ignored
artifacts are `.scratch/llm-lab-interfaces-v1.json`,
`.scratch/llm-lab-interfaces-schema-v3.json`,
`.scratch/llm-lab-interfaces-v4.json` and
`.scratch/llm-lab-interfaces-v5.json` (see the local logs for exact report names).

## Build and validation

`scripts/build-labs.ps1` builds the development UI and a `CGO_ENABLED=0`
executable with `-tags magichandy_labs`. It does not launch the app or select
simulation. Ordinary `npm run build` plus `go build ./cmd/magichandy` produces
the public app. Public assets exclude lab modules, routes and lab-only locale
strings; tagged embed tests require matching UI markers. Public HTTP tests
reject lab endpoints and lab starts. See [ADR 0019](decisions/0019-development-motion-and-llm-labs.md).

Validation covers the public and development frontend builds, the full
frontend suite, full Go/race suites, development-tag Go/race tests, vet,
lint, import boundaries, goleak/Stop teardown, strict proposal parsing,
separate conversation state, compiled C2 seams, bounds and derivative extrema,
live speed limiting, deterministic identity, and rounded buffered path fidelity.
Size and review-runtime measurements are recorded in the
[goal scorecard](goal-scorecard.md). App-level LLM readiness and a text-only
production chat are checked against the exact review build before handoff.

Hardware acceptance remains with the user's next explicit audition. Compare
the three rows at the same saved limits, then try layered and sequenced flow;
record feel, achieved pace, the lab export and `motion_trace.v3`. Transport
mapping, Cloud scheduling, stroke reversal mapping, controller ownership and
Stop remain on their existing paths.

## Design references

[Motor variability research](https://www.nature.com/articles/nn.3616) motivates
testing temporal correlation rather than equating more randomness with more
natural movement. It is not evidence for this device's comfort.
[Ruckig's tutorial](https://docs.ruckig.com/tutorial.html) and
[third-order trajectory paper](https://arxiv.org/abs/2105.04830) distinguish
geometry/timing and velocity/acceleration/jerk constraints. No Ruckig library
or native dependency was added.
[Ollama structured outputs](https://docs.ollama.com/capabilities/structured-outputs)
and [llama.cpp's schema example](https://github.com/ggml-org/llama.cpp/blob/master/examples/json_schema_pydantic_example.py)
support the provider payloads; semantic validation remains application-owned.
