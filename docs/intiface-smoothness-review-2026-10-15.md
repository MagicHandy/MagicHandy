# Handy Intiface smoothness review — 2026-10-15

Observed comparisons on an unloaded original Handy, firmware 4.3.0, through
Intiface Central 3.2.2+46 favored fewer position updates. The accepted change
advertises a **180 ms preferred spacing** for Buttplug's `The Handy (FW4+)`
device. The hard spacing remains the server's advertised limit plus the existing
margin: 110 ms on this host. The shared engine uses the preference for optional
interior points on generated curves. Reversal/hold times, bounds and the shared
velocity ceiling remain mandatory. A representable turn shorter than 180 ms
continues to work; the preference never stretches it to 180 ms.

## Physical comparisons

Each run lasted 12 seconds after startup, with a stopped pause between runs.
Full sweeps and pace waves used 35%, a 0–100 stroke window, original-Handy
calibration and a 1–40 test limit. Creative v2 used the same seed-17 score in
both runs: full outer band, 25% minimum reach, default gesture vocabulary and
35% speed. The tests used the app's ordinary shared engine and transport paths.
No raw BLE motion commands or private playback loop were introduced.

| Trial | Result reported by the operator |
| --- | --- |
| A: existing sampling, 110 ms hard floor | Reference hesitation/catch-up behavior. |
| B: previously parked denser fit | Worse than A; rejected. |
| C: 180 ms hard floor | Smoother than A on full sweeps. Fairly close to the matching Cloud run, but not identical. Also better on the varying-speed pattern. |
| D: wider fits accepted only by numerical error/velocity-step guards | C felt smoother; rejected. |
| E: 180 ms preference with the original hard floor | On repeated Creative v2 playback, smoother and natural while stroke lengths changed. Accepted for integration. |

C's temporary hard-floor change also changed the engine's sampling-window
cadence. E removes that hard-floor change. The inert comparisons of E retain
the positively assessed C shapes for the full-sweep and pace-wave fixtures;
the physical Creative v2 comparison independently exercised E itself. Missed
runs were repeated and are not counted as positive observations.

The integrated production-path build was then tested on both full sweeps and
the same Creative v2 score at 35%. The operator confirmed that both looked good.
Playback was stopped after each run. The review app remains available at
`http://127.0.0.1:50253/#/settings/devices/handy`, with Claude's device UI intact.

Intiface startup receipts were approximately 251–257 ms, including the existing
250 ms anchor. Its Stop receipts were 1–3 ms. The matching Cloud run took
5,080 ms to start and 370 ms for Stop. These are application/server receipts,
not Bluetooth arrival timing or measured physical stopping latency. The
previously identified upstream Stop issue remains open.

## Curve review and trade-off

The 144-case matrix covers eight pattern/mode families, six speeds and three
transport profiles. All final cases compile. All **96 Cloud and generic
Intiface cases are bit-for-bit unchanged**. The final 48 Handy cases match the
tested E prototype. Six overview sheets and detailed position/velocity plots
retain the rejected candidates, including C's failure on a 178 ms Creative v2
turn at 75%.

The preference trades intermediate curve precision for the observed smoother
motion. In the 12-second 35% full-sweep fixture, sampled points decrease from
87 to 53; maximum position approximation error stays 3.92 percentage points,
while RMS error increases from 1.75 to 2.10. For varied Creative v2 at 35%, RMS
increases from 2.16 to 4.24, with maximum error increasing from 6.95 to 12.00.
These are errors in the idealized linear command reconstruction, not carriage
measurements. The operator still preferred the physical Creative v2 result.

Faster curves show larger differences. At 75%, full-sweep RMS increases from
1.90 to 7.78 and maximum error from 3.92 to 13.08. Mean reconstructed travel
changes by at most 0.358% across all 48 Handy cases; no speed multiplier is
applied. Physical comparisons stayed below the repository's 40% unattended-test
limit. Higher-speed and restricted-window feel are not established by the
35% observations. The overview preserves the intended reversal order and
envelopes, with visibly coarser acceleration/deceleration between them.

## Scope and validation

`MotionSamplingCapabilities.PreferredPointInterval` is optional. The selected
Handy protocol supplies the preference; the engine owns fitting. Other linear
devices retain their existing fit. Buffered owners and imported media retain
their output. The fan-out sampler already follows its clock participant's
sampling capabilities, so there is no second motion path for linked devices.
Cloud/native HSP, model prompts, local-model context, voice and Claude's device
UI are unchanged.

Regression coverage checks exact reversals across appends, hard timing and
velocity limits, fast Creative v2 turns, unchanged buffered/media output and
selection of the profile only for the matching Handy protocol. Full Go and race
tests, vet, lint and pure-Go Windows/Linux builds pass. The race run uses the
existing LLVM-MinGW compiler; an initial shell lacked it on PATH and could not
build the race runtime. No new dependency or UI asset is added; the matching
stripped core grows by 1,024 bytes.

The exact review app passes `check-review-llm.ps1` against local Gemma 12B. Its
separate text-only app chat returns `13` in 240 ms, with one provider call, no
repair or fallback and no motion. This confirms readiness; it is not a
controlled model-performance comparison.

Evidence is retained outside git in `.scratch/intiface-smoothness/`: all trial
source overlays, run snapshots and trace exports, the 144-case JSON reports,
`comparison-summary.json`, six `matrix-atlas` sheets, detailed plots and test
logs. No credentials are copied into the comparison profiles or exported.

The installed Intiface is unchanged. Stop delivery is tracked in
[Buttplug issue 971](https://github.com/buttplugio/buttplug/issues/971); the
application change does not incorporate or certify the separate upstream
candidate.
