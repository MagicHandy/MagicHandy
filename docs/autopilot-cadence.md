# Autopilot cadence and autonomy

Status: implementation contract (2026-08-22)

This document defines how Chat Autopilot decides when to change motion and when
to speak. It is intentionally separate from motion-style scoring and from voice
worker configuration.

## Problem

The first Autopilot implementation used one segment-boundary loop for two
different jobs:

1. ask the model for the next motion target; and
2. require a line to speak immediately.

That coupling made the selected motion style accidentally control speaking
frequency, made model latency extend every segment, and caused a deliberate
"hold" to retarget the engine with the same target. The result could feel
chatty, mechanically periodic, and unnecessarily busy at the motion boundary.

StrokeGPT-ReVibed proved three useful contract shapes:

- autospeech has its own bounded clock;
- the user can switch autonomous speech off; and
- speech can have a separately bounded level of motion authority.

Its implementation is not copied directly. It used one synchronous mode loop,
numeric model-authored delays, short 12-45 second defaults, and full motion
authority by default. Those choices can synchronize unrelated work, encourage
repeated timing values, and allow speech generation latency to disturb motion.

## Invariants

- Motion evolution and autonomous speech have independent backend-owned clocks.
- Motion cadence, speech cadence, and deterministic pattern selection use
  independent random streams; changing one preference cannot perturb another
  stream's future sequence.
- Motion style selects motion character. Motion cadence selects how often
  Autopilot considers a semantic change.
- A hold changes scheduler state and diagnostics only. It never calls
  `ApplyTarget`, rebuilds a motion plan, or dispatches transport work.
- The model selects only a timing category: `soon`, `normal`, or `later`.
  Deterministic code samples the actual delay inside the user's saved bounds.
- A model failure falls back to deterministic motion planning. A speech failure
  postpones speech; it cannot stall or stop motion.
- User chat invalidates stale autonomous decisions, blocks new autonomous
  inference until the interactive turn finishes, and resets the speech clock.
- Autopilot never deepens an existing TTS backlog.
- When audio is enabled, the next speech interval begins after browser playback
  completes. A bounded fallback prevents a lost browser acknowledgement from
  freezing speech forever.
- Runtime deadlines, pending choices, and playback acknowledgements are
  ephemeral. Only user preferences are persisted.
- Status snapshots publish the backend observation time and absolute motion/
  speech deadlines. A client may interpolate presentation between snapshots,
  but it cannot select, extend, or rewrite those deadlines.
- Every motion target still enters the one shared motion engine and all normal
  clamps remain authoritative.

## User preferences

Preferences live in the versioned settings document and apply to the active
Autopilot session without restarting motion.

### Spoken check-ins

| Preset | Delay range |
| --- | --- |
| Off | Disabled |
| Quiet | 90-240 seconds |
| Natural | 35-120 seconds |
| Talkative | 15-60 seconds |
| Custom | 8-600 seconds |

### Motion evolution

| Level | Delay range |
| --- | --- |
| 1 | 90-240 seconds |
| 2 | 60-150 seconds |
| 3 | 45-120 seconds |
| 4 | 20-60 seconds |
| 5 | 14-45 seconds |
| 6 | 10-35 seconds |
| 7 | 8-24 seconds |
| 8 | 8-16 seconds |

Motion change rate defaults to level 4; Spoken check-ins defaults to `Natural`.
Stored Steady/Natural/Dynamic values migrate to levels 3/4/6, and an old custom
motion window migrates to the nearest level midpoint. Speech retains its custom
minimum/maximum inputs. All non-off intervals have an eight-second floor.

The numbered setting changes how often Autopilot **reconsiders** the semantic
target; it is not an acceleration or transport multiplier and never forces the
model to change motion. Creative also receives the effective level as a user
preference: higher values mean more frequent meaningful differences are
welcome, while lower values favor continuity. Its model-selected
`segment_seconds` is clamped against the level's backend-authoritative window.
The live decision horizon is returned to the model on the next turn so a local
model can vary it instead of repeatedly selecting the midpoint.

Autopilot itself is the user's continuing authorization for bounded autonomous
choices; an empty or quiet chat is not implicitly a request to hold. The rate is
rendered to the model as a continuity bias over the whole felt phrase, not a
per-decision command. At the high end, contrast may accumulate through smooth
steps across pace, outer travel band, stroke envelope, texture, or sections;
the retained perceptual baseline recognizes that cumulative change without
requiring an abrupt jump. An explicit request to keep motion unchanged always
wins and produces `action:none`, regardless of elapsed time or rate.

### Speech motion authority

- `Chat only`: an autonomous line cannot change motion.
- `Style only`: a line may adjust speed or named area while preserving the
  current pattern.
- `Full motion`: a line may select any enabled pattern as well as speed or area.

`Chat only` is the default. This setting affects only autonomous spoken
check-ins; interactive user chat keeps its existing capability gates.

### Adaptive timing

Motion and speech each have an independent adaptive-timing switch. When
enabled, the model returns `soon`, `normal`, or `later`; code maps that category
to an overlapping portion of the selected range and samples a concrete delay.
When disabled, code samples the full selected range. The model never emits
seconds or a deadline.

The status API carries `status_at`, `motion_change_due_at`, and
`speech_due_at` as RFC3339 timestamps, with the former remaining-millisecond
fields retained for compatibility. The Chat sidebar subtracts the backend
observation time once, advances the display at 250 ms resolution, and replaces
that local anchor on every ordinary two-second state poll. This is presentation
only: the scheduler still fires solely from its backend `time.Time` deadline.
Pause freezes the displayed remainder, including when its SSE state arrives
before a fresh status poll; resume continues from the reconciled backend value.

### Autonomous target variation

The model still decides whether a motion turn changes anything. `action:none`
remains a first-class hold and never retargets the engine. When the model does
request a pattern change, the turn-specific catalog temporarily omits recently
played patterns whenever at least four other enabled choices remain. The
omission is a strict allow-list for that autonomous turn, not a hidden library
toggle: interactive chat always receives the complete enabled catalog, and the
model can keep the current pattern while changing only pace by omitting
`pattern_id` and using `speed_percent`.

A pattern outside the turn-specific catalog receives the normal single repair
attempt only on interactive chat. Autonomous motion skips that repair and lets
the established planner fallback supply a bounded semantic target immediately:
live testing found a 26B model copied the unavailable current ID into both its
first answer and repair, doubling latency without recovering. Model-visible
status therefore also describes an omitted current pattern without repeating
its unavailable ID. This avoids silently accepting a latched output while
preserving model ownership of the hold/change decision.

Area focus is not backend-randomized. Every motion turn names the three areas
that differ from the live area and highlights one context-derived suggestion to
avoid presenting a small model with three equally weighted optional choices.
The suggestion varies with the segment and current pattern but authorizes
nothing: the model may use it when it fits or omit `area` to deliberately
preserve the live focus. This makes spatial contrast explicit without forcing a
focus change or overriding conversation context.

### Intra-segment sway

Longer cadence windows created a second problem the first pass did not solve.
The pre-existing midpoint drift fired one speed step at exactly `duration/2` and
only when a segment carried `DriftToSpeedPercent`, which **only the Freestyle
planner sets** — so Autopilot's model-chosen segments never had intra-segment
variation at all, before or after the cadence work. Routing Autopilot to its own
scheduler then left `driftAt`/`driftDone` write-only, and a Steady target could
hold perfectly constant for two minutes.

Sway replaces it with a sampled schedule of **speed-only** waypoints across the
segment interior:

- Speed only, inside the current pattern and area, so a waypoint can never be a
  semantic change in disguise and does not disturb the recognizable feel that
  longer segments exist to establish.
- Offsets are sampled inside evenly divided slots rather than fixed at the
  midpoint, so the texture is not metronomic. Two consecutive schedules for the
  same segment length differ, while consecutive waypoints remain at least six
  seconds apart.
- Count is earned by segment length (one per 8s after a 16s floor, hard cap 6),
  then scaled by the model's variability category. Edge guards, six-second
  spacing, and a one-second jitter reserve can lower that count when a short
  segment cannot fit genuinely sampled timing. This **self-balances against the
  change level**: a 10s level-6 segment has no room and gets none because it
  is already changing constantly, while a 120s Steady segment earns the most
  because it is the one at risk of feeling static.
- Amplitude is a share of the user's own speed band (14% normal, 26% restless),
  and every waypoint is clamped inside it. Sway widens nothing.
- A waypoint equal to the current speed is dropped, because that is exactly the
  no-op retarget this work removed from the hold path.
- Waypoints pop on read, not on success. Retrying a failing adjustment would let
  one bad waypoint starve the speech clock queued behind it.
- Every schedule carries the segment generation that created it. Chat handoffs,
  settings changes, Stop, and other generation changes discard stale waypoints
  so texture sampled for one target cannot modify its replacement.
- Pause shifts waypoint, session-buildup, and speed-history clocks with the segment
  deadline. Resume continues the remaining schedule instead of firing a burst
  of overdue adjustments or counting paused time as motion progress.

**Measured combined retarget rate** (one boundary plus earned waypoints; the
pre-change loop produced roughly 4-9/min):

| Legacy-equivalent level | Segment | settled | normal | restless |
| --- | --- | --- | --- | --- |
| 6 (Dynamic) | 10s | 6.0 | 6.0 | 6.0 |
| 6 (Dynamic) | 35s | 1.7 | 5.1 | 6.9 |
| 4 (Natural) | 20s | 3.0 | 6.0 | 6.0 |
| 4 (Natural) | 60s | 1.0 | 4.0 | 7.0 |
| 3 (Steady) | 45s | 1.3 | 5.3 | 8.0 |
| 3 (Steady) | 120s | 0.5 | 2.0 | 3.5 |

Worst case is 8.0/min, under the churn this work removed, and the texture lands
where it was missing: a two-minute Steady target went from 0.5 changes/min to
3.5. `TestCombinedRetargetRateStaysUnderThePreChangeChurn` prints this table and
fails if any cell exceeds 9.

### Variability

`variability` is a second model axis beside `next`, and the two are independent:
a long stretch can still breathe, and a short one can stay flat.

| Category | Effect |
| --- | --- |
| `settled` | No waypoints. The target holds flat until the next boundary. |
| `normal` | About half the earned allowance, 14% amplitude. |
| `restless` | The full allowance, 26% amplitude. |

It is a category rather than a number for the same reason `next` is: a local
model emits a category reliably, and backend sampling guarantees variety even
when the model answers `normal` every turn. Motion turns require this field.
That keeps a deliberate model hold from acquiring backend-invented sway merely
because the field was omitted; a missing or unknown category gets the normal
one-shot repair path, then the explicit planner fallback if repair fails. A
spoken check-in also requires the field when its saved authority admits a target
motion. Chat-only speech omits both motion and variability, so speech cannot
silently acquire a backend-selected texture either.

### Session tracking

An independent switch, on by default, that lets the model see elapsed session
time, how long the current speed has held, whether speed has been rising or
easing, and accumulated motion sameness. The latter is expressed as the age of
the current semantic phrase, how many reconsiderations preserved it, and the
current consecutive hold count. Phrase identity includes spatial geometry,
range envelope, texture, and section structure while excluding pace and the
scheduler horizon; a small speed nudge therefore cannot disguise a repeated
shape as a new phrase.

These values are **inert read-only observations**. They are backend-computed,
included with decision traces, and authorize nothing. There is no maximum hold
duration and no deterministic "change now" threshold: the model remains free
to hold at every age, while finally having enough context to recognize when a
series of individually reasonable holds has accumulated into prolonged
repetition. Off omits all of the facts from the prompt rather than sending
zeros, because a model cannot reason from a number that means "unknown".

Creative's strict autonomous JSON contract lists the same phrase fields as the
semantic parser, including span envelopes and multi-section phrases. Variable
span profiles require an inner span of at least 20 and strictly below the
effective widest span. The prompt provides the usable range for the current
target, and validation returns the complete invariant to the one-shot repair
path so a narrow current span can be widened in the same update instead of
producing repeated malformed plans.

The model also receives a compact summary measured from the final compiled
curve: commanded mean travel, peak carriage velocity, mean stroke length, and
the least-varied 12-second stroke CV/range. Phrase freshness uses one normalized
perceptual distance over compiled position bounds, stroke metrics, profile, and
topology. Speed is intentionally excluded because pace age is tracked
separately. Several small scalar edits are compared with the retained baseline
and therefore cannot reset phrase age one by one; a materially different felt
curve resets it. The exact values are recorded beside the decision's session
facts in diagnostics.

When the ongoing direction calls for several distinct or evolving sequences,
the autonomous contract identifies `sections` as the matching 2–4-idea macro
vocabulary. One geometry still means one coherent idea, and a section array is
not required for an ordinary adjustment or hold. A semantically new textured
single phrase receives a fresh deterministic bounded seed; speed-only,
horizon-only, and semantic no-op updates preserve the current seed and plan.

### Session buildup

A separate switch, **off by default**, that renders a visible fill bar and tells
the model to aim higher within the allowed speed range as it fills.

The reason this is not the hidden-escalation pattern
[goals-and-guardrails.md](goals-and-guardrails.md) rules out comes down to four
properties, all load bearing:

- **Visible.** The value is rendered in the Autopilot card, so nothing about the
  progression is hidden from the person it is happening to.
- **User-armed.** Off is the default, and off removes the buildup from the prompt
  entirely — the same discipline the capability gates use.
- **Bounded.** A percentage with a full mark, not a counter that grows.
- **Backend-owned.** Active elapsed time is the only automatic input to the bar.
  The model sees the percentage and may react to it, but its response cannot
  advance, ease, or write the clock.

The buildup positions intent *inside* the user's existing speed band. It never
widens the band, the focus range, or any capability gate.

The configured duration is authoritative: absent pause or explicit user
placement, a 30-minute buildup reaches 50% after 15 active minutes and 100% after
30. Autopilot pause freezes the clock. The user can place or reset it; either
operation re-anchors elapsed progress at that value. Duration accepts any
positive whole number of minutes; there is no product-level maximum, only an
implementation guard against overflowing Go's duration type. Placement is
refused with a 409 while no Autopilot session exists — `Start` clears buildup for
a fresh run, so accepting a placement beforehand would store a value discarded a
moment later. Buildup requires session tracking; the settings validator rejects
the combination rather than letting the document express a state the runtime
would silently ignore.

The active-time regression also covers the reported 20-minute setting: it
advances exactly one percentage point per 12 active seconds, reads 30% after six
active minutes, remains 30% through five simulated paused minutes, and reaches
60% only after another six active minutes.

In the Chat control, Motion change rate is an explicit numbered 1–8 scale and
Spoken check-ins is a labeled set-point slider. Speech reveals its custom timing
row directly beneath that control. Session buildup and its
duration are primary controls as well; only adaptive timing, speech-motion
authority, and the lower-level tracking switch remain under Advanced.

## Scheduling

Motion is the higher-priority clock. The manager starts the first target
immediately, then plans the next target shortly before its due time using the
last measured decision latency. Planning runs while the current pattern keeps
playing. A result is applied only if its mode generation is still current.

Speech is checked after due motion work. If TTS is already queued or active,
the check-in is postponed without calling the model. Publishing a line and
queueing its audio remain lockstep operations through the canonical chat and
voice paths. A speech-authorized target is applied only after that line has been
committed to canonical Chat, so a persistence failure cannot leave an
unexplained device change. Browser playback reports completion to the backend
so the next speech interval begins from what the user actually heard, not from
inference completion.

**The acknowledgement fires on any terminal outcome, not only on success.** The
first implementation acknowledged only after audio played, so failed synthesis or
blocked autoplay skipped it and the backend waited out its full two-minute
fallback — silently turning a Talkative 15-60s cadence into one line every two
minutes with nothing on screen to explain it. Autoplay blocking is the common
trigger: Chrome rejects `play()` without a prior user gesture. Whether the audio
was heard or not, the turn is over and the clock should restart. A worker-side
cancellation is excluded, because the backend cancels and reschedules that case
itself, and `/played` now accepts `done` or `failed` but still refuses
`canceled`. The bounded fallback covers a genuinely lost HTTP call, which is what
it was always meant to be for.

Pause freezes both cadence clocks, intra-segment waypoints, session progress,
and speed-history time. Emergency Stop cancels both clocks, in-flight model
work, pending speech, and playback exactly as it does today.

## Model contracts

Motion planning and spoken check-ins use separate strict JSON contracts and
separate prompts.

Motion:

```json
{"motion":{"action":"target","pattern_id":"p-...","speed_percent":35},"next":"normal","variability":"restless"}
```

`motion` may be omitted or use `{"action":"none"}` for a hold. No reply is
requested or accepted.

Speech:

```json
{"reply":"...","next":"later"}
```

Depending on the saved speech authority, `motion` is either unavailable,
restricted to speed/area changes, or fully available. The existing semantic
motion parser, enabled-pattern lookup, and settings clamps still validate any
motion field. When speech includes target motion, it must also include
`"variability":"settled"|"normal"|"restless"`; variability is omitted when
speech does not change motion.

## Diagnostics and acceptance

Mode status exposes independent motion and speech due times, whether a motion
choice is planned, and whether speech is waiting for playback. Motion trace rows
record the model timing category, sampled dwell, decision latency, semantic
change, fallback, and scheduler-only hold. Speech traces record the timing
category plus backlog, publication, playback, and fallback transitions.

Acceptance requires:

- a hold produces zero engine retargets;
- speech and motion ranges can be changed independently;
- user chat discards a stale plan and postpones autonomous speech;
- TTS backlog produces no additional line or request;
- playback completion starts the next speech interval;
- failed synthesis and blocked autoplay also start the next interval, and a
  worker-side cancellation does not;
- a missing playback acknowledgement eventually uses the bounded fallback
  (covered by `TestSpeechPlaybackFallbackRecoversALostAcknowledgement`, which the
  first pass left untested);
- sway never leaves the user speed band and never plans a no-op waypoint;
- session buildup never widens a limit, and a disabled switch keeps its section
  out of the prompt entirely;
- pause preserves remaining time and Stop leaves no autonomous goroutine or
  voice request alive; and
- frontend controls render backend snapshots and remain locked while offline or
  read-only;
- the transport-free `TestLiveAutopilotCreativeHighRateAutonomy` scorecard uses
  the production prompt, mapper, compiler, and retained phrase baseline, and
  requires both a materially different felt curve and changed range character
  over a short max-rate run without prescribing a turn on which either occurs;
- `TestLiveAutopilotCreativeCompiledPhrase` still honors an explicit hold after
  a long max-rate phrase; and
- `TestLiveAutopilotSpeechNovelty` exercises the recent-line summary and rejects
  repeated lines or monotonous sentence openings without prescribing persona
  prose.
