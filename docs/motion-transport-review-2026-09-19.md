# Motion transport quality corrections — September 19, 2026

The reported stepped Bluetooth movement and endpoint stutter led to three
reproduced application problems: an immediate transport timing floor discarded
authored reversals in favor of a 125 ms sampling grid; position quantization
created avoidable command-speed changes; and Browser Bluetooth undercounted
bridge latency while treating a successful write as evidence of playback.

The corrections stay in the shared engine and its transport boundary. They
address those mechanisms; the reporter's device, firmware, Intiface version,
settings and session trace were unavailable, so physical resolution of that
specific complaint remains unverified. No hardware was connected or commanded.

## Implementation

### Immediate transport fitting

Intiface advertises a minimum 50 ms segment budget, or its larger device timing
gap plus the existing margin. The shared engine still inspects dense path
samples and exact knots. It fits the intervening travel subject to that output
interval and retains representable reversals, including across append windows.
Dense linear media also takes this path instead of bypassing the timing floor.
An unrepresentable reversal is rejected with a useful timing error rather than
silently changing direction or rhythm. Immediate command pacing, asynchronous
acknowledgements and late-command Stop guards are unchanged.

The Full sweeps reproduction at 43% now retains the 100% destination at 597 ms;
the old positive-floor grid missed it and reached 99% at 625 ms. Tests exercise
30/50/100 ms floors and twelve consecutive windows of every current catalog
recipe at 10/25/43. The selected Intiface budget covers both zero and positive
advertised radio gaps.

### Rounded command timing

For steady continuous buffered motion, optional point timestamps can move toward
the time at which the plan actually crosses their encoded position. A candidate
must reduce its local maximum adjacent velocity jump, stay within 12 ms and
preserve positional tolerance. Reversal knots and the incoming/outgoing append
boundary segments remain fixed. The final integer intervals also enforce the
device profile's velocity ceiling after position rounding; this closes two
reproduced Handy 2 Standard overshoots.

Local improvement alone was insufficient: early experiments changed the next
append partition and increased some full-window velocity jumps. Those variants
were rejected. The final fit protects the partition and its adjacent points.
Immediate transports use the interval/velocity projection without the optional
crossing-time adjustment, avoiding small cross-window reversals there.

Recipes, prompts, random seeds, stroke limits, direction mapping and peak-speed
profiles are unchanged. Live transitions retain their existing transition fit.
Buffered media retains its authored timeline; this change does not retime video
playback or change its filter policy.

### Browser Bluetooth buffering and feedback

Refill planning now measures the full backend-to-browser acknowledgement round
trip. Browser-reported GATT execution time remains separately available in the
bridge acknowledgement. The transport requests at least 1.5 seconds of
interactive lead and 5 seconds for media, and explicitly enables
`pause_on_starving`.

A successful GATT write is labeled `submitted` or `play_requested`. Supported
passive HSP notifications supply playback state and bounded buffer/progress
fields. Progress reports are throttled to four per second; state changes such
as starvation are forwarded immediately. The existing shared-engine health
check can then stop a starved run rather than showing an assumed healthy one.

The backend accepts feedback only from the current gateway and active stream,
bound to the latest backend command, with monotonically increasing report
sequence numbers. Stop and disconnect retire feedback; a reconnect starts a
new sequence. Regression tests cover overlapping HTTP reports, foreign clients,
old streams, delayed reports after Stop and reuse of a stream after another Play.

This is passive telemetry, not a guarantee that every submitted point has been
accepted. Firmware that supplies no supported notifications continues to show
the honest requested/submitted state. No active playback polling or wait for
optional firmware RPC completion was introduced. Stop remains independent of
those notifications.

## Numerical and visual review

Baseline: `9803ef6f72295f69ae36518c3b372930f84f5bb1`. The same fixtures and settings
were exported before and after the change using the shared-engine atlas tools
described in [motion visual review](motion-visual-review.md).

| Matrix | Cases | Lower maximum adjacent command-speed jump | Unchanged | Higher |
| --- | ---: | ---: | ---: | ---: |
| Current catalog and Flow experiments, 10/25/43, Original Handy | 69 | 45 | 24 | 0 |
| Creative v2 and original Creative, 10/45/85, all three profiles | 135 | 92 | 43 | 0 |

All 204 cases compiled. The analyzed initial quantized sequences contained zero
stationary edges. Mean per-case maximum adjacent velocity jump changed from
77.42 to 74.11 percentage points/s in the first matrix and from 120.83 to
115.94 in the second. At the first Full sweeps 43% reversal, the adjacent Cloud
segment velocities changed from +31.25/−30.30 to +26.32/−26.32 percentage
points/s, a 14.5% reduction in that join's jump. These are command measurements,
not carriage telemetry or a physical comfort score.

All twelve overview sheets were inspected. Detailed plots included Full sweeps
at 10/25/43, free-roaming Creative at 10/45/85, tip-anchored drift, soft
turnarounds, the continuous reference and the original Creative 23681 outlier.
Macro reach and rhythm remain intact; the command velocity plots still have
piecewise-linear steps, with some rounding pulses reduced. No model-selection
contract changed, and no new LLM motion selections were evaluated.

Generated artifacts remain ignored under `.scratch/motion-feel-fixes/`:

- `atlas.json`, `creative-v2-atlas.json`, both `*-plots/` directories and their
  overview sheets, detailed figures and HTML indexes.
- `wire-analysis.json`, `creative-v2-wire-analysis.json` and `comparison.json`.
- `transport-samples.json`, `capture.go` and `before-after.png`.
- Full Go/race, frontend, lint, build, regression and readiness logs, plus
  `review-app-trace.json` (zero motion rows) and `chat-readiness.json`.

The earlier baseline and rejected initial experiments are retained under
`.scratch/motion-feel-review/`. The twelve Fake transport captures use a
deliberately artificial 12-second lead and one-hour dispatch tick before an
immediate Stop, to capture sampling profiles without waiting. They are not
production dispatch-latency or buffer-throughput measurements.

## Validation and remaining physical acceptance

Regression coverage includes minimum intervals across appends, exact reversals,
dense authored media, quantized speed ceilings, immutable committed points,
full bridge latency, starvation policy and stale-feedback rejection. Existing
motion/transport Stop teardown, goroutine, retarget and architecture tests remain
enabled. Frontend tests cover passive state changes arriving inside the progress
throttle and notifications arriving after local Stop.

The Windows review uses Go 1.26.4 and Node 24.15.0. The frontend passes 645 tests
in 87 files, TypeScript, localization and its production build. Full Go/race,
vet, zero-issue lint and a stripped `CGO_ENABLED=0` build pass on the final code.
One repeat full-suite run encountered an unrelated Windows TTS helper child
startup timeout; the unchanged helper passed five consecutive independent runs
and the subsequent full suite. Its failure log is retained; the teardown test
was neither weakened nor skipped.

The current isolated simulator at `http://127.0.0.10:50007/#/chat` uses the
available local Ollama `huihui_ai/granite4.1-abliterated:3b`. The provider probe
returns non-empty text, and the actual app chat returns “The app is ready for
review.” in 1,421 ms, including 1,270 ms model load, with one provider call and
no repair, fallback or motion action. LLM motion and Autopilot
remain off, the engine is idle, and Bluetooth is disconnected. Artifact and
sampler cost measurements are in the [goal scorecard](goal-scorecard.md).

For physical acceptance, record app/device/firmware/Intiface versions, selected
transport and profile, limits, latency distribution, buffer/starvation events
and a sanitized trace. Compare steady Full sweeps first, then Creative,
startup, live retargets, media and Stop. The only new measured latency in the
regression suite is a synthetic 35 ms bridge delay outside a 1 ms browser timer;
it verifies accounting, not radio performance. Firmware-specific immediate
movement behavior and background-browser scheduling still need device testing.
