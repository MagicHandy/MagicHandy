# ADR 0050: External Autopilot planners: own timing and prepared decisions

## Status

Accepted 2026-10-16. Measured in transport-free simulated sessions only; see the
[auto timing review](../autopilot-auto-timing-review-2026-10-16.md). Amends the
model-timing rule and the planning lead in
[autopilot-cadence.md](../autopilot-cadence.md) for external planning routes
only. Local planning is unchanged.

## Context

Autopilot decides when to reconsider motion with a backend clock. Motion change
rate 1–8 selects a window (level 4: 20–60 seconds), and the model may only pick
`soon`, `normal` or `later`, which the backend samples inside that window. Small
local models settle on one category, so backend sampling keeps their timing
varied.

In Creative v2, the default mode, and Layered, the planner's category was not
even read: `completeLayeredAutopilot` returned `normal` for every turn. Every
stretch was a uniform draw from the middle of the window, so its length was
unrelated to what the motion was doing. A build was re-decided before it landed,
an accent could not be shorter than the window, and a stretch the person was
enjoying ended on the same clock as any other.

Each decision was also planned one measured latency (plus 0.5 seconds) before
its boundary. An external call slower than the previous one therefore made the
change late, and a slow outlier delayed it by up to the 10-second lead cap and
more.

Hosted models (ChatGPT, OpenAI, OpenRouter and compatible connections) answer
in 2–6 seconds over their own request lanes and can judge a horizon from the
motion and the conversation.

## Decision

1. **Auto timing is an explicit switch.** `autopilot.auto_motion_timing`, off
   by default, is honored only when the motion planning route resolves to a
   hosted connection (`PlanningSettings().IsHosted()`). The Autopilot card
   offers it only for a hosted Autopilot route. Local planning, speech
   check-ins, Decisions and technical candidate selection keep the sampled
   window.
2. **The model chooses seconds; the backend keeps the deadline.** Motion turns
   on that route require `next_check_seconds` (8–300) in place of the
   category. The scheduler varies it by up to ±12% from the seeded cadence
   stream and clamps it to 8–300 seconds. Holds carry a horizon too. A missing
   or unusable value falls back to the sampled window. Chat, pause, Stop,
   settings changes and generation invalidation keep their authority: a chat
   turn still brings the next decision forward.
3. **Timing guidance, not rules.** The prompt says that until the next
   decision the person feels one unchanged motion, gives the Motion change
   rate window as the reference for an ordinary stretch, and names reasons to
   leave it: an accent, a build, a stretch the person enjoys or asked to keep.
   The planner's last six horizons are shown as observations. There is no
   text-matched rule and no bound inside 8–300 seconds. Without the reference,
   GPT-6 Sol held one pattern for 48–94 seconds in sessions with no request.
4. **Prepared decisions.** `autopilot.prepare_next_decision`, on by default, is
   honored for any external planner (a hosted route or Decisions). The next
   decision is planned as soon as a stretch is armed and waits, as the
   existing pending choice, for the stretch's deadline. Its scheduler facts and
   the chat ages it reads are projected to that deadline, so without an
   interruption they are what will be true when it applies. Chat activity,
   settings changes and Stop discard it, and Autopilot prepares again with the
   person's words once chat finishes; pause shifts it with the deadline.
   Speech check-ins are never prepared.
5. **Local prompts are unchanged.** Schemas and prompts for local planners keep
   their exact bytes; the auto-timing field is added only to hosted turns, and
   local planning keeps its just-in-time lead.

## Consequences

- With Auto timing on, the request rate follows the model: in the measured
  sessions it ranged from about a quarter of the fixed rate during a requested
  hold to slightly above it while the pace was being corrected. The 8-second
  floor bounds the worst case at 7.5 calls per minute, the same floor as the
  fastest fixed level.
- Motion change rate stays meaningful under Auto timing, as the reference
  length rather than a window.
- Prepared decisions add no request while the person does not interrupt. Each
  interruption discards at most one prepared call. Replaying 104 measured
  GPT-6 Sol latencies, the just-in-time lead made 30% of timer boundaries late
  (mean 0.63 seconds, maximum 1.49 seconds); prepared, none was late, and a
  call can take up to the whole stretch before the change waits.
- A prepared decision does not see what happens during its stretch other than
  an interruption. Projected ages assume the stretch plays as armed.
- While a prepared decision waits, the status still counts down to its
  deadline and marks it ready.
- Ramped transitions, queued follow-up edits and phrase-aligned boundaries for
  local models remain separate work.

## Validation

Unit tests cover both settings and their defaults, the schema and prompt
(hosted only, speech excluded, local bytes unchanged), the scheduler's jitter
and bounds, holds, the recent-horizon memory, the hosted end-to-end decision,
prepared planning at the start of a stretch with projected facts, an on-time
change despite a slower call, the just-in-time slip it replaces, discard on
chat, and projected chat ages. `TestLiveAutopilotAutoTiming` (build tag
`liveeval`) drives simulated Creative v2 sessions against a hosted model
through an existing signed-in profile (`MAGICHANDY_HOSTED_DATA_DIR`, used in
place, never copied) or a loopback llama.cpp control. Physical feel is not
established by these runs.
