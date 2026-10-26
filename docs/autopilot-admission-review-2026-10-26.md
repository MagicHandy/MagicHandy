# Autopilot review follow-up — 2026-10-26

## Assessment

Reviewed Claude Code's Autopilot findings from the October 25 session
(`db4e280c-a1af-495f-b0f4-a98b7828abb7`, final review at 22:07 UTC).
All four findings are supported by the current source and targeted tests.
The inherited Stop/network fixes and the completed voice reliability changes
remain intact on `codex/voice-reliability`.

| Finding | Assessment and implementation |
| --- | --- |
| Generated planning context exceeds the typed-message limit | Confirmed. A realistic Layered phrase, pace layer and session context fails before reaching either a hosted or local provider. Autopilot now validates non-empty generated context independently. The unchanged provider-level total input budget still reserves schema, output and reasoning space; interactive messages retain the 4096-byte limit. |
| Failed planning looks like an intentional hold | Confirmed for continuous modes. The backend now counts consecutive failed motion decisions, including first-target failures and prepared decisions. After two, Chat's Autopilot panel shows a concise retry notice with a Diagnostics link. Successful planning, including a valid deliberate hold, clears it. Speech does not clear it; canceled/stale results do not count. |
| Check-ins display the local model despite using hosted conversation routing | Confirmed. Provider/model identity is captured from the resolved speech role at generation and carried through the scheduler to publication. A later routing change cannot relabel that response. |
| Tests check constraints but not autonomous development | Added a live, sequential scenario using the actual generated Autopilot context, parser and shared compiler, plus counterexamples for counted motifs and exact holds. No speed/pattern forcing rule was added. |

Keeping the existing motion after a failed continuous plan remains correct:
inventing a replacement pattern would violate the selected mode. The change
makes that failure visible rather than altering the fallback behavior. No
generator, sampler, transport, Stop semantics, timing policy, inference context
allocation or prompt wording changed.

The unit regression retains full generated context above 4 KiB and verifies
that its configured model budget admits it, a tiny budget rejects it, and
the interactive validator still rejects that same oversized typed message.
It exercises both hosted Advanced and local Intermediate contracts.

## Live model replay

Used the existing eval ChatGPT profile in place, without copying credentials,
with 64K context, None reasoning, no persona and no transport. Four decisions
per scenario reused the prior accepted score and the ongoing human direction.
The generated planning messages were 4248–4551 bytes, all above the old limit.

GPT-6 Sol passed all 12 decisions with one provider call each:

| Scenario | Selected speeds | Pattern result |
| --- | --- | --- |
| Give the model the lead | 35 → 42 → 49 → 43 → 36 | Changed reach/location geometry and texture, including alternating ends. |
| Keep two short upper strokes then one full sweep, develop pace | 35 → 44 → 53 → 62 → 40 | Preserved the two-plus-one motif on all four turns. |
| Keep this exact pattern and speed unchanged | 35 → 35 → 35 → 35 → 35 | Preserved the complete score on all four turns. |

Median generation latency was 5172.5 ms; maximum 9134 ms. This fixes admission
of planning calls, not their inference latency. Existing early preparation and
scheduler bounds remain responsible for timing.

A separate GPT-6.1 Sol attempt returned provider-unavailable errors on every
turn. Those failures are retained alongside the successful run; there is no
claim of GPT-6.1 motion quality from that attempt. No local-model quality gain
is claimed from the mocked local admission regression.

## Visual review

Rendered all 24 attempted results with the shared motion compiler. The atlas
contains nine distinct plots for the 12 successful results and explicit
non-moving records for the 12 provider failures. Inspected the overview and
detailed alternating-end and fastest counted-phrase plots:

- the free-development sequence changes both its outer band and internal
  reach modulation rather than merely renaming the same pattern;
- the counted sequence retains two upper strokes followed by a full sweep,
  with pace changes visible in the compiled trajectories;
- the four explicit holds share one identical output;
- the inspected detailed paths have zero analytical knot velocity and
  acceleration jumps; wire points follow the rounded trajectories.

This is commanded-motion evidence, not physical feedback. Steady trajectories
do not validate new hardware behavior or transition feel; the motion engine
and transport code are unchanged. No real hardware was connected or moved.

## Validation and evidence

Regression coverage includes rich prompt admission, input-budget preservation,
valid holds versus failed planning, scheduler recovery, stale-generation
results, run reset, speech provenance through routing changes, and warning
visibility/recovery. The full frontend suite passes (130 files, 1062 tests),
with typecheck, five-language audit and canonical build. Full Go tests, vet
and lint pass, as does the full race suite. The first race run failed a simulated
Intiface vibrator startup test outside the changed Autopilot paths, without a
race report. That test passed three consecutive repeats and the complete race
rerun passed; the initial failure log is retained.

The user refreshed the isolated review app at `http://127.0.0.1:50301` with
the staged current binary; SHA-256 matched the tested build. The review LLM
probe completed a real generation using Ollama's
`huihui_ai/granite4.1-abliterated:3b`. A separate text-only request through
the app's Chat UI returned "I am ready to chat." in 145 ms, with one provider
call, no repair/fallback and no motion. The review tab remains at `#/chat`,
with the Autopilot controls visible, Autopilot off and LLM motion off. This
small local model is for the review app's text readiness; hosted motion
quality evidence is the GPT-6 Sol replay above.

Ignored local evidence is retained under `.scratch/autopilot-review/`:
`before.log`, `targeted-fixed.log`, both model JSON reports and logs,
`atlas.json`, the rendered `atlas/`, suite logs and measurements. An early
fixture check caught invalid outer bounds and a context allocation that did
not match its selected capability; the final regression uses valid geometry
and the actual capability minimums.

`review-llm.log`, `final-chat.json` and `final-chat.png` retain final-build
readiness and UI evidence. Runtime evidence remains ignored, outside Git.
