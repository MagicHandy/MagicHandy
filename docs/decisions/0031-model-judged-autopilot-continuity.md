# ADR 0031: Model-judged Autopilot continuity and phrased strokes

- Date: 2026-09-25
- Status: Implemented for review; no merge or release authorized
- Extends: ADRs 0023, 0024 and 0025

## Context

Physical feedback reported that Creative v2 Autopilot kept settling into one
gesture: a quick stroke up into a fixed top point with a slower return. The
request that introduced unequal timing asked for a capability, not a
tendency. Simulated sessions through the production path reproduced the habit:
33 of 40 baseline decisions chose tip-faster timing, and 70% of the compiled
up/down stroke pairs had a return at least 1.5 times slower than the upstroke.

Five causes combined. The model chose the accent without being asked. Omitted
edit groups persist, so an accent chosen once lasted the session. The contract
worded unequal timing as a recipe. Each score applied the same contrast and
inertia to every stroke, and near-full strokes landed on exactly the same band
edges. Continuation also kept word lists that ADR 0025 had removed from
interactive chat: once a human line matched motion words, Creative v2 could
only refresh its seed, and neither mode could raise speed or widen the band for
the rest of the session. The same lists decided which lines counted as requests.

Two further defects surfaced. Seventeen of 40 baseline decisions were discarded
because a narrowed band no longer fit the kept focus width. Layered Autopilot
was nearly static, and incidental reversals in its whole-cycle timing fit
capped the pace of alternating geometries.

A follow-up found that pace was a choice made for the context, but a narrow
one. An explicit "go slow" reached 15–25. "That's too much" eased pace only to
20–35, and Creative v2 climbed back within a stretch or two. "I'm close, don't
let me finish yet" left pace at 35–52. Layered answered "mix it up" and
"surprise me" with a seed refresh.

## Decision

Continuity is a model judgment, bounded by the host.

- Autopilot planning in both continuous modes receives the latest human lines,
  up to eight, oldest first and unfiltered, and decides which still apply: a
  pace, a region, a feel, or a wish to keep the motion exactly as it is. The
  word classifiers, their request filter and the continuation validator are
  removed, including the Lab's copy for continuous methods. Automatic changes
  may now raise speed or widen the band after a human request when the model
  judges that the request no longer applies. Saved limits, score validation,
  Stop, controller ownership and mode fences keep their authority.
- History replays earlier assistant turns as speech only. Planning turns no
  longer see an empty-edit envelope for every previous reply.
- Live chat declares a standing wish; the host does not infer it. While
  continuous Autopilot composes, every interactive reply carries a required
  `stay_unchanged` boolean after its reply text. The server remembers `true`
  for that chat session. While the wish stands, planning boundaries hold
  without inference and report `requested_hold`. A later `false`, a new
  Autopilot run or another conversation releases it. Planning turns can neither
  declare nor clear it, and the field is absent when Autopilot is not composing.
- Accents are temporary. When a planning turn changes other controls, it can
  omit direction contrast, rebounds or strong inertia. Those accents then relax
  by a random amount: contrast is multiplied by 0.3–0.7 and becomes even below
  10, one rebound is removed with probability 0.5, and inertia above 30 moves
  40–80% of the way to 30. Holds and seed-only refreshes keep every accent. The
  relaxed score is what is recorded and played, so captured sessions replay
  exactly.
- A narrowed band clamps the focus width instead of rejecting the decision,
  because a clamp has only one reading.
- The wording presents unequal timing, rebounds and inertia as accents for a
  stretch. It asks continuous Autopilot to use the width of the saved speed
  range. Examples no longer name tip-heavy recipes.

Creative v2 strokes are phrased inside the shared plan. Seeded per-stroke
fields add slow pace breathing that only eases below the chosen speed,
occasional short flurries, rare brief rests at a turn, and landing points that
vary inside the band. A held anchor still
lands exactly. Variation 0 still repeats every stroke exactly. The default
variation rises from 35 to 50. Each rest is a fitted leg with zero velocity and
acceleration at both ends, so position, velocity and acceleration remain
continuous. The runtime kinematic limits do not change.

Layered Flow scores compile from legs between the actual turning points.
Same-direction half-cycles merge, and each leg is fitted under the existing
Flow authoring budget. This replaces the single whole-cycle fit, in which
incidental reversals limited alternating geometries.

Pace follows the person, and the model chooses it:

- Both continuous contracts say pace may use all of the saved range. When the
  person says it is too much, or says they are close and the model chooses to
  draw the session out, speed goes near the saved minimum in that decision,
  holds for an appropriate time and then rebuilds. When the model lets them
  finish, it holds or builds. No word triggers either choice.
- Planning sees time. Once the model dropped to the minimum, it stayed there
  for minutes: planning replies are never dialogue, so every planning turn saw
  the person's line as the latest event. Each recent human line now carries
  `seconds_ago`. The continuous facts say how long ago the person last spoke,
  list the speeds of the stretches that began in the last three minutes, and
  say how long pace has stayed in the lower third of the saved range. A
  passing state fades after a minute or two; a lasting preference stays.
- During continuous Autopilot, a live chat turn that leaves the motion
  unchanged moves the next planning boundary to now. Chat chooses its action
  before its reply, so it often answers such a remark in words alone.
- The built-in prompt sets no longer forbid escalating beyond what the person
  asks. They say to follow the person's cues on energy and pace, respect
  limits they set, and lead when the choice is left to the model.
- Layered `evolve` only reseeds the same character. A request for something
  new or mixed up changes geometry, widths, anchor or layers. A relative speed
  step past a saved limit stops at the limit.
- Each continuous contract shows one example of holding off at nearly the
  slowest pace in a single step. Smaller models followed that example where
  they missed the guidance, and Gemma 12B followed it more consistently.

Live chat can bring back an earlier score. During continuous Autopilot, the
scheduler keeps the three latest distinct scores and up to three older ones
that played while the person spoke. A live chat turn sees them with when each
started, how long it played and what the person said while it played. The
edit `{"recall": id}` restores one exactly, and other edits apply on top. A
recalled speed outside today's limits moves to the nearest limit. Planning
turns are never offered recall.

No second motion path, clock, sampler or transport payload is added. All motion
reaches the device through the same prepared plan, sanitizer and transport.

## Alternatives not selected

- Word or phrase rules of any kind, per the user's direction and ADR 0025.
- A host pace trend that follows session buildup. Pace that rises with elapsed
  time is the hidden escalation the guardrails rule out. Buildup stays visible,
  user-armed and only model-reactive. The breathing field only eases pace
  below the chosen speed and recovers.
- Scheduler speed sway for Flow segments. Sway retargets speed mid-segment,
  which recompiles a continuous score and competes with its own pace fields.
  Tempo variation lives inside the plan instead.
- Prompt-only exact holds. In live sessions they lapsed after an unrelated
  remark.
- An optional `keep` flag, or a required one before the reply. The model
  usually skipped the optional flag. Placed before the reply, it read "keep
  going, don't stop" as a wish to freeze the motion.
- Rewriting speech facts. An earlier draft of the review assumed that
  motion-turn narration was spoken and that check-ins received raw score JSON;
  both assumptions were wrong.
- A host timer that rebuilds pace after a fixed time. The model judges the
  time from the facts instead.
- Recall in planning turns. Planning re-read an old "go back" after chat had
  answered it and swapped between scores.
- A fixed count of remembered stretch speeds. Six stretches at a 14 s cadence
  never reached the minute or two after which a slow stretch rebuilds.
- Separate prompt sets for smaller and larger models. The change that helped
  3–4B models most, a concrete example, helped Gemma 12B too. What the smaller
  models miss, judging elapsed time and recall, falls back to staying slow or
  not recalling.

## Consequences

Session variety depends on model judgment and seeded engine phrasing rather
than on host rules. The model can still misread a request, keep an accent
longer than wanted, or misjudge a standing wish. The Autopilot status shows a
requested hold, and one chat line or a fresh run releases it. Because accent
relaxation is random, two live runs differ, but every accepted score, seed and
trace can be replayed. At high speed a rest is about as long as a stroke;
physical feedback should decide whether rests scale with pace. Plots establish
commanded character, not physical comfort.

Autopilot may now rebuild pace after a slow stretch that the person asked for,
without a new request, inside saved limits. The built-in prompt sets no longer
forbid that. Recall offers live chat a few more hundred tokens while earlier
scores exist.

See [the review](../motion-naturalness-review-2026-09-24.md) for measurements,
retained failures, iterations and reproduction.
