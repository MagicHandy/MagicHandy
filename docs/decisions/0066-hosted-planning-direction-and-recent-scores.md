# ADR 0066: Hosted planning keeps a named direction and sees recent scores

## Status

Accepted 2026-11-09. Advanced-capability Layered planning and chat, including
local models explicitly configured as Advanced. Simple and Intermediate
prompts keep their bytes. See the
[follow-through review](../autopilot-followthrough-review-2026-11-08.md).
Extends [ADR 0031](0031-model-judged-autopilot-continuity.md): planning still
gets no recall, but it now sees the scores that played just before the current
one. Timing from [ADR 0050](0050-hosted-autopilot-auto-timing.md) is unchanged.

## Context

An installed alpha.82 session used GPT-6.1 Terra (low reasoning) for chat and
Autopilot in Layered, at change rate 8 with Auto timing and prepared
decisions on. Several requests were followed poorly, although Autopilot
re-planned every 10–16 seconds:

- **A build requested now, with a cue for later.** Asked to bring the person
  toward the edge and to slow down when told they were close, chat changed
  nothing in the session. In eight replays of that turn it built the pace in
  four, changed nothing in two and slowed down in two, reading "edge" as the
  tip of the range. Autopilot then raised pace by 8–15 points every two or
  three decisions, reaching only 30–45% after a minute.
- **Invented strokes.** Asked to keep inventing simple strokes at top speed,
  Autopilot alternated between the same two templates in every decision: a
  short/long range alternation and strokes hopping between the ends. It eased
  below the requested top speed and never used a counted phrase. The planner
  saw only the current score, the continuation text says an earlier
  alternation is not a user constraint, and phrases read as tools for counts
  the person spelled out.
- **An ordered progression.** Asked to begin in the middle and widen each
  stroke, chat chose a smooth widen-and-narrow range wave in five of five
  replays rather than counted steps.

Calling the planner more often would not help. Each GPT decision takes
3.5–8 seconds, the 8-second floor already allows 7.5 calls a minute, and the
failures were in what each decision chose: more calls would only alternate the
two templates faster.

## Decision

1. **Recent scores for hosted Layered planning.** `DecisionInput.RecentScores`
   carries up to three distinct continuous scores that finished within the
   last 180 seconds before the current one, newest first, for motion planning
   only. Hosted Layered planning shows them as "scores played just before the
   current one": what the person has just felt, not targets or requests. They
   carry no recall IDs and none of the person's words, so a planner cannot
   re-read an old request as fresh, which is why ADR 0031 offers recall only
   to chat. Going back to one is described as a repeat, not a new contrast.
   Gesture scores are omitted. A long stretch remains recent when it ends;
   its start time alone must not age it out of this window.
2. **Direction guidance for hosted Layered planning.** When the person names
   where the session should go, such as building toward a peak, bringing them
   close or to the edge, or winding down, the planner keeps moving there across
   stretches until they say it is reached or change it, making each stretch a
   felt step, mostly through pace. Holds and easing back suit open-ended
   stretches, not a requested build. Qualities asked of the strokes themselves, such as simple,
   smooth, full length or fast, last across variations until changed. Counted
   phrases that mix short and long strokes in different regions are tools for
   inventing strokes, not only for counts the person spelled out.
3. **Chat guidance for hosted advanced motion.** A message can pair something
   to do now with something to do on a later cue: chat encodes the part for
   now and lets the cued part wait. Being close, or at the edge, describes how
   near the person is to climax, not a slider position. The hosted phrase
   guide adds that an ordered progression, in which each stroke or turn
   changes from the last, is counted too: one group per step. A vibrator-only
   stopped state does not receive the chat guidance.
4. **No timing change.** Auto timing keeps its 8–300 second range and 8-second
   floor.

Every addition is guidance or an observation for the model. There is no word
or phrase rule (ADR 0025), and pace follows only a destination the person
named: no host trend raises pace with elapsed time, which ADR 0031 rules out.

## Consequences

- Hosted chat prompts grow by about 0.56 KB of system text. Hosted planning
  grows by about 0.25 KB of system text and 0.6–0.8 KB per planning message,
  depending on how many recent scores there are. Request counts are unchanged.
- A planner can still return to an earlier score deliberately; it is told that
  this repeats what the person just felt.
- Simple and Intermediate planners, Decisions, Creative v2 planning and speech
  check-ins are unchanged. "Hosted" here names the Advanced contract, not a
  restriction on provider location; Advanced local models use the same contract.
- Replays of the session's turns used OpenRouter's `openai/gpt-6.1-terra`
  (reasoning low) and the production prompt builders. Chat built the
  requested pace in eight of eight turns, up from four, and Autopilot reached
  60–70% within about a minute. After the cue to ease off it held about 18%
  for about 75 seconds in four of four runs. Inventing used phrases in 8 of 18
  decisions; 7 of 18 were the two old templates, against 18 of 18 before, and
  each six-decision run had six distinct ideas. The widening progression
  became counted steps in five of five replays.
- On 17 default hosted suites the score moved from 230 to 235 of 265 checks:
  seven cases started passing and two stopped, each examined in the review.
  Six opt-in suites ran only partly before the evaluation account's credit ran
  out; their completed pairs show no regression.

## Validation

Unit tests cover the newest-three, 180-second window of `RecentScores`, that
only Advanced-capability Layered planning receives and renders them, that the rendered
scores carry no recall IDs or the person's words, and that each new guide
appears only for Advanced motion. `TestHostedFollowThroughLive`
(`liveeval`) repeats the four situations with synthetic requests and retains
every result for motion-atlas review. Failed provider calls fail the synthetic
scenario even if the unchanged score would otherwise satisfy a hold request.
