# Original Handy Cloud sampling acceptance — 2026-10-24

Status: **accepted for a single Original Handy over Cloud**. This supersedes
the unaccepted velocity-refinement candidate in the
[October 23 investigation](layered-slow-turn-review-2026-10-23.md). The
maintainer authorized hardware comparisons with the device clear, then asked
to implement the preferred result and release an update. All motion used the
real app's Lab/engine/Cloud path; no private device loop or reference-app code
is introduced. The installed session was not changed.

## Comparisons and physical feedback

The same generated full sweeps and two-short/one-full phrase were played with
an Original Handy, firmware 4.3.0, Cloud REST and the existing physical window
0–88. Short strokes cover semantic 72–100, full strokes 0–100. Runs lasted
15 seconds after Play acceptance, with Stop and disconnect between runs.
The comparison source was `f3de9b8b436c84883a7aabcd9fa3c323633fb41c` plus
recorded ignored source overlays. The final production build uses the shared
sampler policy described below.

| Variant | Representation | Outcome |
| --- | --- | --- |
| A | Existing position fit without extra velocity refinement | Baseline |
| B | Detailed fit plus candidate velocity refinement | Little subjective improvement; stutter remained |
| C | Shorter rounded turns / cruise experiment | Stutter remained; not adopted |
| D | 3-point optional-position budget, no extra velocity refinement | Preferred at reference, very slow and compound settings |
| E | 6-point budget | Lost pattern definition; rejected |
| F | Intermediate 1-point budget | D preferred, including the corrected faster pair |
| Production | Bounded, scale-aware D policy | Maintainer confirmed both final runs retained improvement and stopped correctly |

A and B submitted identical points for the reference-speed fixture. The initial
subtle subjective difference therefore does **not** demonstrate a benefit from
the extra velocity refinement. At that pace D reduced roughly 22 submitted
points per full cycle to 8.5. The shorter-turn candidate could not meet the
same derivative constraints at that pace and fell back to the original curve;
the identical C preview was not presented as a different physical test.

The first faster comparison used base speed 80 with both group-rate multipliers
still at 25. Calling that simply “80% speed” was misleading. Its mean commanded
travel was 73.7 percentage points/s and effective equivalent estimate 14.2%.
We repeated F/D at base 80 with both group multipliers 100: mean travel 151.8
points/s, effective equivalent 37.3%, peak 335.7 versus the 363.6 points/s
ceiling, and a 4.112-second two-motif plan. Geometry and bounded turns keep mean
travel below the selected base setting. The maintainer reported clearly faster
motion and still preferred D. These are commanded estimates, not motor telemetry.

The final production repeat contained very slow full sweeps followed by that
faster compound pattern. Feedback: “Yes, both retained it and stopped correctly.”
Earlier D tests still had a small hesitation at direction changes, especially
the short-to-full join. This release improves it; it does not claim to reproduce
the reference app's feel exactly or eliminate every pause.

## Production scope

Cloud advertises a sparse-generated-motion preference through the existing
transport capability interface. The shared engine selects the approximation
budget; the transport does not create trajectories. Only a single Original
Handy, buffered generated continuous curve, outside transitions and imported
media, uses this preference. Multi-device sessions, Handy 2 profiles, immediate
owners, imported media and transition windows retain the old fit.

The budget is at most three semantic percentage points and at most 12% of the
smallest nonzero authored leg, with the original 0.3-point floor. Narrow strokes
therefore keep more detail. Mandatory turns, authored holds, endpoints, counts,
curve duration, prior append anchors, point caps and velocity checks are retained.
The interpolation reserve still accounts for between-probe error. Planned
acceleration/jerk limits and the final sanitizer are unchanged. The discarded
extra velocity-refinement algorithm remains test-only, outside the shipped binary.

This does not alter model-picked speed or introduce a language rule. The same
engine improvement applies to local and hosted generated motion on the accepted
device path. No dependency, background task, inference call or firmware setting
is added. Edge/Return/retarget transition fitting and Stop dispatch are unchanged.

## Verification and retained evidence

- All 27 speed/width/group-rate unit combinations check encoded timing, velocity,
  exact authored turn locations/times and an independently sampled 1 ms path
  error bound. Narrow width, immediate output, transition, Handy 2, media policy,
  transport capability import and linked-session opt-out are covered.
- Four matched inert fixtures reproduce D's exact submitted point arrays in the
  production sampler. Their point counts are 57, 26, 31 and 40 versus B's 138,
  69, 92 and 102. Maximum path error is 2.67–3.10 semantic points, including
  whole-percent rounding, within the selected 3.5-point bound. Counts, periods,
  reversal times and zero stationary time are retained.
- The 29 real runs span 16 distinct fixture/variant pairs. All completed and
  stopped; Stop responses took 261–374 ms. Startup including preposition took
  4.292–6.802 seconds. Per-run median transport latency was 304.5–328 ms.
  The lowest accepted tail lead estimated from main-stream Play acknowledgement
  was 828 ms, in a B run with a 1.748-second operation. This is not buffer
  telemetry. Eight final append operations reported cancellation/network errors
  coincident with Stop; the succeeding Stop commands completed. They are retained
  rather than counted as unexplained failures during ongoing playback.
- Four overview pages render the shared compiled curve and actual submitted
  points/segment velocities for every auditioned pair. Inspection includes the
  compound join in detail and the rejected E. D preserves all intended turns
  with fewer small near-turn commands; its piecewise-linear velocity is not a
  smoother numerical fit than B. The physical preference, not a better velocity
  error score, supports this selection. No firmware interpolation mechanism is
  inferred as proven.

Ignored evidence is in `.scratch/hardware-smoothness-2026-10-24/`: timestamped
captures contain preview, transport trace, final stopped state and diagnostics;
`source.json`, build hashes and overlays record provenance; `feedback.json`,
`summary.json`, `aggregate.json`, `hardware-atlas-1.png` through `-4.png` and
`compound-turn-detail.png` record analysis. Earlier failed model selections and
their rendered paths remain in the October 23 response-depth evidence.

Claude Opus 5.5 Max was consulted with sanitized source and test findings, but
returned no response within ten minutes. No second-opinion approval is claimed.
The production app completed real local generation and a text-only app chat in
351 ms, one provider call, with no repair, fallback or motion. Full Go, optional
Labs, frontend, installer, vet, lint and pure-Go checks are recorded in the
release review; CI additionally executes the Linux race suite.

Physical acceptance is limited to this device/transport and these short runs.
Multi-Handy synchronization, long-session network behavior, loaded COYOTE output
and other hardware are not revalidated by this result.
