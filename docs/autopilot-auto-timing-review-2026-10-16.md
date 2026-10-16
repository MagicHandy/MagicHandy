# Autopilot timing for external planners — October 16, 2026

Decision: [ADR 0050](decisions/0050-hosted-autopilot-auto-timing.md).
Base: alpha.62 (`0e3187e5`).

## Question

Fixed cadence limits how natural Autopilot motion can be. Fixed timing may suit
small local models, but an external planner, whether it handles chat or only
Autopilot, should be able to time its own changes. Separately, an external
planner's latency should not delay a change when the person has not
interrupted.

## What the code did

- In Creative v2 (the default) and Layered, `completeLayeredAutopilot` returned
  `next: normal` for every turn, so every stretch was a uniform draw from the
  middle of the Motion change rate window: 28–52 seconds at level 4, 9–14 at
  level 8, 120–210 at level 1. The Adaptive motion timing switch only narrowed
  that window there.
- Stretch length was unrelated to the motion. Changes landed on a wall-clock
  timer; sway skips continuous scores, so a stretch's pace was flat until the
  boundary.
- Each decision was planned one measured latency plus 0.5 seconds before its
  boundary (1–10 seconds), so a call slower than the previous one made the
  change late.

## Change

- **Auto timing** (off by default, hosted routes only): motion turns return
  `next_check_seconds` (8–300). The scheduler varies it by up to ±12% and
  clamps it; the prompt gives the Motion change rate window as the reference
  for an ordinary stretch and lists recent horizons as observations.
- **Prepared decisions** (on by default, any external planner): the next
  decision is planned as its stretch begins, with facts projected to the
  stretch's end, and waits for the boundary. Chat, settings changes and Stop
  discard it.
- Local schemas, prompts and timing are unchanged.

## Measurement

`TestLiveAutopilotAutoTiming` (build tag `liveeval`) drove simulated Creative v2
sessions through the production AutopilotService, prompt, parser and compiler:
saved speed 15–60%, change rate 4/8 (reference 20–60 seconds), neutral pacing
lines and no persona. Each stretch lasted the planner's own horizon; a human
line cut it short, as a chat turn does. No engine or transport was built.

GPT-6 Sol (Low reasoning) used the existing signed-in profile in
`.scratch/provider-review-data` in place through `MAGICHANDY_HOSTED_DATA_DIR`;
no credential was copied or shown. The control was Gemma 4 12B on a loopback
llama.cpp server with Auto timing forced on.

### GPT-6 Sol horizons

| Scenario | Without the reference | With the reference |
| --- | --- | --- |
| Open, no request | 48–94 s, median 67 | 29–47 s, medians 38 and 42 (two runs, 10 distinct values each) |
| Build gradually | 35–93 s, median 72, lengthening with the climb | 35–52 s, median 44, lengthening early in the climb |
| Faster, then "too fast" at 150 s | 22–58 s, median 43 | 28–44 s, median 38; at once to 17%, then +5 per stretch |
| Keep it like this | 10/10 holds, 120–300 s | 10/10 holds, 180–300 s, median 240 |

No horizon was missing and no turn failed. With the reference, no open-session
horizon was a multiple of 15, and planning calls ran at about 1.5 per minute
in open sessions, 1.7 while correcting the pace and 0.25 during a requested
hold. Fixed level 4 averages about 1.5 per minute and ignores the content.

The user judged 48–94 seconds too long to feel one pattern in a session with no
request. Stating that a stretch is one unchanged pattern and offering the
change-rate window as the reference brought open sessions into it without a
bound: the model still chose 180–300 seconds for a requested hold.

### Gemma 12B control

| Scenario | Without the reference | With the reference |
| --- | --- | --- |
| Open | 120–180 s, 5 values | 40 s on 9 of 12 turns |
| Build gradually | 120 s on all 12 turns | 40 or 45 s only |
| Faster, then "too fast" | 60–240 s, all multiples of 15 | 40 or 45 s only |
| Keep it like this | holds, 120–180 s | holds, 120 s on all 10 turns |

The local model settles on one value with or without guidance, so local
planning keeps the backend-sampled window, which varies by construction.

### Prepared decisions

Across 104 GPT-6 Sol calls, latency had a median of 3.14 s, p95 4.88 s and
maximum 5.64 s. Replaying those latencies through the just-in-time lead made 28
of 93 timer boundaries late (30%), by 0.63 s on average and 1.49 s at most.
Prepared, none was late: the shortest stretch was 22 s. With the lead capped
at 10 seconds, a slow outlier (the October 8 review saw a 45-second incomplete
request) makes a just-in-time change late by most of its duration, up to the
25-second decision timeout; a prepared decision absorbs any call that finishes
within its stretch. Scheduler tests reproduce both cases on a fake clock.

## Limitations

- Small, neutral, simulated sessions; one hosted model; no physical feel.
- ChatGPT plan usage over long sessions is not measured.
- A prepared decision does not see changes during its stretch other than an
  interruption; its projected ages assume the stretch plays as armed.
- Ramped transitions, queued follow-up edits and phrase-aligned boundaries
  for local models remain separate work.
