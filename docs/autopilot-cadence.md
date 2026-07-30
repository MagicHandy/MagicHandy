# Autopilot cadence and autonomy

Status: implementation contract (2026-07-30)

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

| Preset | Delay range |
| --- | --- |
| Steady | 45-120 seconds |
| Natural | 20-60 seconds |
| Dynamic | 10-35 seconds |
| Custom | 8-300 seconds |

The default is `Natural` for both clocks. Custom minimums cannot exceed custom
maximums. All non-off intervals have an eight-second floor.

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
  same segment length differ.
- Count is earned by segment length (one per 20s, hard cap 3), then scaled by the
  model's variability category. This **self-balances against the cadence
  preset**: a 10s Dynamic segment has no room and gets none because it is already
  changing constantly, while a 120s Steady segment earns the most because it is
  the one at risk of feeling static.
- Amplitude is a share of the user's own speed band (14% normal, 26% restless),
  and every waypoint is clamped inside it. Sway widens nothing.
- A waypoint equal to the current speed is dropped, because that is exactly the
  no-op retarget this work removed from the hold path.
- Waypoints pop on read, not on success. Retrying a failing adjustment would let
  one bad waypoint starve the speech clock queued behind it.

**Measured combined retarget rate** (one boundary plus earned waypoints; the
pre-change loop produced roughly 4-9/min):

| Preset | Segment | settled | normal | restless |
| --- | --- | --- | --- | --- |
| Dynamic | 10s | 6.0 | 6.0 | 6.0 |
| Dynamic | 35s | 1.7 | 3.4 | 3.4 |
| Natural | 20s | 3.0 | 6.0 | 6.0 |
| Natural | 60s | 1.0 | 3.0 | 4.0 |
| Steady | 45s | 1.3 | 2.7 | 4.0 |
| Steady | 120s | 0.5 | 1.5 | 2.0 |

Worst case is 6.0/min, under the churn this work removed, and the texture lands
where it was missing: a two-minute Steady target went from 0.5 changes/min to
2.0. `TestCombinedRetargetRateStaysUnderThePreChangeChurn` prints this table and
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
when the model answers `normal` every turn. It is optional on the wire — it was
added after the contract shipped, so an omitted field resolves to `normal` rather
than failing the turn.

### Session tracking

An independent switch, on by default, that lets the model see elapsed session
time, how long the current speed has held, and whether speed has been rising or
easing. It is **inert read-only input**: backend-computed so the model cannot
fabricate it, visible in traces, and authorizing nothing. Off omits the facts
from the prompt entirely rather than sending zeros, because a model cannot reason
from a number that means "unknown".

### Session arc

A separate switch, **off by default**, that renders a visible fill bar and tells
the model to aim higher within the allowed speed range as it fills.

The reason this is not the hidden-escalation pattern
[goals-and-guardrails.md](goals-and-guardrails.md) rules out comes down to four
properties, all load bearing:

- **Visible.** The value is rendered in the Autopilot card, so nothing about the
  progression is hidden from the person it is happening to.
- **User-armed.** Off is the default, and off removes the arc from the prompt
  entirely — the same discipline the capability gates use.
- **Bounded.** A percentage with a full mark, not a counter that grows.
- **Backend-owned.** The model may return `arc: advance|ease|hold` to move the
  bar by at most 6 points per turn. It can never write the value, so it cannot
  sprint the bar to full, and every nudge appears in the trace.

The arc positions intent *inside* the user's existing speed band. It never widens
the band, the focus range, or any capability gate — asserted by
`TestArcNudgeDoesNotTouchSpeedLimits`, which advances the bar 40 times and checks
the motion settings are untouched.

Time is the floor, so a session left running still progresses; nudges let the
model lead or lag that baseline. `ease` exists so the bar is not a ratchet. The
user can place or reset it, and placement is refused with a 409 while no
Autopilot session exists — `Start` clears the arc for a fresh run, so accepting a
placement beforehand would store a value discarded a moment later. The arc
requires session tracking; the settings validator rejects the combination rather
than letting the document express a state the runtime would silently ignore.

## Scheduling

Motion is the higher-priority clock. The manager starts the first target
immediately, then plans the next target shortly before its due time using the
last measured decision latency. Planning runs while the current pattern keeps
playing. A result is applied only if its mode generation is still current.

Speech is checked after due motion work. If TTS is already queued or active,
the check-in is postponed without calling the model. Publishing a line and
queueing its audio remain lockstep operations through the canonical chat and
voice paths. Browser playback reports completion to the backend so the next
speech interval begins from what the user actually heard, not from inference
completion.

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

Pause freezes both clocks. Emergency Stop cancels both clocks, in-flight model
work, pending speech, and playback exactly as it does today.

## Model contracts

Motion planning and spoken check-ins use separate strict JSON contracts and
separate prompts.

Motion:

```json
{"motion":{"action":"target","pattern_id":"...","intensity":35},"next":"normal"}
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
motion field.

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
- the session arc never widens a limit, and a disabled switch keeps its section
  out of the prompt entirely;
- pause preserves remaining time and Stop leaves no autonomous goroutine or
  voice request alive; and
- frontend controls render backend snapshots and remain locked while offline or
  read-only.
