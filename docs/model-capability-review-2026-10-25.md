# Local capability levels and response-quality review — 2026-10-25

This follows the [independent vibration review](intiface-vibrator-review-2026-10-25.md).
The owner requested natural patterning without deterministic language rules,
full guidance when it improves quality, and local Simple / Intermediate /
Advanced levels with the external feature UI available in Advanced.

## Implemented selection

Setup and Chat model settings share a backend-defined capability selector.
Simple remains the default. Intermediate raises context to at least 32K and
output to 512; Advanced raises them to 64K and 1,024. Larger saved budgets are
preserved. Managed llama.cpp and Ollama receive the allocation; an external
llama.cpp server needs its own matching allocation. The UI explains this and
the memory trade-off without moving advanced controls into ordinary chat.

Advanced local models now use the same counted-phrase and independent-vibration
contracts, bounded history, Auto timing, Prepare decisions early, and response
repair as external models. Actual availability follows the resolved role, so
an external chat route with a Simple local Autopilot does not falsely advertise
automatic timing. Repair also has the same stale-result/Stop guards; a missing
guard for local Advanced was found and fixed during integration testing.

Intermediate uses grouped interactive Layered edits, paired outer-range
endpoints, and explicit full-slider versus current-band mixture geometry.
All normalize into the existing semantic score and strict validator. Full
guidance/examples are retained. Shortening the guide lost useful behavior.
The richer grouped contract degraded some Layered Autopilot choices, so that
task keeps its established grammar. Creative v2 benefits from continuity and
constraint-scope guidance without changing its edit grammar. No classifier of
user wording, forced variety, fixed speed choice or fixed coverage band is added.

## Live models and matched results

Local: `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` through the
installed Ollama, seed 10232026, reasoning off, RTX 5070 Ti 16 GB. Simple tests
used the existing 16K / 256-output review settings; Intermediate used production
32K / 512-output settings. Advanced independent-control tests used 64K / 1,024.
External: `gpt-6.1-sol`, low reasoning, 64K application budget, using the existing
credential store in place. Credentials were not copied into review data.
The reported E4B model was unavailable and was not live-tested.

These are small deterministic-scenario evaluations of real generations, not
population success rates or a guarantee for arbitrary conversations. Intent
checks examine parsed changes and preservation of unrequested state, not just
JSON validity or a convincing reply. Elapsed time is per model turn on this host.

| Scenario | Simple 12B | Intermediate 12B | GPT Advanced |
| --- | ---: | ---: | ---: |
| 22-turn motion conversation across Layered and Creative v2 | 19/22; median 914 ms | 21/22; median 899 ms | 22/22; median 4,626 ms |
| 30 Autopilot continuations: exact holds, ongoing constraints, released holds, natural development and timed scope | 24/30; median 1,385 ms | 27/30; median 1,520 ms | 30/30; median 4,452 ms |
| Backend output awareness / unavailable output | Unchanged ordinary contract | 11/11; median 1,203 ms | Covered in capability cases below |
| Independent vibration, ten-turn conversation in each motion mode | Not enabled | Not enabled | 20/20; median 4,298 ms |
| Unsupported counts, individual motors, exact synchronization, long cycles, paused/unarmed/disconnected outputs and absent Handy | Not enabled | Not enabled | 16/16; median 5,129 ms |

The same 12B model in Advanced managed only **4/20** independent-control turns
(median 2,161 ms). Failures included unintended Handy edits, duplicate Creative
v2 fields, and invalid off/follow/steady envelopes. A larger context did not
make this model reliable at that task. Recommend Intermediate for this tested
model; Advanced is available for a capable local model chosen by the owner.

Remaining Intermediate misses are explicit: one upper-band request retained
95 instead of the requested 100 endpoint; three timed Autopilot checks changed
pace too soon or failed to change it after the requested minute. This work
does not conceal these misses with deterministic prose rules. Earlier guide,
schema and context variants are retained, including unsuccessful experiments.

GPT also passed the eight-turn actual HTTP-chat → provider → shared-engine →
fake-Buttplug sequence, including vibration-only startup, later Handy startup,
independent changes, counted phrases and Stop. It passed the 12-case compound
range set and six-turn phrase continuation set. Two earlier cloud timeouts
passed on a separate recheck. One later run accidentally used unsupported
`reasoning=none`; its 16 provider errors remain recorded, and the valid
low-reasoning rerun passed 16/16. Neither outage nor configuration errors are
counted as semantic successes.

## Corrections found through testing

An output-aware local reply could claim it had started vibration while the
only output was unarmed or disconnected. The continuous action schema now
permits only a reply in that backend state, and shared validation enforces the
same restriction even without constrained decoding. Examples describe the
actual human arming step. Other participating outputs retain their normal
authority; no model can change live intensity. A fixture asking the LLM to
implement global Stop was removed from that model-only set because production
Stop deliberately bypasses the model. HTTP/engine Stop coverage remains intact.

Integration tests also cover Advanced local automatic/manual repair, retirement
after a tier change, timing admission, stale prepared providers, local queue
coordination, independent-output arming/Stop, bounded conversation history,
context propagation, old-client settings preservation, and saved-role UI parity.
A final production-path audit caught output facts still gated on hosted routing.
They now follow Intermediate/Advanced capabilities, with a regression that
inspects the actual HTTP chat request and its reply-only schema for an unarmed
vibrator. Advanced local Edge and diagnostic-preview admission use the same
guards as their external counterparts.

## Shared-engine visual review

Artifacts are ignored under `.scratch/model-response-followup-2026-10-25/`.
The first atlas retains **1,266 records from 65 captures**, including invalid
responses, failed intent checks and rejected prompt variants. It renders 416
distinct sampled outputs on 26 overview sheets. The final Intermediate
Autopilot split adds 30 records in `split-atlas/` on two sheets. Every overview
was inspected, followed by detailed outliers. Identical plans share a plot;
the manifest preserves each request/result even when the plot title comes
from its first occurrence. Scalar envelopes were rendered by the shared engine
too: 17 distinct envelopes on two inspected overview sheets.

The full-slider mixture reaches 0 and 100 while preserving anchored short
upper strokes; the failing 65–100 case visibly remains below 100. Whole-loop
plots expose premature pace changes and retained restricted bands, so they
remain failures even when the prose promises compliance. The Advanced 12B
vibration-only request visibly changes the linear score, confirming the
independence failure. GPT's two-short/one-full sequence keeps its counts, full
reach and slower long stroke with continuous planned joins. The detailed
compound plot reports zero planned velocity and acceleration knot jumps;
quantized segment velocity is still piecewise and is not physical telemetry.
The pulse/wave plots show different rhythms, preserved silence, retained cycle
lengths and a smooth start ramp. Follow is shown with no linear input in this
envelope fixture, so its flat line does not represent live motion-follow output.

Representative details in `atlas/`: `motion-291a5a544a1ce8f7.png` (failed upper
endpoint), `motion-2074efeb9da7b223.png` (full-slider mixture),
`motion-bd2a2cf620c6bb76.png` (unwanted linear change), and
`motion-bb2866ab63e3833a.png` (GPT compound timing). No sampler, geometry engine,
transport mapping or physical smoothness policy is changed by these tiers.
No real hardware was connected or moved. The earlier Edge 2 physical acceptance
list remains open; these model/graph tests do not replace it.

## Validation and review app

Go unit/integration tests, race tests, vet, golangci-lint, architecture/goroutine
gates, the pure-Go build, frontend typecheck, localization and all 993 frontend
tests pass. The canonical embedded UI is rebuilt. See the
[budget measurements](goal-scorecard.md) and [ADR 0058](decisions/0058-model-capability-levels.md).

An isolated simulated review app on port 50290 uses the actual installed local
model. Its model probe and a text-only request through the app each complete;
chat uses one provider call, no repair/fallback and no motion. Browser review
verified saved Advanced exposes Auto timing and Prepare decisions early,
then returns to Intermediate as the recommended test configuration. Screenshots
of Advanced settings and timing remain with the ignored evaluation artifacts.
The final build also answers a real local request to start an unavailable,
unarmed vibrator by explaining the connection/intensity step, with one call,
no repair/fallback and no motion. Both final app responses are retained in
`review-release-ready.json` and the accompanying SSE captures.
