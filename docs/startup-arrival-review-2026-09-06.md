# Encoded startup arrival — 2026-09-06

## Report and measured evidence

The user-installed Windows alpha.42 (`f3876fbd`) failed video synchronization
at a fractional seek point with “motion startup lead-in did not settle 1.10 mm
from target, outside 1.0% travel tolerance.” Its process at localhost:49717
used Cloud REST and a Handy Original, stroke 0–100%, normal direction,
speed cap 54%, and video speed limiting disabled. Inspection used read-only
state and trace exports; it did not issue motion or alter the installed app.

The retained current trace contains two failed explicit starts, sequences
243–262 and 263–282. Both requested a semantic endpoint of
16.458333333333336%. Cloud's existing encoder rounds it to HSP `x=16`.
Each start read the same stationary 20.00 mm / 15.33% position six times,
then issued its fail-safe Stop. The reported calibration was 5.00–102.83 mm.
Using those rounded trace bounds gives:

| Quantity | Position |
| --- | ---: |
| Fractional target used by the old comparison | 21.101 mm |
| Target represented by the actual Cloud command | 20.653 mm |
| Measured stationary position | 20.000 mm |
| Existing 1% full-travel tolerance | 0.978 mm |
| Error against the actual command | 0.653 mm |

The failure was a target-coordinate mismatch, not failure to wait for a moving
slider. More retries would repeatedly command the same integer point. Across
these 38 recorded command results, latency was 318–417 ms (median 328 ms).
The separate video/script duration warning did not cause this startup refusal.

Raw exports are retained locally at
`.scratch/startup-settling/installed-traces.json` and
`.scratch/startup-settling/installed-last-motion.json`; user media identifiers
and runtime data are not committed.

## Correction and safety contract

The shared engine uses the owner's existing sampling-resolution contract to
calculate its expected encoded physical endpoint. Quantization before the
stroke window, reversal after semantic quantization, and physical resolution
after the window are handled in their respective order. Arrival and post-Stop
verification use the temporary lead-in window; the already-aligned fast path
uses the final window. This avoids mixing two different encoding grids when
the engine temporarily widens travel to acquire the first point.

Startup trace annotations now include the encoded full-travel target, absolute
target and absolute tolerance alongside the unchanged semantic command.
Authored/media positions, emitted float precision, lead-in trajectory and
timing, six-read bound, stationary-speed check, calibration recovery bounds,
final-window check, and fail-safe Stop are unchanged. Already-arrived retries
can proceed directly instead of repeating a doomed lead-in.

## Validation and remaining physical review

- The captured start and already-settled retry both reproduced the exact
  1.10 mm error before the fix and pass afterward.
- Regressions cover missed encoded targets on both sides, post-Stop drift,
  moving arrivals, continuous precision, reverse half-step rounding, narrow
  windows, physical-step resolution, and different temporary/final targets.
- A position close to the old fractional target but outside the encoded
  target's 1% tolerance is rejected; this is not a larger acceptance threshold.
- Focused startup and Stop/cancellation tests pass 100 race-enabled runs.
  Full Go tests, full race suite, vet, lint, architecture/goleak, and the
  CGO-free build pass locally. Frontend and packaging gates run in PR CI;
  browser assets and dependencies are unchanged.
- `.scratch/startup-settling/capture.go` replays successful lead-in, already-
  settled, and rejected-arrival scenarios through the shared engine and actual
  Cloud request encoder, without hardware. `replay-capture.json` retains all
  three command/trace sequences. `startup-arrival-review.png` shows semantic
  versus encoded targets, replayed telemetry, the unchanged tolerance band,
  and the queued startup hold canceled by Stop. The overview was inspected.
- The isolated review app has a working real Ollama generation. Physical
  confirmation of this correction on the installed instance is still pending;
  captured-state replay is not a new hardware measurement.

The stripped candidate is 19,116,032 bytes, 512 bytes above alpha.42's review
build. There are no new dependencies or browser payload changes.
