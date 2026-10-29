# Top-speed Cloud hardware comparison — 2026-10-29

## Scope and setup

The owner authorized playback and confirmed that the Original Handy was clear
of anyone, could move freely, and playback was stopped in other apps. The
comparison used the existing Cloud REST/HSP transport and normal motion API:

- A: alpha.79 source `f21853ab`, isolated app on port 50313.
- B: reviewed top-speed source `a97ddbe0`, isolated app on port 50314.
- Original Handy calibration; 1–100% saved speed limits; 0–100% saved stroke
  window; Balanced style; direction reversal off; requested speed 100%.
- Identical semantic Layered scores, seed 17, six-cycle loops, no pace
  variation. Each run played for 12 seconds after Start returned.
- Only the Handy participated. LLM motion and Autopilot were off. No installed
  profile was edited and no COYOTE command was sent.

The first full-range pair used separate invocations. The owner confirmed both
Stops were clean and requested a repeat with a shorter pause. The repeat and
the remaining pairs each used one orchestration command: A, acknowledged Stop,
one-second pause, B, acknowledged Stop. Cloud preparation still adds several
seconds before B can play. Preparation was never run concurrently with another
run's playback. Both variants use their production compiler, sampler, engine
and transport; the script does not generate device points.

## Physical feedback

| Matched pair | Owner's observation | Outcome for this scenario |
| --- | --- | --- |
| Full range, 0–100% | Both smooth; B slightly faster | Accepted |
| Short strokes, 80–100% | Same difference; no added chatter, vibration or harshness | Accepted |
| Two strokes from 72–100%, then one from 0–100% | B retained the distinct, smooth two-short/one-full rhythm while running faster, including the transition into the full downward stroke | Accepted |

This supports retaining the candidate for these Original Handy Cloud patterns
at 100%. The feedback does not establish a smoothness improvement over A: both
were smooth. It also does not establish a perceptual numerical speed gain.

The matching shared-engine previews have these commanded timings:

| Pattern | A six-cycle period | B six-cycle period | Commanded mean-travel increase |
| --- | ---: | ---: | ---: |
| Full range | 4,116 ms | 3,768 ms | 9.2% |
| Short strokes | 1,536 ms | 1,452 ms | 5.8% |
| Compound | 2,660 ms | 2,260 ms | 17.7% |

These are compiled trajectory estimates, not carriage telemetry. The sampler
retains sparse output; the hardware's HSP state reported playback rate 1,
pause-on-starvation enabled, and firmware jitter filtering disabled for every
captured final state. The existing transport behavior was not modified for the
audition.

## Runtime evidence

Eight physical runs completed: the initial full-range pair, its repeat, the
short-stroke pair, and the compound pair. All eight ended with the engine
stopped and the Cloud state reporting stopped. The owner explicitly confirmed
physical Stops on the initial pair. No audition reported an unexpected stop
while playing.

For the three single-command comparison pairs:

| Pattern | A / B Start response | A / B Stop response | A / B median append latency |
| --- | ---: | ---: | ---: |
| Full range repeat | 6,088 / 4,794 ms | 331 / 330 ms | 338 / 330 ms |
| Short strokes | 4,797 / 4,880 ms | 347 / 345 ms | 334 / 333 ms |
| Compound | 5,024 / 7,291 ms | 319 / 626 ms | 337 / 325 ms |

These measure application/Cloud command completion, including preparation and
teardown; they do not measure the physical time to begin or halt movement.
The initial pair had Start responses of 2,570/7,982 ms and Stop responses of
1,691/336 ms. Successful append completion was 325–344 ms at the per-run median,
with a maximum of 415 ms across the captured runs.

One alpha.79 compound append ended with a redacted `network_error` after 58 ms
at the instant its final Stop began; the subsequent Stop succeeded 293 ms
later. That timing is consistent with cancellation of in-flight work by Stop,
but the redacted trace does not establish the underlying error text. It is
retained in the evidence rather than counted as an entirely error-free trace.
No unsuccessful transport commands were captured for B.

## Artifacts and remaining coverage

Ignored evidence is under `.scratch/top-speed-followup/hardware/`:

- `20261029-*-A-*.json` / `20261029-*-B-*.json`: settings, preview samples,
  Start/Stop results, live snapshots, Cloud state and full redacted trace.
- `summary.json`: timings, retained unsuccessful command, binary hashes and
  owner feedback.
- `comparison-overview.png`: all three auditioned shared-engine position and
  velocity comparisons, inspected after the runs. These are commanded curves.

The broader [motion-path review](motion-path-review-2026-10-29.md) still leaves
75/85/92% physical comparisons, deliberate slow-return requests, Edge entry and
exit, live high-to-low edits, the conservative envelope alternative, other
Handy models/transports and simultaneous-device behavior open. The automated
coverage for those paths remains useful but does not replace physical feedback.

The current review app remains at `http://127.0.0.1:50314/#/chat`, with playback
stopped, LLM motion off, and a verified local-model text reply. The separate
simulated review app on port 50312 also remains available.
