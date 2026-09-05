# Creative v2 continuous flow refactor

The user reported good individual stretches but a session that felt like
separate patterns in sequence. Inspection confirmed that the implementation
actually selected blocks of local/full strokes, added nested rebound groups,
and used rest-to-rest travel at every destination. Smooth position joins did
not establish continuous motion character.

## User-stopped reference

The existing review app's trace was captured read-only after Stop. It used
Cloud REST, Original Handy limits of 20–60% speed and 0–100% device range.
The final run contained score updates roughly every 10–15 seconds. Its 91
transport results had median latency 349 ms, p95 410.5 ms and maximum 887 ms.
The last selected handoff was at stream 81.882 seconds, after the estimated
Stop time of 79.538 seconds. The favorable playing stretch therefore belonged
to the preceding score (`flow-4d64a5225cbd8f07`), not the newly selected target.

Raw traces and the rendered whole-run/final-stretch command timeline are in
`.scratch/creative-v2-flow-refactor/user-stop-*`. These are queued semantic
commands, with Stop aligned using trace timestamps, not carriage telemetry.
The trace retains IDs and commands rather than the complete earlier semantic
score, so that exact earlier score cannot be reconstructed from its ID alone.
No new device motion was issued during this evaluation.

## Changes

- Reach and pace now follow correlated fields. Intermediate focus mix moves
  through intermediate widths instead of switching between two categories.
  Full-only and local-only remain hard spatial requests. A high local mix no
  longer promises a scheduled full stroke every six cycles.
- Rebound decay changes following excursions and recovers gradually. It does
  not append another nested pattern. The default phrase is 64 cycles; seeds
  still support exact replay, and `evolve` selects a new realization.
- Rounded travel carries acceleration through reversals. Neighboring strokes
  share one acceleration; its correction is distributed through the stroke.
  Local timing is fitted against the actual joint interpolant and rejected if
  it cannot converge within existing velocity, acceleration, jerk and reversal
  limits. The first attempted endpoint-only correction preserved safety but
  erased direction contrast; it was rejected and replaced by this distributed
  correction.
- Gesture edits retain nearby stroke context rather than selecting a matching
  point anywhere in the loop. The existing shared transition, sampler,
  sanitizer, dispatch ownership and Stop still execute the result.
- The compact edit schema and voice/history contracts are retained. Prompt,
  speech facts and localized Lab help describe the new semantics. Autopilot
  guidance asks it to develop the current motion rather than replace all its
  controls at each planning turn.

## Visual and numerical review

The baseline, intermediate 32-cycle candidate and final 64-cycle matrix each
contain 108 cases: nine gesture combinations at 10/45/85 on all three supported
device profiles, plus unchanged Original Creative references. Each renders to
90 distinct figures and six overview sheets. Final and baseline overview sheets
were inspected; detailed comparisons include low/middle/high speeds and the
narrow, rebound and high-contrast outliers.

The live model atlas retains all 33 records, including failed intent checks,
and renders 27 distinct outputs on two overview sheets. Both sheets and the
failed selections were inspected. Two additional captured simulator timelines
cover actual queued app dispatch through Autopilot retargets and Stop, with
trace-time alignment and the canceled queued remainder labeled explicitly.
The full-session timelines still show model-driven changes of region and
concentration: Gemma settles into upper work, and Granite selects a nearly
fixed 40–50% band in the latter half. The refactor removes the generator's
block schedule; it does not eliminate all perceptible changes of intent
between model turns. The existing 750 ms handoff is retained.

The final whole-loop plots replace the rectangular local/full envelope with
progressive narrowing and expansion. Base/tip anchors remain identifiable;
mixed-center work changes width without transfer legs or incidental reversals.
The sampled Original Handy 45% comparisons reduced the largest neighboring
span change from about 36 to 19 percentage points for irregular mixed work,
and 74 to 45 for base rebounds. These sampled proxies are not feel ratings.
Rounded reversals reduce the slow shoulders visible in the local sweep and
narrow-band details. Strong directional contrast still concentrates velocity
in one direction, and extreme requests can saturate the existing limits.

Artifacts are under `.scratch/creative-v2-flow-refactor/`: the three atlases,
`comparison.json`, the captured user timeline, full-app model reports and test
logs. They remain ignored and are not embedded or committed. Use the existing
motion-atlas commands with `-creative-v2`, or `-sessions` and `-llm` for the
full-app reports, following [the visual review process](motion-visual-review.md).

This is a refactor of motion generation and continuity, not proof of natural
physical feel. The finite realization still repeats without evolution. Models
can still choose a large change of intent, settle into one region, omit a
requested control or describe their current motion imprecisely.

## Full-app checks

The full Windows build used llama.cpp b9966. Gemma 4 12B used the already
available CUDA endpoint without changing the user's device session; Granite
4.1 3B used a separate CPU-only worker from the same installed runtime. Tests
ran in an isolated simulator app without copying device credentials.

Both models returned valid responses for all eight Creative v2 control turns.
Gemma passed seven strict intent checks; its lower-end focus of 20% failed the
fixture's exact-zero condition. Granite passed six: it omitted the entire-slider
range edit on start and retained a middle focus when asked for the lower end.
These failures remain in the reports. No claim of perfect motion mapping is made.

Gemma's 130-second no-request Autopilot run accepted ten targets and produced
twelve check-ins; Granite's 100-second run accepted six targets and produced
eight check-ins. Both stopped successfully. Gemma still chose substantial
changes of range and pace and eventually concentrated near the upper end.
Continuous generation improves the path between those choices; it does not
guarantee balanced region coverage or truthful prose. The new wording did not
change voice/history contracts or introduce a compact-model prompt override.

Full Go tests and race tests, vet, lint, frontend typecheck, 473 frontend tests,
localization checks and the canonical production build passed. Existing
kinematic and replay tests cover seeds, direction contrast, device envelopes,
live limits and C2 seams. New checks cover intermediate reach, continuous turn
acceleration and retained stroke context across pace edits. An additional
72-case matrix checks compiled bounds with varied seeds, narrow bands,
independent controls, explicit/default cycle counts and all device profiles.
