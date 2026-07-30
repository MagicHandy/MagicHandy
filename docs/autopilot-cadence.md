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
- a missing playback acknowledgement eventually uses the bounded fallback;
- pause preserves remaining time and Stop leaves no autonomous goroutine or
  voice request alive; and
- frontend controls render backend snapshots and remain locked while offline or
  read-only.
