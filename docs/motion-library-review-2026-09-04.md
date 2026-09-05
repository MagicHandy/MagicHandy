# Continuous library and visual review — 2026-09-04

The active built-in library now consists of ten continuous recipes. All 81
previous built-ins are deprecated for selection, hidden by default and excluded
from the LLM menu. They remain manually playable, with saved preferences and
user-authored content preserved. This implements the user's request for a new
library beyond the existing Creative texture vocabulary.

The earlier conversation audit, physical complaints, transport timing and
continuous-generator experiments are in
[the redesign review](motion-lab-redesign-2026-09-04.md). The governing decision
is [ADR 0020](decisions/0020-continuous-motion-library.md). The compiler and new
library ship in the normal app; Motion Lab, LLM Lab and atlas tooling do not.
The review app selects Pattern Library. Existing installations keep their
saved choice between Creative, Pattern Library and Off.

## Why the previous library was weak

Six overview sheets were inspected, covering every legacy entry and all ten
replacements. Most legacy patterns still spend much of each leg at constant
velocity, with short easing intervals around reversals. Many Full Sweep Run
variants change repetition count or occasional endpoints without changing that
basic character. Broad resets and endpoint changes create variety in the point
list while retaining a mechanical travel profile.

Detailed position, phase, velocity and acceleration plots explain why a
position-only preview could look acceptable. At requested 43%, legacy Stroke
has an approximately 4,908 %/s² acceleration jump at an easing knot; Pulse is
approximately 4,984 %/s². Their finite-segment jerk reports zero because those
segments have constant acceleration. That number does not include acceleration
discontinuities. The new atlas explicitly reports both quantities.

The 81-entry menu also makes language selection harder. Names and near-duplicate
descriptions provide weak distinctions, and a region request, endpoint anchor,
range variation and pace change can be conflated. The old prompt showed motion
fragments while requiring a complete reply envelope. The 3B model frequently
copied those fragments to the root object. Autonomous planning additionally
inherited interactive examples with fields forbidden by its own contract.

Four range textures are not the fundamental limit. More labels on the same
generator increase vocabulary without resolving geometry, timing and selection
problems. The replacement separates those controls and uses continuous, seeded
variation under a small set of observable behaviors.

## The replacement set

| Recipe | Behavior |
| --- | --- |
| Full sweeps | Fixed 0–100 reach, rounded travel, even pace |
| Lower strokes | Fixed 0–40 region |
| Middle strokes | Fixed 30–70 region |
| Upper strokes | Fixed 60–100 region |
| Base-anchored variety | Return to 0; gradually vary reach |
| Tip-anchored variety | Return to 100; gradually vary reach |
| Centered variety | Both endpoints move around the midpoint |
| Traveling window | A 40-wide envelope gradually travels up and back |
| Wide then narrow | Four wide cycles followed by four middle cycles, with blended boundaries |
| Pace wave | Full reach retained while pace gradually rises and falls |

The carrier is sinusoidal, with a seeded low-frequency range field, bounded
parameter modulation and an anticipatory local clock. It compiles immutable
position/velocity/acceleration data into the existing shared plan. Experimental
comfort targets remain 2,400 %/s² acceleration and 24,000 %/s³ jerk; existing
hard runtime gates remain in place. Bounds, exact derivatives, focus projection,
all supported Handy models, live speed limits, wire fidelity and retargeting
are tested through the shared engine. No new transport or playback loop exists.

The first traveling-window prototype moved between three section bands. Its
plot showed directional pull, so it was replaced by continuous center
modulation inside a fixed-width envelope. The final plot is balanced over its
loop. Because its center moves during a stroke, adjacent travel legs still
differ slightly in length (stroke CV about 0.104 at 25%); fixed envelope width
does not mean identical reversal distances. The first 0.8 cycle of each section
also blends from the previous section, so the first cycle of Wide then narrow
is a transition rather than an abrupt new band.

All ten replacement plots at 25% have effectively zero acceleration jumps at
compiled knots. Peaks range from roughly 598 to 1,499 %/s² acceleration and
2,055 to 13,382 %/s³ finite jerk. Wire overlays preserve their rounded travel.
At 43%, Base-anchored variety retains its shape with about 1,927 %/s² peak
acceleration and 18,711 %/s³ peak jerk. These are commanded estimates, not
physical measurements or evidence of comfort. Narrow strokes cycle faster at
the same travel-rate setting, and derivative budgets can reduce achieved pace.
Finite seeded loops still repeat.

## Live LLM experiments

Installed models: Gemma 12B
(`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`) and Granite 3B
(`huihui_ai/granite4.1-abliterated:3b`), through native Ollama with thinking off.
No model downloads, training or new runtime dependencies were needed.

The production-catalog experiment contains thirteen requests repeated twice
per model: ten behaviors, a pace-only edit, hold and Stop. Initial comparisons
covered opaque handles, descriptive IDs and action names (156 trials each).
Complete-envelope examples raised combined intent/structure passes from
88/156 to 130/156. Schemas improved structural reliability further; an
intermediate harness incorrectly rejected two legal omitted-pace updates.
The final harness reads authoritative current state and validates compiled
geometry, rather than judging an isolated JSON object.

| Final check | Gemma 12B | Granite 3B |
| --- | ---: | ---: |
| Production contract, opaque handles, schema, compiled geometry | 26/26 | 24/26 |
| Separate Library Lab, opaque handles, five-turn continuation repeated twice | 10/10 | 6/10 |
| Same Library Lab, descriptive IDs | 10/10 | 10/10 |
| Same Library Lab, action names | 10/10 | 6/10 |
| Controls/sections/layers, mixed schema defaults, strict preservation | 18/18 | 11/18 |
| Same strict cases with schemas for all interfaces | 16/18 | 13/18 |
| Three-turn controls/sequence/layers continuations, strict preservation | 9/9 | 9/9 |
| Autonomous motion and speech contract checks, one request each | 2/2 | 2/2 |

Production keeps opaque handles because they performed best in its broader
naming comparison. LLM Lab offers all three variants because the compact
preview contract behaves differently. Interface, examples, current state and
model matter more than assuming readable function names always win.

The final production failures were Granite preserving Middle strokes when
full-range motion was requested. In the compact Library Lab, Granite sometimes
selected even Full sweeps for a requested Pace wave; a subsequent hold correctly
preserved that wrong selection and remained an intent failure. Direct-control
failures included misspelled keys, omitted outer-band changes and incorrect
section counts. Schemas do not guarantee the requested movement.

The harness was strengthened during visual review: it now checks every
unrequested field and complete section content. A previous continuation check
passed a range layer despite an unrelated ceiling edit. A general prompt
clarification distinguishes outer bounds from span bounds and tells the model
to omit controls when changing only layers or sections. All 18 continuation
checks then passed the stricter comparison. Earlier 34/36 results in the first
redesign document are historical and use narrower checks; they should not be
compared directly with this final 29/36 strict sample. Plain controls with
schema-guided sections/layers remains best for the configured Gemma model.

These focused tests support bounded sequencing and parameter layering as
useful experiments. They do not establish reliable arbitrary pattern programs,
long conversations or physical feel. Raw failures remain visible. Each Lab
trial makes one provider call, never repairs or falls back, and requires a
separate explicit Audition.

## End-to-end and visual evidence

The live HTTP test used Gemma to start Full sweeps at 25%, select tip-anchored
variation, change only pace to 30%, and Stop. It verified actual engine targets,
four single-call responses with no repair/fallback, four append commands, one
Play and a stopped engine. A fake transport captured commands; raw responses
and trace rows were exported. No physical device was commanded. The test also
fixed a narrow authorization defect: “Keep that shape and change only speed to
30” was previously discarded. The added authorization permits only the matching
numeric pace edit on a running library target, retaining shape, area, negation
and question boundaries.

The final atlas at `.scratch/motion-atlas-final/index.html` contains 461 cases,
275 distinct steady-motion plots, six overview sheets and a captured app
timeline. It includes every built-in at 10/25/43%, both final control-schema
experiments, production and compact-library model outputs, the live app test
and the successful browser Lab proposal. Every request keeps its record;
identical output shares a figure, and rejected replies/Stop have no moving
plot. Failed middle-region and missing-pace-wave selections are directly
visible beside their requests. Detailed plots of all ten new recipes, legacy
Stroke/Pulse, speed extremes, a failed section count and the captured timeline
were inspected. The full-library overview was also inspected.

The [visual review process](motion-visual-review.md) is now repository
contribution guidance. Artifacts are generated with matplotlib outside the app,
remain ignored and contain no credentials. The browser atlas is a local inert
report. Startup/retarget coverage is distinguished from steady playback; the
captured timeline shades queued output canceled by Stop.

All public/development Go tests, full race suites, vet, lint, import boundaries,
goleak/Stop checks and frontend typecheck/build/tests passed. The frontend has
442 passing tests. Public route/start tests and linker inspection confirm that
labs are absent while the shared compiler is present. Ten exported library
bakes reimport through the v1 format as editable approximations, not portable
recipes. Measurements are in the [scorecard](goal-scorecard.md).

The review app at `http://127.0.0.1:49841` has labs enabled, simulation off,
Original Handy settings and preserved 10–43% speed limits. Native generation
readiness and a text-only app chat passed, the latter with one provider call,
no repair/fallback and no motion. A browser-only Library Lab request produced
Tip-anchored variety and left the engine idle. The read-only Cloud check at
23:43 UTC returned `DeviceNotConnected` after 477 ms. Physical acceptance remains
open for the user's next connected audition. Cloud scheduling, mapping,
direction reversal, controller ownership and Stop remain on established paths.
