# Hosted Layered operation review — October 18, 2026

## Failure and final behavior

The supplied October 16 diagnostic came from alpha.63, `5e02daa0`, using
ChatGPT `gpt-6-sol` and Layered motion. A completed, single-call response
supplied absolute `controls` and relative `change_by` for six of the same
fields. The validator correctly rejected it with `cannot set and adjust the
same control: min_percent`. Zero deltas were not the whole problem:
`anchor_percent` also contained an absolute zero and a relative -10.

Follow-up live tests exposed three related problems: named geometries were
combined with contradictory shape fields, one layer was both edited and
removed, and Autopilot's generic pace-development instruction competed with
lasting user requests to retain a pace. The final hosted contract addresses
those causes while keeping interpretation with the model:

- Every numeric control and layer period has one explicit `set` or `adjust`
  operation. Zero remains a real value, not an omission marker.
- `composition: "geometry"` selects a named shape and exposes independent pace
  and outer-band controls, without competing width, anchor, variation-mode or
  range/center-layer edits. `composition: "full_slider"` is a separate choice
  because it replaces the outer band. `composition: "manual"` retains all
  controls and layers for custom shapes. Empty edits preserve the score.
- Layers are keyed by axis. Each chooses `{"op":"edit","value":{...}}`
  or `{"op":"remove"}`. The choice comes before its optional patch fields,
  so a constrained decoder cannot enter removal merely by emitting `op` first.
- Hosted conversational Autopilot resolves active human directions before
  choosing development. Silence, repeated speeds, elapsed time and change
  preference do not cancel a lasting direction. A later request can replace it.
  This guidance applies to hosted Layered and Creative v2; the technical-only
  planner retains its existing state-only guidance.
- Code-owned examples are translated fully on their first rendering. The
  strict-schema adapter also restores omitted optional enum fields inside
  operation unions without treating their nullable placeholders as selectors.

The backend translates only explicit structured operations into the existing
domain edit. No keyword detection, language classification, inferred motion
locks, conflict guessing, corrective retry or extra provider call was added.
Saved limits, recall, lifecycle authority, the shared compiler, dispatch and
Stop remain the authorities. Invalid or ambiguous proposals still reject
atomically. Raw responses remain available in diagnostics.

Runtime provider selection enables the hosted contract for chat, Autopilot
and hosted second opinions, including when the immediate model is local.
Local Ollama/llama.cpp and Labs keep their compact contract; the Creative v2
edit grammar is unchanged. Persona text and conversation history are not
rewritten. No dependency, UI bundle or background task was added.

## Regression and integration validation

- Complete-score equivalence with original domain edits covers every named
  geometry, custom shapes, absolute zero, relative controls and periods,
  edit/removal operations, recall, hold and empty edits.
- Atomic rejection tests cover duplicate fields, unknown operations, competing
  legacy groups, conflicting geometry fields, edit/removal collisions, malformed
  values and bounds. Lifecycle, auto timing, reviewer and inspector tests pass.
- Real Responses-adapter tests exercise both ChatGPT plan usage and API mode,
  including wrapped action unions and strict nullable output, in one call.
- Independent JSON Schema validation accepts eight valid examples and rejects
  six invalid examples in both domain and strict wire schemas. The archived
  overlap, geometry/anchor conflict and edit/removal conflict are included.
- `go test ./...`, `go test -race -timeout 20m ./...`, `go vet ./...`,
  `golangci-lint run ./...` and the `CGO_ENABLED=0` build pass. Frontend
  typecheck, all 913 tests and build pass; canonical dist is unchanged.
- Ten local Layered/Creative v2 state fixtures retain identical interactive,
  autonomous and schema hashes. No additional local inference work is added.

## Real ChatGPT evaluation

The opt-in live tests use the existing protected host sign-in with `gpt-6-sol`,
reasoning off, synthetic requests and production chat/planner/reviewer services.
They create no engine or transport and copy no credentials into review data.
All intermediate responses, including wrong choices, remain in ignored reports.

The original operation-only evaluation retained seventeen generations: all
parsed, but two of three Autopilot probes raised a requested fixed 25% pace to
30%/28%. The expanded baseline reproduced four geometry conflicts in fifteen
calls. The following iterations explain why prompt guidance alone was not used
as the final fix:

| Retained report | Parsed | Intent checks | Finding |
| --- | ---: | ---: | --- |
| Expanded baseline | 11/15 | 10/15* | Conflicting geometry fields; missed drift request |
| Prompt guidance only | 26/30 | 26/30 | Four geometry contradictions remained |
| Exclusive composition | 29/30 | 29/30 | One layer edited and removed together |
| Per-axis operations | 30/30 | 26/30 | Shared composition selector could steer decoding into full-slider travel |
| Separate full-slider selector | 30/30 | 29/30 | One reply changed unrequested pace/band |
| First-pass examples and planning check | 36/36 | 34/36 | Custom-layer edits exposed late edit/remove selection |
| Final expanded planning | **36/36** | **36/36** | All eighteen cases passed twice |
| Final operation/reviewer suite | **8/8** | **8/8** | Recall, relative edits, explanation, Autopilot and correction passed |

*The baseline JSON retains its original 9/15 flag count. The hold-all case
returned the exact unchanged score and a valid 180-second next check; the test
incorrectly imposed a 120-second ceiling. The evaluator now uses the production
8–300 second limits. That fixture error is not an application failure and the
original evidence is not rewritten.

The final 44 calls all parsed and met their intent checks, with no repair or
fallback. Latency was 2,698–6,581 ms, median 4,279 ms. This is a small host/session
sample, not a provider benchmark or a guarantee of arbitrary intent accuracy.
The eighteen planning cases cover numeric, paraphrased and Spanish pace
retention; a ten-minute-old lasting constraint; a fixed travel band with pace
free to develop; exact hold; a later release from 25% to 35%; custom widths and
anchor; and all nine named geometries. Checks compare actual scores with the
requested dimensions, including unchanged outer bands, rather than trusting
reply text. The eight additional cases exercise zero, relative speed, relative
period, alternating ends, explanation without motion, recall plus adjustment,
Autopilot continuation and a hosted correction of a missed local reduction.

These results address the observed failures. Other models, longer conversations
and ambiguous requests can still produce semantically wrong but valid choices;
the existing validators and optional reviewer remain relevant. No real device
was connected or moved, and no physical smoothness or Stop claim is made here.

## Shared-engine visual review

The combined atlas retains **233 records**, comprising every baseline,
intermediate and final generation plus the neutral archived-rejection fixture.
The shared compiler rendered 97 distinct outputs and seven overview sheets;
identical outputs share plots. Invalid proposals retain explicit failure cards.
All seven overviews and detailed outliers were inspected.

Detailed comparisons include the wrong full-slider selection for a 20–70 band,
the unrequested 50% pace, lost center alternation in custom widths, and the
unrequested 30% custom-anchor pace. Their smooth curves do not turn them into
intent passes. Final details for fixed-band planning, custom alternation,
custom anchor, full-and-tip and irregular wandering retain their requested
travel relationships. The whole-percent output follows the planned trajectories
without new flattened turns or obvious quantization chatter in those excerpts.
Across the 44 final entries the largest reported knot jumps are approximately
0.0000035 percentage points/s in velocity and 0.0000475 percentage points/s² in
acceleration. These are steady commanded estimates, not carriage telemetry or
startup/retarget tests. The shared engine itself is unchanged.

Artifacts are ignored under `.scratch/layered-planning-followup/`: `baseline.json`,
`candidate-1.json` through `candidate-5.json`, `final-planning.json`,
`final-operations.json`, `atlas-all.json`, `atlas-all/index.html`, schema checks,
local hash captures, readiness/chat evidence and test logs. Earlier reports and
the neutral rejection remain under `.scratch/layered-contract-review/`. The
original diagnostic archive and user conversation are not committed.

To repeat, set `MAGICHANDY_HOSTED_DATA_DIR` to an existing signed-in profile,
`MAGICHANDY_LIVE_MODEL=gpt-6-sol`, and `MAGICHANDY_EXPERIMENT_CAPTURE` to an
absolute ignored path. Run each `TestHostedLayeredLivePlanning` and
`TestHostedLayeredLiveOperations` separately with `go test -tags liveeval
./internal/chat -run '^TestName$' -count=1 -v`, choosing separate capture files.
`MAGICHANDY_EVAL_REPEATS=2` repeats the planning cases. These consume plan usage
and fail on rejected or wrong choices. Export all reports with the documented
`motion-atlas -catalog=false -llm ...` workflow.

## Review app and cost

The current build runs in an isolated simulator profile at
`http://127.0.0.1:50257/#/chat`, with Layered selected and an available local
Gemma model. `scripts/check-review-llm.ps1` completed real generation. A
text-only app chat returned a non-empty explanation in 1,048 ms, one provider
call, no repair/fallback and no motion; this is separate from the live ChatGPT
evaluation. Existing user-launched sessions and credentials were preserved.

With identical Go 1.26.9 Windows amd64, `CGO_ENABLED=0`, trimpath, stripped and
version-metadata flags, the core grows from alpha.63's 23,264,768 B to
23,321,600 B (+56,832 B, 0.24%). That includes the earlier operation fix and
this follow-up. No new RSS or cold-start measurement is claimed.
