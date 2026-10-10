# Technical-context Autopilot: pace and reach — October 9, 2026

Follow-up to the [hosted-model review](cloud-model-review-2026-10-08.md) and
[ADR 0039](decisions/0039-hosted-model-connections.md).

## Question

With technical context, the default when chat stays local and a separate model
plans Autopilot, Creative v2 and Layered Autopilot used its own refinement
prompt: the Lab contract, one sentence of instructions, temperature zero, and a
rule that rejected any change to pace or outer range. Local Autopilot instead
plans with the continuous Autopilot contract, its pace policy, the session
buildup and recent speed and position history. This review measures what the
technical path actually did and whether it can share the local planner without
sharing anything personal.

## Measurement

`autopilotDecide` ran simulated 16-stretch sessions (30 seconds per stretch,
saved speed range 10–80%, motion style balanced, change preference 4/8) against
local Gemma 12B (`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`)
with both context policies on the same model, so only the planner differed.
Scenarios: an open session from 30%, a session with an eight-minute buildup
armed from 20%, and a session where chat had set 15% after the person asked to
keep it slow. Each cell has two sessions; the earlier technical path has one.

| Scenario | Planner | Speed span | Last minus first third | Held or rejected |
| --- | --- | ---: | ---: | ---: |
| Open | Technical, before | 0 | 0 | 4/16 |
| Open | Technical, after | 24.5 | +8.7 | 0/32 |
| Open | Conversation | 20.0 | +1.5 | 0/32 |
| Buildup | Technical, before | 0 | 0 | 3/16 |
| Buildup | Technical, after | 28.0 | +7.8 | 0/32 |
| Buildup | Conversation | 33.0 | +12.6 | 0/32 |
| Kept slow | Technical, before | 0 | 0 | 1/16 |
| Kept slow | Technical, after | 51.5 | +41.5 | 0/32 |
| Kept slow | Conversation | 8.0 | +1.7 | 0/32 |

Before the change, technical Autopilot varied texture in most stretches but held
every session at its starting pace and outer range, including with the buildup
armed; its rejected pace and range proposals became holds. After it, technical
sessions use pace across the saved range and follow the buildup much like the
conversation planner, at the same latency (median about 1.3 s).

The kept-slow sessions show the boundary of technical context. While chat's 15%
remained in the three-minute speed history, the technical planner kept 15–17%
and developed reach and texture instead; once it left the history, pace built
to the upper half. The conversation planner, which reads "keep it slow", stayed
in the lower third throughout. Motion state cannot tell a lasting wish from a
passing one, so the context setting's existing advice applies: choose
conversation context when conversation continuity matters.

The earlier refinement prompt also interacted with the shared reach guide: with
the guide in the Lab contract, the model widened the default 5–95 range to 0–100
on every call and the range rule rejected all of them. Removing that path removes
the interaction.

## Change

- In Creative v2 and Layered, technical context now uses the same Autopilot
  planner as conversation context, shown motion state only: the built-in
  behavior profile and utility voice, with no conversation, persona, memories,
  reaction style or spoken lines. The turn says the conversation is not shared.
- The scheduler's speed history marks speeds set through chat, without any
  words. Technical planning names the most recent one and keeps pace near it
  while it remains in that history, developing texture, reach and location
  instead.
- Pace and outer reach may change within saved limits; the shared validators,
  engine and Stop are unchanged. Hosted planning keeps admission fencing, one
  generation, no repair and no local fallback.
- Pattern, Decisions and Dynamic technical planning are unchanged. Dynamic's
  local planner reads custom anchor names, which technical context excludes, so
  it keeps its candidate refinement for now.

## Visual review

The shared-engine atlas renders 240 cases, 180 distinct plots and 12 overview
sheets from the after, conversation and before sessions. All were inspected:
every accepted score stays inside its band without spikes or extra reversals.
Slow technical stretches at 15% show narrower outer ranges (about 12–90) with
broad and local strokes; the before sessions show texture-only variation at one
pace. These are commanded estimates, not device feedback.

## Verification and limits

Tests cover the technical prompt (no personal text, chat-set pace named, local
planning unchanged), speed provenance in the scheduler, and an HTTP path in
which a hosted technical plan changes pace and range and still abstains from the
library fallback. Sessions are simulated at the planner boundary with one local
model; they do not establish hosted-model behavior, physical comfort or how
often a real session's admission fence discards a plan. Reports, logs, the
probe sources and the atlas are retained under
`.scratch/technical-autopilot-20261009/`.
