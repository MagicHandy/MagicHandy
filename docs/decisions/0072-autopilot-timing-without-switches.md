# ADR 0072: Autopilot timing without switches

## Status

Accepted 2026-11-12 at the owner's direction. Amends
[ADR 0050](0050-hosted-autopilot-auto-timing.md) and
[autopilot-cadence.md](../autopilot-cadence.md).

## Context

The Autopilot card carried five timing switches:
- Adaptive motion timing and Adaptive speech timing;
- Auto timing;
- Prepare decisions early;
- Session tracking.

It also offered custom check-in seconds. The owner asked to keep what the
model manages well with the model, and to drop switches that only tune its
timing.

The switches did less than their names suggest:
- **Adaptive timing never added randomness.** Every delay was already a random
  draw inside the change-rate window. On, the model's soon, normal or later
  answer narrowed the draw to an overlapping portion of the window; off,
  that answer was ignored. Layered and Creative v2 never return an answer,
  so for them the switch only chose between the whole window and its middle
  60%.
- **The overlap made the answer barely felt.** In a 20–60 s window, soon
  averaged 31 s, normal 40 s and later 49 s.
- **Session tracking is inert context.** It was on by default, and the
  buildup required it.
- **Auto timing and prepared decisions already followed the planner's
  features.** The switches only allowed turning them off. Auto timing's
  only cost note was "more requests", and that is a question of how short a
  stretch may be.

## Decision

1. **The model's timing answer always counts.** The scheduler draws from a
   triangle inside the person's window:
   - soon covers the first 40%, peaking at 12%;
   - normal covers the middle 50%, peaking at the centre;
   - later covers the last 40%, peaking at 88%.

   In a 20–60 s window that averages about 27, 40 and 53 s. Draws stay
   varied and never leave the window.
2. **Planners that choose their own timing always do,** when their route
   supports it.
   - The shortest stretch they may ask for is half the shortest ordinary
     stretch at the person's change rate, never below 8 s. That is 10 s at
     the default rate and 45 s at the calmest.
   - The person's own setting therefore caps the request rate.
   - Long stretches stay possible up to 5 minutes, as ADR 0050 measured.
3. **External planners always prepare the next decision** while a stretch
   plays, as before by default. Chat, settings changes and Stop still
   discard it.
4. **Session facts always reach the planner:** elapsed time, speed history
   and phrase sameness. Session buildup is still the person's choice and
   no longer needs another switch.
5. **Session buildup states the finish it stands for.** The prompt used to
   give only a percentage and "let it inform pacing". It now says:
   - the session length the person set and the time left;
   - that it is the person's finish target, so the strongest stretch
     belongs as it fills;
   - once full, that the finish has arrived and the strongest stretch
     continues until the person changes it.

   It still widens nothing, and the model still cannot move its clock.
6. **The Autopilot card keeps:**
   - **How often it changes** (the change rate);
   - **Spoken check-ins**: Off, Quiet, Natural or Talkative;
   - **Session buildup** and its length;
   - **Extra planning** for Intermediate planners;
   - under Advanced, whether check-ins may change motion.

   Custom check-in seconds still load from older settings and show as
   Custom until changed. The removed switches load from older settings and
   are ignored.

## Consequences

- Fewer controls, and the model's timing choices are visible in practice.
- A person who wanted strictly uniform timing can no longer ask for it; the
  triangle keeps the variety without the switch.
- A hosted planner can no longer be held to the sampled window. Its floor
  follows the change rate instead.
- The owner clarified that the intended finish target is **Session buildup**,
  not the removed Session tracking switch. Keep that optional visible control.
  A verbal target is also supported, but following a duration depends on the
  model, available motion vocabulary and reconsideration schedule; it is not
  an exact-time guarantee. A planner without conversation access needs the bar.

## Corrected finish target evaluation, 2026-11-12

The first review's numerical table was withdrawn. Its checkpoints ended at
510 s, then it evaluated the 480 s deadline using the decision made at 510 s.
It also used a linear ramp approximation and the midpoint of waves instead
of the actual tempo envelope. It did not exercise the enabled auto-timing
contract and could pass despite failed provider calls.

`TestFinishTargetLive` now samples the production tempo envelope, observes the
deadline **before** applying the next decision, enables the real auto-timing
contract, and retains every call and error. These are scripted planning
checkpoints, not a complete simulation of the randomized scheduler.

At 480 s in the corrected final Terra run:

| Target | Native tempo | Layered selected pace |
|---|---:|---:|
| Verbal only | 85% | 76% |
| Buildup only | 89% | 75% |
| Both | 90% | 72% |

All six sessions built upward. The results do **not** show a guaranteed 100%
at the target time, nor should every unspecified strong finish mean exactly
100%. Layered uses reconsiderations to develop pace; native tempo can express
a timed ramp directly. Native tempo values above are engine samples; Layered
values are semantic selected pace, not measured physical speed.

`TestTempoTimedRequestsLive` separately checks an explicit target made partway
through a session, later praise, a revised deadline, an elapsed gentle interval,
and a newer gentle request with the bar full. It feeds real envelopes and
updated request ages back across decisions. Reconsiderations use the model's
bounded horizon or the measured category means; scheduler jitter and network
latency remain separate from this deterministic-clock evaluation.

- GPT-6.1 Terra Low: **5/5**, one response per scenario, median 2.96 s.
- Gemma 12B Intermediate: **4/5**, ten decisions, median 0.88 s per call.
  The missed case took too long to return after the gentle interval.
- Gemma E4B Simple: **3/5**, twelve decisions. It also missed the target after
  unrelated praise. It ran on CPU; its latency is not a GPU comparison.
- Extra Intermediate timing instructions were tried and discarded: they did
  not improve the results. No text matching or deterministic language rules
  were added, and Simple did not gain a sequence vocabulary.

See [the completion review](../vacuglide-completion-review-2026-11-12.md) for
artifacts, full regression results, UI corrections and remaining hardware work.

## Validation

- `internal/modes`: draws stay in the window and vary, and soon, normal and
  later land apart. The auto-timing floor follows the change rate.
- `internal/config`: retired switches are ignored, the floor follows the
  change rate, and the buildup stands alone.
- `internal/httpapi`: auto timing follows the planner's features.
- Frontend: the card offers no retired switch, four check-in choices, and
  the buildup on its own.
