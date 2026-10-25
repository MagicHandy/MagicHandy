# Alpha.74 release review — 2026-10-25

Dates use America/New_York. The maintainer requested fixes for Intiface
vibration, model capability levels, local-model evaluation, Claude's model
settings redesign, an Intermediate Autopilot latency toggle, and an updated
llama.cpp followed by a release. The release branch starts at alpha.73,
`25b0e193`. No installed-user profile or physical device is changed in this
integration review.

## Scope and prior evidence

The [vibrator review](intiface-vibrator-review-2026-10-25.md),
[capability evaluation](model-capability-review-2026-10-25.md),
[E4B and prompt-set evaluation](e4b-simple-review-2026-10-25.md), and
[model settings review](chat-model-settings-review-2026-10-25.md) retain
response quality, rejected alternatives, source review, visual atlases and
limitations. ADRs 0057–0060 describe the shared-engine envelope, model tiers,
manual resume and planning note. These remain model/command evidence, not
physical vibrator or smoothness telemetry.

Claude's uncommitted model-settings work was imported as a source snapshot
from `MagicHandy-model-settings` at base `06f934da`; that checkout was not
modified. The canonical UI was rebuilt. Models and Model roles remain the two
settings cards, capability is per model/connection, motion options stay on
Chat, and reply length stays under Prompts & memory. Existing external
connections retain Advanced when their old saved settings omit the new level.
New compatible endpoints default Simple. Backend projections drive feature
visibility and allocation floors.

## Intermediate planning toggle

Chat > Autopilot > Extra planning defaults on, preserving the evaluated
Intermediate decision note. Off removes only that note and its schema/output
reservation; it uses the already evaluated shorter Intermediate decision path.
Simple E4B planning and Advanced contracts are unchanged. The label explains
the observed 0.4–0.7 s per-decision cost on this host without promising the same
timing on other hardware. ADR 0060 retains the original paired measurements.

Persistence tests cover explicit false and old saved settings. HTTP tests
switch off/on/off, send an older-client update without the new field, and check
the next production Autopilot request for both Layered and Creative v2. The
frontend follows the resolved planner capability, handles save failure, and
keeps the control absent for unrelated tiers and modes. The toggle does not
change context allocation, cadence, safety limits, or provider call count.

## Runtime update

The optional managed runtime advances from b11149 to upstream
[b11533](https://github.com/ggml-org/llama.cpp/releases/tag/b11533), commit
`64df9183f5ca178c049a392f35e795d8e97c97b3`. CPU and CUDA 12.4 installers both
completed in isolated data directories, verified the pinned archive sizes and
SHA-256 digests, and reported build 11533 / commit 64df9183f. Existing installed
runtimes and model files were left untouched. The setup module's displayed
source version now derives from the runtime constants.

b11874 was also installed and tested, then rejected for this release. With
identical E4B prompts, settings and five sampling seeds (10252026–10292026),
it passed 76/110 conversation intent checks versus 84/110 on b11149. Warm
repetition retained the difference. The rendered chat templates and sampler
settings matched; upstream Gemma execution changes are a possible cause, not
a proven root cause. b11533 predates the Gemma embedding refactor and reproduced
the old runtime's per-seed results: 18, 17, 17, 16 and 16 out of 22.

| Runtime | Chat, five seeds | Autopilot, two seeds |
| --- | --- | --- |
| Previous b11149 | 84/110 | 51/60 (prior captured evaluation) |
| Candidate b11874 | 76/110 | 52/60 |
| Selected b11533 | 84/110 | 51/60 |

Chat medians on the matched old/newest pair were 939/930 ms, so the observed
regression was quality rather than latency. Selected b11533 Autopilot medians
were 1,636/1,861 ms versus the earlier b11149 1,754/1,828 ms: no consistent
latency penalty is established. Small-model misses still include retained
regional restrictions, unnecessary reseeding and timed-hold errors. This does
not make the 84/110 score a general model-quality guarantee.

CPU b11533 completed a real structured E4B readiness generation in 969 ms;
CUDA completed the response matrices. A first candidate harness launch raced
model loading: its 44 HTTP-503 outcomes are retained and were rerun only after
health reported ready. The selected-runtime scores use those complete warm
runs; loading errors are not silently relabeled as intent passes. All runtime
captures, including failed selections and the rejected build, remain under
`.scratch/intermediate-planning-toggle/`.

## Validation and budgets

Full Go and Windows race suites, vet, lint (0 issues), import boundaries,
pure-Go build, frontend typecheck/localization, all 1,004 frontend tests, and
the canonical UI build pass locally. Targeted runtime/HTTP/config/chat tests
were repeated after updating the pin. Installer suites pass on Windows
PowerShell 5.1 and PowerShell 7. The release architecture tests pass with only
alpha.74 added to the reviewed unsigned policy; alpha.65 and alpha.75 stay
rejected. The final PR and merged-main CI must pass before tagging.

The matched stripped Windows core grows from 24,197,120 B at `cd9aef7e` to
24,220,160 B. Canonical UI is 2,805,447 B; main JS is 1,292,736 B raw /
347,451 B gzip, CSS 208,113 B raw / 36,328 B gzip. No runtime Go/npm dependency,
new background loop or extra model round trip is introduced. Optional llama.cpp
archives are excluded from the app installer. The scorecard records these
measurements; no RSS/cold-start improvement is claimed.

The attempted final Claude Opus 5.5 Max independent review reached its account
session limit before returning findings. Earlier independent code and blinded
model reviews are linked above; this is not represented as a final Claude
sign-off. Integration review and automated checks are performed locally.

## Current app and visual acceptance

The final source build runs at `http://127.0.0.1:50297` with fresh isolated data,
remote listening disabled and simulated motion. It uses the available Ollama
Gemma 12B at Intermediate, without copying credentials. `check-review-llm.ps1`
completed real generation. The text-only app chat returned a nonempty reply in
5,777 ms including 5,390 ms cold model loading, using one provider call, with
no repair, fallback or motion. The engine remained idle.

In the combined desktop UI, Extra planning was turned off, reloaded, turned on
and reloaded again; both choices persisted. It appears for the saved
Intermediate planner in Layered, and remains absent in Simple or motion Off.
The final review is left at Chat with the default enabled preference and idle
simulated motion. The previous review instance was preserved after automatic
approval blocked replacing its process; the final app uses a separate fresh
profile instead.

The runtime atlas retains **516 records, 177 distinct plots and 12 overview
sheets**. All sheets were inspected, including failed region selections and
loading failures. Detailed inspection included selected b11533's full sweep
(`motion-b16a1519400f867c.png`) and fixed-speed/free-shape Autopilot
(`motion-4d9fd724ed826e11.png`, span CV 0.238), plus rejected b11874's retained
upper-only region (`motion-d69cfee3e8ae5300.png`). The selected full sweep reaches
0–100 with rounded turns; the varying phrase stays continuous and the sampled
wire follows it. The rejected full-range request instead remains around 65–100.
A continuous curve does not excuse that intent failure. These are shared-engine
commanded plots, not measured carriage motion. Existing sampler, transport and
physical smoothness claims are unchanged by the runtime selection.

## Remaining acceptance and release policy

No physical device is connected or commanded in this review. Real Edge 2
acceptance still needs first playback alone/beside each Handy transport,
independent envelope transitions, Stop/manual resume, reconnect and linked
failure checks. Physical multi-Handy synchronization and loaded COYOTE output
remain unverified. Existing Intiface Central device-Stop and browser-suspension
limits remain disclosed in the public notes.

Only `0.1.0-alpha.74` is newly authorized under ADR 0014's reviewed unsigned
policy. Packaging format, five payload executables, three public assets,
installation behavior and signing requirements are unchanged. Exact source and
binary vulnerability scans, Defender, manifest/outer checksums, clean current-main
provenance and installer lifecycle gates remain mandatory. Microsoft's prior
case does not approve new file hashes. Publication is only from the tag workflow.
